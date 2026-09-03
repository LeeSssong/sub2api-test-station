#!/usr/bin/env bash
set -euo pipefail
umask 077
fail(){ printf 'test_station_host status=failed: %s\n' "$1" >&2; exit 1; }
sha256_file(){ if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"|awk '{print $1}'; else shasum -a 256 "$1"|awk '{print $1}'; fi; }
staging_root= image_archive= image_sha256= compose_file= caddy_file= env_file= source_commit= source_tree= deploy_root=
while (($#)); do
  case "$1" in
    --staging-root|--image-archive|--image-sha256|--compose|--caddy|--env-file|--source-commit|--source-tree|--deploy-root)
      (($# >= 2)) || fail "$1 requires a value"; key=${1#--}; key=${key//-/_}; case "$key" in compose) key=compose_file;; caddy) key=caddy_file;; esac; printf -v "$key" '%s' "$2"; shift 2;;
    *) fail "unknown argument: $1";;
  esac
done
[[ "$staging_root" == /* && -d "$staging_root" && ! -L "$staging_root" ]] || fail 'staging root is invalid'
for path in "$image_archive" "$compose_file" "$caddy_file"; do [[ "$path" == "$staging_root"/* && -f "$path" && ! -L "$path" ]] || fail 'bundle file is invalid'; done
if [[ -n "$env_file" ]]; then [[ "$env_file" == "$staging_root"/* && -f "$env_file" && ! -L "$env_file" ]] || fail 'env file is invalid'; else env_file="$deploy_root/.env"; fi
[[ "$image_sha256" =~ ^[a-f0-9]{64}$ && "$(sha256_file "$image_archive")" == "$image_sha256" ]] || fail 'image archive checksum mismatch'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'source identity is invalid'
if [[ "${TEST_STATION_TEST_MODE:-false}" != true ]]; then [[ "$deploy_root" == /opt/sub2api-test-station || "$deploy_root" == /opt/sub2api-test-station/* ]] || fail 'deploy root is not the independent test station'; fi
[[ -d "$deploy_root" && ! -L "$deploy_root" ]] || fail 'deploy root is invalid'
grep -Eq '^name:[[:space:]]*sub2api-test-station[[:space:]]*$' "$compose_file" || fail 'compose project identity mismatch'
grep -q 'sub2api-test-station-network' "$compose_file" || fail 'compose network identity mismatch'
docker_bin=${DOCKER_BIN:-docker}; command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker is required'
release_dir="$deploy_root/releases/$source_commit"; mkdir -p "$release_dir"
cp "$compose_file" "$release_dir/compose.yaml"; cp "$caddy_file" "$release_dir/Caddyfile"; cp "$env_file" "$release_dir/.env"; chmod 600 "$release_dir/.env"
"$docker_bin" load --input "$image_archive" >/dev/null
compose=("$docker_bin" compose --project-name sub2api-test-station --env-file "$release_dir/.env" -f "$release_dir/compose.yaml")
"${compose[@]}" config --quiet >/dev/null || fail 'Compose preflight failed'
"${compose[@]}" up -d --remove-orphans >/dev/null || fail 'Compose start failed'
for service in test-station-api test-station-worker test-station-detector test-station-caddy; do found=$("${compose[@]}" ps "$service" --format '{{.Name}} {{.State}}' 2>/dev/null || true); [[ "$found" == *healthy* || "$found" == *running* ]] || fail "service is not healthy: $service"; done
state=${RELEASE_STATE:-$deploy_root/release-state.json}; mkdir -p "$(dirname "$state")"; tmp=$(mktemp "$(dirname "$state")/.release-state.XXXXXX"); chmod 600 "$tmp"
python3 - "$tmp" "$source_commit" "$source_tree" "$image_sha256" "$release_dir" <<'PY'
import json,sys,datetime,os
path,commit,tree,digest,release=sys.argv[1:]
value={"source_commit":commit,"source_tree":tree,"image_digest":digest,"release_dir":release,"previous_release_dir":None,"project_name":"sub2api-test-station","result":"succeeded","updated_at":datetime.datetime.now(datetime.timezone.utc).isoformat()}
with open(path,"w",encoding="utf-8") as f: json.dump(value,f,separators=(",",":")); f.write("\n")
os.chmod(path,0o600)
PY
mv -f "$tmp" "$state"
printf 'test_station_host status=succeeded source_commit=%s source_tree=%s release_dir=%s\n' "$source_commit" "$source_tree" "$release_dir"
