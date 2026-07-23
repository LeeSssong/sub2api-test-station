# frozen_string_literal: true

require "digest"
require "fileutils"
require "json"
require "net/http"
require "optparse"
require "time"
require "uri"

module ModelRelease
  class PromotionError < StandardError; end

  module PromotionCanonical
    module_function

    def normalize(value)
      case value
      when Hash
        value.keys.map(&:to_s).sort.to_h { |key| [key, normalize(value.fetch(key))] }
      when Array
        items = value.map { |item| normalize(item) }
        if items.all? { |item| item.is_a?(Hash) && item.key?("account_id") }
          items.sort_by { |item| item.fetch("account_id") }
        elsif items.all? { |item| item.is_a?(Hash) && item.key?("group_id") }
          items.sort_by { |item| item.fetch("group_id") }
        else
          items
        end
      else
        value
      end
    end

    def sha256(value)
      Digest::SHA256.hexdigest(JSON.generate(normalize(value)))
    end
  end

  class NativeClient
    PAGE_SIZE = 100
    GROUP_FIELDS = %w[models_list_config].freeze

    def initialize(base_url:, admin_key_file:, transport: nil)
      @base_url = validate_base_url(base_url)
      @admin_key = read_admin_key(admin_key_file)
      @transport = transport
    end

    def active_accounts
      accounts = []
      page = 1
      loop do
        data = request("GET", "/api/v1/admin/accounts?page=#{page}&page_size=#{PAGE_SIZE}")
        items = data.fetch("items")
        total = Integer(data.fetch("total"))
        raise PromotionError, "native account list is invalid" unless items.is_a?(Array) && total.between?(0, 2000)

        accounts.concat(items.map do |item|
          {
            "account_id" => Integer(item.fetch("id")),
            "status" => item.fetch("status"),
            "schedulable" => item.fetch("schedulable"),
            "group_ids" => item.fetch("group_ids").map { |id| Integer(id) }.sort
          }
        end)
        break if accounts.length == total
        raise PromotionError, "native account list is invalid" if items.empty? || accounts.length > total

        page += 1
        raise PromotionError, "native account list is invalid" if page > 20
      end
      accounts.select { |account| account["status"] == "active" && account["schedulable"] == true }
              .sort_by { |account| account.fetch("account_id") }
    rescue KeyError, ArgumentError, TypeError
      raise PromotionError, "native account list is invalid"
    end

    def account_mapping(account_id)
      data = request("GET", "/api/v1/admin/accounts/#{Integer(account_id)}")
      credentials = data.fetch("credentials")
      mapping = credentials.fetch("model_mapping", {})
      validate_mapping!(mapping)
    rescue KeyError, ArgumentError, TypeError
      raise PromotionError, "native account mapping is invalid"
    end

    def group_config(group_id)
      data = request("GET", "/api/v1/admin/groups/#{Integer(group_id)}")
      config = GROUP_FIELDS.to_h { |field| [field, data.fetch(field)] }
      validate_group_config!(config)
    rescue KeyError, ArgumentError, TypeError
      raise PromotionError, "native group model configuration is invalid"
    end

    def update_account_mapping(account_id, mapping)
      normalized = validate_mapping!(mapping)
      data = request("POST", "/api/v1/admin/accounts/bulk-update", {
        "account_ids" => [Integer(account_id)],
        "credentials" => { "model_mapping" => normalized }
      })
      unless data["success"] == 1 && data["failed"] == 0
        raise PromotionError, "native account update failed"
      end
      nil
    end

    def update_group_config(group_id, config)
      request("PUT", "/api/v1/admin/groups/#{Integer(group_id)}", validate_group_config!(config))
      nil
    end

    private

    def request(method, path, body = nil)
      headers = { "Accept" => "application/json", "x-api-key" => @admin_key }
      headers["Content-Type"] = "application/json" unless body.nil?
      return @transport.call(method: method, path: path, body: body, headers: headers) if @transport

      uri = URI.parse(@base_url + path)
      request_class = { "GET" => Net::HTTP::Get, "POST" => Net::HTTP::Post, "PUT" => Net::HTTP::Put }.fetch(method)
      http_request = request_class.new(uri)
      headers.each { |key, value| http_request[key] = value }
      http_request.body = JSON.generate(body) unless body.nil?
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.open_timeout = 5
      http.read_timeout = 20
      response = http.request(http_request)
      unless response.code.to_i.between?(200, 299) && response.body.bytesize <= 2 << 20
        raise PromotionError, "Sub2API Admin request failed"
      end
      envelope = JSON.parse(response.body)
      envelope.fetch("data", envelope)
    rescue JSON::ParserError, KeyError, SocketError, SystemCallError, Timeout::Error, URI::InvalidURIError
      raise PromotionError, "Sub2API Admin request failed"
    end

    def validate_base_url(value)
      uri = URI.parse(value.to_s.strip.sub(%r{/+\z}, ""))
      unless %w[http https].include?(uri.scheme) && uri.host && uri.userinfo.nil? && uri.query.nil? && uri.fragment.nil?
        raise PromotionError, "Sub2API base URL is invalid"
      end
      uri.to_s
    rescue URI::InvalidURIError
      raise PromotionError, "Sub2API base URL is invalid"
    end

    def read_admin_key(path)
      stat = File.stat(path)
      unless stat.file? && [0o600, 0o640].include?(stat.mode & 0o777)
        raise PromotionError, "Sub2API Admin key file is invalid"
      end
      value = File.read(path, 4096).strip
      raise PromotionError, "Sub2API Admin key file is invalid" if value.empty?

      value
    rescue Errno::ENOENT, Errno::EACCES
      raise PromotionError, "Sub2API Admin key file is invalid"
    end

    def validate_mapping!(mapping)
      unless mapping.is_a?(Hash) && mapping.length <= 256 && mapping.all? do |source, target|
               valid_model_id?(source) && valid_model_id?(target)
             end
        raise PromotionError, "model mapping is invalid"
      end
      mapping.keys.sort.to_h { |key| [key, mapping.fetch(key)] }
    end

    def validate_group_config!(config)
      unless config.is_a?(Hash) && config.keys.sort == GROUP_FIELDS
        raise PromotionError, "group model configuration is invalid"
      end
      models_list = config["models_list_config"]
      unless models_list.is_a?(Hash) && models_list.keys.sort == %w[enabled models] &&
             [true, false].include?(models_list["enabled"]) && models_list["models"].is_a?(Array) &&
             models_list["models"].length <= 256 && models_list["models"].uniq.length == models_list["models"].length &&
             models_list["models"].all? { |model_id| valid_model_id?(model_id) }
        raise PromotionError, "group model configuration is invalid"
      end
      JSON.parse(JSON.generate("models_list_config" => {
        "enabled" => models_list.fetch("enabled"), "models" => models_list.fetch("models").sort
      }))
    end

    def valid_model_id?(value)
      value.is_a?(String) && value.match?(/\A[a-z0-9][a-z0-9._-]{0,127}\z/)
    end
  end

  class Promoter
    REQUIRED_MODES = {
      "relay_ops_mode" => "read_only",
      "feishu_command_mode" => "dry_run",
      "d04_mode" => "read_only",
      "registration_open" => false
    }.freeze
    ROOT_KEYS = %w[
      account_set_sha256
      accounts
      base_config_sha256
      groups
      evaluated_at
      modes
      proposal_id
      schema_version
      status
      target_config_sha256
    ].freeze
    GROUP_FIELDS = NativeClient::GROUP_FIELDS
    HASH_PATTERN = /\A[0-9a-f]{64}\z/
    FORBIDDEN_KEY = /\A(?:api[_-]?key|token|cookie|authorization|password|secret|credentials?|model[_-]?output)\z/i

    def initialize(client:, now: Time.now.utc)
      @client = client
      @now = now.utc
    end

    def preflight(proposal)
      validate_proposal!(proposal)
      raise PromotionError, "proposal is stale" if @now - Time.iso8601(proposal.fetch("evaluated_at")) > 20 * 60

      current_accounts = @client.active_accounts
      unless secure_hash_equal?(proposal.fetch("account_set_sha256"), PromotionCanonical.sha256(current_accounts))
        raise PromotionError, "account set changed"
      end
      current = read_configuration(proposal)
      unless secure_hash_equal?(proposal.fetch("base_config_sha256"), PromotionCanonical.sha256(current))
        raise PromotionError, "model configuration changed"
      end
      current
    end

    def apply(proposal, snapshot_dir:)
      before = preflight(proposal)
      snapshot_path = write_snapshot(snapshot_dir, proposal.fetch("proposal_id"), before)
      changed = []
      begin
        proposal.fetch("accounts").each do |account|
          next if account.fetch("before_mapping") == account.fetch("after_mapping")

          @client.update_account_mapping(account.fetch("account_id"), account.fetch("after_mapping"))
          changed << ["account", account]
        end
        proposal.fetch("groups").each do |group|
          next if group.fetch("before") == group.fetch("after")

          @client.update_group_config(group.fetch("group_id"), group.fetch("after"))
          changed << ["group", group]
        end
        after = read_configuration(proposal)
        unless secure_hash_equal?(proposal.fetch("target_config_sha256"), PromotionCanonical.sha256(after))
          raise PromotionError, "post-write configuration mismatch"
        end
      rescue StandardError
        rollback!(changed, proposal.fetch("base_config_sha256"), proposal)
        raise PromotionError, "model promotion failed and was rolled back"
      end
      { "status" => "published", "proposal_id" => proposal.fetch("proposal_id"), "snapshot_path" => snapshot_path }
    end

    private

    def validate_proposal!(proposal)
      reject_forbidden_keys!(proposal)
      exact_keys!(proposal, ROOT_KEYS, "proposal")
      unless proposal["schema_version"] == 1 && proposal["status"] == "可升级" && proposal["modes"] == REQUIRED_MODES
        raise PromotionError, "proposal metadata is invalid"
      end
      %w[proposal_id account_set_sha256 base_config_sha256 target_config_sha256].each do |field|
        raise PromotionError, "proposal hash is invalid" unless proposal[field].is_a?(String) && proposal[field].match?(HASH_PATTERN)
      end
      parsed = Time.iso8601(proposal.fetch("evaluated_at"))
      raise PromotionError, "proposal timestamp is invalid" if parsed > @now
      expected_id = PromotionCanonical.sha256(proposal.reject { |key, _| key == "proposal_id" })
      raise PromotionError, "proposal hash is invalid" unless secure_hash_equal?(proposal.fetch("proposal_id"), expected_id)
      validate_changes!(proposal)
      before = configuration(proposal, "before")
      after = configuration(proposal, "after")
      raise PromotionError, "base configuration hash is invalid" unless secure_hash_equal?(proposal.fetch("base_config_sha256"), PromotionCanonical.sha256(before))
      raise PromotionError, "target configuration hash is invalid" unless secure_hash_equal?(proposal.fetch("target_config_sha256"), PromotionCanonical.sha256(after))
    rescue ArgumentError, TypeError, KeyError
      raise PromotionError, "proposal is invalid"
    end

    def validate_changes!(proposal)
      accounts = proposal.fetch("accounts")
      groups = proposal.fetch("groups")
      unless accounts.is_a?(Array) && !accounts.empty? && groups.is_a?(Array) && !groups.empty?
        raise PromotionError, "proposal changes are empty"
      end
      account_ids = accounts.map do |account|
        exact_keys!(account, %w[account_id after_mapping before_mapping], "account change")
        id = Integer(account.fetch("account_id"))
        validate_mapping!(account.fetch("before_mapping"))
        validate_mapping!(account.fetch("after_mapping"))
        id
      end
      group_ids = groups.map do |group|
        exact_keys!(group, %w[after before group_id], "group change")
        id = Integer(group.fetch("group_id"))
        validate_group!(group.fetch("before"))
        validate_group!(group.fetch("after"))
        id
      end
      unless account_ids.all?(&:positive?) && account_ids == account_ids.uniq.sort &&
             group_ids.all?(&:positive?) && group_ids == group_ids.uniq.sort
        raise PromotionError, "proposal change IDs are invalid"
      end
    end

    def read_configuration(proposal)
      {
        "accounts" => proposal.fetch("accounts").map do |account|
          { "account_id" => account.fetch("account_id"), "model_mapping" => @client.account_mapping(account.fetch("account_id")) }
        end,
        "groups" => proposal.fetch("groups").map do |group|
          { "group_id" => group.fetch("group_id") }.merge(@client.group_config(group.fetch("group_id")))
        end
      }
    end

    def configuration(proposal, side)
      {
        "accounts" => proposal.fetch("accounts").map do |account|
          { "account_id" => account.fetch("account_id"), "model_mapping" => account.fetch("#{side}_mapping") }
        end,
        "groups" => proposal.fetch("groups").map do |group|
          { "group_id" => group.fetch("group_id") }.merge(group.fetch(side))
        end
      }
    end

    def rollback!(changed, base_hash, proposal)
      changed.reverse_each do |kind, item|
        if kind == "account"
          @client.update_account_mapping(item.fetch("account_id"), item.fetch("before_mapping"))
        else
          @client.update_group_config(item.fetch("group_id"), item.fetch("before"))
        end
      end
      restored = read_configuration(proposal)
      raise PromotionError, "model promotion rollback verification failed" unless secure_hash_equal?(base_hash, PromotionCanonical.sha256(restored))
    rescue StandardError
      raise PromotionError, "model promotion rollback verification failed"
    end

    def write_snapshot(directory, proposal_id, configuration)
      raise PromotionError, "snapshot directory must be absolute" unless directory.is_a?(String) && directory.start_with?(File::SEPARATOR)

      FileUtils.mkdir_p(directory, mode: 0o700)
      path = File.join(directory, "model-release-#{proposal_id[0, 16]}.json")
      payload = JSON.pretty_generate(
        "schema_version" => 1,
        "proposal_id" => proposal_id,
        "created_at" => @now.iso8601,
        "configuration" => configuration
      )
      File.open(path, File::WRONLY | File::CREAT | File::EXCL, 0o600) do |file|
        file.write(payload)
        file.flush
        file.fsync
      end
      path
    rescue Errno::EEXIST, Errno::EACCES, Errno::ENOENT
      raise PromotionError, "local model snapshot could not be created"
    end

    def validate_mapping!(mapping)
      unless mapping.is_a?(Hash) && mapping.length <= 256 && mapping.all? { |source, target| valid_model_id?(source) && valid_model_id?(target) }
        raise PromotionError, "model mapping is invalid"
      end
    end

    def validate_group!(config)
      exact_keys!(config, GROUP_FIELDS, "group model configuration")
      models_list = config["models_list_config"]
      unless models_list.is_a?(Hash) && models_list.keys.sort == %w[enabled models] &&
             [true, false].include?(models_list["enabled"]) && models_list["models"].is_a?(Array) &&
             models_list["models"].length <= 256 && models_list["models"].uniq.length == models_list["models"].length &&
             models_list["models"].all? { |model_id| model_id.is_a?(String) && model_id.match?(/\A[a-z0-9][a-z0-9._-]{0,127}\z/) }
        raise PromotionError, "group model configuration is invalid"
      end
    end

    def valid_model_id?(value)
      value.is_a?(String) && value.match?(/\A[a-z0-9][a-z0-9._-]{0,127}\z/)
    end

    def reject_forbidden_keys!(value)
      case value
      when Hash
        value.each do |key, child|
          raise PromotionError, "proposal contains forbidden fields" if key.to_s.match?(FORBIDDEN_KEY)

          reject_forbidden_keys!(child)
        end
      when Array
        value.each { |child| reject_forbidden_keys!(child) }
      end
    end

    def exact_keys!(value, expected, _path)
      raise PromotionError, "proposal structure is invalid" unless value.is_a?(Hash) && value.keys.sort == expected.sort
    end

    def secure_hash_equal?(left, right)
      return false unless left.bytesize == right.bytesize

      accumulator = 0
      left.bytes.zip(right.bytes) { |a, b| accumulator |= a ^ b }
      accumulator.zero?
    end
  end

  class PromotionCLI
    def self.run(argv, out: $stdout, err: $stderr)
      command = argv.shift
      options = {}
      OptionParser.new do |parser|
        parser.on("--proposal PATH") { |value| options[:proposal] = value }
        parser.on("--snapshot-dir PATH") { |value| options[:snapshot_dir] = value }
        parser.on("--base-url URL") { |value| options[:base_url] = value }
        parser.on("--admin-key-file PATH") { |value| options[:admin_key_file] = value }
      end.parse!(argv)
      raise PromotionError, "command must be preflight or apply" unless %w[preflight apply].include?(command)
      raise PromotionError, "unexpected arguments" unless argv.empty?
      %i[proposal base_url admin_key_file].each { |key| raise PromotionError, "missing required option" unless options[key] }

      proposal = JSON.parse(File.read(options.fetch(:proposal), 2 << 20))
      promoter = Promoter.new(client: NativeClient.new(base_url: options.fetch(:base_url), admin_key_file: options.fetch(:admin_key_file)))
      result = if command == "preflight"
                 promoter.preflight(proposal)
                 { "status" => "preflight_passed", "proposal_id" => proposal.fetch("proposal_id") }
               else
                 raise PromotionError, "missing snapshot directory" unless options[:snapshot_dir]
                 promoter.apply(proposal, snapshot_dir: options.fetch(:snapshot_dir))
               end
      out.puts(JSON.generate(result))
      0
    rescue PromotionError, JSON::ParserError, Errno::ENOENT, OptionParser::ParseError
      err.puts("ERROR: model promotion rejected")
      2
    end
  end
end

if $PROGRAM_NAME == __FILE__
  exit ModelRelease::PromotionCLI.run(ARGV)
end
