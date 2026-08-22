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
  'PAYMENT_PROVIDER: mock' 'UPSTREAM_PROVIDER: mock-upstream' \
  'MOCK_UPSTREAM_URL: http://admin-lab-mock-upstream:8091' \
  'MOCK_PAYMENT_URL: http://admin-lab-mock-payment:8092'; do
  grep -Fq "$needle" <<<"$config" || { echo "missing compose isolation contract: $needle" >&2; exit 1; }
done

if grep -Eq 'DATABASE_HOST: (postgres|sub2api-postgres)|REDIS_HOST: (redis|sub2api-redis)' <<<"$config"; then
  echo 'production database/redis alias leaked into lab compose' >&2
  exit 1
fi
echo 'admin lab compose contract: PASS'
