#!/usr/bin/env bash
set -euo pipefail
umask 077

production_root='/opt/sub2api/production'
service_name='sub2api-updater.service'
binary_path='/usr/local/libexec/sub2api-updater'
state_path='/var/lib/sub2api-updater'
runtime_path='/run/sub2api-updater'
environment_path='/etc/sub2api/sub2api-updater.env'
unit_path="/etc/systemd/system/$service_name"

fail() {
  printf 'sub2api-updater install failed: %s\n' "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require_absolute_file() {
  local path=$1 label=$2
  [[ -f "$path" && ! -L "$path" ]] || fail "$label is missing or a symlink"
}

[[ "$(uname -s)" == 'Linux' ]] \
  || fail 'installation is allowed only on the Linux production host'
[[ -z "${DOCKER_HOST:-}" ]] \
  || fail 'installation must use the default Docker context'
[[ "$EUID" -eq 0 ]] || fail 'installation must run as root'
[[ -d "$production_root" ]] || fail "missing production root: $production_root"
[[ "$(pwd -P)" == "$production_root" ]] \
  || fail "run from the production root: $production_root"

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
[[ "$repo_root" == "$production_root" ]] \
  || fail 'installer must be executed from the production checkout'

require_command install
require_command systemctl
require_command systemd-analyze
require_command docker

[[ "$(docker context show)" == 'default' ]] \
  || fail 'installation must use the default Docker context'

service_source="$repo_root/infra/systemd/sub2api-updater.service"
environment_source="$repo_root/infra/systemd/sub2api-updater.env.example"
executor_source="$repo_root/ops/update-sub2api-host.sh"
updater_dir="$repo_root/sub2api-updater"
require_absolute_file "$service_source" 'systemd unit'
require_absolute_file "$environment_source" 'environment example'
require_absolute_file "$executor_source" 'host executor'
[[ -d "$updater_dir" ]] || fail 'updater Go module is missing'
[[ -f "$repo_root/infra/Caddyfile" ]] || fail 'Caddy configuration is missing'
[[ -f "$repo_root/infra/compose.yaml" ]] || fail 'Compose configuration is missing'
[[ -f "$repo_root/infra/sub2api-update-ui/update-ui.js" ]] \
  || fail 'update UI asset is missing'

systemd-analyze verify "$service_source" \
  || fail 'systemd rejected the updater unit'

staging=$(mktemp -d /tmp/sub2api-updater-install.XXXXXX)
cleanup() {
  rm -rf -- "$staging"
}
trap cleanup EXIT

if [[ -n "${SUB2API_UPDATER_BINARY:-}" ]]; then
  require_absolute_file "$SUB2API_UPDATER_BINARY" 'prebuilt updater binary'
  cp "$SUB2API_UPDATER_BINARY" "$staging/sub2api-updater"
else
  require_command go
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go -C "$updater_dir" build -trimpath -ldflags '-s -w' \
    -o "$staging/sub2api-updater" ./cmd/sub2api-updater
fi
[[ -x "$staging/sub2api-updater" ]] || fail 'updater binary was not built'

install -d -o root -g root -m 0755 /usr/local/libexec
install -d -o root -g root -m 0755 /etc/sub2api
install -d -o root -g root -m 0700 "$state_path"
install -d -o root -g root -m 0755 "$runtime_path"
install -m 0755 -o root -g root "$staging/sub2api-updater" "$binary_path"
install -m 0700 -o root -g root "$executor_source" "$staging/update-sub2api-host.sh"
install -m 0700 -o root -g root "$staging/update-sub2api-host.sh" "$executor_source"
install -m 0600 -o root -g root "$environment_source" "$environment_path"
install -m 0644 -o root -g root "$service_source" "$unit_path"

systemctl daemon-reload
systemctl enable --now sub2api-updater.service
# enable --now leaves an already-active service on the previous binary, which is
# exactly how the updater and its executor drifted apart.
systemctl restart sub2api-updater.service
printf 'sub2api-updater installed: service=%s socket=%s\n' \
  "$service_name" "$runtime_path/updater.sock"
