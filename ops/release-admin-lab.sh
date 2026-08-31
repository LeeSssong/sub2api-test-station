#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'admin lab release failed: %s\n' "$1" >&2; exit 1; }
sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }

worktree=$(pwd -P)
source_commit=$(git rev-parse HEAD) || fail 'not a git worktree'
source_tree=$(git rev-parse 'HEAD^{tree}') || fail 'source tree unavailable'
[[ -z "$(git status --porcelain)" ]] || fail 'worktree is dirty'
"$worktree/ops/assert-sub2api-release-source.sh" --mode rehearsal --worktree "$worktree" \
  || fail 'release source freshness check failed'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'invalid git identity'

ssh_target=${RELEASE_SSH_TARGET:-sub2api-prod}
ssh_key=${RELEASE_SSH_KEY:-$HOME/.ssh/tencent_lighthouse_seoul_sub2api}
ssh_port=${RELEASE_SSH_PORT:-2222}
known_hosts=${RELEASE_SSH_KNOWN_HOSTS:-$HOME/.ssh/known_hosts}
base_url=${RELEASE_BASE_URL:-https://api.xingqiaolab.top}
release_root=${RELEASE_DEPLOY_ROOT:-/opt/sub2api/production}
staging_root=${RELEASE_STAGING_ROOT:-/var/lib/sub2api/release-staging}
ssh_bin=${RELEASE_SSH_BIN:-ssh}; scp_bin=${RELEASE_SCP_BIN:-scp}; docker_bin=${RELEASE_DOCKER_BIN:-docker}

[[ "$ssh_key" == /* && -f "$ssh_key" && "$(mode_of "$ssh_key")" == 600 ]] || fail 'RELEASE_SSH_KEY must be a 0600 file'
[[ "$known_hosts" == /* && -f "$known_hosts" && "$(mode_of "$known_hosts")" == 600 ]] || fail 'RELEASE_SSH_KNOWN_HOSTS must be a 0600 file'
[[ "$ssh_port" =~ ^[1-9][0-9]{0,4}$ && "$ssh_port" -le 65535 ]] || fail 'invalid SSH port'
command -v "$ssh_bin" >/dev/null || fail 'ssh is required'
command -v "$scp_bin" >/dev/null || fail 'scp is required'
command -v "$docker_bin" >/dev/null || fail 'docker is required'

release_id="${source_commit:0:12}"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/admin-lab-release.XXXXXX")
cleanup() { rm -rf -- "$tmp"; }
trap cleanup EXIT
bundle="$tmp/admin-lab-bundle-$release_id.tar"
frontend_tag="sub2api-admin-lab-frontend:release-$source_commit"
frontend_image="$tmp/admin-lab-frontend-$release_id.tar"
backend_tag="sub2api-admin-lab-backend:release-$source_commit"
backend_image="$tmp/admin-lab-backend-$release_id.tar"
env_file="$tmp/admin-lab.env"
frontend_dist="$tmp/admin-lab-frontend-dist"
frontend_context="$tmp/admin-lab-frontend-context"

for path in infra/Caddyfile infra/compose.admin-lab.yaml infra/.env.admin-lab.example infra/admin-lab/Dockerfile.frontend infra/admin-lab/nginx.conf infra/admin-lab/gateway.conf tools/admin-lab/mock_server.py; do
  [[ -f "$path" && ! -L "$path" ]] || fail "missing release attachment: $path"
done
tar -cf "$bundle" infra/Caddyfile infra/compose.admin-lab.yaml infra/.env.admin-lab.example infra/admin-lab/Dockerfile.frontend infra/admin-lab/nginx.conf infra/admin-lab/gateway.conf tools/admin-lab/mock_server.py
bundle_sha=$(sha256_file "$bundle")

rm -rf -- "$frontend_dist" "$frontend_context"
VITE_APP_BASE_PATH=/admin/lab/ \
VITE_API_BASE_URL=/admin/lab/api/v1 \
VITE_AUTH_STORAGE_PREFIX=admin_lab_ \
VITE_ADMIN_LAB=1 \
VITE_BUILD_OUT_DIR="$frontend_dist" \
pnpm --dir upstream/sub2api/frontend build >/dev/null \
  || fail 'admin lab frontend build failed'
install -d -m 0700 "$frontend_context"
cp -R "$frontend_dist" "$frontend_context/dist"
find "$frontend_context/dist" -type d -exec chmod 0755 {} +
find "$frontend_context/dist" -type f -exec chmod 0644 {} +
cp infra/admin-lab/nginx.conf "$frontend_context/nginx.conf"
cat >"$frontend_context/Dockerfile" <<'DOCKERFILE'
FROM nginx:1.27-alpine
COPY dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 4173
DOCKERFILE
"$docker_bin" buildx build --platform linux/amd64 --provenance=false --sbom=false --load \
  --tag "$frontend_tag" -f "$frontend_context/Dockerfile" "$frontend_context" >/dev/null \
  || fail 'admin lab frontend image build failed'
"$docker_bin" image save --output "$frontend_image" "$frontend_tag" || fail 'frontend image archive failed'
frontend_sha=$(sha256_file "$frontend_image")

"$docker_bin" buildx build --platform linux/amd64 --provenance=false --sbom=false --load \
  --tag "$backend_tag" -f upstream/sub2api/Dockerfile upstream/sub2api >/dev/null \
  || fail 'admin lab backend image build failed'
"$docker_bin" image save --output "$backend_image" "$backend_tag" || fail 'backend image archive failed'
backend_sha=$(sha256_file "$backend_image")

admin_image="$backend_tag"

random_secret() { openssl rand -hex 32; }
cat >"$env_file" <<EOF
COMPOSE_PROJECT_NAME=sub2api-admin-lab
ADMIN_LAB_GATEWAY_NETWORK=sub2api_default
LAB_ONLY=1
ADMIN_LAB_IMAGE=$admin_image
ADMIN_LAB_FRONTEND_IMAGE=$frontend_tag
ADMIN_LAB_POSTGRES_IMAGE=postgres:18-alpine
ADMIN_LAB_REDIS_IMAGE=redis:7-alpine
ADMIN_LAB_DB_NAME=sub2api_lab
ADMIN_LAB_DB_USER=sub2api_lab
ADMIN_LAB_DB_PASSWORD=$(random_secret)
ADMIN_LAB_REDIS_PASSWORD=$(random_secret)
ADMIN_LAB_ADMIN_EMAIL=admin-lab@example.test
ADMIN_LAB_ADMIN_PASSWORD=$(random_secret)
ADMIN_LAB_JWT_SECRET=$(random_secret)
ADMIN_LAB_CSRF_SECRET=$(random_secret)
ADMIN_LAB_COOKIE_NAME=sub2api_lab_session
ADMIN_LAB_FRONTEND_BASE_PATH=/admin/lab/
ADMIN_LAB_PAYMENT_PROVIDER=mock
ADMIN_LAB_UPSTREAM_PROVIDER=mock-upstream
ADMIN_LAB_EXTERNAL_ALLOWLIST=http://admin-lab-mock-upstream:8091,http://admin-lab-mock-payment:8092
EOF
chmod 600 "$env_file"

remote_tmp=$("$ssh_bin" -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$known_hosts" -p "$ssh_port" "$ssh_target" umask 077 '&&' mktemp -d -p /tmp ".admin-lab-$source_commit.XXXXXX" | tr -d '[:space:]') || fail 'remote staging unavailable'
scp_opts=(-q -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$known_hosts" -P "$ssh_port")
for file in "$bundle" "$env_file" "$frontend_image" "$backend_image" ops/deploy-admin-lab-host.sh; do
  "$scp_bin" "${scp_opts[@]}" "$file" "$ssh_target:$remote_tmp/$(basename "$file")" \
    || fail "admin lab release transfer failed: $(basename "$file")"
done

host_output=$("$ssh_bin" -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n bash -s -- "$remote_tmp" "$staging_root" "$release_id" "$bundle_sha" "$frontend_sha" "$backend_sha" "$source_commit" "$source_tree" "$base_url" "$release_root" <<'REMOTE'
set -euo pipefail
remote=$1; staging=$2; release_id=$3; bundle_sha=$4; frontend_sha=$5; backend_sha=$6; source_commit=$7; source_tree=$8; base_url=$9; release_root=${10}
sudo install -d -o root -g root -m 0700 "$staging"
sudo install -o root -g root -m 0600 "$remote/admin-lab-bundle-$release_id.tar" "$staging/admin-lab-bundle-$release_id.tar"
sudo install -o root -g root -m 0600 "$remote/admin-lab-frontend-$release_id.tar" "$staging/admin-lab-frontend-$release_id.tar"
sudo install -o root -g root -m 0600 "$remote/admin-lab-backend-$release_id.tar" "$staging/admin-lab-backend-$release_id.tar"
sudo install -o root -g root -m 0600 "$remote/admin-lab.env" "$staging/admin-lab.env"
sudo install -o root -g root -m 0755 "$remote/deploy-admin-lab-host.sh" /usr/local/libexec/deploy-admin-lab-host.sh
sudo bash /usr/local/libexec/deploy-admin-lab-host.sh \
  --bundle "$staging/admin-lab-bundle-$release_id.tar" --bundle-sha256 "$bundle_sha" \
  --frontend-archive "$staging/admin-lab-frontend-$release_id.tar" --frontend-sha256 "$frontend_sha" \
  --backend-archive "$staging/admin-lab-backend-$release_id.tar" --backend-sha256 "$backend_sha" \
  --env-file "$staging/admin-lab.env" --source-commit "$source_commit" --source-tree "$source_tree" \
  --base-url "$base_url" --deploy-root "$release_root" --release-staging-root "$staging"
rm -rf -- "$remote"
REMOTE
)
printf '%s\n' "$host_output"
ruby -rjson -e 'v=JSON.parse(STDIN.read); abort unless v["result"]=="succeeded" && v["downtime_required"]==false && v["lab_html_contract"]=="passed"' <<<"$host_output" \
  || fail 'host executor did not report strict lab success'
