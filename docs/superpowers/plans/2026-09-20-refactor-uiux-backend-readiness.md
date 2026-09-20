# refactorUIUXv0.1 Backend Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dependency-aware Sub2API `/readyz` endpoint that returns the JSON contract required by the independent test-station release controller and can never be replaced by embedded SPA HTML.

**Architecture:** Keep `/health` as liveness. Add a narrow readiness checker in native server routes that pings the existing PostgreSQL and Redis clients under one two-second request timeout, then inject it through the current Wire graph. Reuse the embedded frontend bypass function so both modern and legacy middleware pass `/readyz` to Gin.

**Tech Stack:** Go 1.24, Gin, `database/sql`, go-redis v9, Wire, build tag `embed`, Go unit tests.

**Spec:** `docs/superpowers/specs/2026-09-20-refactor-uiux-backend-readiness-design.md`

---

## Global constraints

- Work only in `codex/refactor-uiux-backend-readiness`, based on `origin/main@645ce06834698cebcd6707836a234d7096e0a081`.
- Do not modify migrations, runtime env, Compose, Caddy, frontend pages, business APIs, task queue, or project progress.
- Do not merge, push, deploy, or contact either station.
- `/health` remains a static liveness response.
- `/readyz` response exposes only `status`; never expose dependency identity or underlying errors.
- The root production `/readyz` remains relay-ops-owned; this task changes only the Sub2API backend handler.
- Use TDD for every behavior. Run the focused failing test before implementation, then the focused passing test.
- The baseline `go test ./internal/server/routes` has two unrelated existing failures in gateway error-copy assertions. Do not change those tests; final verification must prove no new failures and report the baseline separately.

### Task 1: Native dependency readiness checker and HTTP contract

**Files:**
- Create: `upstream/sub2api/backend/internal/server/routes/readiness.go`
- Create: `upstream/sub2api/backend/internal/server/routes/common_test.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/common.go`

- [ ] **Step 1: Write failing checker tests**

Create fakes and table-driven tests in `common_test.go`:

```go
type fakeDatabasePinger struct {
    err   error
    calls int
}

func (f *fakeDatabasePinger) PingContext(context.Context) error {
    f.calls++
    return f.err
}

type fakeRedisPinger struct {
    err   error
    calls int
}

func (f *fakeRedisPinger) Ping(context.Context) *redis.StatusCmd {
    f.calls++
    return redis.NewStatusResult("PONG", f.err)
}
```

Cover:

```text
both dependencies succeed -> nil, one call each
database fails -> same non-nil result, Redis not called
Redis fails -> non-nil result after one DB call
```

Construct through an unexported testable constructor `newDependencyReadinessChecker(databasePinger, redisPinger)` so production still exposes a concrete constructor for `*sql.DB` and `*redis.Client`.

- [ ] **Step 2: Run the checker test and verify RED**

Run from `upstream/sub2api/backend`:

```bash
go test ./internal/server/routes -run '^TestDependencyReadinessChecker$' -count=1
```

Expected: compile failure because `newDependencyReadinessChecker` does not exist.

- [ ] **Step 3: Implement the minimum checker**

Create `readiness.go` with:

```go
type ReadinessChecker interface {
    Check(context.Context) error
}

type databasePinger interface {
    PingContext(context.Context) error
}

type redisPinger interface {
    Ping(context.Context) *redis.StatusCmd
}

type dependencyReadinessChecker struct {
    database databasePinger
    redis    redisPinger
}

func NewDependencyReadinessChecker(database *sql.DB, redisClient *redis.Client) ReadinessChecker {
    return newDependencyReadinessChecker(database, redisClient)
}

func newDependencyReadinessChecker(database databasePinger, redisClient redisPinger) ReadinessChecker {
    return &dependencyReadinessChecker{database: database, redis: redisClient}
}

func (r *dependencyReadinessChecker) Check(ctx context.Context) error {
    if r.database == nil || r.redis == nil {
        return errors.New("readiness dependency unavailable")
    }
    if err := r.database.PingContext(ctx); err != nil {
        return err
    }
    return r.redis.Ping(ctx).Err()
}
```

Do not log or wrap dependency errors with host or connection data.

- [ ] **Step 4: Run the checker test and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Write failing HTTP contract tests**

