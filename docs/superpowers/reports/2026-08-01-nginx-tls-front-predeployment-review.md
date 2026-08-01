# Nginx TLS Front Predeployment Review

Date: 2026-08-01

## Decision

The Caddy RSA-2048 certificate rotation did not resolve the reported CC Switch
symptom. Direct local verification still reset a TLS 1.2 connection before an
HTTP response, while TLS 1.3 succeeded. CC Switch 3.16.5 still showed
"查询失败" after refreshing the Xingqiao balance.

The nginx TLS-front configuration remains necessary. It is prepared locally
only; this review does not authorize or perform a production deployment.

## Prepared topology

`nginx-tls-front` is the only Compose service publishing host ports 80 and 443.
It terminates TLS 1.2/1.3 with nginx/OpenSSL and proxies requests to Caddy over
the internal Docker network. Caddy remains the application router and continues
to own automatic certificate management.

The configuration preserves the Authorization, upgrade, forwarding, streaming,
request-size, and long-running request settings needed by API clients.

For ACME renewal, nginx forwards only `/.well-known/acme-challenge/` on port 80
to Caddy's internal port 80; all other HTTP requests receive the HTTPS redirect.
The nginx entrypoint discovers the newest complete certificate/key pair for the
site in Caddy's certificate storage (or uses an explicit source override),
copies it into its writable runtime directory, and reloads nginx after a
detected certificate change. Its foreground process exits as soon as the nginx
master exits, allowing Compose restart policy to act immediately.

Caddy and nginx communicate over a dedicated `tls_front` network. Caddy trusts
only nginx's fixed address on that network, and nginx verifies Caddy's public
certificate and SNI before proxying HTTPS.

## Verification

- `bash tests/infra/validate-nginx-tls-front.sh`
- `bash tests/infra/validate-sub2api-update-routing.sh`
- `bash tests/relay_ops/validate_relay_ops_contract.sh`
- `bash tests/infra/validate-baseline.sh`
- `bash tests/operations/sub2api_blue_green_topology_test.sh`
- Caddy configuration validation using the production Caddy image

The nginx contract renders the real image template with a temporary RSA
certificate and runs `nginx -t`. The blue/green topology test confirms the
front layer does not alter the two API slots, the single worker, or Caddy's
allowed upstream selection. The baseline runs this current release-topology
contract rather than the retired single-instance image-overlay contract.

## Deployment gate

Before deployment, confirm that the existing `caddy_data` volume contains the
Caddy-managed certificate for `SITE_ADDRESS`; this is true for the current
production topology. Deploy in an approved maintenance window because host
ports 80 and 443 change owners. Completion requires public TLS 1.2 and TLS 1.3
handshakes, an authenticated `/v1/usage` request, normal site/API health, and a
successful CC Switch 3.16.5 balance refresh.
