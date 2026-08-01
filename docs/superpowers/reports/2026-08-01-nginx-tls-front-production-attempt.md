# nginx TLS Front Production Attempt

Date: 2026-08-01

## Objective

Place nginx/OpenSSL in front of the existing blue/green Caddy and Sub2API
topology so CC Switch clients can query usage through a TLS 1.2-compatible
endpoint without rebuilding the API, worker, PostgreSQL, Redis, or relay-ops
services.

## Deployment result

- Commit `95a81dc37` was pushed to `origin/main`.
- The production topology was backed up and rendered into a dedicated
  migration candidate without replacing the existing blue/green services.
- Candidate Compose validation, Caddy validation, nginx certificate discovery,
  nginx template validation, and an isolated nginx startup test passed.
- nginx briefly took ownership of host ports 80 and 443. Caddy remained healthy
  as the internal TLS origin on the dedicated `172.30.0.0/29` network.
- The API blue/green slots, worker, relay-ops, PostgreSQL, and Redis retained
  their original container IDs and restart counts during the migration.

## Acceptance failure

The server-side topology worked, but the original client symptom did not:

- TLS 1.2 and TLS 1.3 both completed from the production host through nginx.
- TLS 1.3 completed from the affected Mac.
- TLS 1.2 from the affected Mac was reset during the initial handshake.
- CC Switch 3.18.0 still displayed `查询失败` after a real usage refresh.

The failure is specific to the requested SNI. Against the same production IP,
TLS 1.2 succeeded with `shop.xingqiaolab.top`, an unused Xingqiao subdomain, an
unrelated SNI, or no SNI. It failed only with `api.xingqiaolab.top`. The same
`api.xingqiaolab.top` TLS 1.2 handshake succeeded from the production host.
This isolates the reset to a domain-specific network intervention between the
affected client network and the origin, before nginx or Caddy can process an
HTTP request.

## Rollback

Because the CC Switch acceptance gate failed, the nginx container was removed,
the backed-up Compose and Caddy configurations were restored, and Caddy resumed
direct ownership of host ports 80 and 443. Public `/health` returned HTTP 200
after rollback. The API blue/green slots, worker, relay-ops, PostgreSQL, and
Redis again retained their original container IDs and restart counts.

## Next action

Use a compatible hostname that is not subject to the SNI-specific reset, or
move the public TLS entrypoint to a different public IP/CDN. A new hostname must
receive a valid certificate, route to the existing API origin, and pass TLS
1.2, authenticated `/v1/usage`, and CC Switch refresh acceptance before user
migration.
