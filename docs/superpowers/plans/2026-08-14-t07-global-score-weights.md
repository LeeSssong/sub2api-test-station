# T07 Global Score Weights Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global account-monitor score weight editor backed by independent API and singleton persistence, limited to cost, success, TTFT, and total latency weights.

**Architecture:** The backend owns score truth: a new singleton table stores only four global weights, service methods expose GET/PUT/DELETE semantics, and the global account monitor projection uses those weights when ranking accounts. The frontend reuses `AccountMonitorGroupScoreDialog.vue` through a `mode="group" | "global"` prop; global mode loads weights with a dedicated GET when the dialog opens and saves/resets through dedicated endpoints.

**Tech Stack:** Go 1.26.5, Gin, `database/sql`, PostgreSQL migrations, `go-sqlmock`, Vue 3, TypeScript 5.6, Vitest, Vue Test Utils.

## Global Constraints

- Scope only includes four global score weights: cost `15`, success `45`, TTFT `20`, latency `20`.
- Do not add global API, UI, or persistence for `ttft_target_ms`, `ttft_limit_ms`, `latency_target_ms`, or `latency_limit_ms`.
- Keep group score weight semantics, group thresholds, group API, and group persistence unchanged.
- Keep account monitor projection DTO, `AccountMonitorSchemaVersion`, and existing projection endpoints unchanged.
- Open the global score dialog by calling `GET /admin/account-monitors/global-score-weights`; do not carry global weights in the projection.
- `PUT /admin/account-monitors/global-score-weights` and `DELETE /admin/account-monitors/global-score-weights` must use existing `stepUpAuth`; GET uses ordinary admin auth.
- Storage errors must not silently fall back to defaults; only a missing singleton row returns default weights.
- Do not change advanced scheduler algorithms, Top-K, quota headroom weights, production routing, GitHub Actions, `external-primary`, root `main`, global queue, or project progress ledger.
- Before final whole-branch review, perform the REFRESH_REQUIRED gate by integrating latest root `main` (`fc44bde10` or newer at that time) while preserving root controller global documents.
- Candidate branch must stop at `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or run production verification without root `AUTHORIZE_MERGE_TO_MAIN` containing the target main SHA.

---

## File Structure

- Create `upstream/sub2api/backend/migrations/223_account_monitor_global_score_weights.sql`: expand-only singleton table for four global weights.
- Create `upstream/sub2api/backend/migrations/account_monitor_global_score_weights_migration_test.go`: migration regression test proving the singleton table exists and excludes threshold columns.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_types.go`: repository interface methods and response type for global score weights.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service.go`: global weight service methods, four-weight validation helper, and global `ListWindow` scoring integration.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`: service TDD coverage for default fallback, error propagation, validation, reset, and global ranking.
- Modify `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`: singleton load/save/delete implementation.
- Modify `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`: `sqlmock` coverage for singleton GET/PUT/DELETE and no-threshold persistence.
- Modify `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`: global GET/PUT/DELETE handlers with four-field request contract.
- Modify `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`: handler response and validation coverage.
- Modify `upstream/sub2api/backend/internal/server/routes/admin.go`: add global routes with matching step-up protection for write operations.
- Modify `upstream/sub2api/backend/internal/server/routes/account_monitor_routes_test.go`: route registration and step-up coverage.
- Modify `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`: add global score weight types and API client functions.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue`: add `mode`, keep group mode eight fields, render global mode four fields only.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts`: component mode regression tests.
- Modify `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`: add all-site score settings entry, load global weights on open, save/reset global weights, refresh projection after successful mutations.
- Modify `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`: all-site entry, GET-on-open, global save/reset, reload failure, and group regression coverage.
- Modify `docs/superpowers/reports/2026-08-14-t07-global-score-weights-review.md`: implementation/review handoff report created during final verification.

---

### Task 1: Backend Singleton Persistence And Service API

