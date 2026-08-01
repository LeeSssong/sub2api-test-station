#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-blue-green-topology.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

for command in docker ruby; do
  command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
done

mkdir -p "$FIXTURE/sub2api-data" "$FIXTURE/postgres-data" "$FIXTURE/redis-data"
mkdir -p "$FIXTURE/rehearsal-sub2api-data" "$FIXTURE/rehearsal-postgres-data" "$FIXTURE/rehearsal-redis-data"

cat >"$FIXTURE/secret.env" <<EOF
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=postgres-test-password
POSTGRES_DB=sub2api
REDIS_PASSWORD=redis-test-password
ADMIN_EMAIL=admin@example.test
ADMIN_PASSWORD=admin-test-password
JWT_SECRET=jwt-test-secret
TOTP_ENCRYPTION_KEY=totp-test-key
SUB2API_DATA_DIR=$FIXTURE/sub2api-data
POSTGRES_DATA_DIR=$FIXTURE/postgres-data
REDIS_DATA_DIR=$FIXTURE/redis-data
SITE_ADDRESS=sub2api.example.test
EOF

cat >"$FIXTURE/rehearsal-secret.env" <<EOF
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=postgres-rehearsal-password
POSTGRES_DB=sub2api
REDIS_PASSWORD=redis-rehearsal-password
ADMIN_EMAIL=admin@rehearsal.test
ADMIN_PASSWORD=admin-rehearsal-password
JWT_SECRET=jwt-rehearsal-secret
TOTP_ENCRYPTION_KEY=totp-rehearsal-key
REHEARSAL_SUB2API_DATA_DIR=$FIXTURE/rehearsal-sub2api-data
REHEARSAL_POSTGRES_DATA_DIR=$FIXTURE/rehearsal-postgres-data
REHEARSAL_REDIS_DATA_DIR=$FIXTURE/rehearsal-redis-data
EOF

cat >"$FIXTURE/rehearsal-release.env" <<'EOF'
SUB2API_BLUE_IMAGE=example.invalid/sub2api-blue@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SUB2API_GREEN_IMAGE=example.invalid/sub2api-green@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
SUB2API_WORKER_IMAGE=example.invalid/sub2api-worker@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
REHEARSAL_FAIL_PUBLIC_ACCEPTANCE=false
REHEARSAL_ROLLBACK_IMAGE=example.invalid/sub2api-legacy@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
EOF

rehearsal_compose=(docker compose --env-file "$FIXTURE/rehearsal-secret.env" --env-file "$FIXTURE/rehearsal-release.env" -f "$ROOT/infra/compose.sub2api-rehearsal.yaml")
"${rehearsal_compose[@]}" config --format json >"$FIXTURE/rehearsal-compose.json"

ruby -rjson - "$FIXTURE/rehearsal-compose.json" "$FIXTURE" "$FIXTURE/rehearsal-caddy-command" <<'RUBY'
config = JSON.parse(File.read(ARGV.fetch(0)))
fixture = ARGV.fetch(1)
command_path = ARGV.fetch(2)
services = config.fetch("services")

def assert!(condition, message)
  abort "FAIL: #{message}" unless condition
end

def environment(service)
  service.fetch("environment", {})
end

def bind_source(service, target)
  service.fetch("volumes", []).find { |volume| volume["type"] == "bind" && volume["target"] == target }&.fetch("source")
end

assert!(config["name"] == "sub2api-blue-green-rehearsal", "rehearsal Compose project name must be isolated")
%w[postgres redis sub2api-blue sub2api-green sub2api-worker caddy].each do |name|
  assert!(services.key?(name), "rehearsal topology is missing #{name}")
end
assert!(!services.key?("sub2api"), "legacy single-slot rehearsal service must be removed")

blue = services.fetch("sub2api-blue")
green = services.fetch("sub2api-green")
worker = services.fetch("sub2api-worker")
caddy = services.fetch("caddy")

