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

## 30-Minute Rapid Release Lane

Use this lane for normal source, updater, homepage, and qualified-candidate
changes that do not require a database restore, storage migration, secret
rotation, or a new production topology. Those higher-risk changes leave the
rapid lane and follow their dedicated rehearsal and incident procedures.

The 30-minute limit is an engineering budget, not permission to skip a failed
gate. Stop the release when a required gate fails; do not spend the remaining
window repeatedly retrying unrelated infrastructure.

| Elapsed | Stage | Required outcome |
|---:|---|---|
| 0-3 min | Inventory | Resolve all worktrees, dirty files, remote `main`, changed subsystems, production Docker context, Compose SHA, and protected container identities once. |
| 3-15 min | Parallel qualification | Run only the test matrix for changed subsystems while building independent artifacts in parallel. Each required full suite runs once per final tree. |
| 15-20 min | Pre-stage | Verify immutable image labels/version/architecture, transfer with resume support, compare full SHA-256, and load the candidate without switching the running service. |
| 20-25 min | Minimal rollout | Restart only the updater when updater files changed; recreate only Caddy with `--no-deps` when homepage/Caddy files changed. Do not touch an unchanged service. |
| 25-30 min | Acceptance | Verify public health, candidate readiness, update UI state, changed user-visible behavior, logs, and protected container identities. Push/record the exact final `main` commit. |

### Change-Based Test Matrix

Determine the matrix from `git diff --name-only <deployed-commit>..HEAD`.
Do not rerun a broad suite merely because an ancestry-only merge commit was
added after the same tree already passed.

| Changed area | Required local gates |
|---|---|
| `upstream/sub2api/backend`, release import, or candidate Dockerfile | `go test ./... -count=1`, `go vet ./...`, image build, image label/version/platform verification |
| `upstream/sub2api/frontend` | pinned pnpm install, full Vitest, production build |
| `sub2api-updater`, update UI, updater systemd/executor | updater Go test/vet plus update UI, routing, host executor, and merge-release contract tests |
| `homepage`, Caddyfile, Caddy image | homepage full test/build plus desktop/mobile browser checks in both themes |
| documentation only | the owning document contract test and link/outline validation |

Run independent rows concurrently. After merging worktrees, rerun only gates
whose input tree changed. A clean ancestry merge requires `git diff-tree` to
show no file changes and does not invalidate prior test evidence.

After the homepage test and production build pass, use
`infra/Dockerfile.caddy-prebuilt` to copy the verified `homepage/dist` into the
currently running immutable Caddy image. This offline layer-only build avoids
reinstalling frontend dependencies or contacting Docker Hub. Pin
`BASE_IMAGE` to the inspected current Caddy image ID or local tag, validate the
new image labels, then recreate only Caddy with `--no-deps`.

### Fast-Lane Rules

1. A qualified candidate is built from a reviewed local worktree. Local
   qualification must use the pinned source,
   full subsystem gates, immutable labels, architecture check, archive
   SHA-256, and isolated `--version` execution. Git remains the remote source
   of record; GitHub Actions is not part of this process.
2. Candidate preparation and application promotion are separate operations.
   Loading `xingqiao-sub2api:upstream-<version>` must not recreate the running
   Sub2API container. Only an explicit administrator confirmation may promote
   it later.
3. Worktree consolidation starts with `git cherry` and tree comparison.
   Merge real unique changes once. For patch-equivalent branches, use an
   ancestry-only merge after proving the merge commit has an empty tree diff;
   never replay stale files over `main`.
4. Preserve production's named-volume Compose file. Sync only required
   updater/Caddy inputs, validate the resolved configuration, and change only
   the intended image reference.
5. Capture PostgreSQL, Redis, Sub2API, and relay-ops container ID, start time,
   restart count, image, and health before and after a Caddy/updater-only
   rollout. Any unexpected change fails acceptance.
6. Keep old application images and release records for rollback. Clean local
   worktrees and transfer staging only after production acceptance.

