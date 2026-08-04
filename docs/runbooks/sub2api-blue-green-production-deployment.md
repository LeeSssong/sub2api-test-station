# Sub2API Blue-Green Production Deployment

This runbook is the operator contract for the command-driven Sub2API release path. It does not authorize or start a release. Run the production command only after the user explicitly says `部署生产` (or an unambiguous equivalent) for the tested commit.

## Authorization Boundary

Ordinary `部署生产` authorization permits the controller to build and push the already-tested Git tree, resolve its immutable digest, connect to the production forced-command endpoint, and attempt the zero-downtime blue-green path. It never authorizes stopping PostgreSQL, Redis, Caddy, the active API slot, changing volumes/network/project identity, applying an incompatible migration, or using a maintenance deployment.

`允许停机部署` is a separate authorization. It is valid only after the operator has shown the user the emitted `downtime_required=true` JSON, reason, bounded unavailable-time estimate, and rollback steps. The ordinary controller has no flag that bypasses this gate. A maintenance procedure must be separately planned, reviewed, and invoked after that explicit phrase; do not reinterpret `部署生产` as downtime permission.

## Production Prerequisites

Before accepting `部署生产`, prove all of the following without mutating production:

1. The implementation branch is reviewed, merged into the intended release branch, pushed to the server, and the release worktree is clean and canonical.
2. The complete final-tree verification matrix passed, and a canonical non-symlink `0600` evidence JSON was created with `ops/write-sub2api-test-evidence.sh` after the worktree became clean.
3. Docker Buildx can push Linux AMD64 images to `SUB2API_IMAGE_REPOSITORY`; the registry destination is the approved production repository.
4. `RELEASE_SSH_TARGET`, `RELEASE_SSH_PORT`, and the canonical `0600` non-symlink key and known-hosts files select the production forced-command account. The controller always installs the reviewed executor at `/usr/local/libexec/deploy-sub2api-blue-green-host.sh` from `RELEASE_WORKTREE/ops/deploy-sub2api-blue-green-host.sh`.
5. The production host executor has canonical paths for `DEPLOY_ROOT`, `BASE_COMPOSE`, `SECRET_ENV`, `RELEASE_ENV`, `RELEASE_STATE`, `RELEASE_RECORD_ROOT`, `ADMIN_API_KEY_FILE`, and `GATEWAY_API_KEY_FILE`; the env/key/state files are root-owned `0600` non-symlinks.
6. Production uses Docker context `default`, Compose project `sub2api`, HTTPS `BASE_URL`, and an immutable `NETWORK_CURL_IMAGE` present in `NETWORK_CURL_IMAGE_ALLOWLIST`.
7. The existing release state matches the release env, the active slot/upstream pair, migration hash, and current PostgreSQL/Redis/Caddy container identities. At least 2 GiB disk, 1 GiB available memory, and 10 PostgreSQL connections remain for the parallel slot.
8. The user has issued `部署生产` after reviewing the exact commit, evidence path, image repository, target host, and 1800-second hard stop.

## Future Production Command

From the clean reviewed release worktree, with the following values already installed by the operator, the exact future invocation is:

```bash
export RELEASE_WORKTREE="$(pwd -P)"
export RELEASE_BUILD_CONTEXT="$RELEASE_WORKTREE/upstream/sub2api"
export SUB2API_IMAGE_REPOSITORY='<approved-registry>/<repository>'
export RELEASE_SSH_TARGET='<forced-command-user>@<production-host>'
export RELEASE_SSH_PORT='<production-ssh-port>'
export RELEASE_SSH_KEY='<absolute-0600-production-key-path>'
export RELEASE_SSH_KNOWN_HOSTS='<absolute-0600-known-hosts-path>'
export SUB2API_TEST_EVIDENCE='<absolute-0600-final-tree-evidence.json>'

bash ops/release-sub2api-blue-green.sh \
  --mode production \
  --evidence "$SUB2API_TEST_EVIDENCE"
```

## Preloaded image transport (no registry pull)

Use `RELEASE_TRANSPORT=preloaded` when the production host cannot reach the
registry. The controller builds under a unique temporary tag, saves the image
as an archive, transfers the archive and the host executor over SSH/SCP, and
checks the archive SHA256 plus the Docker image ID after `docker load`. After
the build, it creates a unique release tag containing both the source commit
and the full image-ID digest; that exact tag is the only preloaded reference
accepted by the host. The host runs Compose with `--pull never`; it will fail
closed if the requested image-ID-bound tag or image ID is not present. The staging directory and archive must be
root-owned and non-group-writable on the host.

```bash
export RELEASE_TRANSPORT=preloaded
bash ops/release-sub2api-blue-green.sh \
  --mode production \
  --evidence "$SUB2API_TEST_EVIDENCE"
```

The remote executor is staged under `/usr/local/libexec`, checked
(`root:root`, mode `0755` or `0700`, `bash -n`, and SHA256), and then promoted
with an atomic `mv` to the fixed executor path before the image build starts.
A failed executor transfer or attestation therefore leaves production
unchanged and does not spend time building an image. Preloaded acceptance
probes also require the allowlisted curl image to already exist locally and
run with Docker `--pull never`.

