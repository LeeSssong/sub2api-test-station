package repository

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBalanceEventRepositoryClaimSerializesScopeAndCreatesLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	leaseUntil := now.Add(45 * time.Second)
	repo := newUpstreamBalanceEventRepository(db, func() string { return "lease-test-token" })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*FOR UPDATE`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(sqlmock.NewRows(upstreamBalanceEventColumns))
	mock.ExpectQuery(`(?s)SELECT last_observed_at.*FROM ops_alert_events.*status <> \$4.*last_observed_at IS NOT NULL.*ORDER BY last_observed_at DESC.*LIMIT 1`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(sqlmock.NewRows([]string{"last_observed_at"}))
	mock.ExpectQuery(`(?s)INSERT INTO ops_alert_events.*RETURNING`).
		WithArgs(
			int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid",
			service.UpstreamBalanceNotificationStateLow, 4.5, observedAt, now,
			"lease-test-token", leaseUntil,
		).
		WillReturnRows(upstreamBalanceEventRows().AddRow(
			int64(41), int64(17), service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL,
			"https://upstream.invalid", service.UpstreamBalanceNotificationStateLow, 4.5, observedAt,
			nil, int64(1), 0, nil, "lease-test-token", leaseUntil, "", now,
		))
	mock.ExpectCommit()

	lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
		RuleID:            17,
		ScopeKey:          "https://upstream.invalid",
		NotificationState: service.UpstreamBalanceNotificationStateLow,
		ValueUSD:          4.5,
		ObservedAt:        observedAt,
		Now:               now,
		RepeatInterval:    30 * time.Minute,
		LeaseDuration:     45 * time.Second,
	})

	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(41), lease.EventID)
	require.Equal(t, int64(1), lease.Generation)
	require.Equal(t, "lease-test-token", lease.Token)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryClaimRejectsCompetingScopeLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(false))
	mock.ExpectRollback()

	lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
		RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: service.UpstreamBalanceNotificationStateZero, ValueUSD: 0,
		ObservedAt: time.Now(), Now: time.Now(), RepeatInterval: 5 * time.Minute, LeaseDuration: time.Minute,
	})

	require.NoError(t, err)
	require.False(t, claimed)
	require.Zero(t, lease)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryClaimNotDueDoesNotAllocateLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	lastDeliveredAt := now.Add(-10 * time.Minute)
	tokenCalls := 0
	repo := newUpstreamBalanceEventRepository(db, func() string {
		tokenCalls++
		return ""
	})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*FOR UPDATE`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(upstreamBalanceEventRows().AddRow(
			int64(41), int64(17), service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL,
			"https://upstream.invalid", service.UpstreamBalanceNotificationStateLow, 4.5, observedAt,
			lastDeliveredAt, int64(1), 0, nil, "", nil, "", now.Add(-time.Hour),
		))
	mock.ExpectCommit()

	lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
		RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: service.UpstreamBalanceNotificationStateLow, ValueUSD: 4.5,
		ObservedAt: observedAt, Now: now, RepeatInterval: 30 * time.Minute, LeaseDuration: time.Minute,
	})

	require.NoError(t, err)
	require.False(t, claimed)
	require.Zero(t, lease)
	require.Zero(t, tokenCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryCASRejectsStaleGenerationOrToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(`(?s)UPDATE ops_alert_events.*last_delivered_at.*delivery_generation = \$3.*delivery_lease_token = \$4`).
		WithArgs(int64(41), now, int64(3), "stale-token", service.OpsAlertStatusFiring).
		WillReturnResult(sqlmock.NewResult(0, 0))

	ok, err := repo.ConfirmDelivery(context.Background(), service.UpstreamBalanceDeliveryResult{
		EventID: 41, Generation: 3, LeaseToken: "stale-token", At: now,
	})

	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryResolveRejectsOlderObservation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	older := now.Add(-time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectExec(`(?s)UPDATE ops_alert_events.*AND \(last_observed_at IS NULL OR last_observed_at < \$3\)`).
		WithArgs(int64(17), "https://upstream.invalid", older, service.OpsAlertStatusResolved, service.UpstreamBalanceScopeTypeBaseURL, service.OpsAlertStatusFiring).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	changed, err := repo.Resolve(context.Background(), 17, "https://upstream.invalid", older)

	require.NoError(t, err)
	require.False(t, changed, "an older healthy observation must not resolve a newer firing event")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryClaimRejectsOlderObservation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	newer := now.Add(-time.Minute)
	older := newer.Add(-time.Minute)
	leaseUntil := now.Add(time.Minute)
	repo := newUpstreamBalanceEventRepository(db, func() string { return "stale-claim-token" })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*FOR UPDATE`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(upstreamBalanceEventRows().AddRow(
			int64(41), int64(17), service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL,
			"https://upstream.invalid", service.UpstreamBalanceNotificationStateLow, 4.5, newer,
			now.Add(-time.Hour), int64(2), 0, nil, "", nil, "", now.Add(-2*time.Hour),
		))
	mock.ExpectQuery(`(?s)UPDATE ops_alert_events.*last_observed_at = \$4.*AND \(\s*last_observed_at IS NULL OR last_observed_at < \$4 OR \(\s*last_observed_at = \$4 AND notification_state = \$2 AND metric_value = \$3\s*\)\s*\).*RETURNING`).
		WithArgs(int64(41), service.UpstreamBalanceNotificationStateZero, 0.0, older, true, "stale-claim-token", leaseUntil).
		WillReturnRows(upstreamBalanceEventRows())
	mock.ExpectCommit()

	lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
		RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: service.UpstreamBalanceNotificationStateZero,
		ValueUSD: 0, ObservedAt: older, Now: now, RepeatInterval: 5 * time.Minute, LeaseDuration: time.Minute,
	})

	require.NoError(t, err)
	require.False(t, claimed, "an older state transition must not replace the newer event")
	require.Zero(t, lease)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryClaimRejectsObservationAtOrBeforeResolvedWatermark(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delta time.Duration
	}{
		{name: "older", delta: -time.Minute},
		{name: "equal", delta: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
			watermark := now.Add(-time.Minute)
			repo := newUpstreamBalanceEventRepository(db, func() string {
				t.Fatalf("a stale observation must not allocate a lease token")
				return "unused"
			})

			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
				WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
				WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
			mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*FOR UPDATE`).
				WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
				WillReturnRows(sqlmock.NewRows(upstreamBalanceEventColumns))
			mock.ExpectQuery(`(?s)SELECT last_observed_at.*FROM ops_alert_events.*status <> \$4.*last_observed_at IS NOT NULL.*ORDER BY last_observed_at DESC.*LIMIT 1`).
				WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
				WillReturnRows(sqlmock.NewRows([]string{"last_observed_at"}).AddRow(watermark))
			mock.ExpectCommit()

			lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
				RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: service.UpstreamBalanceNotificationStateZero,
				ValueUSD: 0, ObservedAt: watermark.Add(tc.delta), Now: now,
				RepeatInterval: 5 * time.Minute, LeaseDuration: time.Minute,
			})

			require.NoError(t, err)
			require.False(t, claimed)
			require.Zero(t, lease)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpstreamBalanceEventRepositoryClaimAllowsObservationAfterResolvedWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	watermark := now.Add(-time.Minute)
	observedAt := now
	leaseUntil := now.Add(time.Minute)
	repo := newUpstreamBalanceEventRepository(db, func() string { return "new-lease-token" })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock($1)")).
		WithArgs(upstreamBalanceScopeLockID(17, "https://upstream.invalid")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_xact_lock"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*FOR UPDATE`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(sqlmock.NewRows(upstreamBalanceEventColumns))
	mock.ExpectQuery(`(?s)SELECT last_observed_at.*FROM ops_alert_events.*status <> \$4.*last_observed_at IS NOT NULL.*ORDER BY last_observed_at DESC.*LIMIT 1`).
		WithArgs(int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid", service.OpsAlertStatusFiring).
		WillReturnRows(sqlmock.NewRows([]string{"last_observed_at"}).AddRow(watermark))
	mock.ExpectQuery(`(?s)INSERT INTO ops_alert_events.*RETURNING`).
		WithArgs(
			int64(17), service.UpstreamBalanceScopeTypeBaseURL, "https://upstream.invalid",
			service.UpstreamBalanceNotificationStateZero, 0.0, observedAt, now,
			"new-lease-token", leaseUntil,
		).
		WillReturnRows(upstreamBalanceEventRows().AddRow(
			int64(42), int64(17), service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL,
			"https://upstream.invalid", service.UpstreamBalanceNotificationStateZero, 0.0, observedAt,
			nil, int64(1), 0, nil, "new-lease-token", leaseUntil, "", now,
		))
	mock.ExpectCommit()

	lease, claimed, err := repo.Claim(context.Background(), service.UpstreamBalanceClaimInput{
		RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: service.UpstreamBalanceNotificationStateZero,
		ValueUSD: 0, ObservedAt: observedAt, Now: now,
		RepeatInterval: 5 * time.Minute, LeaseDuration: time.Minute,
	})

	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(42), lease.EventID)
	require.Equal(t, "new-lease-token", lease.Token)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryGetCurrentReturnsLeaseStateForFinalReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(time.Minute)

	mock.ExpectQuery(`(?s)SELECT.*FROM ops_alert_events.*WHERE id = \$1`).
		WithArgs(int64(41)).
		WillReturnRows(upstreamBalanceEventRows().AddRow(
			int64(41), int64(17), service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL,
			"https://upstream.invalid", service.UpstreamBalanceNotificationStateZero, 0.0, now.Add(-time.Minute),
			nil, int64(4), 1, now.Add(time.Minute), "lease-token", leaseUntil, "", now.Add(-time.Hour),
		))

	event, err := repo.GetCurrent(context.Background(), 41)

	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, int64(4), event.DeliveryGeneration)
	require.Equal(t, "lease-token", event.DeliveryLeaseToken)
	require.Equal(t, service.UpstreamBalanceNotificationStateZero, event.NotificationState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryRecordFailureUsesLeaseCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	next := now.Add(time.Minute)

	mock.ExpectExec(`(?s)UPDATE ops_alert_events.*delivery_attempt_count = delivery_attempt_count \+ 1.*next_attempt_at = \$4.*last_delivery_error_code = \$5.*delivery_generation = \$2.*delivery_lease_token = \$3`).
		WithArgs(int64(41), int64(3), "lease-token", next, "feishu_http_error", service.OpsAlertStatusFiring).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := repo.RecordFailure(context.Background(), service.UpstreamBalanceDeliveryFailure{
		EventID: 41, Generation: 3, LeaseToken: "lease-token",
		NextAttemptAt: next, ErrorCode: "feishu_http_error",
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryWithScopeLockUsesScopedSessionLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, func() string { return "unused-token" })

	firstLockID := upstreamBalanceScopeLockID(17, "https://first.invalid")
	secondLockID := upstreamBalanceScopeLockID(17, "https://second.invalid")
	require.NotEqual(t, firstLockID, secondLockID)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(firstLockID).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(firstLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	called := false
	locked, err := repo.WithScopeLock(context.Background(), 17, "https://first.invalid", func(context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	require.True(t, locked)
	require.True(t, called)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamBalanceEventRepositoryContractCannotCarrySecrets(t *testing.T) {
	for _, value := range []any{
		service.UpstreamBalanceClaimInput{},
		service.UpstreamBalanceDeliveryLease{},
		service.UpstreamBalanceDeliveryResult{},
		service.UpstreamBalanceDeliveryFailure{},
		service.UpstreamBalanceEvent{},
	} {
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumField(); i++ {
			name := regexp.MustCompile(`(?i)(password|credential|api.?key|card|payload)`).FindString(typ.Field(i).Name)
			require.Empty(t, name, "%s must not expose sensitive repository input field %s", typ.Name(), typ.Field(i).Name)
		}
	}
}

func TestUpstreamBalanceEventRepositorySilenceByActionTokenIsScopedToActiveEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUpstreamBalanceEventRepository(db, nil)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	until := now.Add(6 * time.Hour)
	hash := strings.Repeat("a", 64)
	mock.ExpectExec(`(?s)UPDATE ops_alert_events.*silenced_until.*action_token_hash = \$4`).
		WithArgs(until, service.OpsAlertStatusFiring, service.UpstreamBalanceScopeTypeBaseURL, hash).
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := repo.SilenceByActionToken(context.Background(), hash, until, now)

	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func upstreamBalanceEventRows() *sqlmock.Rows {
	return sqlmock.NewRows(upstreamBalanceEventColumns)
}