**Files:**
- Create: `upstream/sub2api/backend/migrations/223_account_monitor_global_score_weights.sql`
- Create: `upstream/sub2api/backend/migrations/account_monitor_global_score_weights_migration_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Produces: `AccountMonitorGlobalScoreWeightsResponse` with fields `Cost`, `Success`, `TTFT`, `Latency`, `UpdatedBy`, `UpdatedAt *time.Time`, `IsDefault`.
- Produces repository methods: `LoadGlobalScoreWeights(ctx context.Context) (AccountMonitorScoreWeights, error)`, `SaveGlobalScoreWeights(ctx context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorScoreWeights, error)`, `ResetGlobalScoreWeights(ctx context.Context) error`.
- Produces service methods: `GetGlobalScoreWeights(ctx context.Context) (AccountMonitorGlobalScoreWeightsResponse, error)`, `UpdateGlobalScoreWeights(ctx context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorGlobalScoreWeightsResponse, error)`, `ResetGlobalScoreWeights(ctx context.Context, actorID int64) (AccountMonitorGlobalScoreWeightsResponse, error)`.
- Produces helper: `defaultGlobalScoreWeightsResponse() AccountMonitorGlobalScoreWeightsResponse`.

- [ ] **Step 1: Write the migration test**

Add `TestAccountMonitorGlobalScoreWeightsMigrationCreatesFourWeightSingleton` to `upstream/sub2api/backend/migrations/account_monitor_global_score_weights_migration_test.go`, following the existing migration tests that read embedded SQL through `FS.ReadFile`.

```go
func TestAccountMonitorGlobalScoreWeightsMigrationCreatesFourWeightSingleton(t *testing.T) {
	sqlBytes, err := FS.ReadFile("223_account_monitor_global_score_weights.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"create table if not exists account_monitor_global_score_weights",
		"singleton boolean primary key default true check (singleton)",
		"cost_weight smallint not null check (cost_weight >= 0)",
		"success_weight smallint not null check (success_weight >= 0)",
		"ttft_weight smallint not null check (ttft_weight >= 0)",
		"latency_weight smallint not null check (latency_weight >= 0)",
		"updated_by bigint not null",
		"updated_at timestamptz not null default now()",
		"cost_weight + success_weight + ttft_weight + latency_weight = 100",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("global table must not include threshold column %s", forbidden)
		}
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./migrations -run TestAccountMonitorGlobalScoreWeightsMigrationCreatesFourWeightSingleton -count=1`
Expected: FAIL because the migration file does not exist yet.

- [ ] **Step 2: Add the expand-only migration**

Create `upstream/sub2api/backend/migrations/223_account_monitor_global_score_weights.sql`.

```sql
CREATE TABLE IF NOT EXISTS account_monitor_global_score_weights (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    cost_weight SMALLINT NOT NULL CHECK (cost_weight >= 0),
    success_weight SMALLINT NOT NULL CHECK (success_weight >= 0),
    ttft_weight SMALLINT NOT NULL CHECK (ttft_weight >= 0),
    latency_weight SMALLINT NOT NULL CHECK (latency_weight >= 0),
    updated_by BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cost_weight + success_weight + ttft_weight + latency_weight = 100)
);
```

Run: `cd upstream/sub2api/backend && go test ./migrations -run TestAccountMonitorGlobalScoreWeightsMigrationCreatesFourWeightSingleton -count=1`
Expected: PASS.

- [ ] **Step 3: Write repository tests before implementation**

Extend `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go` with `TestAccountMonitorRepositoryPersistsGlobalScoreWeights`.

```go
func TestAccountMonitorRepositoryPersistsGlobalScoreWeights(t *testing.T) {
	db, mock, cleanup := newAccountMonitorRepoMock(t)
	defer cleanup()
	repo := NewAccountMonitorRepository(db)
	ctx := context.Background()
	updatedAt := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"cost_weight", "success_weight", "ttft_weight", "latency_weight", "updated_by", "updated_at"}).
			AddRow(25, 35, 20, 20, int64(9), updatedAt))
	weights, err := repo.LoadGlobalScoreWeights(ctx)
	if err != nil {
		t.Fatalf("LoadGlobalScoreWeights() error = %v", err)
	}
	if weights.Cost != 25 || weights.Success != 35 || weights.TTFT != 20 || weights.Latency != 20 || weights.UpdatedBy != 9 || !weights.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("weights = %#v", weights)
	}

	returnedAt := time.Date(2026, 8, 14, 10, 5, 0, 0, time.UTC)
	mock.ExpectQuery("INSERT INTO account_monitor_global_score_weights").
		WithArgs(25, 35, 20, 20, int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"cost_weight", "success_weight", "ttft_weight", "latency_weight", "updated_by", "updated_at"}).
			AddRow(25, 35, 20, 20, int64(9), returnedAt))
	saved, err := repo.SaveGlobalScoreWeights(ctx, 9, service.AccountMonitorScoreWeights{Cost: 25, Success: 35, TTFT: 20, Latency: 20})
	if err != nil {
		t.Fatalf("SaveGlobalScoreWeights() error = %v", err)
	}
	if saved.Cost != 25 || saved.Success != 35 || saved.TTFT != 20 || saved.Latency != 20 || saved.UpdatedBy != 9 || !saved.UpdatedAt.Equal(returnedAt) {
		t.Fatalf("saved weights = %#v", saved)
	}

	mock.ExpectExec("DELETE FROM account_monitor_global_score_weights").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.ResetGlobalScoreWeights(ctx); err != nil {
		t.Fatalf("ResetGlobalScoreWeights() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/repository -run TestAccountMonitorRepositoryPersistsGlobalScoreWeights -count=1`
Expected: FAIL because repository methods are missing.

- [ ] **Step 4: Implement repository methods**

In `upstream/sub2api/backend/internal/service/account_monitor_types.go`, add three repository interface methods exactly as listed above.

In `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`, add:

```go
func (r *accountMonitorRepository) LoadGlobalScoreWeights(ctx context.Context) (service.AccountMonitorScoreWeights, error) {
	var weights service.AccountMonitorScoreWeights
	err := r.db.QueryRowContext(ctx, `
		SELECT cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		FROM account_monitor_global_score_weights
		WHERE singleton = TRUE
	`).Scan(&weights.Cost, &weights.Success, &weights.TTFT, &weights.Latency, &weights.UpdatedBy, &weights.UpdatedAt)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return weights, nil
}

func (r *accountMonitorRepository) SaveGlobalScoreWeights(ctx context.Context, actorID int64, weights service.AccountMonitorScoreWeights) (service.AccountMonitorScoreWeights, error) {
	if actorID <= 0 {
		return service.AccountMonitorScoreWeights{}, errors.New("invalid actor id")
	}
	if err := validateFourScoreWeights(weights); err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	var saved service.AccountMonitorScoreWeights
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO account_monitor_global_score_weights (
			singleton, cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		) VALUES (TRUE, $1, $2, $3, $4, $5, NOW())
		ON CONFLICT (singleton) DO UPDATE SET
			cost_weight = EXCLUDED.cost_weight,
			success_weight = EXCLUDED.success_weight,
			ttft_weight = EXCLUDED.ttft_weight,
			latency_weight = EXCLUDED.latency_weight,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
		RETURNING cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
	`, weights.Cost, weights.Success, weights.TTFT, weights.Latency, actorID).Scan(
		&saved.Cost, &saved.Success, &saved.TTFT, &saved.Latency, &saved.UpdatedBy, &saved.UpdatedAt,
	)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return saved, nil
}

