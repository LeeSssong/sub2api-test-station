# P0 Native Session Stability Verification

## Scope

- Restore Sub2API v0.1.177 native refresh rotation by reverting `c25fb9ad1`.
- Reject explicit `session_binding_enabled=true` writes unless deployment-only `security.session_binding_allowed` and `server.trusted_proxies` were both explicitly configured.
- Preserve omitted security-setting compatibility and the disabled-binding behavior across proxy IP changes.
- Exclude T12, migrations, production data changes, and unrelated features.

## TDD evidence

- `TestAuthServiceRefreshTokenPair_RejectsReusedRotatedToken` failed before the revert because the second use returned success, then passed after the revert with `ErrRefreshTokenInvalid`.
- `TestSettingService_UpdateSettings_SessionBindingRequiresExplicitTrustedProxies` failed before the gate because the unsafe enable was persisted, then passed after the gate.
- Existing omission tests exposed an initial over-broad validation. The final implementation gates only actual writes while keeping both stored-true omission and default-false behavior compatible.
- Production read-only preflight proved `SERVER_TRUSTED_PROXIES` was already set during the incident. A new RED regression showed the first gate still accepted that state; the final two-condition gate requires a separate default-off deployment opt-in that production leaves unset.

## Verification results

- `go test -tags=unit ./internal/service -run 'TestAuthServiceRefreshTokenPair_RejectsReusedRotatedToken|TestAuthServiceBindEmailIdentity_RevokesExistingAccessAndRefreshTokens|TestSettingService_UpdateSettings_SessionBinding' -count=1`: PASS.
- `go test -tags=unit ./internal/server/middleware -run 'SessionBinding' -count=1`: PASS.
- `go test -tags=unit ./internal/handler/admin -run 'TestUpdateSettings.*(SessionBinding|ForwardedClientIP|SecuritySwitches)' -count=1`: PASS.
- `go test -tags=unit ./internal/config -run 'TestLoadHTTPIngressSafetyDefaults|TestLoadSessionBindingAllowedFromEnvironment|TestLoadExplicit.*TrustedProxies|TestLoadTrustedProxies' -count=1`: PASS.
- `go test -tags=unit ./internal/config ./internal/service ./internal/server/middleware ./internal/repository ./internal/handler/admin -run '^$'`: PASS.
- `gofmt -d` on changed Go files: clean.
- `git diff --check`: PASS.
- Custom rotation lock/replay symbol scan: no matches.
- Core `auth_service.go`, service/cache interface, and Redis refresh cache diff against official v0.1.177 candidate `458c27c1c`: empty.

## Release properties

- Migration: none.
- Configuration mutation: none; production must retain `session_binding_enabled=false`.
- Expected downtime: false; deployment must stop if preflight reports otherwise.
- Rollback: existing blue-green previous slot/image; keep session binding disabled.

## Not yet verified

- Merged-main focused checks and push.
- Blue-green preflight/deployment.
- Public health endpoints.
- Real browser login, `/auth/me`, navigation, and reload persistence.
- Absence of new production `auth.session_binding.mismatch` events after verification traffic.
