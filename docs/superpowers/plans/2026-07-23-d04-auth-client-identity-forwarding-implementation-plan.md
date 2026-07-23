# D04 Authentication Client Identity Forwarding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve Caddy-derived client identity across the D04 authentication proxy so Sub2API session binding sees the same IP and User-Agent during login and subsequent browser requests.

**Architecture:** Extend the existing bounded D04 native request forwarder with three explicitly allowed Caddy-derived proxy identity headers while dropping client-supplied `X-Real-IP`. Keep Sub2API session binding enabled and deploy a new immutable image by recreating only the D04 `internal-test-service` container.

**Tech Stack:** Go 1.24, `net/http`, Go testing, Docker Compose, Caddy, Sub2API trusted proxies

## Global Constraints

- Modify `internal-test-service` only; do not modify Sub2API code or configuration.
- Do not recreate Sub2API, PostgreSQL, Redis, Caddy, or relay-ops.
- Keep D04 in `read_only` mode with registration closed.
- Do not read or print production secrets, passwords, tokens, cookies, or API keys.
- Keep Sub2API `session_binding_enabled=true`.
- Preserve the existing endpoint, body-size, same-origin, redirect, response-header, and daily-login boundaries.

---

### Task 1: Preserve Trusted Proxy Identity Headers

**Files:**
- Modify: `internal-test-service/internal/app/app_test.go:47-102`
- Modify: `internal-test-service/internal/app/app.go:125-137`

**Interfaces:**
- Consumes: `forwardRequest(context.Context, string, string, string, []byte, http.Header) (authproxy.Response, error)`
- Produces: the same function and response contract, with three Caddy-derived request headers forwarded unchanged and client-supplied `X-Real-IP` dropped

- [x] **Step 1: Write the failing test**

Extend `TestNewWiresBoundedNativeAuthAndPublicSettingsProxy` so the test auth request contains Caddy-derived identity headers and the upstream handler requires them:

```go
proxyHeaders := http.Header{
	"X-Forwarded-For":   {"203.0.113.24"},
	"X-Forwarded-Proto": {"https"},
	"X-Forwarded-Host":  {"api.example.test"},
}

for key, want := range proxyHeaders {
	if got := r.Header.Values(key); !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}
if got := r.Header.Values("X-Real-IP"); len(got) != 0 {
	t.Fatalf("X-Real-IP = %v, want absent", got)
}
```

Apply the headers to each login and login/2fa request:

```go
req.Header.Set("Origin", "https://example.com")
req.Header.Set("X-Real-IP", "203.0.113.24")
for key, values := range proxyHeaders {
	req.Header.Del(key)
	for _, value := range values {
		req.Header.Add(key, value)
	}
}
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd internal-test-service
go test ./internal/app -run TestNewWiresBoundedNativeAuthAndPublicSettingsProxy -count=1
```

Expected: FAIL because the upstream receives no `X-Forwarded-For` value.

- [x] **Step 3: Implement the minimal allowlist change**

Update the request-header allowlist in `forwardRequest`:

```go
for _, key := range []string{
	"Accept",
	"Accept-Language",
	"Authorization",
	"Content-Type",
	"Cookie",
	"Origin",
	"User-Agent",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
} {
	for _, value := range headers.Values(key) {
		req.Header.Add(key, value)
	}
}
```

- [x] **Step 4: Run focused and full verification**

Run:

```bash
cd internal-test-service
gofmt -w internal/app/app.go internal/app/app_test.go
go test ./internal/app -run TestNewWiresBoundedNativeAuthAndPublicSettingsProxy -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

Expected: all commands exit 0 with no test failures or vet diagnostics.

- [x] **Step 5: Review the focused diff**

Run:

```bash
git diff --check -- internal-test-service/internal/app/app.go internal-test-service/internal/app/app_test.go
git diff -- internal-test-service/internal/app/app.go internal-test-service/internal/app/app_test.go
```

Expected: only the proxy-header test and explicit allowlist change appear.

### Task 2: Deploy Only the D04 Service

**Files:**
- Runtime build input: `infra/Dockerfile.internal-test`
- Runtime Compose input: `infra/compose.d04-read-only.yaml`
- Production destination: `/opt/sub2api/production`

**Interfaces:**
- Consumes: verified `internal-test-service` source and the existing external `sub2api_default` network
- Produces: a healthy immutable D04 image whose auth forwarder preserves Caddy identity headers

- [x] **Step 1: Capture the production boundary snapshot**

Record container IDs, start times, restart counts, and health for Sub2API,
PostgreSQL, Redis, Caddy, relay-ops, and D04 without inspecting environment
values or secrets.

- [x] **Step 2: Copy only verified D04 build inputs**

Transfer `internal-test-service` and `infra/Dockerfile.internal-test` to a
restricted temporary build directory on the production host. Do not overwrite
the active Compose file or any secret.

- [x] **Step 3: Build the immutable repair image**

Build:

```bash
docker build \
  -f infra/Dockerfile.internal-test \
  -t sub2api-internal-test:d04-auth-client-identity-20260723-v1 \
  .
```

Expected: image build exits 0.

- [x] **Step 4: Point only the D04 overlay at the new image and recreate it**

Back up `compose.d04-read-only.yaml`, replace only the D04 image tag, validate
with `docker compose config --quiet`, and run:

```bash
docker compose -f compose.d04-read-only.yaml up -d --no-deps internal-test-service
```

Expected: only `sub2api-d04-internal-test-service-1` receives a new container ID.

- [x] **Step 5: Verify health, policy, and client identity propagation**

Confirm D04 is healthy with `D04_MODE=read_only` and
`D04_REGISTRATION_OPEN=false`. Send a harmless same-origin invalid login request
through the public Caddy endpoint and confirm the Sub2API access log records the
request's public client IP rather than the D04 container IP. Do not log or print
the request body.

- [x] **Step 6: Reconcile the production boundary**

Compare the pre/post snapshot. Sub2API, PostgreSQL, Redis, Caddy, and relay-ops
must retain their original container IDs, start times, and restart counts.
Confirm `session_binding_enabled=true` remains unchanged.

- [x] **Step 7: Roll back on any failed gate**

If build, health, identity propagation, or boundary reconciliation fails,
restore the prior Compose file and recreate only `internal-test-service`. Preserve
the failed container logs without including secrets in the report.
