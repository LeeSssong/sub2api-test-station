# Candidate Upstream Admin Intake Production Verification

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `PASS`  
**Production modes:** `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`

## Scope

This verification covers the administrator workflow for adding a candidate upstream and its independent low-budget monitoring Key through `/ops`. It does not add a real candidate, approve a paid probe, change a public group, switch a route, alter pricing or balances, or expose a Key outside the managed secret file.

## Implementation

- `/ops` now provides the six-field candidate form: name, Base URL, pricing URL, usage URL, optional performance URL and one-time monitoring Key.
- `FileSecretStore` installs the Key under a SHA-256-derived filename with `O_EXCL` and mode `0600`; the managed directory must be an existing non-symlink directory with mode `0700`.
- PostgreSQL stores only a `file:` reference, SHA-256 fingerprint and last four characters. Repository failure removes the installed file and the caller-owned Key buffer is cleared before return.
- The legacy candidate secret directory remains read-only. The only new writable mount is `/var/lib/relay-ops/candidate-keys`; the container root filesystem and every existing secret mount remain read-only.
- Candidate creation does not enable paid synchronization or SSE probes.

The focused security review was incorporated in `v2`:

- a partial unique index rejects duplicate `candidate_probe_key` fingerprints;
- repository failures return stable `500 CANDIDATE_CREATE_FAILED` responses instead of validation `400` responses;
- cleanup failures preserve `errors.Is` classification without exposing original error text;
- mutation endpoints accept an exact parsed `application/json` media type and reject lookalikes such as `application/jsonp`.
- the Compose contract now scopes relay-ops security, secret-mount and resource assertions to the `relay-ops` service block; a mutation test confirms that removing only its `read_only: true` is rejected.

## Local Verification

The following fresh checks passed after implementation and review hardening. The race suite used Go 1.24.13 Bookworm with an isolated PostgreSQL 18 instance so store migrations and integration tests did not skip:

- `go test ./... -race -count=1`
- `go vet ./...`
- `gofmt -l .` returned no files
- `node --check internal/http/static/ops.js`
- `node --check internal/http/static/ops-admin.js`
- `bash tests/relay_ops/validate_relay_ops_contract.sh`
- `ruby ops/upstream-benchmark-v2.rb validate`
- `git diff --check`

## Production Deployment

- Image: `sub2api-relay-ops:candidate-admin-intake-20260721-v2`
- AMD64 image ID: `sha256:e103d218572255afbc1123a9a553f9ce5cf242d435d5636b7b2c4bf976e6c7f5`
- Final relay-ops container: `cae24309eb70`, healthy, restart count `0`.
- Managed host directory: `/opt/sub2api/production/secrets/candidate-managed-keys`, owner `10002:10002`, mode `0700`.
- Final deployment backup: `/opt/sub2api/production/compose.yaml.bak-candidate-admin-intake-20260721-v2`.
- Only relay-ops was recreated. Sub2API, PostgreSQL, Redis and Caddy container IDs were unchanged.
- `/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` all returned HTTP `200`.
- Startup migration created `secret_refs_candidate_probe_fingerprint_uidx` as a unique index.

## Authenticated Browser Acceptance

The existing authenticated Chrome `/ops` session was refreshed against the initial production image. The `v2` review hardening did not change the form, JavaScript or rendered UI, so the accepted browser contract remains applicable; no second candidate submission was made during the focused `v2` deployment.

Passed:

- The candidate form contains exactly five URL/name fields plus one password field.
- The Key input is `type="password"` with `autocomplete="new-password"`.
- The page warns against using a production recharge Key and states that candidate creation does not enable paid probes.
- A non-secret rollback test used `https://127.0.0.1/v1` with matching HTTPS evidence pages and the invalid Key `test`.
- The server rejected the private target; the page displayed a stable failure message and cleared the Key input.
- The candidate table remained empty.

After the rejection and again after the `v2` deployment, the managed directory contained zero files. PostgreSQL counts were also `0 0 0` for candidate upstreams, candidate secret references and `candidate.create` audit events.

## Zero-Mutation Evidence

The same redacted canonical snapshot was captured before and after deployment. It includes public groups `2/6`, accounts `2/7/8/9`, schedulable/binding/concurrency/cost state, sorted model IDs and the configured primary/backup matrix.

```text
before: 4791b8f093077dc50316daa8e0f5c16aaf18d0d402aa47ca1b9bc0380020e1e3
after v1: 4791b8f093077dc50316daa8e0f5c16aaf18d0d402aa47ca1b9bc0380020e1e3
after v2: 4791b8f093077dc50316daa8e0f5c16aaf18d0d402aa47ca1b9bc0380020e1e3
```

The Feishu routing file SHA-256 also remained identical:

```text
before: 3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
after v1: 3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
after v2: 3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
```

The production modes were unchanged, and the mount delta contained exactly one entry: the managed candidate Key directory as a bind-mounted read-write path. Restricted evidence remains under `/opt/sub2api/production/evidence/candidate-admin-intake-20260721/`.

## Remaining Boundary

The administrator can now enter the first real candidate and its dedicated low-budget Key through `/ops`. A real candidate should use its actual public URLs and a separately created low-limit Key. Paid candidate probes remain disabled until separately approved; creating a candidate only installs metadata and begins non-paid collection supported by the existing scheduler configuration.
