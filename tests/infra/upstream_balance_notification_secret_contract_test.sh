#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

production_json=$(
  SUB2API_RELEASE_ENV_FILE="$ROOT/config/releases/sub2api.env" \
  SUB2API_BLUE_IMAGE=example.invalid/sub2api-blue@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  SUB2API_GREEN_IMAGE=example.invalid/sub2api-green@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  SUB2API_WORKER_IMAGE=example.invalid/sub2api-worker@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc \
  SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 \
  SUB2API_ACTIVE_SLOT=blue \
  SUB2API_PREVIOUS_SLOT=green \
  SUB2API_MODEL_DETECTOR_TOKEN=example-only-detector-token \
  SUB2API_UPSTREAM_BALANCE_SECRET_HOST_DIR=/tmp/sub2api-upstream-balance-contract \
  docker compose \
    --project-name sub2api-secret-contract \
    --project-directory "$ROOT" \
    --env-file "$ROOT/infra/.env.example" \
    -f "$ROOT/infra/compose.yaml" \
    config --format json
)

ruby -rjson -e '
  config = JSON.parse(STDIN.read)
  services = config.fetch("services")
  worker = services.fetch("sub2api-worker")
  environment = worker.fetch("environment")
  expected = {
    "SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED" => "false",
    "SUB2API_UPSTREAM_BALANCE_FEISHU_APP_ID_FILE" => "/run/secrets/upstream-balance/feishu-app-id",
    "SUB2API_UPSTREAM_BALANCE_FEISHU_APP_SECRET_FILE" => "/run/secrets/upstream-balance/feishu-app-secret",
    "SUB2API_UPSTREAM_BALANCE_FEISHU_CHAT_ID_FILE" => "/run/secrets/upstream-balance/feishu-alert-chat-id",
    "SUB2API_UPSTREAM_BALANCE_FEISHU_RECIPIENTS_FILE" => "/run/secrets/upstream-balance/feishu-alert-recipients.json",
    "SUB2API_UPSTREAM_BALANCE_LOGIN_REGISTRY_FILE" => "/run/secrets/upstream-balance/upstream-login-registry.json"
  }
  expected.each do |key, value|
    raise "worker environment mismatch for #{key}" unless environment[key] == value
  end

  secret_mounts = worker.fetch("volumes").select { |volume| volume["target"] == "/run/secrets/upstream-balance" }
  raise "worker must have exactly one upstream balance secret mount" unless secret_mounts.length == 1
  mount = secret_mounts.first
  raise "secret mount must be a read-only bind" unless mount["type"] == "bind" && mount["read_only"] == true
  raise "secret mount source mismatch" unless mount["source"] == "/tmp/sub2api-upstream-balance-contract"
  raise "secret mount must not create the host directory" unless mount.dig("bind", "create_host_path") == false

  %w[sub2api-blue sub2api-green].each do |name|
    service = services.fetch(name)
    raise "#{name} must not receive notification environment" if service.fetch("environment").keys.any? { |key| key.start_with?("SUB2API_UPSTREAM_BALANCE_") }
    raise "#{name} must not mount notification secrets" if service.fetch("volumes").any? { |volume| volume["target"] == "/run/secrets/upstream-balance" }
  end
' <<<"$production_json"

for compose_file in infra/compose.acceptance.yaml infra/compose.sub2api-rehearsal.yaml; do
  if rg -q 'SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED|/run/secrets/upstream-balance' "$ROOT/$compose_file"; then
    printf 'notification sending must remain disabled and unmounted in %s\n' "$compose_file" >&2
    exit 1
  fi
done
