package accounting

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"github.com/shopspring/decimal"
)

type serviceRepository struct {
	usage       map[string]UsageTotals
	cash        map[string]CashEventTotals
	windows     []DayWindow
	cashWindows []DayWindow
	snapshots   []DailySnapshot
}

func (r *serviceRepository) ReadUsageTotals(_ context.Context, window DayWindow, _ ExclusionPolicy) (UsageTotals, error) {
	r.windows = append(r.windows, window)
	return r.usage[window.Start.Format("2006-01-02")], nil
}

func (r *serviceRepository) ReadCashEventTotals(_ context.Context, window DayWindow) (CashEventTotals, error) {
	r.cashWindows = append(r.cashWindows, window)
	return r.cash[window.Start.Format("2006-01-02")], nil
}

func (r *serviceRepository) UpsertDailySnapshot(_ context.Context, snapshot DailySnapshot) error {
	r.snapshots = append(r.snapshots, snapshot)
	return nil
}

func (r *serviceRepository) CreateCashEvent(context.Context, domain.AdminActor, CashEventInput, string) (CashEvent, bool, error) {
	panic("not used")
}

func (r *serviceRepository) ReadDailySnapshot(context.Context, time.Time) (DailySnapshot, bool, error) {
	panic("not used")
}

func (r *serviceRepository) ListCashEvents(context.Context, time.Time, time.Time, int) ([]CashEvent, error) {
	panic("not used")
}

func shanghaiServiceDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, accountingLocation)
}

func TestRecomputeRecentWritesOnlyStartDateThroughYesterday(t *testing.T) {
	repository := &serviceRepository{
		usage: map[string]UsageTotals{},
		cash:  map[string]CashEventTotals{},
	}
	service := Service{
		Repository: repository,
		Timezone:   accountingLocation,
		StartDate:  shanghaiServiceDate(2026, time.August, 1),
		Now: func() time.Time {
			return time.Date(2026, time.August, 4, 0, 15, 0, 0, accountingLocation)
		},
	}
	got, err := service.RecomputeRecent(context.Background())
	if err != nil {
		t.Fatalf("RecomputeRecent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("snapshot count = %d, want 3", len(got))
	}
	for i, want := range []string{"2026-08-01", "2026-08-02", "2026-08-03"} {
		if got[i].ReportDate.Format("2006-01-02") != want {
			t.Fatalf("snapshot[%d] date = %s, want %s", i, got[i].ReportDate.Format("2006-01-02"), want)
		}
	}
	if len(repository.snapshots) != 3 {
		t.Fatalf("upsert count = %d, want 3", len(repository.snapshots))
	}
}

func TestRecomputeRecentClipsToActivationDate(t *testing.T) {
	repository := &serviceRepository{usage: map[string]UsageTotals{}, cash: map[string]CashEventTotals{}}
	service := Service{
		Repository: repository,
		Timezone:   accountingLocation,
		StartDate:  shanghaiServiceDate(2026, time.August, 3),
		Now:        func() time.Time { return shanghaiServiceDate(2026, time.August, 4) },
	}
	got, err := service.RecomputeRecent(context.Background())
	if err != nil {
		t.Fatalf("RecomputeRecent: %v", err)
	}
	if len(got) != 1 || got[0].ReportDate.Format("2006-01-02") != "2026-08-03" {
		t.Fatalf("snapshots = %#v, want only 2026-08-03", got)
	}
}

func TestRecomputeDateRejectsPreActivationHistory(t *testing.T) {
	service := Service{
		Repository: &serviceRepository{usage: map[string]UsageTotals{}, cash: map[string]CashEventTotals{}},
		Timezone:   accountingLocation,
		StartDate:  shanghaiServiceDate(2026, time.August, 1),
		Now:        func() time.Time { return shanghaiServiceDate(2026, time.August, 4) },
	}
	_, err := service.RecomputeDate(context.Background(), shanghaiServiceDate(2026, time.July, 31))
	if err == nil || err.Error() != "report date 2026-07-31 is before ledger start date 2026-08-01" {
		t.Fatalf("error = %v, want activation-date rejection", err)
	}
}

func TestRecomputeDateAllowsFutureBaseline(t *testing.T) {
	repository := &serviceRepository{usage: map[string]UsageTotals{}, cash: map[string]CashEventTotals{}}
	service := Service{
		Repository: repository,
		Timezone:   accountingLocation,
		StartDate:  shanghaiServiceDate(2026, time.August, 2),
		Now:        func() time.Time { return shanghaiServiceDate(2026, time.August, 1) },
	}
	snapshot, err := service.RecomputeDate(context.Background(), shanghaiServiceDate(2026, time.August, 2))
	if err != nil {
		t.Fatalf("RecomputeDate future baseline: %v", err)
	}
	if !snapshot.ExternalRevenueCNY.IsZero() || !snapshot.ResourceCostCNY.IsZero() {
		t.Fatalf("future baseline = %#v, want zero totals", snapshot)
	}
}

func TestRecomputeDateUsesHalfOpenShanghaiWindowAndBuildsSnapshot(t *testing.T) {
	repository := &serviceRepository{
		usage: map[string]UsageTotals{
			"2026-08-02": {
				ExternalRevenueCNY: decimal.NewFromInt(12),
				ExternalRequests:   2,
				InternalRequests:   1,
				CustomerCostCNY:    decimal.NewFromInt(3),
				InternalCostCNY:    decimal.NewFromInt(4),
			},
		},
		cash: map[string]CashEventTotals{
			"2026-08-02": {OutflowCNY: decimal.NewFromInt(5), EventCount: 1},
		},
	}
	service := Service{
		Repository: repository,
		Timezone:   accountingLocation,
		StartDate:  shanghaiServiceDate(2026, time.August, 1),
		Now:        func() time.Time { return shanghaiServiceDate(2026, time.August, 4) },
	}
	snapshot, err := service.RecomputeDate(context.Background(), shanghaiServiceDate(2026, time.August, 2))
	if err != nil {
		t.Fatalf("RecomputeDate: %v", err)
	}
	if snapshot.OperatingGrossProfitCNY.String() != "5" || snapshot.CashNetResultCNY.String() != "7" {
		t.Fatalf("snapshot profit/net = %s/%s", snapshot.OperatingGrossProfitCNY, snapshot.CashNetResultCNY)
	}
	if len(repository.windows) != 1 || len(repository.cashWindows) != 1 {
		t.Fatalf("read windows = %d/%d, want one each", len(repository.windows), len(repository.cashWindows))
	}
	window := repository.windows[0]
	if !window.Start.Equal(time.Date(2026, time.August, 1, 16, 0, 0, 0, time.UTC)) ||
		!window.End.Equal(time.Date(2026, time.August, 2, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("window = %#v, want [2026-08-01 16:00Z, 2026-08-02 16:00Z)", window)
	}
}
