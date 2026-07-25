# frozen_string_literal: true

require "json"
require "net/http"
require "optparse"
require "tempfile"
require "time"
require "uri"
require_relative "model-release-policy"
require_relative "evaluate-model-release-readiness"

module ModelRelease
  class NativeSnapshotReader
    PAGE_SIZE = 100
    MAX_RESPONSE_BYTES = 2 << 20
    MODEL_ID = /\A[a-z0-9][a-z0-9._-]{0,127}\z/

    def initialize(base_url:, admin_key_file:)
      @base_url = validate_base_url(base_url)
      @admin_key = read_admin_key(admin_key_file)
    end

    def active_accounts
      page = request("GET", "/api/v1/admin/accounts?page=1&page_size=#{PAGE_SIZE}")
      items = page.fetch("items")
      total = Integer(page.fetch("total"))
      unless items.is_a?(Array) && items.length == total && total.between?(0, PAGE_SIZE)
        raise ValidationError, "native account list is invalid"
      end
      accounts = items.filter_map do |item|
        next unless item["status"] == "active" && item["schedulable"] == true

        {
          "account_id" => Integer(item.fetch("id")), "status" => "active", "schedulable" => true,
          "group_ids" => item.fetch("group_ids").map { |id| Integer(id) }.sort
        }
      end
      validate_unique_ids!(accounts, "account_id", "native account list")
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native account list is invalid"
    end

    def public_groups
      groups = request("GET", "/api/v1/admin/groups/all")
      raise ValidationError, "native group list is invalid" unless groups.is_a?(Array)

      selected = groups.select { |group| group["status"] == "active" && group["is_exclusive"] == false }.map do |group|
        id = Integer(group.fetch("id"))
        detail = request("GET", "/api/v1/admin/groups/#{id}")
        {
          "group_id" => id, "name" => detail.fetch("name"),
          "models_list_config" => normalize_models_list(detail.fetch("models_list_config"))
        }
      end
      validate_unique_ids!(selected, "group_id", "native group list")
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native group list is invalid"
    end

    def account_mapping(account_id)
      detail = request("GET", "/api/v1/admin/accounts/#{Integer(account_id)}")
      normalize_mapping(detail.fetch("credentials", {}).fetch("model_mapping", {}))
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native account mapping is invalid"
    end

    def sync_upstream_models(account_id)
      data = request("POST", "/api/v1/admin/accounts/#{Integer(account_id)}/models/sync-upstream")
      normalize_models(data.fetch("models"), allow_empty: false)
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native model discovery is invalid"
    end

    def pricing_complete?(model_id)
      raise ValidationError, "model ID is invalid" unless valid_model_id?(model_id)

      encoded = URI.encode_www_form_component(model_id)
      data = request("GET", "/api/v1/admin/channels/model-pricing?model=#{encoded}")
      data["found"] == true && data["input_price"].is_a?(Numeric) && data["output_price"].is_a?(Numeric)
    end

    private

    def request(method, path)
      uri = URI.parse(@base_url + path)
      request = (method == "POST" ? Net::HTTP::Post : Net::HTTP::Get).new(uri)
      request["Accept"] = "application/json"
      request["x-api-key"] = @admin_key
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.open_timeout = 5
      http.read_timeout = 30
      response = http.request(request)
      unless response.code.to_i.between?(200, 299) && response.body.bytesize <= MAX_RESPONSE_BYTES
        raise ValidationError, "Sub2API native snapshot request failed"
      end
      envelope = JSON.parse(response.body)
      envelope.fetch("data", envelope)
    rescue JSON::ParserError, SocketError, SystemCallError, Timeout::Error, URI::InvalidURIError
      raise ValidationError, "Sub2API native snapshot request failed"
    end

    def validate_base_url(value)
      uri = URI.parse(value.to_s.strip.sub(%r{/+\z}, ""))
      unless %w[http https].include?(uri.scheme) && uri.host && uri.userinfo.nil? && uri.query.nil? && uri.fragment.nil?
        raise ValidationError, "Sub2API base URL is invalid"
      end
      uri.to_s
    rescue URI::InvalidURIError
      raise ValidationError, "Sub2API base URL is invalid"
    end

    def read_admin_key(path)
      stat = File.stat(path)
      unless stat.file? && [0o600, 0o640].include?(stat.mode & 0o777)
        raise ValidationError, "Sub2API Admin key file is invalid"
      end
      value = File.read(path, 4096).strip
      raise ValidationError, "Sub2API Admin key file is invalid" if value.empty?

      value
    rescue Errno::ENOENT, Errno::EACCES
      raise ValidationError, "Sub2API Admin key file is invalid"
    end

    def normalize_mapping(value)
      unless value.is_a?(Hash) && value.length <= 4096 && value.all? do |source, target|
               valid_model_id?(source) && valid_model_id?(target)
             end
        raise ValidationError, "native account mapping is invalid"
      end
      value.keys.sort.to_h { |key| [key, value.fetch(key)] }
    end

    def normalize_models_list(value)
      unless value.is_a?(Hash) && value.keys.sort == %w[enabled models] && [true, false].include?(value["enabled"])
        raise ValidationError, "native group model list is invalid"
      end
      { "enabled" => value.fetch("enabled"), "models" => normalize_models(value.fetch("models"), allow_empty: true) }
    end

    def normalize_models(value, allow_empty:)
      unless value.is_a?(Array) && (allow_empty || !value.empty?) && value.length <= 4096 &&
             value.all? { |model_id| valid_model_id?(model_id) } && value.uniq.length == value.length
        raise ValidationError, "native model list is invalid"
      end
      value.sort
    end

    def valid_model_id?(value)
      value.is_a?(String) && value.match?(MODEL_ID)
    end

    def validate_unique_ids!(items, field, label)
      ids = items.map { |item| item.fetch(field) }
      raise ValidationError, "#{label} is invalid" unless ids.all?(&:positive?) && ids.uniq.length == ids.length

      items.sort_by { |item| item.fetch(field) }
    end
  end

  class SnapshotCollector
    SAFE_MODES = {
      "relay_ops_mode" => "read_only", "feishu_command_mode" => "dry_run"
    }.freeze

    def initialize(reader:, policy:, now: Time.now.utc)
      @reader = reader
      @policy = policy
      @now = now.utc
    end

    def collect(snapshot_id:)
      raise ValidationError, "snapshot ID is required" unless snapshot_id.is_a?(String) && !snapshot_id.empty?

      accounts = @reader.active_accounts
      groups = @reader.public_groups
      raise ValidationError, "active accounts are empty" if accounts.empty?
      raise ValidationError, "public groups are empty" if groups.empty?

      discovered = accounts.to_h do |account|
        [account.fetch("account_id"), @reader.sync_upstream_models(account.fetch("account_id")).uniq.sort]
      end
      published = published_catalog(groups)
      decision = @policy.candidate_set(
        discovered_models: discovered.values.flatten,
        published_models: published.fetch("models")
      )
      base_configuration = {
        "accounts" => accounts.map do |account|
          { "account_id" => account.fetch("account_id"), "model_mapping" => @reader.account_mapping(account.fetch("account_id")) }
        end,
        "groups" => groups.map do |group|
          { "group_id" => group.fetch("group_id"), "models_list_config" => group.fetch("models_list_config") }
        end
      }
      snapshot_accounts = accounts.map do |account|
        account.merge(
          "discovery_recorded_at" => @now.iso8601,
          "discovered_models" => discovered.fetch(account.fetch("account_id")),
          "balance_usd" => nil, "financial_recorded_at" => nil,
          "quality_source" => nil, "quality_recorded_at" => nil, "sample_count" => nil,
          "success_rate" => nil, "error_rate" => nil, "ttft_p95_ms" => nil,
          "total_latency_p95_ms" => nil, "qualifications" => {}
        )
      end
      {
        "schema_version" => 1, "snapshot_id" => snapshot_id, "captured_at" => @now.iso8601,
        "account_set_sha256" => Canonical.sha256(accounts),
        "base_config_sha256" => Canonical.sha256(base_configuration), "modes" => SAFE_MODES,
        "published" => published,
        "public_groups" => groups.map { |group| { "group_id" => group.fetch("group_id"), "name" => group.fetch("name") } },
        "accounts" => snapshot_accounts,
        "pricing" => decision.candidate_models.map do |model_id|
          { "model_id" => model_id, "complete" => @reader.pricing_complete?(model_id) }
        end,
        "base_configuration" => base_configuration
      }
    end

    private

    def published_catalog(groups)
      configs = groups.map { |group| group.fetch("models_list_config") }
      enabled = configs.map { |config| config.fetch("enabled") }
      return { "families" => [], "models" => [] } if enabled.none?
      raise ValidationError, "public group model catalogs differ" unless enabled.all?

      catalogs = configs.map { |config| config.fetch("models").sort }
      raise ValidationError, "public group model catalogs differ" unless catalogs.uniq.length == 1 && !catalogs.first.empty?

      models = catalogs.first
      families = models.map do |model_id|
        classification = @policy.classify(model_id)
        raise ValidationError, "published model catalog is unsupported" unless classification.state == "candidate"

        classification.family
      end.uniq.sort_by { |family| family.split(".").map(&:to_i) }
      { "families" => families, "models" => models }
    end
  end

  class SnapshotCLI
    def self.run(argv, out: $stdout, err: $stderr)
      command = argv.shift
      options = {}
      OptionParser.new do |parser|
        parser.on("--policy PATH") { |value| options[:policy] = value }
        parser.on("--base-url URL") { |value| options[:base_url] = value }
        parser.on("--admin-key-file PATH") { |value| options[:admin_key_file] = value }
        parser.on("--output PATH") { |value| options[:output] = value }
        parser.on("--snapshot-id ID") { |value| options[:snapshot_id] = value }
        parser.on("--now TIME") { |value| options[:now] = value }
      end.parse!(argv)
      raise ValidationError, "command must be collect" unless command == "collect"
      raise ValidationError, "unexpected arguments" unless argv.empty?
      %i[policy base_url admin_key_file output].each do |key|
        raise ValidationError, "missing required option" unless options[key]
      end
      raise ValidationError, "output path must be absolute" unless File.absolute_path(options[:output]) == options[:output]

      now = options[:now] ? Time.iso8601(options[:now]).utc : Time.now.utc
      snapshot_id = options[:snapshot_id] || "MODEL-RELEASE-#{now.strftime('%Y%m%dT%H%M%SZ')}"
      snapshot = SnapshotCollector.new(
        reader: NativeSnapshotReader.new(base_url: options[:base_url], admin_key_file: options[:admin_key_file]),
        policy: Policy.load(options[:policy]), now: now
      ).collect(snapshot_id: snapshot_id)
      write_atomic(options[:output], JSON.pretty_generate(snapshot))
      out.puts(JSON.generate("snapshot_id" => snapshot_id, "account_set_sha256" => snapshot.fetch("account_set_sha256")))
      0
    rescue ValidationError, JSON::ParserError, OptionParser::ParseError, Errno::ENOENT, Errno::EACCES, ArgumentError
      err.puts("ERROR: model release snapshot collection rejected")
      2
    end

    def self.write_atomic(path, content)
      Tempfile.create([".model-release-snapshot-", ".json"], File.dirname(path)) do |file|
        file.chmod(0o640)
        file.write(content)
        file.flush
        file.fsync
        File.rename(file.path, path)
      end
    end
    private_class_method :write_atomic
  end
end

if $PROGRAM_NAME == __FILE__
  exit ModelRelease::SnapshotCLI.run(ARGV)
end