func (r *accountMonitorRepository) ResetGlobalScoreWeights(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM account_monitor_global_score_weights
		WHERE singleton = TRUE
	`)
	return err
}
```

Add repository-local `validateFourScoreWeights` that checks only non-negative four weights and sum `100`; do not inspect threshold fields.

Run: `cd upstream/sub2api/backend && go test ./internal/repository -run 'TestAccountMonitorRepositoryPersists(Global|Group)ScoreWeights' -count=1`
Expected: PASS.

- [ ] **Step 5: Write service tests before implementation**

In `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`, extend `accountMonitorRepoStub` with `globalWeights AccountMonitorScoreWeights`, `globalWeightsErr error`, `globalWeightsSaveErr error`, `globalWeightsResetErr error`, `globalWeightsSaved []AccountMonitorScoreWeights`, `globalWeightsSavedAt time.Time`, `globalWeightsReset bool`, and `loadGlobalScoreWeightsCalls int`, then implement the new interface methods in the stub. The stub `SaveGlobalScoreWeights` must append the sanitized weights, attach `UpdatedBy: actorID` and deterministic `UpdatedAt: globalWeightsSavedAt`, and return that saved record without calling `LoadGlobalScoreWeights`.

Add tests:

```go
func TestAccountMonitorServiceGlobalScoreWeightsDefaultAndErrors(t *testing.T) {
	repo := &accountMonitorRepoStub{globalWeightsErr: sql.ErrNoRows}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)

	got, err := svc.GetGlobalScoreWeights(context.Background())
	if err != nil {
		t.Fatalf("GetGlobalScoreWeights() error = %v", err)
	}
	if !got.IsDefault || got.Cost != 15 || got.Success != 45 || got.TTFT != 20 || got.Latency != 20 || got.UpdatedAt != nil {
		t.Fatalf("default response = %#v", got)
	}

	repo.globalWeightsErr = errors.New("database unavailable")
	if _, err := svc.GetGlobalScoreWeights(context.Background()); err == nil {
		t.Fatal("expected storage error to propagate")
	}
}

func TestAccountMonitorServiceUpdatesGlobalScoreWeightsWithoutThresholdPersistence(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	savedAt := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	repo.globalWeightsSavedAt = savedAt

	got, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{
		Cost: 30, Success: 30, TTFT: 20, Latency: 20,
		TTFTTargetMS: 1, TTFTLimitMS: 2, LatencyTargetMS: 3, LatencyLimitMS: 4,
	})
	if err != nil {
		t.Fatalf("UpdateGlobalScoreWeights() error = %v", err)
	}
	if got.IsDefault || got.Cost != 30 || got.Success != 30 || got.TTFT != 20 || got.Latency != 20 {
		t.Fatalf("saved response = %#v", got)
	}
	if got.UpdatedBy != 12 || got.UpdatedAt == nil || !got.UpdatedAt.Equal(savedAt) {
		t.Fatalf("audit fields = updated_by %d updated_at %#v", got.UpdatedBy, got.UpdatedAt)
	}
	saved := repo.globalWeightsSaved[len(repo.globalWeightsSaved)-1]
	if saved.TTFTTargetMS != 0 || saved.TTFTLimitMS != 0 || saved.LatencyTargetMS != 0 || saved.LatencyLimitMS != 0 {
		t.Fatalf("global save must not persist thresholds: %#v", saved)
	}
}

func TestAccountMonitorServiceGlobalSaveErrorDoesNotReread(t *testing.T) {
	repo := &accountMonitorRepoStub{globalWeightsSaveErr: errors.New("write failed")}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: 30, Success: 30, TTFT: 20, Latency: 20}); err == nil {
		t.Fatal("expected save error")
	}
	if repo.loadGlobalScoreWeightsCalls != 0 {
		t.Fatalf("UpdateGlobalScoreWeights reread after save error: %d", repo.loadGlobalScoreWeightsCalls)
	}
}

func TestAccountMonitorServiceRejectsInvalidGlobalScoreWeights(t *testing.T) {
	svc := NewAccountMonitorService(&accountMonitorRepoStub{}, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: 30, Success: 30, TTFT: 20, Latency: 19}); err == nil {
		t.Fatal("expected invalid sum error")
	}
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: -1, Success: 61, TTFT: 20, Latency: 20}); err == nil {
		t.Fatal("expected negative weight error")
	}
	if _, err := svc.ResetGlobalScoreWeights(context.Background(), 0); err == nil {
		t.Fatal("expected invalid actor error")
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorService(Global|RejectsInvalidGlobal)' -count=1`
Expected: FAIL because service methods and response type are missing.

- [ ] **Step 6: Implement service methods and four-weight validation**

In `upstream/sub2api/backend/internal/service/account_monitor_types.go`, add:

```go
type AccountMonitorGlobalScoreWeightsResponse struct {
	Cost      int        `json:"cost"`
	Success   int        `json:"success"`
	TTFT      int        `json:"ttft"`
	Latency   int        `json:"latency"`
	UpdatedBy int64      `json:"updated_by"`
	UpdatedAt *time.Time `json:"updated_at"`
	IsDefault bool       `json:"is_default"`
}
```

In `upstream/sub2api/backend/internal/service/account_monitor_service.go`, implement:

```go
var ErrAccountMonitorInvalidScoreWeights = errors.New("invalid account monitor score weights")
```

```go
func (s *AccountMonitorService) GetGlobalScoreWeights(ctx context.Context) (AccountMonitorGlobalScoreWeightsResponse, error) {
	weights, err := s.repo.LoadGlobalScoreWeights(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultGlobalScoreWeightsResponse(), nil
	}
	if err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("load global score weights: %w", err)
	}
	return globalScoreWeightsResponse(weights, false), nil
}

func (s *AccountMonitorService) UpdateGlobalScoreWeights(ctx context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorGlobalScoreWeightsResponse, error) {
	if actorID <= 0 {
		return AccountMonitorGlobalScoreWeightsResponse{}, errors.New("invalid actor id")
	}
	weights = fourAccountMonitorScoreWeights(weights)
	if err := validateAccountMonitorFourScoreWeights(weights); err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, err
	}
	saved, err := s.repo.SaveGlobalScoreWeights(ctx, actorID, weights)
	if err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("save global score weights: %w", err)
	}
	return globalScoreWeightsResponse(saved, false), nil
}

func (s *AccountMonitorService) ResetGlobalScoreWeights(ctx context.Context, actorID int64) (AccountMonitorGlobalScoreWeightsResponse, error) {
	if actorID <= 0 {
		return AccountMonitorGlobalScoreWeightsResponse{}, errors.New("invalid actor id")
	}
	if err := s.repo.ResetGlobalScoreWeights(ctx); err != nil {
		return AccountMonitorGlobalScoreWeightsResponse{}, fmt.Errorf("reset global score weights: %w", err)
	}
	return defaultGlobalScoreWeightsResponse(), nil
}
```

Add helper functions:

```go
func fourAccountMonitorScoreWeights(weights AccountMonitorScoreWeights) AccountMonitorScoreWeights {
	return AccountMonitorScoreWeights{Cost: weights.Cost, Success: weights.Success, TTFT: weights.TTFT, Latency: weights.Latency}
}

func validateAccountMonitorFourScoreWeights(weights AccountMonitorScoreWeights) error {
	if weights.Cost < 0 || weights.Success < 0 || weights.TTFT < 0 || weights.Latency < 0 {
		return fmt.Errorf("%w: score weights must be non-negative", ErrAccountMonitorInvalidScoreWeights)
	}
	if weights.Cost+weights.Success+weights.TTFT+weights.Latency != 100 {
		return fmt.Errorf("%w: score weights must sum to 100", ErrAccountMonitorInvalidScoreWeights)
	}
	return nil
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorService(Global|RejectsInvalidGlobal|UpdatesAndResetsGroup|RejectsScoreWeights)' -count=1`
Expected: PASS.

- [ ] **Step 7: Commit Task 1**

Run:

```bash
git add upstream/sub2api/backend/migrations/223_account_monitor_global_score_weights.sql \
  upstream/sub2api/backend/migrations/account_monitor_global_score_weights_migration_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_types.go \
  upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/repository/account_monitor_repo.go \
  upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "feat: persist global account monitor score weights"
```

---

### Task 2: Backend Global Ranking And Admin API

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/account_monitor_routes_test.go`

**Interfaces:**
- Consumes: Task 1 service methods and repository methods.
- Produces HTTP handlers: `GetGlobalScoreWeights`, `UpdateGlobalScoreWeights`, `ResetGlobalScoreWeights`.
- Produces routes: `GET /admin/account-monitors/global-score-weights`, `PUT /admin/account-monitors/global-score-weights`, `DELETE /admin/account-monitors/global-score-weights`.
- Updates internal helper signature: `projectGlobalWindowQuality(..., weights AccountMonitorScoreWeights) []AccountMonitorAccount`.

- [ ] **Step 1: Write ranking test before implementation**

In `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`, add a ListWindow test showing global weights change ordering while group weights remain isolated.

```go
func TestAccountMonitorListWindowUsesPersistedGlobalScoreWeights(t *testing.T) {
	rate := 1.0
	accounts := []Account{
		{ID: 1, Name: "cheap", Status: "active", Schedulable: true, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, RateMultiplier: &rate},
		{ID: 2, Name: "fast", Status: "active", Schedulable: true, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, RateMultiplier: &rate},
	}
	ttftCheap := 4000.0
	ttftFast := 500.0
	latency := 1000.0
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: AccountMonitorDefaultIntervalSeconds},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{},
		aggregates: map[int64]AccountMonitorAggregate{
			1: {SampleCount: 5, SuccessSampleCount: 5, TTFTSampleCount: 5, LatencySampleCount: 5, SuccessRate: 1, TTFTP50MS: &ttftCheap, LatencyP95MS: &latency, LastCheckedAt: ptrTime(time.Now().UTC())},
			2: {SampleCount: 5, SuccessSampleCount: 5, TTFTSampleCount: 5, LatencySampleCount: 5, SuccessRate: 1, TTFTP50MS: &ttftFast, LatencyP95MS: &latency, LastCheckedAt: ptrTime(time.Now().UTC())},
		},
		groups: []AccountMonitorGroup{{ID: 7, Name: "Group", RateMultiplier: 1, ScoreWeights: AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20}}},
		globalWeights: AccountMonitorScoreWeights{Cost: 0, Success: 0, TTFT: 100, Latency: 0},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatalf("ListWindow() error = %v", err)
	}
	if got := []int64{page.Accounts[0].AccountID, page.Accounts[1].AccountID}; !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Fatalf("global ranking account ids = %v", got)
	}
	if page.SchemaVersion != AccountMonitorSchemaVersion {
		t.Fatalf("schema version changed: %d", page.SchemaVersion)
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/service -run TestAccountMonitorListWindowUsesPersistedGlobalScoreWeights -count=1`
Expected: FAIL because ListWindow still uses `DefaultAccountMonitorScoreWeights`.

- [ ] **Step 2: Implement global ranking read path**

In `ListWindow`, after settings load and before row projection, call `GetGlobalScoreWeights(ctx)`. Convert the response to four weights plus default thresholds for scoring:

```go
globalWeightsResponse, err := s.GetGlobalScoreWeights(ctx)
if err != nil {
	return AccountMonitorPage{}, err
}
globalWeights := normalizeAccountMonitorScoreWeights(AccountMonitorScoreWeights{
	Cost: globalWeightsResponse.Cost, Success: globalWeightsResponse.Success,
	TTFT: globalWeightsResponse.TTFT, Latency: globalWeightsResponse.Latency,
})
```

Pass `globalWeights` into `projectGlobalWindowQuality` and replace the existing `DefaultAccountMonitorScoreWeights` argument at the global score calculation site.

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorListWindowUsesPersistedGlobalScoreWeights|TestAccountMonitorServiceGlobalScoreWeightsDefaultAndErrors' -count=1`
Expected: PASS.

- [ ] **Step 3: Write handler tests before implementation**

In `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`, add tests for global GET default, PUT ignoring threshold fields, DELETE default, invalid sum, and database errors not being reported as `400`.

```go
func TestAccountMonitorHandlerGlobalScoreWeightsCRUD(t *testing.T) {
	repo := &accountMonitorHandlerRepoStub{globalWeightsErr: sql.ErrNoRows}
	h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	})
	router.GET("/global-score-weights", h.GetGlobalScoreWeights)
	router.PUT("/global-score-weights", h.UpdateGlobalScoreWeights)
	router.DELETE("/global-score-weights", h.ResetGlobalScoreWeights)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/global-score-weights", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"is_default":true`) {
		t.Fatalf("GET response = %d %s", get.Code, get.Body.String())
	}

	body := strings.NewReader(`{"cost":25,"success":35,"ttft":20,"latency":20,"ttft_target_ms":1}`)
	put := httptest.NewRecorder()
	router.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/global-score-weights", body))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT response = %d %s", put.Code, put.Body.String())
	}
	if saved := repo.savedGlobalWeights[len(repo.savedGlobalWeights)-1]; saved.TTFTTargetMS != 0 {
		t.Fatalf("threshold field leaked into global save: %#v", saved)
	}

	del := httptest.NewRecorder()
	router.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/global-score-weights", nil))
	if del.Code != http.StatusOK || !strings.Contains(del.Body.String(), `"is_default":true`) {
		t.Fatalf("DELETE response = %d %s", del.Code, del.Body.String())
	}
}

