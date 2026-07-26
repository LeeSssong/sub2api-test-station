# Sub2API Official Image Release Runbook

This runbook promotes a pinned official Sub2API image by recreating only the
`sub2api` service. PostgreSQL, Redis, Caddy, and relay-ops are not recreated.
The CLI path and the official admin UI update button both end at the same
root-owned host executor; neither path invokes the in-container Docker updater.

Production commands must start with `ssh sub2api-prod` and run from
`/opt/sub2api/production`. A `/Users/...` path, Docker context `colima`, or
Compose project `sub2api-deploy` identifies the local Mac deployment and must
never be used for production. The generic release orchestrator currently
models bind-mounted rehearsal storage and must not be invoked with
`--mode production` against the named-volume production deployment.

## Prerequisites

- Production was started with Compose project `sub2api`. Rehearsal uses
  the isolated project `sub2api-official-rehearsal`; the two names are not
  interchangeable.
- `docker`, Docker Compose, `jq`, `sha256sum`, and at least 2 GiB free at both
  the application-data and release-backup roots are available.
- The release environment contains exactly one immutable
  `SUB2API_IMAGE=repository:version@sha256:<64-hex-digest>` value. Do not put
  API keys in it.
- The secret environment and the protected admin and bounded gateway API-key
  files already exist. Their values must never be pasted into a terminal,
  release record, ticket, or chat.
- Production uses the named volumes `sub2api_sub2api_data`,
  `sub2api_postgres_data`, and `sub2api_redis_data`. Their runtime destinations
  remain `/app/data`, `/var/lib/postgresql/data`, and `/data`.
- The previous custom image is still present locally. It is only a rollback
  candidate after the operator has established `ROLLBACK_COMPATIBLE=true`.

## Rehearsal

Changing only a Compose project name does not isolate bind mounts. Never point
a rehearsal variable at `/opt/sub2api/production`, a production data root, or
the public production URL. Start in the checked-out release repository and
create a disposable context:

```bash
export RELEASE_TOOL_ROOT="$(pwd -P)"
export REHEARSAL_ROOT="$(mktemp -d /tmp/sub2api-official-rehearsal.XXXXXX)"
export REHEARSAL_SUB2API_DATA_DIR="$REHEARSAL_ROOT/app-data"
export REHEARSAL_POSTGRES_DATA_DIR="$REHEARSAL_ROOT/postgres-data"
export REHEARSAL_REDIS_DATA_DIR="$REHEARSAL_ROOT/redis-data"
mkdir -p "$REHEARSAL_SUB2API_DATA_DIR" "$REHEARSAL_POSTGRES_DATA_DIR" \
  "$REHEARSAL_REDIS_DATA_DIR" "$REHEARSAL_ROOT/backups" "$REHEARSAL_ROOT/records"
chmod 0700 "$REHEARSAL_ROOT" "$REHEARSAL_SUB2API_DATA_DIR" \
  "$REHEARSAL_POSTGRES_DATA_DIR" "$REHEARSAL_REDIS_DATA_DIR" \
  "$REHEARSAL_ROOT/backups" "$REHEARSAL_ROOT/records"

export COMPOSE_PROJECT_NAME=sub2api-official-rehearsal
export DEPLOY_ROOT="$RELEASE_TOOL_ROOT/infra"
export BASE_COMPOSE="$DEPLOY_ROOT/compose.sub2api-rehearsal.yaml"
export IMAGE_OVERLAY="$DEPLOY_ROOT/compose.sub2api-release.yaml"
export SECRET_ENV="$REHEARSAL_ROOT/secret.env"
export RELEASE_ENV="$REHEARSAL_ROOT/release.env"
export SUB2API_DATA_DIR="$REHEARSAL_SUB2API_DATA_DIR"
export POSTGRES_DATA_DIR="$REHEARSAL_POSTGRES_DATA_DIR"
export REDIS_DATA_DIR="$REHEARSAL_REDIS_DATA_DIR"
export BACKUP_ROOT="$REHEARSAL_ROOT/backups"
export RELEASE_RECORD_ROOT="$REHEARSAL_ROOT/records"
export ADMIN_API_KEY_FILE="$REHEARSAL_ROOT/admin-api-key"
export GATEWAY_API_KEY_FILE="$REHEARSAL_ROOT/gateway-api-key"
export BASE_URL=http://127.0.0.1:18080
export REHEARSAL_ROLLBACK_IMAGE='xingqiao-sub2api:v0.1.164-contact-v1'
export PREVIOUS_EXPECTED_VERSION=0.1.164
```