Add a fake checker:

```go
type fakeReadinessChecker struct {
    check func(context.Context) error
}

func (f fakeReadinessChecker) Check(ctx context.Context) error { return f.check(ctx) }
```

Test `readinessHandler(checker, timeout)` and `RegisterCommonRoutes`:

```text
/health remains 200 application/json status=ok
ready checker success -> 200 application/json status=ready
checker error -> 503 application/json status=not_ready and no error detail
nil checker -> 503 status=not_ready
checker blocks until ctx.Done with 10ms test timeout -> returns 503 promptly
unregistered method POST /readyz is not accepted as successful readiness
```

The timeout test must use the request context and must not sleep for the production two seconds.

- [ ] **Step 6: Run the HTTP tests and verify RED**

```bash
go test ./internal/server/routes -run '^Test(CommonRoutesHealthAndReadiness|ReadinessHandlerTimeout)$' -count=1
```

Expected: compile failure because the handler signature and `/readyz` route are absent.

- [ ] **Step 7: Implement the HTTP route**

Modify `common.go`:

```go
const readinessProbeTimeout = 2 * time.Second

func RegisterCommonRoutes(r *gin.Engine, readiness ReadinessChecker) {
    r.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "ok"})
    })
    r.GET("/readyz", readinessHandler(readiness, readinessProbeTimeout))
    // existing routes unchanged
}

func readinessHandler(readiness ReadinessChecker, timeout time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        if readiness == nil {
            c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
            return
        }
        ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
        defer cancel()
        if err := readiness.Check(ctx); err != nil {
            c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
            return
        }
        c.JSON(http.StatusOK, gin.H{"status": "ready"})
    }
}
```

Keep event logging and setup status behavior unchanged.

- [ ] **Step 8: Run Task 1 tests and commit**

```bash
gofmt -w internal/server/routes/readiness.go internal/server/routes/common.go internal/server/routes/common_test.go
go test ./internal/server/routes -run '^Test(DependencyReadinessChecker|CommonRoutesHealthAndReadiness|ReadinessHandlerTimeout)$' -count=1
git diff --check
git add internal/server/routes/readiness.go internal/server/routes/common.go internal/server/routes/common_test.go
git commit -m "feat: add native backend readiness"
```

Expected: focused tests PASS.

### Task 2: Embedded frontend readiness bypass

**Files:**
- Modify: `upstream/sub2api/backend/internal/web/embed_on.go`
- Modify: `upstream/sub2api/backend/internal/web/embed_test.go`

- [ ] **Step 1: Write the failing bypass test**

Add:

```go
func TestEmbeddedFrontendBypassesReadiness(t *testing.T) {
    require.True(t, shouldBypassEmbeddedFrontend("/readyz"))
    require.False(t, shouldBypassEmbeddedFrontend("/readyz/details"))
}
```

Also add `/readyz` to both existing `apiPaths` lists under `TestFrontendServer_Middleware/skips_api_routes` and `TestServeEmbeddedFrontend/skips_api_routes`, preserving the table structure.

- [ ] **Step 2: Run the embed test and verify RED**

```bash
go test -tags embed ./internal/web -run '^TestEmbeddedFrontendBypassesReadiness$' -count=1
```

Expected: FAIL because `/readyz` currently falls through to SPA handling.

- [ ] **Step 3: Implement the exact bypass**

In `shouldBypassEmbeddedFrontend`, add:

```go
trimmed == "/readyz" ||
```

Place it next to `/health`. Do not bypass `/readyz/*` or other SPA routes.

- [ ] **Step 4: Run modern and legacy middleware tests**

```bash
gofmt -w internal/web/embed_on.go internal/web/embed_test.go
go test -tags embed ./internal/web -run 'Test(EmbeddedFrontendBypassesReadiness|FrontendServer_Middleware|ServeEmbeddedFrontend)$' -count=1
git diff --check
```