func TestAccountMonitorHandlerGlobalScoreWeightsStorageErrorsAreNotBadRequest(t *testing.T) {
	for _, tt := range []struct {
		name string
		errField string
		method string
		body string
	}{
		{name: "get load error", errField: "load", method: http.MethodGet},
		{name: "put save error", errField: "save", method: http.MethodPut, body: `{"cost":25,"success":35,"ttft":20,"latency":20}`},
		{name: "delete reset error", errField: "reset", method: http.MethodDelete},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountMonitorHandlerRepoStub{}
			switch tt.errField {
			case "load":
				repo.globalWeightsErr = errors.New("database unavailable")
			case "save":
				repo.globalWeightsSaveErr = errors.New("database unavailable")
			case "reset":
				repo.globalWeightsResetErr = errors.New("database unavailable")
			}
			h := NewAccountMonitorHandler(service.NewAccountMonitorService(repo, nil, nil, nil, nil), nil, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
			})
			router.GET("/global-score-weights", h.GetGlobalScoreWeights)
			router.PUT("/global-score-weights", h.UpdateGlobalScoreWeights)
			router.DELETE("/global-score-weights", h.ResetGlobalScoreWeights)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, "/global-score-weights", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusBadRequest {
				t.Fatalf("storage error returned 400: %s", recorder.Body.String())
			}
		})
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandlerGlobalScoreWeights(CRUD|StorageErrorsAreNotBadRequest)' -count=1`
Expected: FAIL because handler methods do not exist.

