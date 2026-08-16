# P0 Native Session Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Sub2API v0.1.177 native refresh rotation and prevent session binding from being enabled without explicit deployment opt-in plus a trusted-proxy policy.

**Architecture:** Remove the custom refresh lock/replay extension by reverting its isolated commit. Add a default-off deployment permission and validate both deployment conditions at the existing settings write boundary; leave token issuance, session binding enforcement, and frontend refresh coordination native.

**Tech Stack:** Go, Gin, Redis cache interface, existing typed service errors, focused Go tests.

## Global Constraints

- T12 remains frozen and must not be included.
- No migration, production credential/token mutation, GitHub Actions, full-repository test, or unrelated validation.
- `session_binding_enabled` remains false in production.
- Do not accept raw Cloudflare headers or `SERVER_TRUSTED_PROXIES` alone as an activation signal.

---

### Task 1: Lock the official refresh behavior with a failing regression

**Files:**
- Create: `upstream/sub2api/backend/internal/service/auth_service_native_refresh_test.go`
- Revert: runtime and tests introduced by commit `c25fb9ad1`

**Interfaces:**
- Consumes: `AuthService.GenerateTokenPair` and `AuthService.RefreshTokenPair`
- Produces: regression proving a rotated refresh token cannot replay a successful response

- [x] Add a unit test that generates a token pair, refreshes it once successfully, then asserts reuse of the original refresh token returns `ErrRefreshTokenInvalid`.
- [x] Run `go test -tags=unit ./internal/service -run TestAuthServiceRefreshTokenPair_RejectsReusedRotatedToken -count=1` and confirm it fails because the custom replay marker returns success.
- [x] Revert `c25fb9ad1`, resolving no unrelated changes and retaining the new regression.
- [x] Re-run the focused test and confirm it passes.
- [x] Confirm the diff no longer contains `RefreshTokenRotationLocker`, `RefreshTokenRotationResult`, rotation lock keys, AES-GCM replay code, or the replay-specific test.

### Task 2: Add the session-binding configuration gate with TDD

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/setting_update.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_service_update_test.go`
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/config/config_test.go`

**Interfaces:**
- Consumes: `config.Config.Security.SessionBindingAllowed`, `config.Config.Server.TrustedProxiesConfigured`, and `SystemSettings.SessionBindingEnabled`
- Produces: default-off deployment permission plus typed activation errors

- [x] Add tests showing enable without explicit trusted proxies is rejected and not persisted and disable remains accepted.
- [x] Run those tests and confirm the unsafe-enable case fails before implementation.
- [x] Add the minimum validation at the settings persistence boundary after omission handling.
- [x] Re-run the focused tests and confirm all cases pass.
- [x] Use production read-only preflight to prove trusted proxies alone are insufficient, add a RED regression, then require default-off `security.session_binding_allowed` plus trusted proxies.

### Task 3: Preserve the disabled-binding proxy-IP behavior

**Files:**
- Modify: `upstream/sub2api/backend/internal/server/middleware/session_binding_test.go`

**Interfaces:**
- Consumes: `enforceSessionBinding`, `SessionBinding`, and a setting repository returning `session_binding_enabled=false`
- Produces: regression proving a changed edge IP is ignored when optional binding is disabled

- [x] Add a focused middleware test with a token binding from one IP and a request binding from another IP while the setting is false.
- [x] Assert enforcement returns true and does not emit a 401 response.
- [x] Run the middleware session-binding tests.

### Task 4: Verify, document, integrate, and release

**Files:**
- Create: `docs/superpowers/reports/2026-08-17-p0-native-session-stability-verification.md`
- Update after production: root-only global progress and queue documents

- [x] Run only focused auth, session-binding, setting, refresh-cache tests and related package compile checks.
- [x] Run `gofmt` on changed Go files and `git diff --check`.
- [x] Self-review the diff for native parity, scope, configuration behavior, migration absence, and rollback.
- [ ] Commit the candidate, merge to the clean root `main`, repeat the focused merged-tree checks, push, and use the existing local/host blue-green chain.
- [ ] Require `downtime_required=false`; otherwise stop before switching.
- [ ] Verify `/healthz`, `/readyz`, and `/health`, then verify a real browser login through `/auth/me`, navigation, and reload.
- [ ] Verify no new `auth.session_binding.mismatch` events and confirm `session_binding_enabled=false`.

## Verification Commands

- `go test -tags=unit ./internal/service -run 'TestAuthServiceRefreshTokenPair_RejectsReusedRotatedToken|TestSettingService_UpdateSettings_SessionBinding' -count=1`
- `go test -tags=unit ./internal/server/middleware -run 'SessionBinding' -count=1`
- `go test -tags=unit ./internal/handler/admin -run 'TestUpdateSettingsRejectsSessionBinding' -count=1`
- `go test ./internal/service ./internal/server/middleware ./internal/repository ./internal/handler/admin -run '^$'`
- `git diff --check`

## Acceptance

- [ ] Native Sub refresh behavior is restored.
- [ ] The known unsafe session-binding configuration cannot be written.
- [ ] Production stays healthy and a fresh login remains active.
- [ ] No T12 files, migration, or unrelated feature enters the release.

## Risks

- A deployment that intentionally uses session binding must explicitly configure `server.trusted_proxies`; this is deliberate fail-closed behavior.
- Previously revoked token families cannot be recovered and require one new login.
