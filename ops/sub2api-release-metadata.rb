#!/usr/bin/env ruby
# frozen_string_literal: true

require "date"
require "json"
require "optparse"
require "rubygems"
require "tempfile"
require "time"
require "uri"

module Sub2APIReleaseMetadata
  MAX_RELEASE_BYTES = 1 << 20
  MAX_PROVENANCE_BYTES = 128 << 10
  SHA_PATTERN = /\A[0-9a-f]{40}\z/
  VERSION_PATTERN = /\A[0-9]+(?:\.[0-9]+){1,2}\z/

  module_function

  def bounded_read(path, maximum)
    raise ArgumentError, "input is not a regular file" unless File.file?(path)
    raise ArgumentError, "input exceeds size limit" if File.size(path) > maximum

    File.binread(path)
  end

  def parse_provenance(path)
    text = bounded_read(path, MAX_PROVENANCE_BYTES)
    version = text[/^- Release tag: `v?([^`]+)`$/, 1]
    commit = text[/^- Source commit: `([^`]+)`$/, 1]
    raise ArgumentError, "invalid provenance version" unless version&.match?(VERSION_PATTERN)
    raise ArgumentError, "invalid provenance commit" unless commit&.match?(SHA_PATTERN)

    { "base_version" => version, "base_commit" => commit }
  end

  def parse_release(path, official_commit)
    raise ArgumentError, "invalid official commit" unless official_commit.match?(SHA_PATTERN)

    release = JSON.parse(bounded_read(path, MAX_RELEASE_BYTES))
    raise ArgumentError, "release must be an object" unless release.is_a?(Hash)
    raise ArgumentError, "draft release is not eligible" unless release["draft"] == false
    raise ArgumentError, "prerelease is not eligible" unless release["prerelease"] == false

    tag = release.fetch("tag_name")
    version = tag.to_s.sub(/\Av/, "")
    raise ArgumentError, "invalid stable version" unless version.match?(VERSION_PATTERN)

    published_at = Time.iso8601(release.fetch("published_at")).utc.iso8601
    url = URI.parse(release.fetch("html_url"))
    expected_path = "/Wei-Shaw/sub2api/releases/tag/#{tag}"
    unless url.is_a?(URI::HTTPS) && url.host == "github.com" && url.path == expected_path &&
           url.query.nil? && url.fragment.nil?
      raise ArgumentError, "invalid official release URL"
    end

    {
      "version" => version,
      "tag" => tag,
      "official_commit" => official_commit,
      "name" => release.fetch("name").to_s,
      "body" => release.fetch("body", "").to_s,
      "published_at" => published_at,
      "html_url" => url.to_s
    }
  rescue JSON::ParserError, KeyError, URI::InvalidURIError, ArgumentError => e
    raise ArgumentError, e.message
  end

  def atomic_write(path, content)
    directory = File.dirname(File.expand_path(path))
    raise ArgumentError, "output directory does not exist" unless Dir.exist?(directory)

    Tempfile.create([".sub2api-release-", ".tmp"], directory, mode: File::RDWR, encoding: "UTF-8") do |file|
      file.chmod(0o600)
      file.write(content)
      file.flush
      file.fsync
      file.close
      File.rename(file.path, path)
      File.chmod(0o600, path)
    end
  end

  def discover(options)
    provenance = parse_provenance(options.fetch(:provenance))
    release = parse_release(options.fetch(:release), options.fetch(:official_commit))
    base_sha = options.fetch(:base_sha)
    raise ArgumentError, "invalid base SHA" unless base_sha.match?(SHA_PATTERN)

    latest = Gem::Version.new(release.fetch("version"))
    current = Gem::Version.new(provenance.fetch("base_version"))
    raise ArgumentError, "official release is older than provenance" if latest < current

    result = provenance.merge(release).merge(
      "base_sha" => base_sha,
      "has_update" => latest > current ||
        release.fetch("official_commit") != provenance.fetch("base_commit")
    )
    atomic_write(options.fetch(:output), JSON.pretty_generate(result) + "\n")
  end

  def advance_provenance(options)
    metadata = JSON.parse(bounded_read(options.fetch(:metadata), MAX_RELEASE_BYTES))
    version = metadata.fetch("version")
    commit = metadata.fetch("official_commit")
    tag = metadata.fetch("tag")
    annotated_tag = options.fetch(:annotated_tag)
    imported = Date.iso8601(options.fetch(:imported)).iso8601
    raise ArgumentError, "invalid version" unless version.match?(VERSION_PATTERN)
    raise ArgumentError, "invalid tag" unless tag == "v#{version}" || tag == version
    raise ArgumentError, "invalid commit" unless commit.match?(SHA_PATTERN)
    raise ArgumentError, "invalid annotated tag" unless annotated_tag.match?(SHA_PATTERN)

    text = bounded_read(options.fetch(:provenance), MAX_PROVENANCE_BYTES)
    replacements = {
      /^- Release tag: .*$/ => "- Release tag: `#{tag}`",
      /^- Source commit: .*$/ => "- Source commit: `#{commit}`",
      /^- Annotated tag object: .*$/ => "- Annotated tag object: `#{annotated_tag}`",
      /^- Imported: .*$/ => "- Imported: `#{imported}`"
    }
    replacements.each do |pattern, replacement|
      raise ArgumentError, "provenance field is missing" unless text.match?(pattern)

      text = text.sub(pattern, replacement)
    end
    atomic_write(options.fetch(:provenance), text)
  rescue JSON::ParserError, KeyError, Date::Error => e
    raise ArgumentError, e.message
  end
end

def parse_options(arguments, keys)
  options = {}
  parser = OptionParser.new
  keys.each do |key|
    flag = "--#{key.to_s.tr("_", "-")} VALUE"
    parser.on(flag) { |value| options[key] = value }
  end
  parser.parse!(arguments)
  raise ArgumentError, "unexpected arguments" unless arguments.empty?
  missing = keys.reject { |key| options.key?(key) }
  raise ArgumentError, "missing options: #{missing.join(", ")}" unless missing.empty?

  options
end

begin
  command = ARGV.shift
  case command
  when "discover"
    options = parse_options(ARGV, %i[release provenance base_sha official_commit output])
    Sub2APIReleaseMetadata.discover(options)
  when "advance-provenance"
    options = parse_options(ARGV, %i[metadata provenance imported annotated_tag])
    Sub2APIReleaseMetadata.advance_provenance(options)
  else
    raise ArgumentError, "unknown command"
  end
rescue StandardError => e
  warn "sub2api_release_metadata status=failed reason=#{e.class}"
  exit 1
end
