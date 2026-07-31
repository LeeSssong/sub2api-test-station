package accounting

import (
	"context"
	"fmt"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

// UsageReader reads native Sub2API usage totals for one half-open time window.
type UsageReader interface {
	ReadUsageTotals(context.Context, DayWindow, ExclusionPolicy) (UsageTotals, error)
}

// CashEventReader reads manually recorded cash events for one half-open window.
type CashEventReader interface {
	ReadCashEventTotals(context.Context, DayWindow) (CashEventTotals, error)
}

// SnapshotWriter persists one whole-site daily snapshot idempotently.
type SnapshotWriter interface {
	UpsertDailySnapshot(context.Context, DailySnapshot) error
}

// Repository is the accounting persistence contract. The read/write methods
// are kept together so HTTP and scheduler callers share the same invariants.
type Repository interface {
	UsageReader
	CashEventReader
	SnapshotWriter
	CreateCashEvent(context.Context, domain.AdminActor, CashEventInput, string) (CashEvent, bool, error)
	ReadDailySnapshot(context.Context, time.Time) (DailySnapshot, bool, error)
	ListCashEvents(context.Context, time.Time, time.Time, int) ([]CashEvent, error)
}

// Service calculates and persists whole-site CNY accounting reports.
type Service struct {
	Repository Repository
	Timezone   *time.Location
	StartDate  time.Time
	Exclusions ExclusionPolicy
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) timezone() *time.Location {
	if s.Timezone != nil {
		return s.Timezone
	}
	return accountingLocation
}

func (s Service) startDate() time.Time {
	if s.StartDate.IsZero() {
		return time.Time{}
	}
	return LocalDay(s.StartDate.In(s.timezone()))
}

// RecomputeRecent recomputes the previous three completed Shanghai calendar
// days, clipped at the configured ledger activation date.
func (s Service) RecomputeRecent(ctx context.Context) ([]DailySnapshot, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("accounting repository is required")
	}
	if s.startDate().IsZero() {
		return nil, fmt.Errorf("ledger start date is required")
	}
	now := s.now().In(s.timezone())
	yesterday := localDayIn(now, s.timezone()).AddDate(0, 0, -1)
	first := yesterday.AddDate(0, 0, -2)
	if first.Before(s.startDate()) {
		first = s.startDate()
	}
	snapshots := make([]DailySnapshot, 0, 3)
	for day := first; !day.After(yesterday); day = day.AddDate(0, 0, 1) {
		snapshot, err := s.RecomputeDate(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("recompute %s: %w", day.Format("2006-01-02"), err)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// RecomputeDate calculates and upserts exactly one snapshot. Dates before
// activation are rejected; callers may explicitly request a current or future
// date to materialize a zero baseline during activation.
func (s Service) RecomputeDate(ctx context.Context, date time.Time) (DailySnapshot, error) {
	if s.Repository == nil {
		return DailySnapshot{}, fmt.Errorf("accounting repository is required")
	}
	location := s.timezone()
	reportDate := localDayIn(date.In(location), location)
	startDate := s.startDate()
	if startDate.IsZero() {
		return DailySnapshot{}, fmt.Errorf("ledger start date is required")
	}
	if reportDate.Before(startDate) {
		return DailySnapshot{}, fmt.Errorf("report date %s is before ledger start date %s",
			reportDate.Format("2006-01-02"), startDate.Format("2006-01-02"))
	}
	window := dayWindowIn(reportDate, location)
	usage, err := s.Repository.ReadUsageTotals(ctx, window, s.Exclusions)
	if err != nil {
		return DailySnapshot{}, fmt.Errorf("read usage totals: %w", err)
	}
	cash, err := s.Repository.ReadCashEventTotals(ctx, window)
	if err != nil {
		return DailySnapshot{}, fmt.Errorf("read cash event totals: %w", err)
	}
	snapshot := BuildSnapshot(reportDate, usage, cash)
	if err := s.Repository.UpsertDailySnapshot(ctx, snapshot); err != nil {
		return DailySnapshot{}, fmt.Errorf("upsert daily snapshot: %w", err)
	}
	return snapshot, nil
}

// CreateCashEvent delegates validated cash-event persistence.
func (s Service) CreateCashEvent(ctx context.Context, actor domain.AdminActor, input CashEventInput, idempotencyKey string) (CashEvent, bool, error) {
	if s.Repository == nil {
		return CashEvent{}, false, fmt.Errorf("accounting repository is required")
	}
	return s.Repository.CreateCashEvent(ctx, actor, input, idempotencyKey)
}

// ReadDailySnapshot returns a persisted snapshot for one local calendar day.
func (s Service) ReadDailySnapshot(ctx context.Context, date time.Time) (DailySnapshot, bool, error) {
	if s.Repository == nil {
		return DailySnapshot{}, false, fmt.Errorf("accounting repository is required")
	}
	return s.Repository.ReadDailySnapshot(ctx, localDayIn(date.In(s.timezone()), s.timezone()))
}

// ListCashEvents returns manually recorded events in the given half-open
// window. The persistence layer applies the maximum result limit.
func (s Service) ListCashEvents(ctx context.Context, from, to time.Time, limit int) ([]CashEvent, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("accounting repository is required")
	}
	return s.Repository.ListCashEvents(ctx, from, to, limit)
}

func localDayIn(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func dayWindowIn(date time.Time, location *time.Location) DayWindow {
	start := localDayIn(date, location)
	return DayWindow{Start: start, End: start.AddDate(0, 0, 1)}
}