- [ ] **Step 4: Implement handler methods**

In `account_monitor_handler.go`, add the standard library `errors` import if it is not already present, then add a four-field request type:

```go
type accountMonitorGlobalScoreWeightsRequest struct {
	Cost    int `json:"cost"`
	Success int `json:"success"`
	TTFT    int `json:"ttft"`
	Latency int `json:"latency"`
}
```

Add handlers:

```go
func (h *AccountMonitorHandler) GetGlobalScoreWeights(c *gin.Context) {
	weights, err := h.monitorService.GetGlobalScoreWeights(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) UpdateGlobalScoreWeights(c *gin.Context) {
	var req accountMonitorGlobalScoreWeightsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	weights, err := h.monitorService.UpdateGlobalScoreWeights(c.Request.Context(), subject.UserID, service.AccountMonitorScoreWeights{
		Cost: req.Cost, Success: req.Success, TTFT: req.TTFT, Latency: req.Latency,
	})
	if err != nil {
		if errors.Is(err, service.ErrAccountMonitorInvalidScoreWeights) {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_SCORE_WEIGHTS", err.Error()))
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) ResetGlobalScoreWeights(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	weights, err := h.monitorService.ResetGlobalScoreWeights(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandler(GlobalScoreWeights|GroupScoreWeights)' -count=1`
Expected: PASS.

- [ ] **Step 5: Write route step-up test before implementation**

In `upstream/sub2api/backend/internal/server/routes/account_monitor_routes_test.go`, add assertions that global GET is reachable without invoking the step-up middleware while PUT/DELETE invoke it.

```go
func TestAccountMonitorGlobalScoreWeightRoutesUseStepUpForWritesOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &accountMonitorRouteRepoStub{}
	monitorService := service.NewAccountMonitorService(repo, &accountMonitorRouteAccountRepoStub{}, nil, nil, nil)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		AccountMonitor: adminhandler.NewAccountMonitorHandler(monitorService, nil, nil, nil),
	}}
	router := gin.New()
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer admin" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
		}
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	var stepUpCalls int
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) {
		stepUpCalls++
		c.Next()
	})
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	get := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-monitors/global-score-weights", nil)
	getRequest.Header.Set("Authorization", "Bearer admin")
	router.ServeHTTP(get, getRequest)
	if get.Code == http.StatusNotFound || stepUpCalls != 0 {
		t.Fatalf("GET status=%d stepUpCalls=%d", get.Code, stepUpCalls)
	}

	put := httptest.NewRecorder()
	putRequest := httptest.NewRequest(http.MethodPut, "/api/v1/admin/account-monitors/global-score-weights", strings.NewReader(`{"cost":15,"success":45,"ttft":20,"latency":20}`))
	putRequest.Header.Set("Authorization", "Bearer admin")
	putRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(put, putRequest)
	if stepUpCalls != 1 {
		t.Fatalf("PUT stepUpCalls=%d, want 1", stepUpCalls)
	}

	del := httptest.NewRecorder()
	delRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/account-monitors/global-score-weights", nil)
	delRequest.Header.Set("Authorization", "Bearer admin")
	router.ServeHTTP(del, delRequest)
	if stepUpCalls != 2 {
		t.Fatalf("DELETE stepUpCalls=%d, want 2", stepUpCalls)
	}
}
```

Run: `cd upstream/sub2api/backend && go test ./internal/server/routes -run TestAccountMonitorGlobalScoreWeightRoutesUseStepUpForWritesOnly -count=1`
Expected: FAIL because routes are missing.

