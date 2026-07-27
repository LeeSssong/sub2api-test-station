#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'candidate_loader_install status=failed\n' >&2
  exit 1
}

[[ "$(uname -s)" == Linux ]] || fail
[[ ${EUID:-$(id -u)} -eq 0 ]] || fail
[[ "$(pwd -P)" == /opt/sub2api/production ]] || fail
[[ -z ${DOCKER_HOST:-} ]] || fail
command -v docker >/dev/null 2>&1 || fail
[[ "$(docker context show)" == default ]] || fail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
loader_source=${SUB2API_CANDIDATE_LOADER_BINARY:-}
public_key_file=${SUB2API_CANDIDATE_PUBLIC_KEY_FILE:-}
[[ "$loader_source" = /* && -f "$loader_source" && -x "$loader_source" ]] || fail
[[ "$public_key_file" = /* && -f "$public_key_file" && ! -L "$public_key_file" ]] || fail
key_mode=$(stat -c '%a' "$public_key_file")
(( (8#$key_mode & 8#022) == 0 )) || fail
key=$(tr -d '\r\n' <"$public_key_file")
[[ "$key" =~ ^ssh-ed25519[[:space:]][A-Za-z0-9+/=]+([[:space:]].*)?$ ]] || fail
ssh-keygen -lf "$public_key_file" >/dev/null 2>&1 || fail

install -d -o root -g root -m 0755 /usr/local/libexec
install -m 0755 -o root -g root "$loader_source" /usr/local/libexec/sub2api-candidate-loader
install -m 0755 -o root -g root "$root/ops/sub2api-candidate-ssh.sh" /usr/local/libexec/sub2api-candidate-ssh
install -d -o root -g root -m 0755 /etc/sub2api
install -m 0600 -o root -g root "$root/infra/sub2api-candidate-loader.env.example" \
  /etc/sub2api/sub2api-candidate-loader.env
install -d -o root -g root -m 0700 /var/lib/sub2api-candidate-loader

sudoers_temp=$(mktemp /tmp/sub2api-candidate-sudoers.XXXXXX)
cleanup() {
  rm -f -- "$sudoers_temp"
}
trap cleanup EXIT
printf '%s\n' \
  'ubuntu ALL=(root) NOPASSWD: /usr/local/libexec/sub2api-candidate-loader' \
  >"$sudoers_temp"
chmod 0440 "$sudoers_temp"
visudo -cf "$sudoers_temp" >/dev/null
install -m 0440 -o root -g root "$sudoers_temp" /etc/sudoers.d/sub2api-candidate-loader
visudo -cf /etc/sudoers.d/sub2api-candidate-loader >/dev/null

ubuntu_home=$(getent passwd ubuntu | cut -d: -f6)
[[ "$ubuntu_home" = /home/* && -d "$ubuntu_home" ]] || fail
install -d -o ubuntu -g ubuntu -m 0700 "$ubuntu_home/.ssh"
authorized_keys="$ubuntu_home/.ssh/authorized_keys"
touch "$authorized_keys"
chown ubuntu:ubuntu "$authorized_keys"
chmod 0600 "$authorized_keys"
forced_key='restrict,command="/usr/local/libexec/sub2api-candidate-ssh" '"$key"
if ! grep -Fqx "$forced_key" "$authorized_keys"; then
  printf '%s\n' "$forced_key" >>"$authorized_keys"
fi

printf 'candidate_loader_install status=succeeded\n'
