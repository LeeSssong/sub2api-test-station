# D04 Public Registration and Daily Login Credit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace D04 invitation/manual-check-in behavior with configurable public registration capped at 15 users and one automatic USD 20 credit on each launch user's first successful Shanghai-day authentication.

**Architecture:** Keep Sub2API authoritative for authentication and user UI. Extend `internal-test-service` into a bounded transparent proxy for registration, password login, and 2FA completion; store only launch roster and credit state in SQLite, and reuse the existing idempotent Admin API balance writer.

**Tech Stack:** Go 1.24.13, `net/http`, modernc SQLite, Caddy, Docker Compose, shell contract tests.

## Global Constraints

- Reuse Sub2API native user pages; do not create another user center.
- Production stays `D04_MODE=read_only` until separately approved write acceptance.
- `D04_REGISTRATION_OPEN=true` never overrides `D04_MAX_USERS=15`.
- Successful registration immediately receives that Shanghai day's USD 20 credit.
- Never persist or log credentials, authentication bodies, cookies, tokens, or TOTP values.
- Retire invitation, referral, and manual check-in public behavior without deleting historical rows or secrets.
- Do not change upstream routing, pricing, multipliers, unrelated balances, Keys, probes, relay-ops mode, or Feishu command mode.

---

### Task 1: Policy Configuration and Non-Destructive Store Migration

**Files:**
- Modify: `internal-test-service/internal/config/config.go`
- Modify: `internal-test-service/internal/config/config_test.go`
- Modify: `internal-test-service/internal/domain/model.go`
- Modify: `internal-test-service/internal/domain/model_test.go`
- Modify: `internal-test-service/internal/store/schema.sql`
- Modify: `internal-test-service/internal/store/sqlite.go`
- Modify: `internal-test-service/internal/store/sqlite_test.go`

**Interfaces:**
- Produces: `Config.RegistrationOpen bool`, `Config.DailyLoginCredit domain.MicroUSD`.
- Produces: `domain.GrantDailyLogin`, `domain.DailyLoginCredit`.
- Produces: `(*Store).EnrollLaunchUser(ctx, user, maxUsers) (bool, error)` and `store.ErrLaunchFull`.

- [ ] **Step 1: Write failing config and atomic-cap tests**

```go
func TestLoadRegistrationAndDailyLoginPolicy(t *testing.T) {
    env := validEnv(t)
    env["D04_REGISTRATION_OPEN"] = "true"
    env["D04_DAILY_LOGIN_CREDIT_USD"] = "20"
    cfg, err := Load(func(key string) string { return env[key] })
    if err != nil || !cfg.RegistrationOpen || cfg.DailyLoginCredit != 20_000_000 {
        t.Fatalf("cfg=%+v err=%v", cfg, err)
    }
}

func TestEnrollLaunchUserEnforcesHardCap(t *testing.T) {
    st := openTestStore(t)
    for id := int64(1); id <= 15; id++ {
        created, err := st.EnrollLaunchUser(context.Background(), InternalUser{UserID: id, JoinedAt: time.Now()}, 15)
        if err != nil || !created { t.Fatalf("id=%d created=%v err=%v", id, created, err) }
    }
    _, err := st.EnrollLaunchUser(context.Background(), InternalUser{UserID: 16, JoinedAt: time.Now()}, 15)
    if !errors.Is(err, ErrLaunchFull) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run RED**

```bash
docker run --rm -e GOMAXPROCS=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./internal/config ./internal/domain ./internal/store -p 1 -count=1
```

Expected: compilation fails for missing fields, constants, error, and method.

- [ ] **Step 3: Implement policy types and atomic roster insertion**

```go
const (
    GrantDailyLogin = "daily_login_credit"
    DailyLoginCredit MicroUSD = 20_000_000
)

