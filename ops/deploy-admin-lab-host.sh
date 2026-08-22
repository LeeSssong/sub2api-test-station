#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'admin lab host deploy failed: %s\n' "$1" >&2; exit 1; }
sha256_file() { sha256sum "$1" | awk '{print $1}'; }
bundle=''; bundle_sha=''; frontend_archive=''; frontend_sha=''; env_file=''; source_commit=''; source_tree=''; base_url=''; deploy_root=''; release_staging=''
ADMIN_LAB_ENV=''
while (($#)); do case "$1" in
  --bundle) bundle=$2; shift 2;; --bundle-sha256) bundle_sha=$2; shift 2;;
  --frontend-archive) frontend_archive=$2; shift 2;; --frontend-sha256) frontend_sha=$2; shift 2;;
  --env-file) env_file=$2; shift 2;; --source-commit) source_commit=$2; shift 2;; --source-tree) source_tree=$2; shift 2;;
  --base-url) base_url=$2; shift 2;; --deploy-root) deploy_root=$2; shift 2;; --release-staging-root) release_staging=$2; shift 2;;
  *) fail "unknown argument: $1";; esac; done
[[ "$bundle" == "$release_staging"/* && -f "$bundle" ]] || fail 'bundle path is invalid'
[[ "$frontend_archive" == "$release_staging"/* && -f "$frontend_archive" ]] || fail 'frontend archive path is invalid'
[[ "$env_file" == "$release_staging"/* && -f "$env_file" ]] || fail 'env path is invalid'
[[ "$bundle_sha" =~ ^[a-f0-9]{64}$ && "$(sha256_file "$bundle")" == "$bundle_sha" ]] || fail 'bundle checksum mismatch'
[[ "$frontend_sha" =~ ^[a-f0-9]{64}$ && "$(sha256_file "$frontend_archive")" == "$frontend_sha" ]] || fail 'frontend checksum mismatch'
[[ "$(stat -c '%a' "$env_file")" == 600 ]] || fail 'lab env must be mode 0600'
ADMIN_LAB_ENV="$env_file"
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'source identity invalid'
[[ "$deploy_root" == /opt/sub2api/production ]] || fail 'unexpected deploy root'
[[ "$base_url" =~ ^https://[A-Za-z0-9.-]+$ ]] || fail 'base URL invalid'
docker network inspect sub2api_default >/dev/null 2>&1 || fail 'production gateway network is missing'

rollback() {
  docker compose --project-name sub2api-admin-lab --project-directory "$deploy_root/infra" --env-file "$deploy_root/admin-lab/.env" -f "$deploy_root/infra/compose.admin-lab.yaml" down --remove-orphans >/dev/null 2>&1 || true
  if [[ -f "$deploy_root/admin-lab/Caddyfile.backup" ]]; then
    install -o root -g root -m 0644 "$deploy_root/admin-lab/Caddyfile.backup" "$deploy_root/Caddyfile"
    docker exec sub2api-caddy-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1 || true
  fi
}
trap rollback ERR

stage=$(mktemp -d "$deploy_root/.admin-lab-release-$source_commit.XXXXXX")
cleanup() { rm -rf -- "$stage"; }
finish() {
  status=$?
  if (( status != 0 )); then
    rollback
  fi
  cleanup
  exit "$status"
}
trap finish EXIT
tar -xf "$bundle" -C "$stage" || fail 'bundle extraction failed'
[[ -f "$stage/infra/Caddyfile" && -f "$stage/infra/compose.admin-lab.yaml" && -f "$stage/infra/admin-lab/gateway.conf" && -f "$stage/tools/admin-lab/mock_server.py" ]] || fail 'bundle contents incomplete'
docker load --input "$frontend_archive" >/dev/null || fail 'frontend image load failed'
frontend_image=$(grep '^ADMIN_LAB_FRONTEND_IMAGE=' "$env_file" | cut -d= -f2-)
[[ -n "$frontend_image" ]] || fail 'frontend image missing from env'
docker image inspect "$frontend_image" >/dev/null || fail 'loaded frontend image tag missing'

# Keep the isolated lab identity stable across releases. PostgreSQL and Redis
# volumes retain their credentials, and rotating them on every bundle would
# make a failed rollout permanently unable to reconnect to its own volumes.
effective_env="$stage/admin-lab.env"
install -o root -g root -m 0600 "$env_file" "$effective_env"
if [[ -f "$deploy_root/admin-lab/.env" ]]; then
  for key in \
    ADMIN_LAB_DB_NAME ADMIN_LAB_DB_USER ADMIN_LAB_DB_PASSWORD \
    ADMIN_LAB_REDIS_PASSWORD ADMIN_LAB_ADMIN_EMAIL ADMIN_LAB_ADMIN_PASSWORD \
    ADMIN_LAB_JWT_SECRET ADMIN_LAB_CSRF_SECRET ADMIN_LAB_COOKIE_NAME; do
    value=$(awk -F= -v wanted="$key" '$1 == wanted { sub(/^[^=]*=/, ""); print; exit }' "$deploy_root/admin-lab/.env")
    [[ -n "$value" && "$value" != *$'\n'* ]] || continue
    sed -i "s|^${key}=.*|${key}=${value}|" "$effective_env"
  done
fi

install -d -o root -g root -m 0755 "$deploy_root/infra/admin-lab" "$deploy_root/tools/admin-lab" "$deploy_root/admin-lab"
install -o root -g root -m 0644 "$deploy_root/Caddyfile" "$deploy_root/admin-lab/Caddyfile.backup"
install -o root -g root -m 0644 "$stage/infra/Caddyfile" "$deploy_root/Caddyfile"
install -o root -g root -m 0644 "$stage/infra/compose.admin-lab.yaml" "$deploy_root/infra/compose.admin-lab.yaml"
install -o root -g root -m 0644 "$stage/infra/.env.admin-lab.example" "$deploy_root/infra/.env.admin-lab.example"
install -o root -g root -m 0644 "$stage/infra/admin-lab/Dockerfile.frontend" "$deploy_root/infra/admin-lab/Dockerfile.frontend"
install -o root -g root -m 0644 "$stage/infra/admin-lab/nginx.conf" "$deploy_root/infra/admin-lab/nginx.conf"
install -o root -g root -m 0644 "$stage/infra/admin-lab/gateway.conf" "$deploy_root/infra/admin-lab/gateway.conf"
install -o root -g root -m 0644 "$stage/tools/admin-lab/mock_server.py" "$deploy_root/tools/admin-lab/mock_server.py"
install -o root -g root -m 0600 "$effective_env" "$deploy_root/admin-lab/.env"

compose=(docker compose --project-name sub2api-admin-lab --project-directory "$deploy_root/infra" --env-file "$deploy_root/admin-lab/.env" -f "$deploy_root/infra/compose.admin-lab.yaml")
"${compose[@]}" config --quiet || fail 'lab compose config failed'
"${compose[@]}" up -d --no-build --wait || fail 'lab compose did not become ready'
docker exec sub2api-caddy-1 caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null || fail 'Caddy lab route validation failed'
docker exec sub2api-caddy-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null || fail 'Caddy lab route reload failed'

html=$(curl -ksS --fail --max-time 20 "$base_url/admin/lab/") || fail 'public lab route probe failed'
grep -Fq '/admin/lab/assets/' <<<"$html" || fail 'public lab HTML does not contain lab asset base path'
  if grep -Eq 'src="/assets/|href="/assets/' <<<"$html"; then fail '主站 HTML returned for lab path'; fi
for service in admin-lab-api admin-lab-worker admin-lab-frontend admin-lab-gateway admin-lab-postgres admin-lab-redis admin-lab-mock-upstream admin-lab-mock-payment; do
  id=$("${compose[@]}" ps -q "$service")
  [[ -n "$id" ]] || fail "lab service missing: $service"
done
printf '{"schema_version":1,"result":"succeeded","downtime_required":false,"source_commit":"%s","source_tree":"%s","lab_html_contract":"passed","services":"admin-lab-api,admin-lab-frontend,admin-lab-gateway,admin-lab-postgres,admin-lab-redis"}\n' "$source_commit" "$source_tree"
