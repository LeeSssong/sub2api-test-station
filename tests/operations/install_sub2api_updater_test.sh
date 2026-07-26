#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
SERVICE="$ROOT/infra/systemd/sub2api-updater.service"
ENV_EXAMPLE="$ROOT/infra/systemd/sub2api-updater.env.example"
INSTALLER="$ROOT/ops/install-sub2api-updater.sh"
MAIN="$ROOT/sub2api-updater/cmd/sub2api-updater/main.go"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "missing ${1#"$ROOT/"}"
}

require_file "$SERVICE"
require_file "$ENV_EXAMPLE"
require_file "$INSTALLER"
require_file "$MAIN"

service=$(<"$SERVICE")
environment=$(<"$ENV_EXAMPLE")

for setting in \
  'User=root' \
  'Group=root' \
  'Type=simple' \
  'WorkingDirectory=/opt/sub2api/production' \
  'EnvironmentFile=/etc/sub2api/sub2api-updater.env' \
  'RuntimeDirectory=sub2api-updater' \
  'RuntimeDirectoryMode=0755' \
  'RuntimeDirectoryPreserve=restart' \
  'StateDirectory=sub2api-updater' \
  'StateDirectoryMode=0700' \
  'UMask=0077' \
  'NoNewPrivileges=true' \
  'PrivateTmp=true' \
  'ProtectHome=true' \
  'ProtectSystem=strict' \
  'ProtectKernelTunables=true' \
  'ProtectControlGroups=true' \
  'RestrictSUIDSGID=true' \
  'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6' \
  'ReadWritePaths=/opt/sub2api/production /var/lib/sub2api-updater'; do
  rg -Fq "$setting" <<<"$service" || fail "service is missing: $setting"
done

for forbidden in \
  'docker compose down' \
  'POSTGRES_PASSWORD=' \
  'ADMIN_API_KEY=' \
  'Bearer '; do
  ! rg -Fq "$forbidden" "$SERVICE" "$ENV_EXAMPLE" "$INSTALLER" \
    || fail "packaging files contain forbidden content: $forbidden"
done

for setting in \
  'SUB2API_UPDATER_SOCKET=/run/sub2api-updater/updater.sock' \
  'SUB2API_UPDATER_STATE=/var/lib/sub2api-updater/state.json' \
  'SUB2API_UPDATER_EXECUTOR=/opt/sub2api/production/ops/update-sub2api-host.sh' \
  'SUB2API_UPDATER_OFFICIAL_API=https://api.xingqiaolab.top' \
  'SUB2API_UPDATER_OFFICIAL_DIAL_ADDRESS=127.0.0.1:443' \
  'SUB2API_UPDATER_ORIGIN=https://api.xingqiaolab.top' \
  'SUB2API_BASE_URL=https://api.xingqiaolab.top' \
  'SUB2API_ADMIN_API_KEY_FILE=/opt/sub2api/production/secrets/sub2api-admin-api-key' \
  'SUB2API_GATEWAY_API_KEY_FILE=/opt/sub2api/production/secrets/sub2api-gateway-api-key' \
  'SUB2API_UPDATER_GITHUB_LATEST_RELEASE='; do
  rg -Fq "$setting" <<<"$environment" || fail "env example is missing: $setting"
done

rg -Fq 'GOOS=linux GOARCH=amd64 CGO_ENABLED=0' "$INSTALLER" \
  || fail 'installer does not cross-compile the pinned target'
rg -Fq 'SUB2API_UPDATER_BINARY' "$INSTALLER" \
  || fail 'installer does not accept a prebuilt updater binary'
! rg -n '^require_command go$' "$INSTALLER" \
  || fail 'installer requires Go before checking for a prebuilt binary'
! rg -Fq 'getent group caddy' "$INSTALLER" \
  || fail 'installer requires a host caddy group that may not exist'
! rg -Fq -- '-g caddy' "$INSTALLER" \
  || fail 'installer requires a host caddy group that may not exist'
rg -Fq 'install -d -o root -g root -m 0755 "$runtime_path"' "$INSTALLER" \
  || fail 'installer does not create the runtime directory without a host caddy group'
rg -Fq 'WriteTimeout:      16 * time.Minute' "$MAIN" \
  || fail 'updater HTTP write timeout is shorter than the Caddy update response window'
rg -Fq -- '--official-dial-address ${SUB2API_UPDATER_OFFICIAL_DIAL_ADDRESS}' "$SERVICE" \
  || fail 'updater service does not pin official authentication to loopback Caddy'
rg -Fq 'uname -s' "$INSTALLER" || fail 'installer does not enforce the Linux host boundary'
rg -Fq '/opt/sub2api/production' "$INSTALLER" \
  || fail 'installer does not enforce the production directory boundary'
rg -Fq 'docker context show' "$INSTALLER" \
  || fail 'installer does not verify the default Docker context'
rg -Fq 'require_command docker' "$INSTALLER" \
  || fail 'installer does not require Docker'
rg -Fq 'systemctl daemon-reload' "$INSTALLER" \
  || fail 'installer does not reload systemd'
rg -Fq 'systemctl enable --now sub2api-updater.service' "$INSTALLER" \
  || fail 'installer does not enable the updater service'
rg -Fq 'systemctl restart sub2api-updater.service' "$INSTALLER" \
  || fail 'installer does not restart an already-running updater onto the new binary'
rg -Fq 'systemd-analyze verify' "$INSTALLER" \
  || fail 'installer does not verify the unit before activation'
rg -Fq 'install -m 0755' "$INSTALLER" || fail 'installer does not install the binary mode'
rg -Fq 'install -m 0700' "$INSTALLER" || fail 'installer does not install the executor mode'
rg -Fq 'install -m 0600' "$INSTALLER" || fail 'installer does not install the environment mode'
rg -Fq 'install -m 0644' "$INSTALLER" || fail 'installer does not install the unit mode'

if [[ "$(uname -s)" != 'Linux' ]]; then
  marker=$(mktemp)
  trap 'rm -f "$marker"' EXIT
  if SUB2API_INSTALL_TEST_MARKER="$marker" bash "$INSTALLER" >"$marker.out" 2>&1; then
    fail 'installer accepted a non-Linux host'
  fi
  rg -qi 'Linux.*production host|production host.*Linux' "$marker.out" \
    || fail 'non-Linux refusal did not explain the host boundary'
  [[ ! -s "$marker" ]] || fail 'installer mutated state before refusing the host'
fi

if command -v systemd-analyze >/dev/null 2>&1; then
  systemd-analyze verify "$SERVICE" \
    || fail 'systemd-analyze rejected the updater unit'
else
  printf 'SKIP: systemd-analyze is unavailable; unit verification not run\n'
fi

printf 'PASS: Sub2API updater packaging contracts\n'
