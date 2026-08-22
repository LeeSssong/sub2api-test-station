#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
ENV_FILE=infra/.env.admin-lab.example
COMPOSE_FILE=infra/compose.admin-lab.yaml

command -v docker >/dev/null || { echo 'docker is required' >&2; exit 1; }
docker compose --project-name sub2api-admin-lab --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config --quiet

config=$(docker compose --project-name sub2api-admin-lab --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config)
for needle in \
  'admin-lab-postgres' 'admin-lab-redis' 'sub2api-admin-lab-network' 'sub2api_default' \
  'sub2api-admin-lab-postgres-data' 'sub2api-admin-lab-redis-data' \
  'context: /' 'dockerfile: Dockerfile' \
  'PAYMENT_PROVIDER: mock' 'UPSTREAM_PROVIDER: mock-upstream' 'NOTIFICATION_TRANSPORT: lab-outbox' \
  'MOCK_UPSTREAM_URL: http://admin-lab-mock-upstream:8091' \
  'MOCK_PAYMENT_URL: http://admin-lab-mock-payment:8092'; do
  grep -Fq "$needle" <<<"$config" || { echo "missing compose isolation contract: $needle" >&2; exit 1; }
done

# API and worker must be locally buildable from the checked-out Sub2API source,
# while retaining a deterministic image tag for promotion/reuse.  An image-only
# declaration would silently pull an unrelated registry artifact and violate
# the lab's source/build isolation contract.
for service in admin-lab-api admin-lab-worker; do
  service_config=$(docker compose --project-name sub2api-admin-lab --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config | awk "/^  ${service}:/{flag=1; next} flag && /^  [^ ]/{flag=0} flag")
  grep -Fq 'build:' <<<"$service_config" || { echo "$service missing local build strategy" >&2; exit 1; }
  grep -Fq 'context:' <<<"$service_config" || { echo "$service missing build context" >&2; exit 1; }
  grep -Fq 'dockerfile: Dockerfile' <<<"$service_config" || { echo "$service missing backend Dockerfile" >&2; exit 1; }
  grep -Fq 'image: sub2api-admin-lab:local' <<<"$service_config" || { echo "$service missing deterministic local image tag" >&2; exit 1; }
done

if grep -Eq 'DATABASE_HOST: (postgres|sub2api-postgres)|REDIS_HOST: (redis|sub2api-redis)' <<<"$config"; then
  echo 'production database/redis alias leaked into lab compose' >&2
  exit 1
fi
echo 'admin lab compose contract: PASS'