## Authorized additive-migration maintenance release

Use this path only after the host preflight has emitted `downtime_required=true`
for `migration_set_changed` and the user has explicitly authorized
`允许停机部署`. The controller requires `--maintenance-authorized`; the host
executor additionally requires `--maintenance-from-hash` to equal the active
hash below. No other migration transition is accepted:

```text
from e95b3512ccfc5b5103b4547857c437338921fd6bb463b7f2078c9ee24da4f0fc
to   337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76
files 196_account_procurement_cost.sql — adds nullable procurement cost, nullable effective time, and a non-negative procurement-cost constraint.
```

Invoke the same controller with the explicit maintenance flag:

```bash
bash ops/release-sub2api-blue-green.sh \
  --mode production \
  --evidence "$SUB2API_TEST_EVIDENCE" \
  --maintenance-authorized
```

The host executor enforces a bounded (default 300-second, maximum 600-second)
unavailable window, stops only the API and worker services, starts the candidate
worker to apply the additive migration, then restores the API route through the
existing Caddy container. PostgreSQL, Redis, and Caddy are never stopped or
recreated. `196_account_procurement_cost.sql` is forward-compatible: it only
adds nullable columns and a non-negative constraint. An application rollback
restores the previous API/worker images and Caddy upstream; it does not
automatically remove those database fields or the constraint. Preserve the
`.partial` and failure record if rollback itself fails.

Exact manual recovery command (only after reviewing the preserved partial record):

For a preloaded recovery, `--image` must be the exact
`<repository>:release-<source-commit>-<image-id-without-sha256-prefix>` tag;
do not substitute the older commit-only release tag.

```bash
sudo -n env RELEASE_PRELOADED_IMAGE=true \
  bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh \
  --mode production --image '<previous-release-tag>' \
  --preloaded-archive '/var/lib/sub2api/release-staging/<archive>.tar' \
  --preloaded-archive-sha256 '<archive-sha256>' \
  --preloaded-image-id '<image-id-sha256>' \
  --source-commit '<previous-40-hex-commit>' --source-tree '<previous-40-hex-tree>' \
  --tested-tree '<previous-40-hex-tree>' \
  --migrations-hash 176e6659b45bffbf11f5e1fce7dfbaf60906fe974553d7156fdc516231f4f5d0 \
  --deadline-epoch "$(date -u +%s -d '+600 seconds')"
```

Do not edit `RELEASE_STATE`, skip hash checks, or stop shared services during
recovery.

When the Caddy route file itself changes, stage it in the reviewed deploy root,
then validate and reload the existing container atomically (never recreate it):

```bash
cd "$DEPLOY_ROOT"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
sudo -n cp -a Caddyfile "Caddyfile.$stamp.bak"
caddy_id=$(docker compose --project-name sub2api --env-file "$SECRET_ENV" \
  --env-file "$RELEASE_ENV" -f "$BASE_COMPOSE" ps -q caddy)
docker compose --project-name sub2api --env-file "$SECRET_ENV" \
  --env-file "$RELEASE_ENV" -f "$BASE_COMPOSE" exec -T \
  -e SUB2API_ACTIVE_UPSTREAM="$(jq -r .active_upstream "$RELEASE_STATE")" \
  caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
docker compose --project-name sub2api --env-file "$SECRET_ENV" \
  --env-file "$RELEASE_ENV" -f "$BASE_COMPOSE" exec -T \
  -e SUB2API_ACTIVE_UPSTREAM="$(jq -r .active_upstream "$RELEASE_STATE")" \
  caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
test "$(docker compose --project-name sub2api --env-file "$SECRET_ENV" \
  --env-file "$RELEASE_ENV" -f "$BASE_COMPOSE" ps -q caddy)" = "$caddy_id"
```

If validation or reload fails, restore `Caddyfile.$stamp.bak`, validate/reload
again, and retain the backup and command output as recovery evidence.

Do not run this command from this document, substitute guessed values, paste secrets, or run it before authorization. The controller rejects a dirty or mismatched tree. Its 1800-second monotonic budget includes build, push, digest resolution, SSH execution, cutover, and acceptance; each stage is also capped at 600 seconds.

## First-Topology Bootstrap Gate

The current legacy single-application production topology is not a steady-state blue-green release. If `RELEASE_STATE` is absent, or PostgreSQL/Redis/Caddy identity cannot be uniquely proved, the executor returns `legacy_topology_bootstrap` before candidate startup or Caddy reload. This is expected and cannot be bypassed by ordinary `部署生产` authorization.

Before requesting `允许停机部署` for the first topology installation, separately prove that the legacy application and new worker cannot run singleton jobs concurrently, relay-ops remains reachable through internal Caddy, Compose project/network/volumes and PostgreSQL/Redis/Caddy identities remain unchanged, and the legacy upstream can be restored with compatible data. Publish the maintenance estimate and exact rollback procedure for user review.