- [ ] **Step 6: Register routes**

In `registerAccountMonitorRoutes`, add these routes before `/:account_id/history` so `global-score-weights` cannot be parsed as `account_id`:

```go
monitors.GET("/global-score-weights", h.Admin.AccountMonitor.GetGlobalScoreWeights)
monitors.PUT("/global-score-weights", gin.HandlerFunc(stepUpAuth), h.Admin.AccountMonitor.UpdateGlobalScoreWeights)
monitors.DELETE("/global-score-weights", gin.HandlerFunc(stepUpAuth), h.Admin.AccountMonitor.ResetGlobalScoreWeights)
```

Run: `cd upstream/sub2api/backend && go test ./internal/server/routes -run 'TestAccountMonitor(GlobalScoreWeightRoutesUseStepUpForWritesOnly|Routes)' -count=1`
Expected: PASS.

- [ ] **Step 7: Backend targeted regression**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'AccountMonitor.*(GlobalScoreWeights|ScoreThresholds|GroupScoreWeights)' -count=1
go test ./internal/repository -run 'AccountMonitorRepository.*ScoreWeights' -count=1
go test ./internal/repository -run TestMigrationsSchema -count=1
go test ./internal/service -run 'AccountMonitor.*(GlobalScoreWeights|ScoreWeights|ListWindowUsesPersistedGlobalScoreWeights|QualityScore)' -count=1
go test ./internal/handler/admin -run 'AccountMonitorHandler.*ScoreWeights' -count=1
go test ./internal/server/routes -run 'AccountMonitor.*Routes' -count=1
```

Expected: all PASS. `TestMigrationsSchema` uses PostgreSQL/testcontainers; when the environment cannot run containers or cannot reach Docker, record the exact command, error output, and environment limitation in the verification report so the root gate can rerun it in a capable environment. Do not replace it with only SQL string tests when the environment supports PostgreSQL/testcontainers.

- [ ] **Step 8: Commit Task 2**

Run:

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go \
  upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go \
  upstream/sub2api/backend/internal/server/routes/admin.go \
  upstream/sub2api/backend/internal/server/routes/account_monitor_routes_test.go
git commit -m "feat: expose global account monitor score weights"
```

---

### Task 3: Frontend Global Score Weight Interaction

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: backend API paths from Task 2.
- Produces type: `AccountMonitorGlobalScoreWeights`.
- Produces API methods: `getGlobalScoreWeights()`, `updateGlobalScoreWeights(weights)`, `resetGlobalScoreWeights()`.
- Updates component prop: `mode?: 'group' | 'global'`; default is `'group'`.
- Global save event payload contains only `cost`, `success`, `ttft`, `latency`.

- [ ] **Step 1: Record API client contract before implementation**

Use `AccountMonitorView.spec.ts` mocks in Step 5 to assert the view calls the new client methods, and manually inspect `accountMonitor.ts` in Step 4 to verify exact endpoint paths.

Expected client signatures:

```ts
export interface AccountMonitorGlobalScoreWeights {
  cost: number
  success: number
  ttft: number
  latency: number
  updated_by?: number
  updated_at?: string | null
  is_default?: boolean
}

export type AccountMonitorFourScoreWeights = Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency'>
```

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand`
Expected: current tests PASS before client edits; later view tests will fail until implementation.

- [ ] **Step 2: Implement API client functions**

In `accountMonitor.ts`, add the interface/type above and functions:

```ts
export async function getGlobalScoreWeights(): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.get<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights')
  return data
}

export async function updateGlobalScoreWeights(weights: AccountMonitorFourScoreWeights): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.put<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights', weights)
  return data
}

export async function resetGlobalScoreWeights(): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.delete<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights')
  return data
}
```

Add these functions to `accountMonitorAPI`.

Run: `cd upstream/sub2api/frontend && pnpm typecheck`
Expected: PASS if type exports are consistent.

- [ ] **Step 3: Write dialog mode tests before implementation**

Update `AccountMonitorGroupScoreDialog.spec.ts`:

```ts
it('renders only four weights and emits only four fields in global mode', async () => {
  const wrapper = mount(AccountMonitorGroupScoreDialog, {
    props: { show: true, mode: 'global', groupId: 0, weights },
    global: { stubs: { BaseDialog: { template: '<div><h2>{{ title }}</h2><slot /><slot name="footer" /></div>', props: ['title'] } } },
  })

  expect(wrapper.text()).toContain('全局评分规则')
  expect(wrapper.text()).not.toContain('服务指标评分范围')
  expect(wrapper.findAll('input')).toHaveLength(4)

  await wrapper.get('[data-test="save-score-weights"]').trigger('click')
  expect(wrapper.emitted('save')?.[0]).toEqual([{ cost: 15, success: 45, ttft: 20, latency: 20 }])
})

it('keeps group mode thresholds and threshold validation unchanged', async () => {
  const wrapper = mount(AccountMonitorGroupScoreDialog, {
    props: { show: true, mode: 'group', groupId: 3, groupName: 'Production', weights },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
  })

  expect(wrapper.text()).toContain('服务指标评分范围')
  expect(wrapper.findAll('input')).toHaveLength(8)
})
```

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts --runInBand`
Expected: FAIL because mode is missing.

- [ ] **Step 4: Implement dialog mode**

In `AccountMonitorGroupScoreDialog.vue`:

- Add `mode?: 'group' | 'global'` with default `'group'`.
- Make title computed:

```ts
const dialogTitle = computed(() => props.mode === 'global'
  ? '全局评分规则'
  : `分组评分规则${props.groupName ? ` · ${props.groupName}` : ''}`)
```

- Render threshold block only with `v-if="mode === 'group'"`.
- Treat `thresholdsValid` as `true` in global mode.
- In `save()`, emit only four fields for global mode:

```ts
if (props.mode === 'global') {
  emit('save', { cost: Number(draft.cost), success: Number(draft.success), ttft: Number(draft.ttft), latency: Number(draft.latency) })
  return
}
```

