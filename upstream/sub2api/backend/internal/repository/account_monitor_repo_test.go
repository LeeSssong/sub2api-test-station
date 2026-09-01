package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountMonitorRepositoryHealthHistoryErrorRollsBackWithoutEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepositoryWithOutbox(db)
	at := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM account_monitor_results").WithArgs(int64(7)).WillReturnError(errors.New("history unavailable"))
	mock.ExpectRollback()
	err = repo.InsertResult(context.Background(), service.AccountMonitorProbeResult{AccountID: 7, ModelID: "probe", Status: "success", CheckedAt: at}, "7d4b56d2-8223-4f77-8d22-f6a93d818980")
	if err == nil {
		t.Fatal("expected history query error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryEnsuresProbeBucketTerminalIdempotently(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	bucket := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	mock.ExpectExec(`(?s)INSERT INTO account_monitor_bucket_terminals.*BOOL_OR\(r\.status = 'success'\).*HAVING COUNT\(r\.account_id\) > 0.*ON CONFLICT \(group_id, bucket_start\) DO UPDATE.*EXCLUDED\.status = 'success'`).
		WithArgs(int64(7), bucket, "7d4b56d2-8223-4f77-8d22-f6a93d818980", sqlmock.AnyArg(), "5m0s").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.EnsureProbeBucketTerminal(context.Background(), 7, []int64{11, 12}, bucket, "7d4b56d2-8223-4f77-8d22-f6a93d818980"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryProjectMonitorV4UsesLogicalRequestProjection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	projector, ok := repo.(service.AccountMonitorHybridProjectionRepository)
	if !ok {
		t.Fatal("account monitor repository must implement hybrid projection")
	}
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	updatedAt := end.Add(-time.Minute)
	mock.ExpectQuery(`(?s)WITH scopes AS.*groups AS.*buckets AS.*raw_usage_candidates AS.*input_tokens.*cache_creation_tokens.*real_events AS.*PARTITION BY rc\.group_id, rc\.request_key.*selected_events AS.*missing_probe_counts AS.*has_real IS NOT TRUE AND probe_missing.*cache_hit_rate.*request_count`).
		WithArgs(start, end, "5m0s", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count",
			"probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count",
			"latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
		}).AddRow(7, 75.0, 4, 3, 2, 1, 2, 2, 0, 120.0, 2, 800.0, 3, 0.4, updatedAt, true))

	projection, err := projector.ProjectMonitorV4Groups(context.Background(), []service.MonitorV2GroupAccountScope{{GroupID: 7, AccountID: 11}}, start, end, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	row := projection[7]
	if row.RequestCount != 4 || row.SuccessCount != 3 || row.RealRequestCount != 2 || row.RealSuccessCount != 1 {
		t.Fatalf("request projection = %#v", row)
	}
	if row.ProbeFallbackBucketCount != 2 || row.ProbeFallbackRequestCount != 2 || row.SuccessRate == nil || *row.SuccessRate != 75 {
		t.Fatalf("probe projection = %#v", row)
	}
	if row.TTFTP95MS == nil || *row.TTFTP95MS != 120 || row.TTFTSampleCount != 2 || row.LatencyP95MS == nil || *row.LatencyP95MS != 800 || row.LatencySampleCount != 3 {
		t.Fatalf("P95 projection = %#v", row)
	}
	if row.CacheHitRate == nil || *row.CacheHitRate != 0.4 {
		t.Fatalf("cache hit rate projection = %#v", row)
	}
	if *row.CacheHitRate < 0 || *row.CacheHitRate > 1 {
		t.Fatalf("cache hit rate must be a 0..1 ratio, got %v", *row.CacheHitRate)
	}
	if !row.CurrentOperational || row.SourceUpdatedAt == nil || !row.SourceUpdatedAt.Equal(updatedAt) {
		t.Fatalf("status projection = %#v", row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryGroupRealRequestProjectionDeduplicatesAcrossAccountsByFinalEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	groupRepo, ok := repo.(service.AccountMonitorGroupRealRequestRepository)
	require.True(t, ok)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mock.ExpectQuery(`(?s)WITH usage_events AS.*request_id AS request_id.*usage_completeness.*error_events AS.*NULL::text AS request_id.*PARTITION BY e\.group_id, e\.request_key ORDER BY e\.created_at DESC, e\.successful DESC`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "account_id", "request_count", "success_count", "error_count", "revenue", "account_cost", "cost_complete", "success_rate", "ttft_sample_count", "ttft_p95_ms", "latency_p95_ms", "last_observed_at",
		}).AddRow(7, 12, 1, 1, 0, 0.2, 0.1, true, 1.0, 1, 120.0, 800.0, end.Add(-time.Minute)))

	got, err := groupRepo.ListGroupRealRequestAggregates(context.Background(), []int64{7}, []int64{11, 12}, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(1), got[7][12].RequestCount)
	require.Equal(t, int64(1), got[7][12].SuccessCount)
	require.Zero(t, got[7][11].RequestCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountMonitorRepositoryProjectMonitorV4FailClosesMissingProbeBucket(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	projector, ok := repo.(service.AccountMonitorHybridProjectionGroupsRepository)
	if !ok {
		t.Fatal("account monitor repository must implement complete hybrid projection")
	}
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(10*time.Minute + 30*time.Second)
	mock.ExpectQuery(`(?s)WITH scopes AS.*groups AS.*buckets AS.*bucket_matrix AS.*selected_events AS.*COALESCE\(bm\.probe_successful, FALSE\).*WHERE bm\.has_real IS NOT TRUE\s*\), latest_selected AS`).
		WithArgs(start, end, "5m0s", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count",
			"probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count",
			"latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
		}).AddRow(7, 0.0, 1, 0, 0, 0, 1, 1, 1, nil, 0, nil, 0, nil, nil, false))

	projection, err := projector.ProjectMonitorV4GroupsForGroups(
		context.Background(), []int64{7}, nil, start, end, 5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	row := projection[7]
	if row.RequestCount != 1 || row.SuccessCount != 0 || row.SuccessRate == nil || *row.SuccessRate != 0 {
		t.Fatalf("missing-probe projection = %#v, want one failed synthetic request", row)
	}
	if row.ProbeFallbackBucketCount != 1 || row.ProbeFallbackRequestCount != 1 {
		t.Fatalf("missing-probe counters = %#v, want one synthetic bucket", row)
	}
	if row.MissingProbeTerminalCount != 1 {
		t.Fatalf("missing-probe terminal count = %d, want 1", row.MissingProbeTerminalCount)
	}
	if row.TTFTP95MS != nil || row.LatencyP95MS != nil || row.CacheHitRate != nil || row.CurrentOperational {
		t.Fatalf("missing-probe timing/status = %#v", row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryProjectMonitorV4ConstructsGroupMatrixWithoutAccountScopes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	projector, ok := repo.(service.AccountMonitorHybridProjectionGroupsRepository)
	if !ok {
		t.Fatal("account monitor repository must implement complete hybrid projection")
	}
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(5*time.Minute + 10*time.Second)
	mock.ExpectQuery(`(?s)WITH scopes AS.*SELECT unnest\(\$6::bigint\[\]\).*CROSS JOIN buckets.*LEFT JOIN real_buckets`).
		WithArgs(start, end, "5m0s", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count",
			"probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count",
			"latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
		}).AddRow(99, 0.0, 1, 0, 0, 0, 1, 1, 1, nil, 0, nil, 0, nil, nil, false))

	projection, err := projector.ProjectMonitorV4GroupsForGroups(
		context.Background(), []int64{99}, nil, start, end, 5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	row, exists := projection[99]
	if !exists || row.RequestCount != 1 || row.SuccessRate == nil || *row.SuccessRate != 0 {
		t.Fatalf("group without scopes = %#v, want one fail-closed bucket", projection)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryLoadsAndSavesSingletonSettings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	updatedAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT interval_seconds, updated_by, updated_at\n\t\t\tFROM account_monitor_settings\n\t\t\tWHERE id = 1")).
		WillReturnRows(sqlmock.NewRows([]string{"interval_seconds", "updated_by", "updated_at"}).
			AddRow(300, 7, updatedAt))
	settings, err := repo.LoadSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.IntervalSeconds != 300 || settings.UpdatedBy != 7 || !settings.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("settings = %#v", settings)
	}

	mock.ExpectExec("INSERT INTO account_monitor_settings").WithArgs(60, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.SaveSettings(context.Background(), service.AccountMonitorSettings{IntervalSeconds: 60, UpdatedBy: 9}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryProjectMonitorV2GroupsUsesOneNativeQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	projector, ok := repo.(service.AccountMonitorGroupProbeRepository)
	if !ok {
		t.Fatal("account monitor repository must implement native Monitor V2 projection")
	}
	start := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	freshSince := end.Add(-2 * time.Minute)
	latency := 1000.4
	ttft := 420.5
	bucketLatency := 900.5
	latestCheckedAt := end.Add(-30 * time.Second)
	mock.ExpectQuery(`(?s)WITH scopes AS.*generate_series.*account_monitor_results.*checked_at >= \$.*checked_at < \$.*status = 'success'.*PERCENTILE_CONT\(0\.50\).*AVG\(.*latency_ms.*SELECT\s+bm\.group_id,\s+bm\.bucket_start,\s+bm\.bucket_status,\s+bm\.bucket_has_result,\s+bm\.bucket_latency_ms.*`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "bucket_start", "bucket_status", "bucket_has_result", "bucket_latency_ms",
			"operational_bucket_count", "total_bucket_count", "ttft_p50_ms", "average_latency_ms",
			"ttft_sample_count", "latency_sample_count", "current_status",
			"source_updated_at",
		}).
			AddRow(int64(7), start, "operational", true, bucketLatency, 1, 1, ttft, latency, 3, 2, "operational", latestCheckedAt).
			AddRow(int64(8), start, "unavailable", true, nil, 0, 1, nil, nil, 0, 0, "unavailable", latestCheckedAt).
			AddRow(int64(9), start, "unavailable", false, nil, 0, 1, nil, nil, 0, 0, "unavailable", nil))

	projection, err := projector.ProjectMonitorV2Groups(
		context.Background(),
		[]service.MonitorV2GroupAccountScope{{GroupID: 7, AccountID: 1}, {GroupID: 8, AccountID: 2}, {GroupID: 9, AccountID: 3}},
		start, end, freshSince, time.Hour,
	)
	if err != nil {
		t.Fatalf("ProjectMonitorV2Groups() error = %v", err)
	}
	if len(projection) != 3 || projection[7].Status != "operational" || projection[8].Status != "unavailable" {
		t.Fatalf("projection = %#v", projection)
	}
	if projection[7].TTFTP50MS == nil || *projection[7].TTFTP50MS != 421 || projection[7].AverageLatencyMS == nil || *projection[7].AverageLatencyMS != 1000 {
		t.Fatalf("group 7 metrics = %#v", projection[7])
	}
	if projection[7].SourceUpdatedAt == nil || !projection[7].SourceUpdatedAt.Equal(latestCheckedAt) {
		t.Fatalf("group 7 source_updated_at = %#v", projection[7].SourceUpdatedAt)
	}
	if projection[8].SourceUpdatedAt == nil {
		t.Fatalf("group 8 source_updated_at = %#v, want non-nil", projection[8].SourceUpdatedAt)
	}
	if len(projection[8].Timeline) != 1 || !projection[8].Timeline[0].HasResult || projection[8].Timeline[0].LatencyMS != nil {
		t.Fatalf("failed group timeline = %#v, want failed result evidence without latency", projection[8].Timeline)
	}
	if len(projection[9].Timeline) != 1 || projection[9].Timeline[0].HasResult {
		t.Fatalf("empty group timeline = %#v, want no result evidence", projection[9].Timeline)
	}
	if len(projection[7].Timeline) != 1 || projection[7].Timeline[0].LatencyMS == nil || *projection[7].Timeline[0].LatencyMS != 901 {
		t.Fatalf("group 7 timeline = %#v", projection[7].Timeline)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryProjectMonitorV2GroupsRejectsInvalidInputWithoutQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	projector, ok := repo.(service.AccountMonitorGroupProbeRepository)
	if !ok {
		t.Fatal("account monitor repository must implement native Monitor V2 projection")
	}
	start := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	if _, err := projector.ProjectMonitorV2Groups(context.Background(), nil, start, end, end, time.Hour); err != nil {
		t.Fatalf("empty scopes should be a no-op: %v", err)
	}
	if _, err := projector.ProjectMonitorV2Groups(context.Background(), []service.MonitorV2GroupAccountScope{{GroupID: 7, AccountID: 1}}, end, start, start, time.Hour); err == nil {
		t.Fatal("expected invalid range error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryPersistsGlobalScoreWeights(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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

func TestAccountMonitorRepositoryRejectsOverflowSizedGlobalScoreWeights(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	overflowWeight := int(^uint(0) >> 1)
	_, err = NewAccountMonitorRepository(db).SaveGlobalScoreWeights(context.Background(), 9, service.AccountMonitorScoreWeights{
		Cost: overflowWeight, Success: overflowWeight, TTFT: 101, Latency: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "between 0 and 100") {
		t.Fatalf("SaveGlobalScoreWeights() error = %v, want local range validation", err)
	}
}

func TestAccountMonitorRepositoryPersistsSanitizedProbeResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	checkedAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	ttft, latency, statusCode := 80.0, 240.0, 200

	mock.ExpectExec("INSERT INTO account_monitor_results").WithArgs(
		"7d4b56d2-8223-4f77-8d22-f6a93d818980", int64(7), "gpt-4o-mini", "success", "",
		&statusCode, &ttft, &latency, checkedAt,
	).WillReturnResult(sqlmock.NewResult(1, 1))
	err = repo.InsertResult(context.Background(), service.AccountMonitorProbeResult{
		AccountID: 7, ModelID: "gpt-4o-mini", Status: "success", HTTPStatus: &statusCode,
		TTFTMS: &ttft, LatencyMS: &latency, CheckedAt: checkedAt,
	}, "7d4b56d2-8223-4f77-8d22-f6a93d818980")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryListsLatestProbeModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	checkedAt := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT DISTINCT ON \(account_id\).*account_id, model_id, status.*FROM account_monitor_results`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "model_id", "status", "error_code", "http_status", "ttft_ms", "latency_ms", "checked_at"}).
			AddRow(7, "gpt-latest-probe", "success", "", 200, 80.0, 240.0, checkedAt))

	latest, err := repo.ListLatest(context.Background(), []int64{7})
	if err != nil {
		t.Fatal(err)
	}
	if latest[7].ModelID != "gpt-latest-probe" {
		t.Fatalf("latest model_id = %q, want gpt-latest-probe", latest[7].ModelID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryListGroupAggregatesPropagatesGroupID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	groupRepo, ok := repo.(service.AccountMonitorGroupAggregateRepository)
	if !ok {
		t.Fatal("account monitor repository must implement group aggregates")
	}
	checkedAt := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)WITH group_usage AS.*u\.group_id = \$1.*ops_error_logs.*e\.group_id = \$1.*`).
		WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "sample_count", "success_count", "error_count", "success_rate", "success_sample_count", "ttft_sample_count", "latency_sample_count", "ttft_p50_ms", "ttft_p95_ms", "latency_p50_ms", "latency_p95_ms", "last_checked_at"}).
			AddRow(int64(7), 3, 3, 0, 1.0, 3, 3, 3, 80.0, 90.0, 200.0, 220.0, checkedAt))

	aggregates, err := groupRepo.ListGroupAggregates(context.Background(), 42, []int64{7}, checkedAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if aggregates[7].SampleCount != 3 || aggregates[7].SuccessRate != 1 {
		t.Fatalf("group aggregate = %#v", aggregates[7])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryReadsAggregatesAndDeletesExpiredHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	until := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	lastChecked := time.Date(2026, 7, 25, 7, 59, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)SELECT[[:space:]]+account_id.*checked_at >= \$2.*checked_at < \$3`).WithArgs(sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(7, 4, 3, 1, 0.75, 4, 3, 2, 80.0, 120.0, 200.0, 300.0, lastChecked))
	aggregates, err := repo.ListAggregates(context.Background(), []int64{7}, since, until)
	if err != nil {
		t.Fatal(err)
	}
	row := aggregates[7]
	if row.SampleCount != 4 || row.SuccessSampleCount != 4 || row.TTFTSampleCount != 3 || row.LatencySampleCount != 2 || row.SuccessRate != 0.75 || row.TTFTP95MS == nil || *row.TTFTP95MS != 120 {
		t.Fatalf("aggregate = %#v", row)
	}

	mock.ExpectExec("DELETE FROM account_monitor_results").WithArgs(since).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM account_monitor_bucket_terminals").WithArgs(since).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.DeleteBefore(context.Background(), since); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryProbeLatencyAggregatesUseOnlySuccessfulProbes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC)

	successOnlyMetrics := `(?s)` +
		`COUNT\(\*\) FILTER \(WHERE status = 'success'\)::int,\s*` +
		`COUNT\(ttft_ms\) FILTER \(WHERE status = 'success'\)::int,\s*` +
		`COUNT\(latency_ms\) FILTER \(WHERE status = 'success'\)::int,\s*` +
		`PERCENTILE_CONT\(0\.50\).*FILTER \(WHERE status = 'success' AND ttft_ms IS NOT NULL\),\s*` +
		`PERCENTILE_CONT\(0\.95\).*FILTER \(WHERE status = 'success' AND ttft_ms IS NOT NULL\),\s*` +
		`PERCENTILE_CONT\(0\.50\).*FILTER \(WHERE status = 'success' AND latency_ms IS NOT NULL\),\s*` +
		`PERCENTILE_CONT\(0\.95\).*FILTER \(WHERE status = 'success' AND latency_ms IS NOT NULL\)`

	mock.ExpectQuery(successOnlyMetrics).
		WithArgs(sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(7, 2, 1, 1, 0.5, 1, 1, 1, 80.0, 80.0, 200.0, 200.0, until.Add(-time.Minute)))
	if _, err := repo.ListAggregates(context.Background(), []int64{7}, since, until); err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery(successOnlyMetrics).
		WithArgs(sqlmock.AnyArg(), since).
		WillReturnRows(sqlmock.NewRows([]string{
			"sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(2, 1, 1, 0.5, 1, 1, 1, 80.0, 80.0, 200.0, 200.0, until.Add(-time.Minute)))
	if _, err := repo.(*accountMonitorRepository).LoadAggregate(context.Background(), []int64{7}, since); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryListsRealRequestWindowMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	observedAt := time.Date(2026, 8, 1, 23, 59, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)WITH window_usage.*output_tokens.*FROM usage_logs.*account_id = ANY\(\$1\).*created_at >= \$2.*created_at < \$3.*SUM\(total_cost\).*PERCENTILE_CONT\(0\.50\).*first_token_ms.*PERCENTILE_CONT\(0\.95\).*duration_ms.*SUM\(output_tokens\).*duration_ms - first_token_ms`).
		WithArgs(sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "request_count", "success_count", "error_count", "base_cost", "success_rate", "ttft_sample_count", "latency_sample_count", "ttft_p50", "latency_p95", "output_rate_sample_count", "output_tokens", "generation_ms", "last_observed_at"}).
			AddRow(7, 4, 3, 1, 8.0, 0.75, 3, 4, 80.0, 300.0, 2, 240, 4000.0, observedAt))
	aggregates, err := repo.(*accountMonitorRepository).ListWindowAggregates(context.Background(), []int64{7}, since, until)
	if err != nil {
		t.Fatal(err)
	}
	got := aggregates[7]
	if got.RequestCount != 4 || got.SuccessCount != 3 || got.ErrorCount != 1 || got.BaseCost != 8 || got.SuccessRate != 0.75 || got.TTFTP50MS == nil || *got.TTFTP50MS != 80 || got.LatencyP95MS == nil || *got.LatencyP95MS != 300 || got.OutputRateTokensPerSecond == nil || *got.OutputRateTokensPerSecond != 60 || got.OutputRateSampleCount != 2 || got.LastObservedAt == nil || !got.LastObservedAt.Equal(observedAt) {
		t.Fatalf("window aggregate = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryCorrelatesOnlyMatchingWindowErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)

	// Every clause below excludes a distinct false positive: another account or
	// request, NULL request IDs, token-count calls, non-error statuses, and rows
	// before the selected window or at its exclusive upper boundary.
	mock.ExpectQuery(`(?s)EXISTS \(.*FROM ops_error_logs e.*e\.account_id = u\.account_id.*e\.request_id IS NOT NULL.*u\.request_id IS NOT NULL.*e\.request_id = u\.request_id.*e\.created_at >= \$2.*e\.created_at < \$3.*COALESCE\(e\.is_count_tokens, FALSE\) = FALSE.*COALESCE\(e\.status_code, 0\) >= 400.*FROM usage_logs u.*u\.created_at >= \$2.*u\.created_at < \$3.*COUNT\(\*\) FILTER \(WHERE has_error\)`).
		WithArgs(sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "request_count", "success_count", "error_count", "base_cost", "success_rate", "ttft_sample_count", "latency_sample_count", "ttft_p50", "latency_p95", "output_rate_sample_count", "output_tokens", "generation_ms", "last_observed_at"}).
			AddRow(7, 6, 5, 1, 8.0, 5.0/6.0, 5, 6, 80.0, 300.0, 2, 240, 4000.0, until.Add(-time.Minute)))

	aggregates, err := repo.ListWindowAggregates(context.Background(), []int64{7}, since, until)
	if err != nil {
		t.Fatal(err)
	}
	if got := aggregates[7].ErrorCount; got != 1 {
		t.Fatalf("error count = %d, want only the matching HTTP error", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryPersistsGroupScoreWeightsAndReadsNativeGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	updatedAt := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT[[:space:]]+cost_weight").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"cost_weight", "success_weight", "ttft_weight", "latency_weight", "ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms", "updated_by", "updated_at"}).
			AddRow(15, 45, 20, 20, 1200, 5000, 12000, 60000, 3, updatedAt))
	weights, err := repo.LoadGroupScoreWeights(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if weights != (service.AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20, TTFTTargetMS: 1200, TTFTLimitMS: 5000, LatencyTargetMS: 12000, LatencyLimitMS: 60000, UpdatedBy: 3, UpdatedAt: updatedAt}) {
		t.Fatalf("weights = %#v", weights)
	}

	mock.ExpectExec("INSERT INTO account_monitor_group_score_weights").WithArgs(int64(7), 20, 40, 20, 20, 1000, 5000, 10000, 60000, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.SaveGroupScoreWeights(context.Background(), 7, 9, service.AccountMonitorScoreWeights{Cost: 20, Success: 40, TTFT: 20, Latency: 20, TTFTTargetMS: 1000, TTFTLimitMS: 5000, LatencyTargetMS: 10000, LatencyLimitMS: 60000}); err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec("DELETE FROM account_monitor_group_score_weights").WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.ResetGroupScoreWeights(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT[[:space:]]+g.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "platform", "rate_multiplier", "rpm_limit", "account_count", "active_account_count", "rate_limited_account_count", "customer_visible", "require_privacy_set", "native_order", "cost_weight", "success_weight", "ttft_weight", "latency_weight", "ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms", "updated_by", "updated_at"}).
			AddRow(7, "public", "active", "openai", 1.25, 60, 4, 3, 1, true, true, 4, 15, 45, 20, 20, 1000, 5000, 10000, 60000, 0, time.Time{}).
			AddRow(8, "private", "disabled", "anthropic", 2.0, 0, 9, 2, 3, false, false, 9, 20, 40, 20, 20, 1200, 6000, 15000, 50000, 3, updatedAt))
	groups, err := repo.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].ID != 7 || !groups[0].CustomerVisible || groups[1].CustomerVisible {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[1].ScoreWeights.UpdatedBy != 3 || groups[1].ScoreWeights.Success != 40 {
		t.Fatalf("group weights = %#v", groups[1].ScoreWeights)
	}
	if groups[0].ScoreWeights.TTFTTargetMS != 1000 || groups[1].ScoreWeights.LatencyLimitMS != 50000 {
		t.Fatalf("group thresholds = %#v", groups)
	}
	if groups[0].Status != "active" || groups[0].Platform != "openai" || groups[0].RPMLimit != 60 || groups[0].AccountCount != 4 || groups[0].ActiveAccountCount != 3 || groups[0].RateLimitedAccountCount != 1 {
		t.Fatalf("native group summary = %#v", groups[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryExcludesSoftDeletedGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)

	// Monitor tabs must use the same active-group set as /admin/groups. A
	// missing g.deleted_at guard would expose soft-deleted groups here.
	mock.ExpectQuery("(?s)SELECT[[:space:]]+g\\.id.*FROM[[:space:]]+groups[[:space:]]+g.*WHERE[[:space:]]+g\\.deleted_at[[:space:]]+IS[[:space:]]+NULL").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "platform", "rate_multiplier", "rpm_limit", "account_count", "active_account_count", "rate_limited_account_count", "customer_visible", "require_privacy_set", "native_order", "cost_weight", "success_weight", "ttft_weight", "latency_weight", "ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms", "updated_by", "updated_at"}).
			AddRow(7, "active", "active", "openai", 1.25, 0, 1, 1, 0, true, false, 4, 15, 45, 20, 20, 1000, 5000, 10000, 60000, 0, time.Time{}))

	groups, err := repo.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != 7 {
		t.Fatalf("groups = %#v, want only active group 7", groups)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryKeepsHistoricalGroupEvidenceIndependentOfCurrentMembership(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 1, 7, 55, 0, 0, time.UTC)
	lastChecked := time.Date(2026, 8, 1, 7, 59, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)WITH group_usage AS.*group_errors AS.*FROM ops_error_logs.*COALESCE\(e\.is_count_tokens, FALSE\) = FALSE.*COALESCE\(e\.status_code, 0\) >= 400.*UNION ALL.*FROM group_errors`).WithArgs(int64(7), sqlmock.AnyArg(), since).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(11, 4, 3, 1, 0.75, 4, 3, 2, 80.0, 120.0, 240.0, 300.0, lastChecked))
	aggregates, err := repo.(*accountMonitorRepository).ListGroupAggregates(context.Background(), 7, []int64{11}, since)
	if err != nil {
		t.Fatal(err)
	}
	row := aggregates[11]
	if row.SampleCount != 4 || row.SuccessSampleCount != 4 || row.TTFTSampleCount != 3 || row.LatencySampleCount != 2 || row.SuccessRate != 0.75 || row.TTFTP50MS == nil || *row.TTFTP50MS != 80 || row.TTFTP95MS == nil || *row.TTFTP95MS != 120 || row.LatencyP50MS == nil || *row.LatencyP50MS != 240 || row.LatencyP95MS == nil || *row.LatencyP95MS != 300 {
		t.Fatalf("group aggregate = %#v", row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryGroupAggregateIncludesStandaloneErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 1, 7, 55, 0, 0, time.UTC)
	lastChecked := time.Date(2026, 8, 1, 7, 59, 0, 0, time.UTC)
	mock.ExpectQuery("(?s)WITH group_usage AS.*group_errors AS.*FROM ops_error_logs.*UNION ALL.*FROM group_errors").
		WithArgs(int64(7), sqlmock.AnyArg(), since).
		WillReturnRows(sqlmock.NewRows([]string{
			"sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(1, 0, 1, 0.0, 1, 0, 0, nil, nil, nil, nil, lastChecked))
	aggregate, err := repo.(*accountMonitorRepository).LoadGroupAggregate(context.Background(), 7, []int64{11}, since)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.SampleCount != 1 || aggregate.SuccessCount != 0 || aggregate.ErrorCount != 1 || aggregate.SuccessRate != 0 {
		t.Fatalf("standalone error aggregate = %#v", aggregate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryGroupAggregateDoesNotCorrelateNullRequestIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 1, 7, 55, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)group_errors AS.*u\.request_id IS NOT NULL.*e\.request_id IS NOT NULL`).
		WithArgs(int64(7), sqlmock.AnyArg(), since).
		WillReturnRows(sqlmock.NewRows([]string{
			"sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(1, 1, 0, 1.0, 1, 0, 0, nil, nil, nil, nil, since))
	if _, err := repo.(*accountMonitorRepository).LoadGroupAggregate(context.Background(), 7, []int64{11}, since); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryListsRecentTimelinesInOneBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	older := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	mock.ExpectQuery(`(?s)ROW_NUMBER\(\) OVER \(PARTITION BY account_id ORDER BY checked_at DESC, id DESC\).*WHERE ranked\.position <= \$2.*ORDER BY account_id, checked_at ASC`).
		WithArgs(sqlmock.AnyArg(), 24).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "status", "error_code", "http_status", "ttft_ms", "latency_ms", "checked_at"}).
			AddRow(7, "failed", "timeout", 504, nil, nil, older).
			AddRow(7, "success", "", 200, 800.0, 1200.0, newer).
			AddRow(8, "success", "", 200, 900.0, 1400.0, newer))

	timelines, err := repo.ListTimelines(context.Background(), []int64{7, 8}, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(timelines[7]) != 2 || timelines[7][0].Status != "failed" || !timelines[7][1].CheckedAt.Equal(newer) || len(timelines[8]) != 1 {
		t.Fatalf("timelines = %#v", timelines)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	timelineRepo, ok := NewAccountMonitorRepository(db).(service.AccountMonitorRealRequestTimelineRepository)
	if !ok {
		t.Fatal("account monitor repository must implement real request timelines")
	}
	since := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	mock.ExpectQuery(`(?s)WITH usage_events AS.*usage_completeness.*= 'complete'.*bucket_index.*ORDER BY account_id, bucket_index`).
		WithArgs(sqlmock.AnyArg(), since, until, 3600.0).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "bucket_index", "request_count", "success_count", "failure_count", "ttft_p95_ms"}).
			AddRow(7, 3, 5, 4, 1, 6200.0).
			AddRow(7, 22, 2, 2, 0, 900.0))

	timelines, err := timelineRepo.ListRealRequestTimelines(context.Background(), []int64{7, 8}, since, until, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(timelines[7]) != 24 || len(timelines[8]) != 24 {
		t.Fatalf("timeline lengths = %d/%d, want 24/24", len(timelines[7]), len(timelines[8]))
	}
	if timelines[7][0].RequestCount != 0 || timelines[7][0].TTFTP95MS != nil {
		t.Fatalf("empty bucket = %#v", timelines[7][0])
	}
	if timelines[7][3].RequestCount != 5 || timelines[7][3].FailureCount != 1 || timelines[7][3].TTFTP95MS == nil || *timelines[7][3].TTFTP95MS != 6200 {
		t.Fatalf("filled bucket = %#v", timelines[7][3])
	}
	if !timelines[7][23].EndAt.Equal(until) {
		t.Fatalf("last bucket end = %s, want %s", timelines[7][23].EndAt, until)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryRealRequestAggregateRequiresCompleteUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	aggregateRepo, ok := NewAccountMonitorRepository(db).(service.AccountMonitorRealRequestRepository)
	if !ok {
		t.Fatal("account monitor repository must implement real request aggregates")
	}
	since := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	mock.ExpectQuery(`(?s)WITH usage_events AS.*usage_completeness.*= 'complete'.*dedup AS.*PARTITION BY e\.account_id, e\.request_key`).
		WithArgs(sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "request_count", "success_count", "error_count", "revenue", "account_cost", "cost_complete", "success_rate",
			"ttft_sample_count", "ttft_p95_ms", "latency_p95_ms", "last_observed_at",
		}).AddRow(7, 1, 0, 1, 0.0, 0.0, true, 0.0, 0, nil, nil, until.Add(-time.Minute)))

	got, err := aggregateRepo.ListRealRequestAggregates(context.Background(), []int64{7}, since, until)
	if err != nil {
		t.Fatal(err)
	}
	if got[7].RequestCount != 1 || got[7].SuccessCount != 0 || got[7].ErrorCount != 1 {
		t.Fatalf("aggregate = %#v, want partial usage to remain a failure", got[7])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMonitorRepositoryGroupWindowAggregatesIncludeRequestedGroupID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	observedAt := until.Add(-time.Minute)

	mock.ExpectQuery(`(?s)WITH group_usage.*output_tokens.*FROM usage_logs u.*u\.group_id = \$1.*u\.account_id = ANY\(\$2\).*u\.created_at >= \$3.*u\.created_at < \$4.*FROM ops_error_logs e.*e\.group_id = \$1.*e\.account_id = ANY\(\$2\).*e\.created_at >= \$3.*e\.created_at < \$4.*SUM\(output_tokens\).*duration_ms - first_token_ms`).
		WithArgs(int64(77), sqlmock.AnyArg(), since, until).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "request_count", "success_count", "error_count", "base_cost", "success_rate", "ttft_sample_count", "latency_sample_count", "ttft_p50", "latency_p95", "output_rate_sample_count", "output_tokens", "generation_ms", "last_observed_at"}).
			AddRow(11, 4, 3, 1, 8.0, 0.75, 3, 4, 80.0, 300.0, 2, 240, 4000.0, observedAt))

	aggregates, err := repo.(*accountMonitorRepository).ListGroupWindowAggregates(context.Background(), 77, []int64{11}, since, until)
	if err != nil {
		t.Fatal(err)
	}
	got := aggregates[11]
	if got.RequestCount != 4 || got.SuccessCount != 3 || got.ErrorCount != 1 || got.BaseCost != 8 || got.SuccessRate != 0.75 || got.TTFTP50MS == nil || *got.TTFTP50MS != 80 || got.LatencyP95MS == nil || *got.LatencyP95MS != 300 || got.OutputRateTokensPerSecond == nil || *got.OutputRateTokensPerSecond != 60 || got.OutputRateSampleCount != 2 || got.LastObservedAt == nil || !got.LastObservedAt.Equal(observedAt) {
		t.Fatalf("group window aggregate = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