`REHEARSAL_ROOT`, all three `REHEARSAL_*_DIR` values, and every path consumed by
the orchestrator must be absolute. Install a rehearsal-only `0600` secret env,
the exact official release env, and bounded API keys into the temporary root.
Do not symlink them or reference a production secret file directly:

```bash
install -m 0600 "$REHEARSAL_SECRET_ENV_SOURCE" "$SECRET_ENV"
install -m 0600 config/releases/sub2api.env "$RELEASE_ENV"
install -m 0600 "$REHEARSAL_ADMIN_API_KEY_SOURCE" "$ADMIN_API_KEY_FILE"
install -m 0600 "$REHEARSAL_GATEWAY_API_KEY_SOURCE" "$GATEWAY_API_KEY_FILE"
```

Verify the selected source backup before using it. Then provision only the
isolated PostgreSQL and Redis services with the same project directory, env
files, and base Compose file that the orchestrator will inspect:

```bash
(cd "$SOURCE_BACKUP" && sha256sum -c SHA256SUMS)

docker compose --project-name "$COMPOSE_PROJECT_NAME" \
  --project-directory "$DEPLOY_ROOT" \
  --env-file "$SECRET_ENV" --env-file "$RELEASE_ENV" \
  -f "$BASE_COMPOSE" up -d postgres redis

docker compose --project-name "$COMPOSE_PROJECT_NAME" \
  --project-directory "$DEPLOY_ROOT" \
  --env-file "$SECRET_ENV" --env-file "$RELEASE_ENV" \
  -f "$BASE_COMPOSE" exec -T postgres \
  sh -c 'exec pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges' \
  <"$SOURCE_BACKUP/sub2api.dump"

tar -C "$REHEARSAL_SUB2API_DATA_DIR" -xzf "$SOURCE_BACKUP/app-data.tar.gz"

docker compose --project-name "$COMPOSE_PROJECT_NAME" \
  --project-directory "$DEPLOY_ROOT" \
  --env-file "$SECRET_ENV" --env-file "$RELEASE_ENV" \
  -f "$BASE_COMPOSE" up -d --build sub2api caddy
```

Only Caddy publishes a port, exactly `127.0.0.1:18080:8080`. It serves
the locally built homepage at `/support`, enforces the Docker update/rollback
guards, and proxies all other traffic to the unexposed rehearsal `sub2api`.
Configure the restored support menu through the same isolated Compose context,
then invoke the orchestrator:

```bash
SUB2API_COMPOSE_PROJECT="$COMPOSE_PROJECT_NAME" \
  SUB2API_PROJECT_DIRECTORY="$DEPLOY_ROOT" \
  SUB2API_SECRET_ENV_FILE="$SECRET_ENV" \
  SUB2API_RELEASE_ENV_FILE="$RELEASE_ENV" \
  SUB2API_COMPOSE_FILE="$BASE_COMPOSE" \
  SUB2API_IMAGE_OVERLAY="$IMAGE_OVERLAY" \
  SUB2API_DATA_DIR="$REHEARSAL_SUB2API_DATA_DIR" \
  bash "$RELEASE_TOOL_ROOT/ops/configure-sub2api-support.sh"

ROLLBACK_COMPATIBLE=true \
  bash "$RELEASE_TOOL_ROOT/ops/deploy-sub2api-release.sh" --mode rehearsal
```