assert!(environment(blue)["SERVER_PROCESS_ROLE"] == "api", "rehearsal blue slot must be API-only")
assert!(environment(green)["SERVER_PROCESS_ROLE"] == "api", "rehearsal green slot must be API-only")
assert!(environment(worker)["SERVER_PROCESS_ROLE"] == "worker", "rehearsal must have one singleton worker role")
worker_roles = services.count { |_name, service| environment(service)["SERVER_PROCESS_ROLE"] == "worker" }
assert!(worker_roles == 1, "rehearsal topology must contain exactly one worker role")

app_data = File.join(fixture, "rehearsal-sub2api-data")
assert!(bind_source(blue, "/app/data") == app_data, "rehearsal blue slot must use isolated app storage")
assert!(bind_source(green, "/app/data") == app_data, "rehearsal green slot must share isolated app storage")
assert!(bind_source(worker, "/app/data") == app_data, "rehearsal worker must share isolated app storage")
assert!(bind_source(services.fetch("postgres"), "/var/lib/postgresql/data") == File.join(fixture, "rehearsal-postgres-data"),
        "rehearsal PostgreSQL must use isolated storage")
assert!(bind_source(services.fetch("redis"), "/data") == File.join(fixture, "rehearsal-redis-data"),
        "rehearsal Redis must use isolated storage")

services.each do |name, service|
  service.fetch("volumes", []).each do |volume|
    next unless volume["type"] == "bind"
    assert!(volume.fetch("source").start_with?(fixture + "/"), "#{name} must never mount a non-fixture path")
  end
  service.fetch("ports", []).each do |port|
    assert!(port["host_ip"] == "127.0.0.1", "#{name} may publish only localhost rehearsal ports")
  end
end

assert!(environment(caddy)["SUB2API_ACTIVE_UPSTREAM"] == "sub2api-blue:8080", "rehearsal Caddy must start on the selected slot")
assert!(environment(caddy)["REHEARSAL_FAIL_PUBLIC_ACCEPTANCE"] == "false", "rehearsal failure hook must be deterministic and disabled by default")
command = caddy.fetch("command")
command_text = command.is_a?(Array) ? command.join("\n") : ""
assert!(command_text.include?("{$$REHEARSAL_FAIL_PUBLIC_ACCEPTANCE}"),
        "rehearsal Caddy must preserve its public-acceptance failure placeholder")
assert!(command_text.include?("{$$SUB2API_ACTIVE_UPSTREAM}"),
        "rehearsal Caddy must preserve its active-upstream placeholder")
File.write(command_path, command.fetch(2))

puts "PASS: isolated two-slot rehearsal topology"
RUBY

