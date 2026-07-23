# D04 Public Registration and Daily Login Verification

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `PASS / SINGLE-USER PRODUCTION ACCEPTANCE COMPLETE`  
**Production mode retained:** `D04_MODE=read_only`

## Verified Functional Contract

- Sub2API remains authoritative for `/register`, `/login`, password login, 2FA completion, sessions, dashboard, API Keys, usage and profile pages. D04 does not add a second user center.
- Caddy intercepts only `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/login/2fa`, and `GET /api/v1/settings/public` for the launch policy.
- `D04_REGISTRATION_OPEN` defaults to `false`; effective registration is native-open AND D04-open AND write-mode AND below 15 users AND within the qualified budget gate.
- The 15-user cap is atomic across replicas. A 20-way concurrent fixture produced exactly 15 successful forwards, roster inserts and daily grants; two independent service/store instances sharing SQLite could reserve only one remaining slot before any native registration was forwarded.
- Successful registration enrolls the native Sub2API user and immediately applies the same Shanghai-day USD 20 login grant.
- A launch user's first successful authentication per Shanghai calendar day applies USD 20 with `d04-login-<user>-<date>` idempotency. Same-day registration/login/2FA concurrency has one provider effect; the next Shanghai day has a second effect.
- Non-launch users keep their native authentication response and receive no D04 balance action. Grant or reconciliation failure never converts native authentication success into failure.
- Invitation, join-link, manual check-in and launch referral endpoints are absent from the HTTP router. Public settings force `invitation_code_enabled=false` and `affiliate_enabled=false` while preserving unrelated native settings.
- Historical invitation/referral tables remain readable. New usage does not issue or reconcile launch referral rewards, and historical referral reservations no longer consume the active launch budget.
- The D04 operations report now shows daily-login grants and contains no launch check-in or referral-reward wording.

## Security And Failure Boundaries

- Authentication request and response bodies are held in memory only and limited to 1 MiB.
- The upstream deadline is fixed at 20 seconds; only fixed native paths and allowlisted request/response headers are forwarded. Redirects are returned to the native browser unchanged and are never followed by the proxy, so credentials cannot be replayed to a redirect target.
- Browser authentication POSTs require a matching same-origin `Origin`; missing or cross-origin requests are rejected before forwarding.
- Daily-credit work has a bounded two-second child deadline and cannot turn native registration/login success into a failure. An existing pending or uncertain grant checks provider history before the global read-only gate, so a confirmed effect remains reconcilable after a safety lock.
- `D04_MAX_USERS` is accepted only from `1..15`. Write mode requires an explicit `D04_TOTAL_BUDGET_USD`; Compose no longer supplies a silent USD 100 write budget.
- Passwords, cookies, Authorization values, access tokens and 2FA input are neither logged nor stored.
- Unknown paths are rejected before forwarding. Retired D04 public paths return 404.
- Uncertain provider writes retain the existing read-only lock and provider-history reconciliation behavior.
- The invitation-key mount remains only as rollback material; application startup no longer reads or requires it.

## Verification

Fresh local verification used Go 1.24.13 Bookworm with `CGO_ENABLED=1` and the race detector:

```text
go test ./... -p 1 -race -count=1
go vet ./...
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
docker compose -f infra/compose.d04-read-only.yaml config --quiet
git diff --check
```

Focused evidence includes config validation, schema migration, 20-way registration cap, dual-store reservation capacity, 12-way same-day login idempotency, Shanghai midnight, uncertain-write recovery after read-only lock, auth response passthrough, redirect non-following, bounded credit work, public-settings overlay, 1 MiB limit, strict same-origin/CORS headers, SQLite locking and restart persistence.

## Production Boundary And Next Gate

The public-registration image was deployed in read-only mode after the local gates passed. The deployment rebuilt only `internal-test-service` and, because the new auth/public-settings routes require it, Caddy. Sub2API, PostgreSQL, Redis and relay-ops were not recreated. No registration, credential-bearing login, balance, route, multiplier, price, Key, candidate or probe state was changed.

Final production evidence:

```text
D04 image: sub2api-internal-test:d04-public-registration-20260721-v1 (healthy, restart count 0)
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
RELAY_OPS_MODE=read_only
RELAY_OPS_FEISHU_COMMAND_MODE=dry_run
Sub2API/PostgreSQL/Redis restart count: 0
```

