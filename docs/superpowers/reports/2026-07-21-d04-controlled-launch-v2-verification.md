# D04 Controlled Launch v2 Verification

> **Superseded policy note (updated 2026-07-22):** The invitation, manual check-in and referral-reward launch policy in this historical production report has been replaced by configurable public registration, a hard 15-user cap and automatic USD 20 first-login credit. The replacement is deployed and has passed one-user low-budget acceptance; D04 is now back in `read_only` with registration closed. See `2026-07-21-d04-public-registration-daily-login-verification.md` and `2026-07-22-d04-launch-readiness-verification.md`.

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `PASS` for read-only deployment and registration safety hardening  
**Production mode:** `D04_MODE=read_only`, `D04_COST_POLICY_QUALIFIED=false`

## Scope

This increment hardens the invitation-only first-launch service without creating users, invitations, grants, usage rows, or provider writes. It adds authenticated encryption for invitation codes at rest and recovers registrations whose provider-side invitation was consumed before the local response was committed.

## Implementation Evidence

- Invitation codes are stored as `v1:` AES-256-GCM ciphertext. The `join_id` is authenticated as associated data, so ciphertext cannot be moved between invitations.
- The database retains only a SHA-256 code hash for lookup; plaintext is returned only after decrypting with the dedicated server-side key file.
- The key file is hex-encoded, 32 bytes after decoding, and must not be group/other writable. No plaintext fallback is accepted.
- Registration completion atomically marks the invitation, inserts the internal user, and reserves the referral grant with an idempotency key.
- Write-mode reconciliation reads provider invitation `used_by`, verifies the provider user, and completes the same local transaction. Read-only mode performs no recovery writes.

## Automated Gates

```text
go test ./... -p 1 -count=1
go test ./... -p 1 -race -count=1 (GOMAXPROCS=1, CGO_ENABLED=1)
go vet ./...
bash tests/internal_test/validate_internal_test_contract.sh
git diff --check
```

All gates passed. Focused tests cover cipher round-trip/tamper rejection, unsafe key permissions, encrypted storage, join-state decryption, provider-side used invitation recovery, transaction idempotency, official Sub2API pagination/fields, and end-to-end referral behavior.

## Production Deployment

- Only the standalone D04 service was recreated with `--no-deps --force-recreate`.
- Image: `sub2api-internal-test:d04-read-only-20260721-v2`
- AMD64 manifest: `sha256:00202d01bb166e437609ff7fea98bf710aac1d01afa82c746d572f96b29bb4c0`
- Container: running, `healthy`, restart count `0`.
- Dedicated invitation key: server-local `0600`, owner `10001:10001`; the value was never displayed, logged, or committed.
- Scheduler health checks passed after multiple ticks.
- SQLite counts remained zero: `audit_events`, `credit_grants`, `internal_users`, `invitations`, `jobs`, `usage_cursors`, `usage_records`.
- Existing `sub2api`, PostgreSQL, Redis, Caddy, and relay-ops container IDs remained unchanged.

## v3 Follow-up

The follow-up v3 added strict method checks, paginated balance-history reads, and provider-evidence reconciliation for pending check-in/referral grants. Its AMD64 manifest is `sha256:3b52f06d3ca6cd2d0cf256bbf1e21463a2f7516f3b97ee307d5aba1fc8395dbc`; the D04 container remained healthy with restart count `0`. The production Caddyfile was then remounted and Caddy alone was recreated because the prior single-file bind mount retained the old inode. The public checks now return `405 Allow: POST` for `GET /internal-test/api/checkin`, a D04 JSON `404` for an unknown join link, and `403 D04_REGISTRATION_CLOSED` for registration POST while read-only. No D04 database rows or Sub2API route objects changed.

## Non-functional Boundary

The separate non-functional baseline remains `DONE_WITH_CONCERNS`: TLS, HTTP redirects, health/login/register reachability, resource headroom, and zero-cost operation passed. A complete-denominator follow-up ran 5 concurrent workers with 12 `GET /health` requests each: `60/60` returned HTTP 200, with no other status or transport error; P50 was `1.779s` and P95 `4.779s`. This is a single-location ingress observation, not a model-capacity claim. XM PLUS/PRO paid compatibility and concurrency qualification remain blocked on real credentials, mappings, budget approval, billing evidence, and cleanup scope.

## Next Gate

Keep D04 in `read_only` until the replacement public-registration image is deployed in read-only mode and a controlled low-budget production acceptance is explicitly approved. The isolated acceptance covers the effective registration switch, one native registration, immediate same-day USD 20 login credit, same-day replay, budget fail-closed behavior and three-way balance reconciliation. It does not cover invitation, manual check-in or referral reward behavior. No XM routing change, candidate creation, paid probe or `enabled` mode is authorized by this report.