rehearsal_caddy_command=$(<"$FIXTURE/rehearsal-caddy-command")
rehearsal_caddy_command=${rehearsal_caddy_command//\$\$/\$}
rehearsal_caddy_command=${rehearsal_caddy_command/exec caddy run --config \/etc\/caddy\/Caddyfile --adapter caddyfile/exec caddy adapt --config \/etc\/caddy\/Caddyfile --adapter caddyfile --pretty}
readonly REHEARSAL_CADDY_IMAGE=caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
for failure_hook in false true; do
  adapted=$(docker run --rm \
    --env "SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080" \
    --env "REHEARSAL_FAIL_PUBLIC_ACCEPTANCE=$failure_hook" \
    "$REHEARSAL_CADDY_IMAGE" /bin/sh -ec "$rehearsal_caddy_command")
  ruby -rjson -e 'JSON.parse(STDIN.read)' <<<"$adapted" \
    || fail "rehearsal Caddy hook value $failure_hook did not adapt"
done
printf 'PASS: rehearsal Caddy failure hook adapts for false and true\n'

cat >"$FIXTURE/release.env" <<'EOF'
SUB2API_BLUE_IMAGE=example.invalid/sub2api-blue@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SUB2API_GREEN_IMAGE=example.invalid/sub2api-green@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
SUB2API_WORKER_IMAGE=example.invalid/sub2api-worker@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
# The legacy variable lets the pre-topology Compose file render so this test
# can fail on the missing permanent services rather than interpolation.
SUB2API_IMAGE=example.invalid/sub2api-legacy@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
EOF

compose=(docker compose --env-file "$FIXTURE/secret.env" --env-file "$FIXTURE/release.env" -f "$ROOT/infra/compose.yaml")
"${compose[@]}" config --format json >"$FIXTURE/compose.json"

cat >"$FIXTURE/invalid-release.env" <<'EOF'
SUB2API_BLUE_IMAGE=example.invalid/sub2api-blue@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SUB2API_GREEN_IMAGE=example.invalid/sub2api-green@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
SUB2API_WORKER_IMAGE=example.invalid/sub2api-worker@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
SUB2API_ACTIVE_UPSTREAM=untrusted-upstream:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
SUB2API_IMAGE=example.invalid/sub2api-legacy@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
EOF

invalid_compose=(docker compose --env-file "$FIXTURE/secret.env" --env-file "$FIXTURE/invalid-release.env" -f "$ROOT/infra/compose.yaml")
"${invalid_compose[@]}" config --format json >"$FIXTURE/invalid-compose.json"

ruby -rjson - "$FIXTURE/compose.json" "$FIXTURE" <<'RUBY'
config = JSON.parse(File.read(ARGV.fetch(0)))
fixture = ARGV.fetch(1)
services = config.fetch("services")

def fail!(message)
  warn "FAIL: #{message}"
  exit 1
end

def assert!(condition, message)
  fail!(message) unless condition
end

def service!(services, name)
  services.fetch(name)
rescue KeyError
  fail!("missing service #{name}")
end

def bind_source(service, target)
  service.fetch("volumes", []).find { |volume| volume["type"] == "bind" && volume["target"] == target }&.fetch("source")
end

def healthy_dependencies(service)
  service.fetch("depends_on", {}).select { |_name, dependency| dependency["condition"] == "service_healthy" }.keys.sort
end

def environment(service)
  service.fetch("environment", {})
end

blue = service!(services, "sub2api-blue")
green = service!(services, "sub2api-green")
worker = service!(services, "sub2api-worker")
relay_ops = service!(services, "relay-ops")
caddy = service!(services, "caddy")

assert!(config["name"] == "sub2api", "Compose project identity must remain sub2api")
assert!(services.key?("postgres") && services.key?("redis"), "shared PostgreSQL and Redis services must remain present")

expected_images = {
  "sub2api-blue" => "example.invalid/sub2api-blue@sha256:" + "a" * 64,
  "sub2api-green" => "example.invalid/sub2api-green@sha256:" + "b" * 64,
  "sub2api-worker" => "example.invalid/sub2api-worker@sha256:" + "c" * 64
}
expected_images.each do |name, image|
  assert!(service!(services, name)["image"] == image, "#{name} must use its distinct release image variable")
end

assert!(environment(blue)["SERVER_PROCESS_ROLE"] == "api", "blue must run with SERVER_PROCESS_ROLE=api")
assert!(environment(green)["SERVER_PROCESS_ROLE"] == "api", "green must run with SERVER_PROCESS_ROLE=api")
assert!(environment(worker)["SERVER_PROCESS_ROLE"] == "worker", "worker must run with SERVER_PROCESS_ROLE=worker")

assert!(environment(blue) == environment(green), "blue and green must share identical request-serving environment")
assert!(bind_source(blue, "/app/data") == File.join(fixture, "sub2api-data"), "blue must use the shared Sub2API data bind")
assert!(bind_source(green, "/app/data") == bind_source(blue, "/app/data"), "green must use the blue data bind")
assert!(bind_source(worker, "/app/data") == bind_source(blue, "/app/data"), "worker must use the shared Sub2API data bind")
assert!(blue["networks"] == green["networks"] && green["networks"] == worker["networks"], "all Sub2API roles must share the Compose network")
assert!(healthy_dependencies(blue) == %w[postgres redis], "blue must use shared healthy PostgreSQL and Redis")
assert!(healthy_dependencies(green) == healthy_dependencies(blue), "green must use the blue shared dependencies")
assert!(healthy_dependencies(worker) == healthy_dependencies(blue), "worker must use the shared healthy dependencies")
assert!(!worker.fetch("depends_on", {}).key?("caddy"), "worker must not depend on public Caddy")

assert!(environment(relay_ops)["RELAY_OPS_SUB2API_URL"] == "http://caddy:8081", "relay-ops must reach Sub2API through internal Caddy")

caddy_dependencies = caddy.fetch("depends_on", {})
%w[relay-ops sub2api-blue sub2api-green].each do |service|
  assert!(!caddy_dependencies.key?(service), "Caddy must not wait for #{service}")
end
assert!(caddy.fetch("expose", []).include?("8081"), "Caddy must expose internal port 8081")
assert!(caddy.fetch("ports", []).none? { |port| port["target"] == 8081 }, "Caddy must not publish internal port 8081")

caddy_environment = environment(caddy)
upstream_keys = caddy_environment.keys.grep(/SUB2API.*UPSTREAM/)
assert!(upstream_keys == ["SUB2API_ACTIVE_UPSTREAM"], "Caddy must receive exactly one active-upstream environment key")
allowed_upstreams = ["sub2api-blue:8080", "sub2api-green:8080"]
assert!(allowed_upstreams.include?(caddy_environment["SUB2API_ACTIVE_UPSTREAM"]), "Caddy active upstream must be blue or green")
assert!(caddy_environment["SUB2API_ACTIVE_UPSTREAM"] == "sub2api-blue:8080", "fixture active upstream must render into Caddy")

command = caddy.fetch("command", nil)
assert!(command.is_a?(Array) && command.first(2) == ["/bin/sh", "-ec"] && command.length == 3,
        "Caddy must define a shell startup guard for the active upstream")

puts "PASS: rendered blue/green/worker topology"
RUBY

# infra/Dockerfile.caddy's final stage adds static assets to this exact pinned
# Caddy runtime. Validating the Caddyfile in that runtime avoids rebuilding the
# unrelated homepage asset stage while exercising the deployed Caddy binary.
readonly CADDY_RUNTIME_IMAGE=caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
docker image inspect "$CADDY_RUNTIME_IMAGE" >/dev/null

caddy_guard=$(ruby -rjson - "$FIXTURE/invalid-compose.json" <<'RUBY'
config = JSON.parse(File.read(ARGV.fetch(0)))
command = config.fetch("services").fetch("caddy").fetch("command")
abort "FAIL: Caddy command must be a shell guard" unless command.is_a?(Array) && command.first(2) == ["/bin/sh", "-ec"] && command.length == 3
puts command.fetch(2)
RUBY
)

if invalid_output=$(docker run --rm \
  --env "SITE_ADDRESS=sub2api.example.test" \
  --env "SUB2API_ACTIVE_UPSTREAM=untrusted-upstream:8080" \
  "$CADDY_RUNTIME_IMAGE" /bin/sh -ec "$caddy_guard" 2>&1); then
  fail "Caddy startup accepted an invalid active upstream"
fi
[[ "$invalid_output" == *"invalid SUB2API_ACTIVE_UPSTREAM"* ]] || fail "Caddy did not report its invalid active upstream"
printf 'PASS: Caddy startup rejects invalid active upstreams\n'

for upstream in sub2api-blue:8080 sub2api-green:8080; do
  caddy=(docker run --rm
    --env "SITE_ADDRESS=sub2api.example.test"
    --env "SUB2API_ACTIVE_UPSTREAM=$upstream"
    --volume "$ROOT/infra/Caddyfile:/etc/caddy/Caddyfile:ro"
    "$CADDY_RUNTIME_IMAGE")
  "${caddy[@]}" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
  adapted=$("${caddy[@]}" caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile --pretty 2>/dev/null)
  [[ "$adapted" == *"$upstream"* ]] || fail "Caddy did not adapt the allowed upstream $upstream"
  [[ "$adapted" != *"sub2api:8080"* ]] || fail "Caddy adapted a removed sub2api:8080 route"
done

printf 'PASS: Caddy validates both allowed blue/green upstreams\n'