`/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` returned HTTP 200. The public settings response was `no-store` and forced `registration_enabled=false`, `invitation_code_enabled=false` and `affiliate_enabled=false`. `/internal-test/status`, `/internal-test/join` and `/internal-test/checkin` all returned HTTP 404. A password-free POST without `Origin` returned `403 ORIGIN_REJECTED`; a same-origin empty registration POST returned `403 D04_REGISTRATION_CLOSED`. The second check proves the read-only policy without forwarding a registration request.

During acceptance, the original Caddy host-file replacement left a file-level read-only bind mount on the old inode. This was detected because the new login route still reached native Sub2API. Recreating only Caddy remounted the validated file; a follow-up contract test now requires an explicit `/internal-test/*` 404 rule so the native SPA cannot mask retired paths. Caddy restart count is 0 after the recreate. Backups remain on the server beside the active `Caddyfile` and `compose.d04-read-only.yaml`.

This gate was subsequently approved and completed on 2026-07-22 with one isolated user and one USD 20 grant. The proposal below is retained as the pre-execution boundary; it is no longer pending and must not be repeated as routine evidence.

### Proposed bounded write acceptance

This proposal is not authorized by design approval alone. It requires a separate explicit approval because it creates one real Sub2API user and one USD 20 site-balance mutation.

- Temporarily set `D04_MODE=write` and `D04_REGISTRATION_OPEN=true`, retaining `D04_MAX_USERS=15`.
- Use a conservative qualified cost multiplier of `1000` basis points (the highest currently configured production upstream cost multiplier) and a USD 2.00 total acceptance budget. The single USD 20 site balance therefore occupies the entire acceptance budget and prevents a second D04 grant.
- Register one operator-controlled isolated user through the native page. Do not place its password, Cookie or token in chat, files, commands, reports or logs.
- Verify immediate USD 20 credit, same-day login idempotency, D04/provider/balance history reconciliation and no route changes. Do not send a model request as part of this acceptance.
- Immediately restore `D04_MODE=read_only` and `D04_REGISTRATION_OPEN=false`. Preserve the isolated user and its D04 ledger only if it will be used for the next Shanghai-day login proof; otherwise cleanup requires a separately verified application/API path and must not edit PostgreSQL or SQLite directly.

The full 15-user launch budget remains a separate business decision. USD 2.00 was only the isolated acceptance ceiling and must not be reused as the public-launch budget.

## 2026-07-22 Rollback Recheck

An isolated write overlay was briefly started to prepare the approved acceptance window, but no registration account was submitted and no daily credit or other balance mutation occurred. The service was immediately recreated using only `compose.d04-read-only.yaml`.

Post-rollback evidence: `D04_MODE=read_only`, `D04_REGISTRATION_OPEN=false`, container `healthy`, restart count `0`, OOM `false`; the running Compose label contains only `/opt/sub2api/production/compose.d04-read-only.yaml`. Public `/healthz` and `/readyz` returned `200`, and same-origin empty registration returned `403 D04_REGISTRATION_CLOSED`. The existing route canonical hash remained `4791b8f093077dc50316daa8e0f5c16aaf18d0d402aa47ca1b9bc0380020e1e3`; no Sub2API route, multiplier, price, balance, Key, candidate, probe or database state was changed.

## 2026-07-22 Single-user Acceptance

A later, separately approved acceptance window completed the proposed write path:

- one isolated launch user was created, provider user ID `17`;
- D04 recorded one `daily_login_credit` grant with status `succeeded` and amount `20,000,000` micro-USD;
- provider current balance is USD 20 and one provider balance-history row matches the grant;
- a same-day login succeeded without creating a second provider or D04 effect;
- provider usage and D04 usage records remain zero, so no model request was used for acceptance;
- final mode is `D04_MODE=read_only`, registration is closed, and same-origin registration returns `403 D04_REGISTRATION_CLOSED`;
- the narrow route projection was unchanged before/after at `b6e6ee12f484a2a919d993da56fa293904672ff2c16b65afef5caa6398832ec4`.

During Admin API method discovery, an unintended empty settings PUT reset 21 settings. The issue was immediately detected and fully recovered through the official Sub2API `v0.1.161` Admin API using audit evidence, not direct PostgreSQL writes. The final settings SHA-256 is `52eff24fce0338ee4f8f81ad12a5d1406c46b6de050c99587035cdfd1f71a28e`. Full reconciliation, incident details, and process prevention are recorded in `docs/superpowers/reports/2026-07-22-d04-single-user-low-budget-acceptance.md`.

The next D04 gate is launch readiness and a separately approved controlled opening. Do not issue another acceptance grant merely to reconfirm the already verified path.