The orchestrator resolves all containers with the selected Compose context,
requires runtime `working_dir` and `config_files` labels to match it, creates a
fresh rehearsal backup, and recreates only `sub2api` with
`--no-deps --force-recreate`. Preserve the promoted `0600` record outside the
temporary root before teardown:

```bash
export PROMOTED_REHEARSAL_RECORD="$(find "$RELEASE_RECORD_ROOT" -maxdepth 1 -type f -name '*.json' -print -quit)"
jq -e '.mode == "rehearsal" and .state == "promoted" and (.checks | all(.[]; . == true))' \
  "$PROMOTED_REHEARSAL_RECORD"
install -m 0600 "$PROMOTED_REHEARSAL_RECORD" "$REHEARSAL_EVIDENCE_RECORD"
```

For at least 30 minutes after a promoted rehearsal, observe the service health,
gateway models endpoint, error rate, and the read-only relay-ops views. Perform
one capped-key non-streaming and streaming inference manually during this
window; automated smoke checks intentionally do not make a paid request.

Keep the resulting `0600` promoted JSON record. It records the prior image and
ID, requested immutable reference, backup path, checks, mode, and outcome.

After evidence is preserved, tear down only the isolated project. The guard on
the temporary root must pass before removing its bind directories:

```bash
docker compose --project-name sub2api-official-rehearsal \
  --project-directory "$DEPLOY_ROOT" \
  --env-file "$SECRET_ENV" --env-file "$RELEASE_ENV" \
  -f "$BASE_COMPOSE" -f "$IMAGE_OVERLAY" down -v

case "$REHEARSAL_ROOT" in
  /tmp/sub2api-official-rehearsal.*) rm -rf -- "$REHEARSAL_ROOT" ;;
  *) printf 'Refusing unexpected rehearsal root: %s\n' "$REHEARSAL_ROOT" >&2; exit 1 ;;
esac
```

Confirm separately that the local `sub2api-deploy` project and its three local
data roots were unchanged. Never adapt this teardown command to production.

## Production

Production is a remote named-volume deployment. Establish the host boundary
before resolving or changing any Compose state:

```bash
ssh -o BatchMode=yes sub2api-prod
set -euo pipefail
test "$(uname -s)" = Linux
test "$(docker context show)" = default
test -z "${DOCKER_HOST:-}"
cd /opt/sub2api/production
test "$(pwd -P)" = /opt/sub2api/production
```

Before every release, create and validate a PostgreSQL dump plus `/app/data`
archive under `backups/release`, record the Sub2API/PostgreSQL/Redis container
IDs, tag the current Sub2API image for rollback, and pull the exact official
`repository:version@sha256:digest` reference. Update only the `sub2api.image`
line in `compose.yaml`, validate the resolved image, then run:

```bash
sudo docker compose --project-name sub2api --env-file .env -f compose.yaml \
  config --quiet
sudo docker compose --project-name sub2api --env-file .env -f compose.yaml \
  up -d --no-deps --force-recreate sub2api
```

Wait for health, compare dependency container IDs and named-volume sources with
the pre-release record, verify the image ID/digest, public health, data counts,
the `xingqiao-support` menu, and recent logs. A failed check rolls back by
restoring the previous image line and recreating only `sub2api`. Never run
`down` or recreate PostgreSQL/Redis for an application image release.

## Host Updater

The durable one-click path runs from the production host with the same
`ops/update-sub2api-host.sh` executor used by the scheduler. The executor
requires Linux, Docker context `default`, the exact production directory
`/opt/sub2api/production`, Compose project `sub2api`, the three named volumes,
and an immutable Docker image ID for a locally qualified Xingqiao build before
it will recreate anything. Official GitHub Releases remain the version source;
vanilla official images are not eligible for production promotion.

### CLI

Use an operation ID supplied by the caller and confirm the complete image ID
and upstream version before running the command. The command prints exactly one terminal result:
`result=promoted`, `result=rolled_back`, or `result=rollback_failed`.