- Keep group default behavior unchanged.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts --runInBand`
Expected: PASS.

- [ ] **Step 5: Write view tests before implementation**

In `AccountMonitorView.spec.ts`, add hoisted mocks:

```ts
getGlobalScoreWeights: vi.fn(),
updateGlobalScoreWeights: vi.fn(),
resetGlobalScoreWeights: vi.fn(),
```

Expose them in the mocked `adminAPI.accountMonitor`.

Add tests:

```ts
it('shows global score settings only on the all-site view and loads weights when opened', async () => {
  getGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
  const wrapper = mountView()
  await flushPromises()

  expect(wrapper.get('[data-test="edit-global-score-weights"]').exists()).toBe(true)
  await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
  await flushPromises()

  expect(getGlobalScoreWeights).toHaveBeenCalledTimes(1)
  const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
  expect(dialog.props('mode')).toBe('global')
  expect(dialog.props('weights')).toMatchObject({ cost: 15, success: 45, ttft: 20, latency: 20 })

  await wrapper.get('[data-test="group-tab-3"]').trigger('click')
  await flushPromises()
  expect(wrapper.find('[data-test="edit-global-score-weights"]').exists()).toBe(false)
  expect(wrapper.find('[data-test="edit-group-score-weights"]').exists()).toBe(true)
})

it('saves and resets global score weights with four-field payloads and reloads active range', async () => {
  getGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
  updateGlobalScoreWeights.mockResolvedValue({ cost: 30, success: 30, ttft: 20, latency: 20, is_default: false })
  resetGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
  const wrapper = mountView()
  await flushPromises()
  await selectRange(wrapper, '7d')
  await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
  await flushPromises()

  list.mockClear()
  wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
    cost: 30, success: 30, ttft: 20, latency: 20,
    ttft_target_ms: 1, ttft_limit_ms: 2, latency_target_ms: 3, latency_limit_ms: 4,
  })
  await flushPromises()

  expect(updateGlobalScoreWeights).toHaveBeenCalledWith({ cost: 30, success: 30, ttft: 20, latency: 20 })
  expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(false)

  await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
  await flushPromises()
  list.mockClear()
  wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
  await flushPromises()

  expect(resetGlobalScoreWeights).toHaveBeenCalledTimes(1)
  expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
})
```

Add a reload-failure test matching the existing group failure tests, expecting the global dialog to remain open and `showSuccess` not to be called.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand`
Expected: FAIL because view global entry and handlers are missing.

- [ ] **Step 6: Implement global view interaction**

In `AccountMonitorView.vue`:

- Add state:

```ts
const globalScoreWeights = ref<AccountMonitorFourScoreWeights>({ cost: 15, success: 45, ttft: 20, latency: 20 })
const scoreDialogMode = ref<'group' | 'global'>('group')
```

- Add an all-site summary/action band shown when `!activeGroup`, with a compact icon button using existing `Icon name="edit"` and `data-test="edit-global-score-weights"`.
- Change the dialog mount from `v-if="activeGroup"` to `v-if="scoreDialogMode === 'global' || activeGroup"`, pass `:mode="scoreDialogMode"`, `:group-id="activeGroup?.id ?? 0"`, `:group-name="activeGroup?.name ?? ''"`, and `:weights="scoreDialogMode === 'global' ? globalScoreWeights : activeGroup!.score_weights"`.
- Implement:

```ts
async function openGlobalScoreDialog(): Promise<void> {
  if (savingScoreWeights.value) return
  scoreDialogMode.value = 'global'
  scoreWeightsError.value = null
  try {
    const weights = await adminAPI.accountMonitor.getGlobalScoreWeights()
    globalScoreWeights.value = { cost: weights.cost, success: weights.success, ttft: weights.ttft, latency: weights.latency }
    showScoreDialog.value = true
  } catch (reason: unknown) {
    scoreWeightsError.value = extractApiErrorMessage(reason, '全局评分权重加载失败')
    appStore.showError(scoreWeightsError.value)
  }
}
```

- Update `openScoreDialog()` to set `scoreDialogMode.value = 'group'`.
- Update `saveScoreWeights(weights)` to branch on `scoreDialogMode.value`. Global mode must call `updateGlobalScoreWeights({ cost, success, ttft, latency })`, reload active range, and show success text `全局评分权重已更新`.
- Update `resetScoreWeights()` to branch on `scoreDialogMode.value`. Global mode must call `resetGlobalScoreWeights()`, reload active range, and show success text `全局评分权重已恢复默认`.
- Keep the existing group save/reset code path and messages unchanged.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand`
Expected: PASS.

- [ ] **Step 7: Frontend targeted regression**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand
pnpm typecheck
```

Expected: all PASS.

- [ ] **Step 8: Commit Task 3**

Run:

```bash
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts \
  upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
git commit -m "feat: add global score weight controls"
```

---

### Task 4: Verification, Guards, Refresh Gate, And Handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-08-14-t07-global-score-weights-review.md`
- No runtime files should change in this task except conflict resolutions required by the REFRESH_REQUIRED gate.

**Interfaces:**
- Consumes: all prior commits.
- Produces: final verification report and `READY_FOR_ROOT_REVIEW` handoff.

- [ ] **Step 1: Run backend focused tests**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'AccountMonitor.*(GlobalScoreWeights|ScoreThresholds)' -count=1
go test ./internal/repository -run 'AccountMonitorRepository.*ScoreWeights' -count=1
go test ./internal/repository -run TestMigrationsSchema -count=1
go test ./internal/service -run 'AccountMonitor.*(GlobalScoreWeights|ScoreWeights|ListWindowUsesPersistedGlobalScoreWeights|QualityScore)' -count=1
go test ./internal/handler/admin -run 'AccountMonitorHandler.*ScoreWeights' -count=1
go test ./internal/server/routes -run 'AccountMonitor.*Routes' -count=1
```

Expected: all PASS.

- [ ] **Step 2: Run frontend focused tests and typecheck**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand
pnpm typecheck
```

Expected: all PASS.