## Successful Timeline

| Elapsed | Stage | Required evidence |
|---:|---|---|
| 0-3 min | Validate evidence and source | Clean worktree; source commit, tree, migration hash, and test evidence match. |
| 3-12 min | Build and push | Linux AMD64 image receives all five qualification labels and resolves to an immutable digest. |
| 12-16 min | Host preflight | State/env, active pair, migration set, shared identities, topology, and resource headroom pass. |
| 16-21 min | Candidate | Pull/start only the inactive API slot; internal health, version, settings, and gateway checks pass. |
| 21-24 min | Cutover | Validate Caddy with the candidate upstream, then graceful reload without recreating Caddy. |
| 24-27 min | Public acceptance | Public health, admin version, and gateway models checks pass. |
| 27-29 min | Persist and worker | Atomically persist release env/state, recreate the sole worker, and verify health/logs. |
| 29-30 min | Finalize | Recheck PostgreSQL/Redis/Caddy identities and atomically finalize the release record. |

Stop when total elapsed time reaches 1800 seconds. A timeout is a failed release, not permission to skip acceptance.

## Downtime Reason Codes

The host executor can emit these `downtime_required` codes; all are fail-closed before the corresponding production mutation:

| Reason code | Meaning / operator action |
|---|---|
| `legacy_topology_bootstrap` | Steady-state release state or unique shared-service identity is absent. Use the separately authorized first-topology maintenance plan. |
| `invalid_active_slot_upstream` | Active slot and Caddy upstream are not the same allowlisted pair. Reconcile state manually; do not guess. |
| `invalid_candidate_upstream` | Derived candidate upstream is outside the exact blue/green allowlist. Stop and review code/state. |
| `migration_set_changed` | Candidate migrations differ from the active release. Review compatibility and plan an authorized maintenance release. |
| `shared_container_identity_changed` | PostgreSQL, Redis, or Caddy identity differs from recorded state. Investigate the unexpected replacement first. |
| `insufficient_disk_headroom` | Less than 2 GiB is available for the parallel release. Reclaim reviewed non-production artifacts or schedule maintenance. |
| `insufficient_memory_headroom` | Less than 1 GiB is available for the parallel API slot. Restore headroom or schedule maintenance. |
| `insufficient_db_connection_headroom` | Ten free PostgreSQL connections cannot be proved. Restore headroom or schedule maintenance. |
| `invalid_candidate_topology` | Candidate Compose cannot render or does not select the exact active/candidate/worker images. Correct configuration offline. |
| `candidate_role_not_api` | The inactive slot is not API-only or the worker role contract cannot be proved. Correct role isolation offline. |

Exit status `2` with `downtime_required=true` is a gate, not a retry signal. Preserve its JSON exactly for the user decision.

## Failure and Cutback

- Before reload: leave the active upstream and release state unchanged. Preserve the failure record; remove only the failed inactive candidate when the record permits it.
- Reload failure or uncertain reload: validate and gracefully reload the recorded previous upstream. Confirm public health/version/models against that previous slot.
- Public acceptance failure: immediately reload the previous upstream, rerun public acceptance, and keep both API slots. Do not update the worker unless application promotion was persisted.
- Worker failure after cutover: restore the previous Caddy upstream, release env/state, and previous worker digest; verify the restored worker and public route.
- Rollback failure: preserve both slots, the `.partial` recovery checkpoint, logs, env/state, and container identities. Enter incident response. Never restore PostgreSQL automatically or rebuild shared services.

## Interrupted Release Recovery

The next executor invocation first inspects the sole recent `0600` `.partial` record. A schema-valid record no older than 1800 seconds is rolled back to its recorded previous upstream/state before a new release can begin. Multiple, malformed, permissive, symlinked, or stale partial records fail closed for manual incident review. Do not delete a partial record to force progress; a finalized success record matching state permits cleanup of only its committed partial.

## Shared-Service Identity Proof

The normal path uses `pull` and `up --no-deps` only for the inactive API slot, graceful `caddy reload` inside the existing Caddy container, and `up --no-deps --force-recreate sub2api-worker` only for the worker. It never runs `docker compose down`, recreates PostgreSQL/Redis/Caddy, removes volumes, restores the database, or stops the previous API slot. Container IDs for PostgreSQL, Redis, and Caddy are captured before candidate startup and compared again before finalization; any difference fails the release.

## State and Record Retention

`RELEASE_ENV`, `RELEASE_STATE`, admin/gateway key files, the release-record directory, lock owner, partial records, temporary header files, and final records are host-executor inputs owned by root and protected with mode `0600` where they are files. Keep the release-record directory root-only. State and final records are atomically replaced; temporary credential headers are removed on success and failure.

Retain the previous API slot, previous immutable image digest, prior worker digest, release env/state, and final records through production acceptance and the normal rollback window. Never put secret values in evidence or records. Remove old records/images only under the separate retention policy after a newer release is verified and no recovery checkpoint remains.