```bash
ssh -o BatchMode=yes sub2api-prod
set -euo pipefail
test "$(uname -s)" = Linux
test "$(docker context show)" = default
test -z "${DOCKER_HOST:-}"
cd /opt/sub2api/production
sudo env \
  SUB2API_BASE_URL='https://<production-caddy-host>' \
  ADMIN_API_KEY_FILE=/opt/sub2api/production/secrets/sub2api-admin-api-key \
  GATEWAY_API_KEY_FILE=/opt/sub2api/production/secrets/gateway-api-key \
  ./ops/update-sub2api-host.sh \
  --contract-version 1 \
  --image 'sha256:<64-hex-image-id>' \
  --version '<version>' \
  --operation-id '<operation-id>'
```

`--contract-version` must match the executor's `HOST_CONTRACT_VERSION`. The
updater binary sends its own value, so a mismatch means the binary and the
executor script drifted apart; reinstall both together with
`ops/install-sub2api-updater.sh` rather than editing either side.

Do not replace the placeholders with a secret. The executor writes its
`0600` record under
`/opt/sub2api/production/release-records/host-updater/` and its verified
backup set under the release backup root. Records contain image IDs, paths,
checks, and outcomes, never Bearer tokens, API keys, passwords, or env-file
contents.

`SUB2API_BASE_URL` is required and must be the HTTPS Caddy entrypoint. The
executor rejects direct `sub2api:8080`, `localhost:8080`, and `127.0.0.1:8080`
values so smoke checks cannot bypass the Caddy update guard.
The two key variables are file paths only; key contents must never appear in a
command, record, ticket, or log.

### Web Button

The official localized update button keeps its confirmation and scheduling UI.
After an active admin session confirms the exact target version, the host
updater service calls this executor with the pinned image and operation ID.
The scheduled operation retains that immutable image reference. It does not
resolve a later release when the timer fires. The web path and the CLI path
therefore share the same backup, named-volume checks, recreate-only mutation,
health checks, smoke checks, and rollback behavior.

Before the button can promote a release, operators import the official source
delta into `upstream/sub2api`, run the repository test gates, and build
`xingqiao-sub2api:upstream-<version>` with these labels:

```text
com.xingqiao.sub2api.qualified=true
com.xingqiao.sub2api.upstream.version=<version>
com.xingqiao.sub2api.upstream.commit=<40-hex-official-commit>
```

The resolver verifies all three labels and pins the resulting immutable image
ID. If the qualified image is absent or mismatched, the request fails before
backup or Compose mutation. This preserves Xingqiao Base URL, frontend,
backend, and navigation behavior across official updates.

The service exposes these same-origin administrative operations:

- `GET /api/v1/admin/system/host-update/status` reads the persisted operation.
- `DELETE /api/v1/admin/system/host-update/schedule` cancels a pending schedule.
- A running operation is not interrupted by cancellation; it completes or
  rolls back under the executor lock.

Every mutation still requires the active admin Bearer session, exact production
Origin, and `X-Admin-UI-Request: 1`. Admin API keys are not accepted for these
operations.

### Install The Host Updater

Install the updater only on the production host. The installer refuses macOS,
non-default Docker contexts, non-root execution, and any checkout other than
`/opt/sub2api/production`. It does not change containers or submit an update.

The production host does not need a Go toolchain when a prebuilt Linux/amd64
binary is supplied. Build that binary from this repository, copy it to a
temporary path on the server, and run:

```bash
ssh -o BatchMode=yes sub2api-prod
set -euo pipefail
cd /opt/sub2api/production
sudo SUB2API_UPDATER_BINARY=/opt/sub2api/production/.staging/sub2api-updater \
  ./ops/install-sub2api-updater.sh
```