func (s *Store) EnrollLaunchUser(ctx context.Context, user InternalUser, maxUsers int) (bool, error) {
    created := false
    err := s.WithTx(ctx, func(tx *sql.Tx) error {
        var exists int
        if err := tx.QueryRowContext(ctx,
            `SELECT COUNT(*) FROM internal_users WHERE user_id=?`, user.UserID).Scan(&exists); err != nil {
            return err
        }
        if exists == 1 { return nil }
        var count int
        if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM internal_users`).Scan(&count); err != nil {
            return err
        }
        if count >= maxUsers { return ErrLaunchFull }
        result, err := tx.ExecContext(ctx,
            `INSERT INTO internal_users(user_id,joined_at) VALUES(?,?)`,
            user.UserID, user.JoinedAt.UTC().Format(time.RFC3339Nano))
        if err != nil { return err }
        rows, err := result.RowsAffected()
        created = rows == 1
        return err
    })
    return created, err
}
```

Parse `D04_REGISTRATION_OPEN` with default `false`; require `D04_DAILY_LOGIN_CREDIT_USD` to equal `20`. Preserve the independent qualified cost-policy and total-budget write gates. Add a migration/index for `(user_id, kind, grant_date)`; keep invitation/referral tables readable.

- [ ] **Step 4: Run GREEN using the Step 2 command**
- [ ] **Step 5: Commit**

```bash
git add internal-test-service/internal/config internal-test-service/internal/domain internal-test-service/internal/store
git commit -m "feat: add D04 public launch policy state"
```

### Task 2: Idempotent Daily Login Credit

**Files:**
- Modify: `internal-test-service/internal/credits/service.go`
- Modify: `internal-test-service/internal/credits/service_test.go`
- Modify: `internal-test-service/internal/credits/budget_test.go`
- Modify: `internal-test-service/internal/testsupport/fake_sub2api.go`

**Interfaces:**
- Produces: `(*credits.Service).GrantDailyLogin(ctx, userID, now) (GrantResult, error)`.
- Consumes existing pending/succeeded/uncertain grant state and Admin API idempotency history.

- [ ] **Step 1: Write failing same-day, midnight, concurrency, and uncertain-write tests**

```go
func TestGrantDailyLoginIsOncePerShanghaiDay(t *testing.T) {
    svc, fake := newCreditTest(t)
    before := time.Date(2026, 7, 21, 15, 59, 0, 0, time.UTC)
    first, _ := svc.GrantDailyLogin(context.Background(), 7, before)
    replay, _ := svc.GrantDailyLogin(context.Background(), 7, before.Add(time.Minute))
    next, _ := svc.GrantDailyLogin(context.Background(), 7, before.Add(2*time.Minute))
    if first.AlreadyApplied || !replay.AlreadyApplied || next.AlreadyApplied { t.Fatal("wrong daily idempotency") }
    if len(fake.BalanceWrites) != 2 { t.Fatalf("writes=%d", len(fake.BalanceWrites)) }
}
```

Add 12 concurrent calls and require one provider effect. For an uncertain write with no provider-history evidence, require a read-only reason and no blind retry.

- [ ] **Step 2: Run RED**

```bash
docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./internal/credits -p 1 -race -count=1
```

Expected: `GrantDailyLogin` is undefined.

- [ ] **Step 3: Extract the existing grant state machine and add daily-login semantics**

```go
func (s *Service) GrantDailyLogin(ctx context.Context, userID int64, now time.Time) (GrantResult, error) {
    date := domain.ShanghaiDate(now, s.Timezone)
    key := fmt.Sprintf("d04-login-%d-%s", userID, date)
    return s.applyDailyGrant(ctx, store.Grant{
        UserID: userID, Kind: domain.GrantDailyLogin, Amount: s.DailyLoginCredit,
        GrantDate: sql.NullString{String: date, Valid: true},
        IdempotencyKey: key, Status: domain.TaskPending, CreatedAt: now,
    }, "D04 daily login credit")
}
```

Remove referral reservation occupancy and usage-triggered rewards from active flow. Keep reconciliation and read-only lock behavior.

- [ ] **Step 4: Run GREEN using the Step 2 command**
- [ ] **Step 5: Commit**

```bash
git add internal-test-service/internal/credits internal-test-service/internal/testsupport
git commit -m "feat: grant D04 credit on daily authentication"
```

### Task 3: Bounded Transparent Authentication Proxy

**Files:**
- Create: `internal-test-service/internal/authproxy/proxy.go`
- Create: `internal-test-service/internal/authproxy/proxy_test.go`
- Modify: `internal-test-service/internal/app/app.go`
- Modify: `internal-test-service/internal/app/app_test.go`

**Interfaces:**
- Produces: `authproxy.Response`, `authproxy.Forwarder`, `authproxy.ExtractUserID`.
- Produces: `(*authproxy.Service).Authenticate(ctx, endpoint, body, headers) (Response, error)`.

- [ ] **Step 1: Write failing passthrough and secret-boundary tests**

```go
func TestAuthenticatePreservesNativeResponseAndCreditsRosterUser(t *testing.T) {
    svc, forward, grant := newProxyTest(t, `{"data":{"user":{"id":7},"access_token":"secret"}}`)
    got, err := svc.Authenticate(context.Background(), "/api/v1/auth/login",
        []byte(`{"email":"u@example.com","password":"secret"}`), http.Header{})
    if err != nil || got.Status != 200 || string(got.Body) != forward.ResponseBody { t.Fatalf("got=%+v err=%v", got, err) }
    if grant.Calls != 1 || grant.UserID != 7 { t.Fatalf("grant=%+v", grant) }
}
```

Add failed login, non-roster, malformed success JSON, 2FA success, oversized body, allowed-header preservation, and captured-log redaction cases.

- [ ] **Step 2: Run RED**

```bash
docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./internal/authproxy ./internal/app -p 1 -race -count=1
```

Expected: the package and interfaces are missing.

- [ ] **Step 3: Implement fixed-endpoint in-memory forwarding**

```go
type Response struct { Status int; Header http.Header; Body []byte }
type Forwarder func(context.Context, string, []byte, http.Header) (Response, error)

type Service struct {
    Forward Forwarder
    IsLaunchUser func(context.Context, int64) (bool, error)
    GrantDailyLogin func(context.Context, int64, time.Time) (credits.GrantResult, error)
    Now func() time.Time
}

func (s *Service) Authenticate(ctx context.Context, endpoint string, body []byte, headers http.Header) (Response, error) {
    if !allowedEndpoint(endpoint) { return Response{}, ErrEndpointNotAllowed }
    response, err := s.Forward(ctx, endpoint, body, headers)
    if err != nil || response.Status < 200 || response.Status >= 300 { return response, err }
    userID := ExtractUserID(response.Body)
    if userID == 0 { return response, nil }
    member, memberErr := s.IsLaunchUser(ctx, userID)
    if memberErr != nil || !member { return response, nil }
    _, _ = s.GrantDailyLogin(ctx, userID, s.Now())
    return response, nil
}
```

Allow only register/login/login-2fa private paths, cap request and response at 1 MiB, use a 20-second timeout, allowlist request/response headers, and never log bodies or authentication headers. Authentication success remains successful when the grant needs reconciliation.

- [ ] **Step 4: Run GREEN using the Step 2 command**
- [ ] **Step 5: Commit**

```bash
git add internal-test-service/internal/authproxy internal-test-service/internal/app
git commit -m "feat: proxy native D04 authentication safely"
```

### Task 4: Public Registration Gate and Immediate Credit

**Files:**
- Rewrite: `internal-test-service/internal/registration/service.go`
- Rewrite: `internal-test-service/internal/registration/service_test.go`
- Modify: `internal-test-service/internal/e2e/e2e_test.go`

**Interfaces:**
- Produces: `(*registration.Service).Register(ctx, body, headers) (authproxy.Response, error)`.
- Consumes atomic roster insertion, fixed auth forwarder, and daily-credit service.

- [ ] **Step 1: Write failing open/closed/cap/concurrency tests**

```go
func TestPublicRegistrationStopsAtFifteen(t *testing.T) {
    svc := newPublicRegistrationTest(t, true, 15)
    for id := int64(1); id <= 15; id++ {
        svc.ForwardResponse = authResponse(id)
        got, err := svc.Register(context.Background(), registrationBody(id), http.Header{})
        if err != nil || got.Status != 200 { t.Fatalf("id=%d got=%+v err=%v", id, got, err) }
    }
    svc.ForwardResponse = authResponse(16)
    got, _ := svc.Register(context.Background(), registrationBody(16), http.Header{})
    if got.Status != http.StatusConflict || !bytes.Contains(got.Body, []byte("D04_REGISTRATION_FULL")) { t.Fatalf("got=%+v", got) }
}
```

Add switch-off, read-only, upstream-failure, duplicate-user, immediate-credit, and 20-concurrent-registration cases. Assert no invitation field is required or inserted.

- [ ] **Step 2: Run RED**

```bash
docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./internal/registration ./internal/e2e -p 1 -race -count=1
```

Expected: old invitation behavior fails.

- [ ] **Step 3: Implement the new gate**

```go
func (s *Service) Register(ctx context.Context, body []byte, headers http.Header) (authproxy.Response, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.mode(ctx) != "write" || !s.RegistrationOpen { return closedResponse(), nil }
    count, err := s.Store.CountRegisteredUsers(ctx)
    if err != nil { return authproxy.Response{}, err }
    if count >= s.MaxUsers { return fullResponse(), nil }
    response, err := s.Forward(ctx, "/api/v1/auth/register", body, headers)
    if err != nil || response.Status < 200 || response.Status >= 300 { return response, err }
    userID := authproxy.ExtractUserID(response.Body)
    if userID == 0 { return authproxy.Response{}, errors.New("registration response missing user id") }
    _, enrollErr := s.Store.EnrollLaunchUser(ctx,
        store.InternalUser{UserID: userID, JoinedAt: s.Now()}, s.MaxUsers)
    if errors.Is(enrollErr, store.ErrLaunchFull) { return fullResponse(), nil }
    if enrollErr != nil {
        _ = s.Store.SetReadOnlyReason(ctx, "uncertain launch roster write")
        return response, nil
    }
    _, _ = s.GrantDailyLogin(ctx, userID, s.Now())
    return response, nil
}
```

Return stable `D04_REGISTRATION_CLOSED` and `D04_REGISTRATION_FULL` JSON. Never add or inspect invitation fields.

- [ ] **Step 4: Run GREEN using the Step 2 command**
- [ ] **Step 5: Commit**

```bash
git add internal-test-service/internal/registration internal-test-service/internal/e2e
git commit -m "feat: gate D04 public registration at fifteen users"
```

### Task 5: HTTP, Caddy, Compose, and Retired Public Surface

**Files:**
- Modify: `internal-test-service/internal/http/server.go`
- Modify: `internal-test-service/internal/http/server_test.go`
- Modify: `internal-test-service/internal/http/nonfunctional_test.go`
- Delete: `internal-test-service/internal/http/join.html`
- Modify: `infra/Caddyfile`
- Modify: `infra/compose.yaml`
- Modify: `infra/compose.d04-read-only.yaml`
- Modify: `tests/internal_test/validate_internal_test_contract.sh`

**Interfaces:**
- Exposes only fixed registration/login/2FA proxy handlers and `/healthz`.
- Retires join, invitation, referral, and check-in public routes.

- [ ] **Step 1: Write failing HTTP and deployment-contract tests**

```go
func TestRetiredLaunchEndpointsAreGone(t *testing.T) {
    handler := newHTTPTest(t)
    for _, path := range []string{"/internal-test/join/x", "/internal-test/api/invitations", "/internal-test/api/checkin"} {
        rr := httptest.NewRecorder()
        handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, nil))
        if rr.Code != http.StatusNotFound { t.Fatalf("%s=%d", path, rr.Code) }
    }
}
```

Update the shell test to require all three Caddy auth paths, registration toggle/default credit variables, and absence of invitation/check-in routes.

- [ ] **Step 2: Run RED**

```bash
docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./internal/http -p 1 -race -count=1
bash tests/internal_test/validate_internal_test_contract.sh
```

Expected: old routes remain and login/2FA interception is absent.

- [ ] **Step 3: Wire fixed auth routes and safe defaults**

```caddyfile
@d04_auth {
    method POST
    path /api/v1/auth/register /api/v1/auth/login /api/v1/auth/login/2fa
}
reverse_proxy @d04_auth internal-test-service:8090
```

Set Compose defaults `D04_REGISTRATION_OPEN=false` and `D04_DAILY_LOGIN_CREDIT_USD=20`. The read-only overlay must explicitly keep registration false, cost policy unqualified, and mode read-only.

- [ ] **Step 4: Run GREEN and Compose validation**

```bash
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
docker compose -f infra/compose.d04-read-only.yaml config --quiet
```

- [ ] **Step 5: Commit**

```bash
git add internal-test-service/internal/http infra tests/internal_test
git commit -m "feat: route native auth through D04 launch policy"
```

### Task 6: Complete Functional and Existing Nonfunctional Regression

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md`

- [ ] **Step 1: Run fresh full verification**

```bash
docker run --rm -e GOMAXPROCS=1 -e CGO_ENABLED=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go test ./... -p 1 -race -count=1
docker run --rm -e GOMAXPROCS=1 -v "$PWD/internal-test-service:/src" -w /src golang:1.24.13-bookworm go vet ./...
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

Expected: all exit 0 with no production or upstream write.

- [ ] **Step 2: Confirm the delegated nonfunctional baseline remains covered**

Run focused concurrent authentication, SQLite locking, header, origin, malformed-token, and secret-redaction tests. Record that the prior 60/60 public health sample is ingress evidence only, not model capacity.

- [ ] **Step 3: Update authority documents and report**

Record exact test evidence, unchanged `D04_MODE=read_only`, and the remaining explicit budget/write production gate. Do not claim production cap or credit behavior from fixtures alone.

- [ ] **Step 4: Commit**

```bash
git add docs/project/current-state.md docs/project/llm-handoff.md docs/superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md
git commit -m "docs: verify D04 public registration and login credits"
```
