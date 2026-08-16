# P0 Native Session Stability Design

## Problem and evidence

Production was behind Cloudflare with `session_binding_enabled=true`, while `api_key_acl_trust_forwarded_ip=false` and `forwarded_client_ip_headers=[]`. Login returned 200, but the next `/auth/me` crossed a different Cloudflare edge IP, produced `auth.session_binding.mismatch`, revoked the refresh-token family, and made `/auth/refresh` return 401. The immediate mitigation restored the official default `session_binding_enabled=false`.

The prior P0 commit `c25fb9ad1` added a Redis rotation lock and an encrypted ten-second replay marker. That code is not part of Sub2API v0.1.177 and does not address the observed binding mismatch.

## Goal

Keep login sessions on the Sub2API v0.1.177 token and refresh design, and prevent the known unsafe configuration from being enabled again through normal settings writes.

## Non-goals

- Do not trust `CF-Connecting-IP` or another raw forwarding header merely because it is present.
- Do not add another refresh state machine, retry protocol, cookie scheme, database table, or migration.
- Do not change token TTLs, token-family revocation semantics, OAuth behavior, user credentials, or production token data.
- Do not include T12 or any other feature.

## Options considered

1. Keep the custom rotation replay logic and only turn binding off. Rejected because it retains an unnecessary divergence from Sub and does not prevent re-enabling the unsafe combination.
2. Remove or permanently disable session binding. Rejected because it removes an official optional capability rather than preserving the native design.
3. Restore official refresh rotation and require an explicit bootstrap trusted-proxy policy before session binding can be enabled. Selected because the auth state machine remains native while the deployment receives a fail-closed configuration gate.

## Design

### Native refresh restoration

Revert the runtime and test changes introduced by `c25fb9ad1`. `RefreshTokenPair` again performs the official sequence: load and validate the old refresh token, delete it, generate one replacement pair in the same family, and reject later reuse. `RevokeRefreshToken` again deletes only the supplied token. No rotation lock, replay marker, AES-GCM payload, or extra cache interface remains.

### Session-binding activation gate

`SettingService` rejects a settings document with `session_binding_enabled=true` unless `config.Server.TrustedProxiesConfigured` is true. This flag is set only by explicit bootstrap configuration of `server.trusted_proxies` or `SERVER_TRUSTED_PROXIES`; it is not writable through the admin settings API.

An explicit empty trusted-proxy list is valid for a directly exposed server and makes Gin ignore forwarding headers. A proxied deployment must explicitly configure the proxy CIDRs. Raw forwarded-client-IP headers and the API-key ACL forwarded-IP switch are not accepted as proof of a trustworthy session identity source.

The gate returns HTTP 400 through the existing typed service-error path with code `SESSION_BINDING_TRUSTED_PROXIES_REQUIRED`. Disabling the feature remains allowed.

## Compatibility and failure semantics

- Existing sessions remain compatible because no token schema or signing key changes.
- The production setting stays `false`; old sessions incorrectly revoked during the incident require one fresh login.
- Settings writes fail before persistence if they would enable binding without the bootstrap policy.
- No migration, downtime, secret, or production account mutation is required.

## Acceptance matrix

- With binding disabled, different proxy IPs between login and later requests do not trigger session-family revocation.
- Reusing an already rotated refresh token is rejected according to official Sub behavior.
- Enabling binding without explicit `server.trusted_proxies` configuration is rejected and not persisted.
- Disabling binding without that configuration is accepted.
- Enabling binding with an explicit trusted-proxy policy remains supported.
- Auth, session-binding, settings, and refresh-token focused tests pass.
- Blue-green preflight reports `downtime_required=false`; health endpoints pass; a real browser login survives `/auth/me`, navigation, and reload; no new binding mismatch is observed; the setting remains false.

## Rollback

Use the existing blue-green previous slot/image. Keep `session_binding_enabled=false` during rollback. No data rollback is needed.

## Approval

Emergency scope and desired native-session behavior were explicitly authorized by the user on 2026-08-17. The root release controller approves this minimum design for immediate implementation.
