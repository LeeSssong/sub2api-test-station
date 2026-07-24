#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
COMPOSE_FILE="$ROOT/infra/compose.sub2api-rehearsal.yaml"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$COMPOSE_FILE" ]] || fail 'isolated rehearsal Compose file is missing'

ruby -ryaml -e '
  path = ARGV.fetch(0)
  raw = File.read(path)
  data = YAML.safe_load(raw, aliases: true)
  abort "wrong project" unless data["name"] == "sub2api-official-rehearsal"
  services = data.fetch("services")
  abort "wrong service set" unless services.keys.sort == %w[caddy postgres redis sub2api]
  abort "named volumes are forbidden" if data.key?("volumes")

  expected_binds = {
    "sub2api" => {
      "type" => "bind",
      "source" => "${REHEARSAL_SUB2API_DATA_DIR:?REHEARSAL_SUB2API_DATA_DIR must be absolute}",
      "target" => "/app/data",
      "bind" => {"create_host_path" => false}
    },
    "postgres" => {
      "type" => "bind",
      "source" => "${REHEARSAL_POSTGRES_DATA_DIR:?REHEARSAL_POSTGRES_DATA_DIR must be absolute}",
      "target" => "/var/lib/postgresql/data",
      "bind" => {"create_host_path" => false}
    },
    "redis" => {
      "type" => "bind",
      "source" => "${REHEARSAL_REDIS_DATA_DIR:?REHEARSAL_REDIS_DATA_DIR must be absolute}",
      "target" => "/data",
      "bind" => {"create_host_path" => false}
    }
  }
  expected_binds.each do |service, bind|
    abort "wrong #{service} bind" unless services.fetch(service).fetch("volumes") == [bind]
  end

  abort "sub2api rollback image is not explicit" unless
    services.dig("sub2api", "image") == "${REHEARSAL_ROLLBACK_IMAGE:?REHEARSAL_ROLLBACK_IMAGE is required}"
  abort "only Caddy may publish a port" unless
    services.dig("caddy", "ports") == ["127.0.0.1:18080:8080"] &&
    %w[sub2api postgres redis].all? { |name| !services.fetch(name).key?("ports") }
  abort "Caddy does not build the isolated homepage artifact" unless
    services.dig("caddy", "build") == {"context" => "..", "dockerfile" => "infra/Dockerfile.caddy"}
  abort "Caddy must not use host bind mounts" if services.fetch("caddy").key?("volumes")
  command = Array(services.dig("caddy", "command")).join("\n")
  abort "Caddy guard route is missing" unless
    command.include?("/api/v1/admin/system/update") &&
    command.include?("/api/v1/admin/system/rollback") &&
    command.include?("DOCKER_DEPLOYMENT_UPDATE_REQUIRED")
  abort "Caddy support route is missing" unless
    command.include?("/support /support/") &&
    command.include?("/support/*") &&
    command.include?("/home-assets/*") &&
    command.include?("/index.html") &&
    command.include?("/srv/home")

  forbidden = ["/opt/sub2api", "${SUB2API_DATA_DIR", "${POSTGRES_DATA_DIR", "${REDIS_DATA_DIR",
    "0.0.0.0:"]
  forbidden.each { |value| abort "forbidden rehearsal content: #{value}" if raw.include?(value) }
' "$COMPOSE_FILE"

fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/app" "$fixture/postgres" "$fixture/redis"

rendered=$(env \
  REHEARSAL_SUB2API_DATA_DIR="$fixture/app" \
  REHEARSAL_POSTGRES_DATA_DIR="$fixture/postgres" \
  REHEARSAL_REDIS_DATA_DIR="$fixture/redis" \
  REHEARSAL_ROLLBACK_IMAGE='xingqiao-sub2api:v0.1.164-contact-v1' \
  POSTGRES_USER=sub2api POSTGRES_PASSWORD=test-only POSTGRES_DB=sub2api \
  REDIS_PASSWORD=test-only ADMIN_EMAIL=admin@example.test ADMIN_PASSWORD=test-only \
  JWT_SECRET=test-only TOTP_ENCRYPTION_KEY=test-only \
  docker compose -f "$COMPOSE_FILE" config --format json)

jq -e --arg app "$fixture/app" --arg postgres "$fixture/postgres" --arg redis "$fixture/redis" '
  .name == "sub2api-official-rehearsal" and
  (.services | keys | sort) == ["caddy", "postgres", "redis", "sub2api"] and
  .services.caddy.ports == [{host_ip:"127.0.0.1", target:8080, published:"18080", protocol:"tcp", mode:"ingress"}] and
  (.services.sub2api.ports // []) == [] and
  (.services.postgres.ports // []) == [] and
  (.services.redis.ports // []) == [] and
  any(.services.sub2api.volumes[]; .type == "bind" and .source == $app and .target == "/app/data") and
  any(.services.postgres.volumes[]; .type == "bind" and .source == $postgres and .target == "/var/lib/postgresql/data") and
  any(.services.redis.volumes[]; .type == "bind" and .source == $redis and .target == "/data")
' <<<"$rendered" >/dev/null || fail 'rendered rehearsal topology violates isolation contract'

printf 'PASS: isolated Sub2API rehearsal Compose contract\n'