- [ ] **Step 3: Run build checks**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes
cd ../frontend
pnpm build
```

Expected: all PASS. If `pnpm build` fails only because of unrelated pre-existing errors outside touched files, record the exact failing files and run the targeted Vitest/typecheck commands above again after confirming touched files typecheck.

- [ ] **Step 4: Run diff and scope guards**

Run from repo root:

```bash
git diff --check efa0ef54cb432e784796add380727bc5366d2d06..HEAD
git diff --name-only efa0ef54cb432e784796add380727bc5366d2d06...HEAD
rg -n "ttft_target_ms|ttft_limit_ms|latency_target_ms|latency_limit_ms" upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue
rg -n "AccountMonitorSchemaVersion\\s*=|schema_version|external-primary|github/workflows|SchedulerScoreWeights|Top-K|quota" upstream/sub2api docs .github
git status --short
```

Expected:

- `git diff --check efa0ef54cb432e784796add380727bc5366d2d06..HEAD` has no output.
- Changed files are limited to the T07 spec, T07 plan, T07 report, account monitor backend/frontend files, tests, and one migration.
- Threshold fields appear only in existing group code, group tests, or tests proving global paths ignore them; they do not appear in the global migration table or global request type.
- `AccountMonitorSchemaVersion` value remains unchanged.
- No `.github/workflows`, `external-primary`, scheduler, Top-K, quota headroom, root queue, or project progress files are changed.
- Worktree is clean after committing.

- [ ] **Step 5: Perform REFRESH_REQUIRED gate before final whole-branch review**

Fetch the latest root main and integrate it into the T07 branch without editing global governance files locally.

```bash
git fetch origin main
git merge --no-ff origin/main
```

If `origin/main` is unavailable in this environment, use the local root `main` only after verifying it points to `fc44bde10` or newer:

```bash
git rev-parse main
git merge --no-ff main
```

Conflict policy:

- Preserve root controller changes in `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, and other global governance files.
- Resolve only conflicts caused by T07-owned files.
- If migration number `223_account_monitor_global_score_weights.sql` collides with newer main, rename to the next unused integer and update this plan/report evidence before continuing.

Run the focused backend/frontend checks from Steps 1 and 2 after the merge.

- [ ] **Step 6: Request independent reviews**

Follow `AGENTS.md` execution discipline:

- Each implementation task must have been performed by a fresh implementer subagent.
- After each task, request an independent task reviewer and resolve findings before the next task.
- After the REFRESH_REQUIRED gate and verification, request a final whole-branch reviewer.

Reviewer brief for each task:

```text
Review T07 global score weights only. Confirm the implementation changes only four global weights, leaves group score thresholds and semantics intact, keeps AccountMonitorSchemaVersion/projection DTO unchanged, enforces step-up for PUT/DELETE, propagates storage errors, and does not touch scheduler/Top-K/quota/external-primary/GitHub Actions/root governance files.
```

- [ ] **Step 7: Write final report**

Create `docs/superpowers/reports/2026-08-14-t07-global-score-weights-review.md`. Populate it from actual command output and `git log --oneline efa0ef54cb432e784796add380727bc5366d2d06..HEAD`; do not leave placeholder tokens in the committed report.

```markdown
# T07 Global Score Weights Review

## Scope

- Implemented four global score weights only: cost, success, TTFT, latency.
- Did not implement global threshold persistence or editing.
- Group score weights and thresholds remain unchanged.
- Account monitor projection DTO and AccountMonitorSchemaVersion remain unchanged.

## Commits

- List every T07 commit SHA and subject from `git log --oneline efa0ef54cb432e784796add380727bc5366d2d06..HEAD`.

## Verification

- Record each command from Steps 1-4 and its result.

## Scope Guards

- `git diff --check efa0ef54cb432e784796add380727bc5366d2d06..HEAD`: PASS
- `git diff --name-only efa0ef54cb432e784796add380727bc5366d2d06...HEAD`: reviewed, only T07 files.
- Threshold guard: PASS
- Scheduler/external-primary/GitHub Actions/root governance guard: PASS

## REFRESH_REQUIRED

- Record the exact main SHA integrated before final whole-branch review.
- Conflict summary names exact conflict files, or says `none` when there were no conflicts.
- Root governance files preserved.

## Review Findings

- Summarize each task reviewer result and final whole-branch reviewer result.

## Release Precheck And Rollback Notes

- Migration is expand-only and creates only `account_monitor_global_score_weights`.
- App rollback leaves the singleton table unused by old code.
- Stop or roll back if global and group weights overwrite each other, PUT/DELETE bypass step-up, storage errors fall back silently, global thresholds appear, or account monitor ranking diverges from backend projection.

## Status

READY_FOR_ROOT_REVIEW
```

- [ ] **Step 8: Commit verification report**

Run:

```bash
git add docs/superpowers/reports/2026-08-14-t07-global-score-weights-review.md
git commit -m "docs: record T07 verification"
git status --short --branch
```

Expected: branch clean and ahead of baseline.

---

## Plan Self-Review

**Spec coverage:** Covered global entry, independent singleton persistence/API, reuse of group dialog, defaults `15/45/20/20`, immediate projection reload after save/reset, refresh persistence, group isolation, step-up permissions, storage failure semantics, expand-only migration, testing, release precheck, rollback, and REFRESH_REQUIRED integration.

**Placeholder scan:** No placeholder tokens, no unresolved implementation gaps, no copy-by-reference task instructions, and no plan step depends on undefined paths or signatures.

**Type consistency:** Backend method names are consistent across service interface, repository, handler, and tests. Frontend `AccountMonitorFourScoreWeights` is the only global save payload, while `AccountMonitorScoreWeights` remains the group-compatible eight-field type.

**Range consistency:** The plan never adds threshold columns to the global table, never adds threshold fields to the global request type, keeps group thresholds in group mode, keeps `AccountMonitorSchemaVersion` unchanged, and includes explicit guards for scheduler, Top-K, quota, GitHub Actions, `external-primary`, root queue, and project ledger.