Expected: PASS; SPA route tests remain unchanged.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/web/embed_on.go internal/web/embed_test.go
git commit -m "fix: bypass spa fallback for readiness"
```

### Task 3: Wire dependency injection and generated graph

**Files:**
- Modify: `upstream/sub2api/backend/internal/server/http.go`
- Modify: `upstream/sub2api/backend/internal/server/router.go`
- Regenerate: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Possibly modify only if generator requires it: `upstream/sub2api/backend/go.sum`

- [ ] **Step 1: Add the compile-breaking source wiring change**

Add `database/sql` imports and `database *sql.DB` parameters to `ProvideRouter`, `SetupRouter`, and `registerRoutes`. Register:

```go
routes.RegisterCommonRoutes(r, routes.NewDependencyReadinessChecker(database, redisClient))
```

Pass `database` through each internal call. Do not change provider sets or create a second database/Redis client.

- [ ] **Step 2: Run compile verification and confirm RED**

```bash
go test ./internal/server ./cmd/server -run '^$' -count=1
```

Expected: FAIL in generated `wire_gen.go` because it still calls `ProvideRouter` without the database argument.

- [ ] **Step 3: Regenerate Wire**

```bash
go generate ./cmd/server
```

Expected: `cmd/server/wire_gen.go` calls `server.ProvideRouter(..., compositeRouteResolver, db, redisClient)`. If `go.sum` gains only Wire tool dependencies, keep it; any unrelated generated source change must be reverted before continuing.

- [ ] **Step 4: Verify generated graph and build**

```bash
gofmt -w internal/server/http.go internal/server/router.go cmd/server/wire_gen.go
go test ./internal/server ./cmd/server -run '^$' -count=1
go build ./cmd/server
go build -tags embed ./cmd/server
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit Task 3**

```bash
git add internal/server/http.go internal/server/router.go cmd/server/wire_gen.go go.sum
git commit -m "chore: wire backend readiness dependencies"
```

If `go.sum` is unchanged, omit it from `git add`.

### Task 4: Final verification and candidate handoff

**Files:**
- Modify: `docs/superpowers/plans/2026-09-20-refactor-uiux-backend-readiness.md` checkbox state only

- [ ] **Step 1: Run focused behavior tests**

```bash
cd upstream/sub2api/backend
go test ./internal/server/routes -run '^Test(DependencyReadinessChecker|CommonRoutesHealthAndReadiness|ReadinessHandlerTimeout)$' -count=1
go test -tags embed ./internal/web -run 'Test(EmbeddedFrontendBypassesReadiness|FrontendServer_Middleware|ServeEmbeddedFrontend)$' -count=1
go test ./internal/server ./cmd/server -run '^$' -count=1
go build ./cmd/server
go build -tags embed ./cmd/server
```

Expected: all commands exit 0.

- [ ] **Step 2: Re-run the known baseline package and compare failures**

```bash
go test ./internal/server/routes -count=1
```

Expected on the frozen baseline: only these two pre-existing failures may remain:

```text
TestGatewayRoutesKeyBillingInfoEndToEnd/simple_mode
TestGatewayRoutesAlphaSearchRejectsUnsupportedGroup
```

If the command passes, record that the baseline drift has been resolved elsewhere. If any additional test fails, stop and fix only readiness-caused regressions.

- [ ] **Step 3: Verify scope and repository state**

From repository root:

```bash
git diff --check
git status --short
git diff --name-only 645ce06834698cebcd6707836a234d7096e0a081..HEAD
! git diff --name-only 645ce06834698cebcd6707836a234d7096e0a081..HEAD | grep -E '(^|/)(migrations|frontend|infra|ops)/'
```

Expected: only the spec, plan, backend readiness route/tests, embed bypass/tests, server wiring, generated Wire file, and possibly `backend/go.sum` differ.

- [ ] **Step 4: Mark plan complete and commit documentation state**

Mark completed checkboxes, then:

```bash
git add docs/superpowers/plans/2026-09-20-refactor-uiux-backend-readiness.md
git commit -m "docs: record backend readiness verification"
```

- [ ] **Step 5: Run fresh post-commit verification and report candidate**

Repeat Steps 1 and 3 after the final commit. Report:

```text
status=READY_FOR_ROOT_REVIEW
downtime_required=false
migration_changes=none
configuration_changes=none
baseline=645ce06834698cebcd6707836a234d7096e0a081
```

Include commits, changed files, focused test results, the two unrelated baseline failures if still present, rollback by reverting the candidate commits, and confirmation that the branch/worktree remains unmerged, unpushed, undeployed, and preserved.
