#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
compose_file="$repo_root/infra/compose.yaml"
example_env="$repo_root/infra/.env.example"
release_env="$repo_root/config/releases/sub2api.env"
release_overlay="$repo_root/infra/compose.sub2api-release.yaml"
expected_image='weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659'
expected_project='sub2api-deploy'

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

for file in "$compose_file" "$example_env" "$release_env" "$release_overlay"; do
  [[ -f "$file" ]] || fail "missing ${file#"$repo_root/"}"
done

validate_release_env() {
  ruby -e '
    expected = "SUB2API_IMAGE=#{ARGV.fetch(1)}\n"
    actual = File.binread(ARGV.fetch(0))
    abort "release env must contain exactly the approved SUB2API_IMAGE assignment" unless actual == expected
  ' "$1" "$expected_image"
}

validate_release_env "$release_env" || fail 'release env must contain exactly one approved image assignment'

if rg -n '^[[:space:]]*SUB2API_IMAGE[[:space:]]*=' "$example_env"; then
  fail 'release env must be the sole SUB2API_IMAGE source'
fi

temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT

printf '' >"$temp_dir/missing.env"
printf 'SUB2API_IMAGE=%s\nSUB2API_IMAGE=%s\n' "$expected_image" "$expected_image" \
  >"$temp_dir/duplicate.env"
printf 'SUB2API_IMAGE=%s\nUNRELATED=value\n' "$expected_image" >"$temp_dir/extra.env"
for invalid_release_env in \
  "$temp_dir/missing.env" \
  "$temp_dir/duplicate.env" \
  "$temp_dir/extra.env"; do
  if validate_release_env "$invalid_release_env" >/dev/null 2>&1; then
    fail "invalid release env was accepted: ${invalid_release_env##*/}"
  fi
done

compose_json=$(docker compose \
  --project-name "$expected_project" \
  --project-directory "$repo_root" \
  --env-file "$example_env" \
  --env-file "$release_env" \
  -f "$compose_file" \
  -f "$release_overlay" \
  config --format json)

ruby -rjson -e '
  expected_image, expected_project, raw = ARGV.shift, ARGV.shift, STDIN.read
  compose = JSON.parse(raw)
  abort "unexpected Compose project #{compose["name"].inspect}" unless compose.fetch("name") == expected_project
  services = compose.fetch("services")
  sub2api = services.fetch("sub2api")
  abort "unexpected Sub2API image #{sub2api["image"].inspect}" unless sub2api.fetch("image") == expected_image
  abort "custom Sub2API build remains" if sub2api.key?("build")

  expected_mounts = {
    "sub2api" => "/app/data",
    "postgres" => "/var/lib/postgresql/data",
    "redis" => "/data",
  }
  expected_mounts.each do |service_name, target|
    mount = services.fetch(service_name).fetch("volumes").find { |candidate| candidate["target"] == target }
    abort "missing #{service_name} mount #{target}" unless mount
    abort "#{service_name} mount #{target} is not a bind" unless mount["type"] == "bind"
    abort "#{service_name} mount #{target} has no source" if mount["source"].to_s.empty?
    abort "#{service_name} mount #{target} is read-only" if mount["read_only"]
  end
' "$expected_image" "$expected_project" <<<"$compose_json"

ruby -ryaml -e '
  service = YAML.safe_load(File.read(ARGV.fetch(0))).fetch("services").fetch("sub2api")
  abort "release overlay changed more than image" unless service.keys == ["image"]
' "$release_overlay"

uninterpolated_compose_json=$(docker compose \
  --project-directory "$repo_root" \
  -f "$compose_file" \
  config --no-interpolate --format json)

ruby -rjson -e '
  expected = {
    "sub2api" => ["${SUB2API_DATA_DIR:?SUB2API_DATA_DIR is required}", "/app/data"],
    "postgres" => ["${POSTGRES_DATA_DIR:?POSTGRES_DATA_DIR is required}", "/var/lib/postgresql/data"],
    "redis" => ["${REDIS_DATA_DIR:?REDIS_DATA_DIR is required}", "/data"],
  }
  compose = JSON.parse(STDIN.read)
  expected.each do |service_name, (source, target)|
    volumes = compose.fetch("services").fetch(service_name).fetch("volumes")
    mount = volumes.find { |candidate| candidate["target"] == target }
    abort "#{service_name} #{target} must use long bind syntax" unless mount
    abort "#{service_name} #{target} must force type bind" unless mount["type"] == "bind"
    abort "#{service_name} #{target} must require its source" unless mount["source"].end_with?(source)
  end
' <<<"$uninterpolated_compose_json"

printf '%s\n' \
  'SUB2API_DATA_DIR=caddy_data' \
  'POSTGRES_DATA_DIR=caddy_data' \
  'REDIS_DATA_DIR=caddy_data' >"$temp_dir/declared-volume-name.env"

malicious_mounts_json=$(docker compose \
  --project-name "$expected_project" \
  --project-directory "$repo_root" \
  --env-file "$example_env" \
  --env-file "$release_env" \
  --env-file "$temp_dir/declared-volume-name.env" \
  -f "$compose_file" \
  -f "$release_overlay" \
  config --format json)

ruby -rjson -e '
  compose = JSON.parse(STDIN.read)
  abort "malicious fixture requires declared caddy_data volume" unless compose.fetch("volumes").key?("caddy_data")
  {
    "sub2api" => "/app/data",
    "postgres" => "/var/lib/postgresql/data",
    "redis" => "/data",
  }.each do |service_name, target|
    mount = compose.fetch("services").fetch(service_name).fetch("volumes").find do |candidate|
      candidate["target"] == target
    end
    abort "#{service_name} #{target} became a named volume" unless mount && mount["type"] == "bind"
    abort "#{service_name} #{target} has no bind source" if mount["source"].to_s.empty?
  end
' <<<"$malicious_mounts_json"

if rg -n --fixed-strings \
  -e 'xingqiao-sub2api' \
  -e '../upstream/sub2api' \
  "$compose_file" "$release_overlay" "$release_env"; then
  fail 'custom Sub2API release input remains'
fi

if rg -n ':latest([[:space:]]|$)' "$compose_file" "$release_overlay" "$release_env"; then
  fail 'floating latest tag is forbidden'
fi

if rg -n '/app/sub2api|/app/frontend|/app/web' "$compose_file"; then
  fail 'Sub2API executable or frontend overlay is forbidden'
fi

printf 'PASS: official Sub2API release contract\n'
