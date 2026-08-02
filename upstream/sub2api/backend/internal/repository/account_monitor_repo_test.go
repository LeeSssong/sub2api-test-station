package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

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

func TestAccountMonitorRepositoryReadsAggregatesAndDeletesExpiredHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db)
	since := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	lastChecked := time.Date(2026, 7, 25, 7, 59, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT[[:space:]]+account_id").WithArgs(sqlmock.AnyArg(), since).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "sample_count", "success_count", "error_count", "success_rate",
			"success_sample_count", "ttft_sample_count", "latency_sample_count",
			"ttft_p50", "ttft_p95", "latency_p50", "latency_p95", "last_checked_at",
		}).AddRow(7, 4, 3, 1, 0.75, 4, 3, 2, 80.0, 120.0, 200.0, 300.0, lastChecked))
	aggregates, err := repo.ListAggregates(context.Background(), []int64{7}, since)
	if err != nil {
		t.Fatal(err)
	}
	row := aggregates[7]
	if row.SampleCount != 4 || row.SuccessSampleCount != 4 || row.TTFTSampleCount != 3 || row.LatencySampleCount != 2 || row.SuccessRate != 0.75 || row.TTFTP95MS == nil || *row.TTFTP95MS != 120 {
		t.Fatalf("aggregate = %#v", row)
	}

	mock.ExpectExec("DELETE FROM account_monitor_results").WithArgs(since).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.DeleteBefore(context.Background(), since); err != nil {
		t.Fatal(err)
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_multiplier", "customer_visible", "native_order", "cost_weight", "success_weight", "ttft_weight", "latency_weight", "ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms", "updated_by", "updated_at"}).
			AddRow(7, "public", 1.25, true, 4, 15, 45, 20, 20, 1000, 5000, 10000, 60000, 0, time.Time{}).
			AddRow(8, "private", 2.0, false, 9, 20, 40, 20, 20, 1200, 6000, 15000, 50000, 3, updatedAt))
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rate_multiplier", "customer_visible", "native_order", "cost_weight", "success_weight", "ttft_weight", "latency_weight", "ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms", "updated_by", "updated_at"}).
			AddRow(7, "active", 1.25, true, 4, 15, 45, 20, 20, 1000, 5000, 10000, 60000, 0, time.Time{}))

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