The release owner keeps a single timer and one checklist. Parallel commands
must have bounded timeouts and retain their exit status. A task exceeding the
budget is reported as an exception with the exact slow stage so the next
release can remove that bottleneck; the normal path must not silently expand
beyond 30 minutes.

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
com.xingqiao.sub2api.source.commit=<40-hex-qualified-source-commit>
```

The resolver verifies all four labels and pins the resulting immutable image
ID. If the qualified image is absent or mismatched, the request fails before
backup or Compose mutation. This preserves Xingqiao Base URL, frontend,
backend, and navigation behavior across official updates.

### Controlled Local Candidate Preparation

GitHub Actions is intentionally not part of the release process. There is no
scheduled or manually dispatched release workflow. An administrator runs the
existing local steps from a reviewed worktree, with production credentials
available only to the step that needs them:

1. `ops/sub2api-release-metadata.rb discover` records the official stable
   release and its source commit.
2. `ops/merge-sub2api-release.sh` overlays Xingqiao customizations, resolves
   the known release conflicts, runs the repository qualification gates, and
   creates the candidate commit and bundle.
3. Build and inspect the pinned `linux/amd64` image, then run
   `ops/publish-sub2api-candidate.sh` to validate labels, publish the immutable
   GHCR digest, and create the audit branch.
4. Stage and verify the digest through `ops/sub2api-candidate-ssh.sh` and the
   root-owned candidate loader without changing the running containers.
5. Run `ops/advance-sub2api-source.sh` only after candidate qualification and
   production staging pass. The admin update dialog then remains the only
   operation that calls the blue-green host executor and promotes the running
   version.

Each step fails closed, writes its own result record, and must be reviewed
before the next step. No step invokes GitHub Actions, restarts shared services,
or calls the administrator update API implicitly.

Qualified images are private:

```text
ghcr.io/leesssong/xingqiao-sub2api:upstream-<version>
ghcr.io/leesssong/xingqiao-sub2api@sha256:<manifest-digest>
```

The version tag is treated as immutable. Existing content is reused only when
its image ID, platform, and four qualification labels match. A mismatch or an
ambiguous registry failure stops the run without a push. Production always
receives the digest reference, never the version tag.

“Candidate image silently prepared” means all of these facts are true:

1. the official delta merged without a guessed conflict resolution;
2. backend, frontend, deployment, updater, and image qualification gates passed;
3. private GHCR contains the exact immutable image;
4. production pulled and isolatedly verified that exact digest;
5. `xingqiao-sub2api:upstream-<version>` points to it locally;
6. the running container identity, image, start time, status, health, restart
   count, and production Compose SHA-256 remained identical;
7. qualified source reached `main` through an ordinary compare-and-swap
   fast-forward;
8. the Feishu card reports facts without a fixed next-step instruction.

Candidate preparation is not deployment. The production forced command cannot
call Docker Compose, an update API, a database client, a container lifecycle
command, or prune. It only pulls, inspects, executes `/app/sub2api --version`
with no network and a read-only filesystem, applies the updater-compatible
local tag, and writes mode `0600` evidence to:

```text
/var/lib/sub2api-candidate-loader/state.json
```

The running image changes only after an administrator confirms the existing
web update dialog. No scheduler invokes that action.

Repeated local runs are idempotent: the same GHCR content and audit branch are
reused, the same candidate source is accepted as already advanced, and a
concurrent third-party `main` commit fails closed. The administrator decides
when to retry a transient failure. Result records use stable categories:
`QUALIFICATION_FAILED`, `PUBLISH_FAILED`, `PRODUCTION_STAGING_FAILED`, and
`SOURCE_ADVANCE_FAILED`; raw stderr is never copied into notifications.

The local operator pipes a short-lived, package-read-only GHCR token through
SSH stdin for production staging. The production loader creates a temporary
mode `0700` Docker config and removes it on every exit path; no long-lived GHCR
credential belongs on the host or in the repository.

Install or rotate the dedicated key by building the Linux AMD64 candidate
loader, generating an Ed25519 key in a temporary mode `0700` directory, and
running `ops/install-sub2api-candidate-loader.sh` from
`/opt/sub2api/production` as root with the prebuilt loader and public-key file.
The installer adds only:

```text
restrict,command="/usr/local/libexec/sub2api-candidate-ssh" <ed25519-public-key>
```

Install the matching private key only in the administrator's protected SSH
configuration, run one end-to-end local preparation, then remove the prior
forced-key line by exact public-key match. Never replace the whole
`authorized_keys` file or print either key.

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

For normal releases, run the controlled local preparation steps above, review
the fact-only result records, and use the existing admin update dialog for the
runtime change. The host updater then owns backup, the durable Compose image
declaration, recreate-only mutation, smoke checks, and rollback.

Pin the exact qualified image ID, preserve the previous image tag and verified
backup, recreate only `sub2api`, and retain the host updater record until
post-release checks pass.