The installer writes the root-owned binary to
`/usr/local/libexec/sub2api-updater`, the root-owned environment file to
`/etc/sub2api/sub2api-updater.env` with mode `0600`, and enables
`sub2api-updater.service`. The service stores its operation state at
`/var/lib/sub2api-updater/state.json` and exposes
`/run/sub2api-updater/updater.sock` for the root Caddy container. The environment
file must contain `SUB2API_BASE_URL`, the admin API-key file path, and a
dedicated gateway API-key file path; values of either key must never be placed
in Git, the environment template, logs, or chat.

Before enabling the service, verify that the gateway API-key file exists and is
readable by the root executor. If it does not exist, stop at installation:
the host executor intentionally refuses to claim a successful release without a
real `/v1/models` smoke check. Do not substitute the admin API key for a gateway
key.

Validate and inspect the service without printing its environment:

```bash
sudo systemd-analyze verify /etc/systemd/system/sub2api-updater.service
sudo systemctl status sub2api-updater.service --no-pager
sudo stat -c '%A %U:%G %n' /usr/local/libexec/sub2api-updater \
  /etc/sub2api/sub2api-updater.env /var/lib/sub2api-updater
```

### Backup And Rollback

Before the first image mutation, the executor atomically promotes a `0700`
backup set containing the PostgreSQL custom dump, six record counts,
`/app/data` archive, and `SHA256SUMS`. It validates the dump with the pinned
PostgreSQL 18 image and retains the backup after promotion or rollback.

The previous container image ID is tagged as
`sub2api-host-updater:rollback-<operation-id>`. A failed health or smoke gate
restores the previous `services.sub2api.image` declaration and runs only:

```text
docker compose ... up -d --no-deps --force-recreate sub2api
```

The executor never runs `docker compose down`, recreates a dependency, or
restores the database automatically. `result=rolled_back` means the previous
image and protected container identities were recovered. Stop and investigate
`result=rollback_failed`; preserve the `0600` record and backup, inspect the
host updater and Docker logs, and use a separately approved database-restore
incident procedure if data repair is required.

## Gate Meaning

- **Project identity:** production must use `sub2api`; rehearsal must use
  `sub2api-official-rehearsal`. All inspected services and all helper Compose
  calls must use the selected project.
- **Storage identity:** production must retain its three existing named volumes;
  rehearsal uses isolated binds. Never translate one layout into the other.
- **Baseline and backup:** the running custom image is captured, then the
  PostgreSQL dump, app-data archive, SQL counts, and checksums are atomically
  promoted before any image mutation.
- **Image and health:** the requested digest must resolve to the recreated
  container image ID and be healthy within 180 seconds.
- **Smoke:** health, version, support menu, protected update/rollback guards,
  record-count monotonicity, and gateway `/v1/models` must pass.
- **Production approval:** an operator confirms the exact digest and supplies
  a matching promoted rehearsal record.

## Automatic Rollback

The script never assumes that old application code can safely use data after a
new image has started. When post-release smoke fails, it restores the previous
image only with an explicit compatibility decision:

```bash
ROLLBACK_COMPATIBLE=true bash ops/deploy-sub2api-release.sh --mode rehearsal
```

This still recreates only `sub2api`; it never calls `docker compose down`,
recreates dependencies, or renews anonymous volumes. A successful rollback
gets a `rolled_back` record. A missing compatibility declaration or failed
rollback smoke gets `rollback_failed`; stop and investigate rather than
repeating release attempts.

The release rollback does **not** restore PostgreSQL or app-data. Database
restore is a separate incident operation: verify the selected backup's
`SHA256SUMS`, restore into an isolated empty PostgreSQL target first, validate
the six record counts and administrator access, then obtain a separate change
decision before any production data cutover. Do not point `pg_restore` at the
live production database as an image-release response.

## Future Releases

For every new official digest: connect through `ssh sub2api-prod`, capture the
current IDs and mounts, make a verified backup, pull the exact digest, update
only the production `sub2api.image` declaration, and recreate only `sub2api`.
Keep the previous image tag and backup until post-release checks pass. Do not
use the Docker admin UI updater: changes made inside a container are not the
durable deployment declaration and are lost on recreation.
