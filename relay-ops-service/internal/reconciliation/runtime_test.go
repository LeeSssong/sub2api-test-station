package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type runtimeCollector struct {
	request CollectionRequest
	result  CollectionResult
	err     error
	order   *[]string
}

func (c *runtimeCollector) Collect(_ context.Context, request CollectionRequest) (CollectionResult, error) {
	if c.order != nil {
		*c.order = append(*c.order, "collect")
	}
	c.request = request
	return c.result, c.err
}

type runtimeImporter struct {
	accountID int64
	start     time.Time
	end       time.Time
	err       error
	order     *[]string
}

func (i *runtimeImporter) Import(_ context.Context, accountID int64, start, end time.Time) (ImportResult, error) {
	if i.order != nil {
		*i.order = append(*i.order, "import")
	}
	i.accountID, i.start, i.end = accountID, start, end
	return ImportResult{}, i.err
}

type runtimeRepository struct {
	summary Summary
	operationsSummary OperationsSummary
	dailyRows         []OperationsDailyRow
}

func (r runtimeRepository) ReadReconciliationSummary(context.Context, int64, time.Time, time.Time, string) (Summary, error) {
	return r.summary, nil
}

func (runtimeRepository) ListUpstreamCostExceptions(context.Context, int64, int) ([]Exception, error) {
	return nil, nil
}

func (runtimeRepository) CreateManualUpstreamCostForException(context.Context, int64, ManualAdjustmentInput) (Transaction, bool, error) {
	return Transaction{}, false, nil
}

func (runtimeRepository) MarkOverdueUpstreamCostExceptions(context.Context, time.Time, time.Duration) (int64, error) {
	return 0, nil
}

func (r runtimeRepository) ReadOperationsSummary(context.Context, OperationsScope) (OperationsSummary, error) {
	return r.operationsSummary, nil
}

func (r runtimeRepository) ListOperationsDaily(context.Context, OperationsScope) ([]OperationsDailyRow, error) {
	return r.dailyRows, nil
}

func TestRuntimeServiceRefreshCollectsBeforeReadingSummary(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	order := make([]string, 0, 2)
	importer := &runtimeImporter{order: &order}
	collector := &runtimeCollector{result: CollectionResult{AccountsSucceeded: 2}, order: &order}
	service := RuntimeService{Repository: runtimeRepository{summary: Summary{TotalAttempts: 3}}, Importer: importer, Collector: collector}

	summary, err := service.RefreshReconciliation(context.Background(), 8, start, end, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalAttempts != 3 || summary.CollectionPartial {
		t.Fatalf("summary=%#v", summary)
	}
	if collector.request.Trigger != TriggerAdminRefresh || collector.request.AccountID != 8 || !collector.request.Start.Equal(start) || !collector.request.End.Equal(end) {
		t.Fatalf("request=%#v", collector.request)
	}
	if importer.accountID != 8 || !importer.start.Equal(start) || !importer.end.Equal(end) {
		t.Fatalf("import request=%d %s %s", importer.accountID, importer.start, importer.end)
	}
	if got := strings.Join(order, ","); got != "import,collect" {
		t.Fatalf("order=%s", got)
	}
}

func TestRuntimeServiceReturnsSummaryWhenCollectionIsPartial(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	collector := &runtimeCollector{err: errors.New("one account unavailable")}
	service := RuntimeService{Repository: runtimeRepository{summary: Summary{TotalAttempts: 2}}, Collector: collector}

	summary, err := service.RefreshReconciliation(context.Background(), 0, start, start.Add(time.Hour), "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !summary.CollectionPartial {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestRuntimeServiceDailyCloseUsesDailyTrigger(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	collector := &runtimeCollector{}
	service := RuntimeService{Repository: runtimeRepository{}, Collector: collector}

	_, err := service.DailyClose(context.Background(), start, start.Add(72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if collector.request.Trigger != TriggerDailyClose || !collector.request.Start.Equal(start) {
		t.Fatalf("request=%#v", collector.request)
	}
}

func TestRuntimeServiceForwardsOperationsSummaryAndHistory(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	scope := OperationsScope{GroupID: runtimePtrInt64(17), Start: start, End: start.Add(24 * time.Hour), Currency: "USD", Timezone: "UTC"}
	wantSummary := OperationsSummary{TotalAttempts: 4, Scope: scope}
	wantDaily := []OperationsDailyRow{{Day: "2026-08-01", TotalAttempts: 4}}
	service := RuntimeService{Repository: runtimeRepository{operationsSummary: wantSummary, dailyRows: wantDaily}}

	gotSummary, err := service.ReadOperationsSummary(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if gotSummary.TotalAttempts != wantSummary.TotalAttempts || gotSummary.Scope != wantSummary.Scope {
		t.Fatalf("summary=%#v", gotSummary)
	}
	gotDaily, err := service.ListOperationsDaily(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDaily) != 1 || gotDaily[0].Day != wantDaily[0].Day || gotDaily[0].TotalAttempts != wantDaily[0].TotalAttempts {
		t.Fatalf("daily=%#v", gotDaily)
	}
}

func runtimePtrInt64(value int64) *int64 { return &value }
