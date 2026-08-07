package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/adminauth"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"example.invalid/relay-ops-service/internal/upstreams"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func TestPricingIsAnonymousFilteredAndDoesNotLeakUpstreamCosts(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, value := range []string{"GPT-Pro", "gpt-5.6-sol", "$1.25", "$10.00", "272k"} {
		if !strings.Contains(body, value) {
			t.Fatalf("missing %q", value)
		}
	}
	for _, leak := range []string{"Neko", "0.10x", "actual_cost", "secret"} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %q", leak)
		}
	}
}

func TestRetiredOpsAndAcknowledgementRoutesAreNotMounted(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/ops"},
		{http.MethodGet, "/relay-ops/api/ops-view"},
		{http.MethodPost, "/relay-ops/api/incidents/ack"},
		{http.MethodPost, "/relay-ops/api/feishu/events"},
		{http.MethodGet, "/relay-ops/static/ops.js"},
		{http.MethodGet, "/relay-ops/static/ops-admin.js"},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d want=404", tt.method, tt.path, recorder.Code)
		}
	}
}

func TestCreateUpstreamPassesAdapterTypeAndReturnsIt(t *testing.T) {
	t.Parallel()

	upstreamService := &fakeProductionUpstreamService{}
	instance := &server{dependencies: Dependencies{
		BaseOrigin: "https://api.example.com",
		Upstreams:  upstreamService,
	}}
	handler := adminauth.RequireAdmin(fakeAdminVerifier{identity: adminauth.Identity{UserID: 9, Role: "admin", Status: "active"}}, http.HandlerFunc(instance.createUpstream))
	request := httptest.NewRequest(http.MethodPost, "/relay-ops/api/upstreams", strings.NewReader(`{"name":"billing source","base_url":"https://billing.example/v1","adapter_type":"sub2api","pricing_url":"https://billing.example/pricing","group_ids":[3]}`))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Origin", "https://api.example.com")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if upstreamService.input.AdapterType != "sub2api" {
		t.Fatalf("adapter type passed to service = %q", upstreamService.input.AdapterType)
	}
	var response map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response["adapter_type"] != "sub2api" {
		t.Fatalf("response adapter_type = %#v", response["adapter_type"])
	}
}

func TestNoPerformanceRouteAndResponsivePricingStateExist(t *testing.T) {
	t.Parallel()
	server := newTestServer(fakeOps{})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/performance", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("performance status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing?q=missing", nil))
	if !strings.Contains(recorder.Body.String(), "没有匹配的模型") {
		t.Fatalf("empty state missing: %s", recorder.Body.String())
	}
}

func TestAccountingRoutesRequireAnActiveAdministrator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity adminauth.Identity
		bearer   string
		want     int
	}{
		{name: "missing session", want: http.StatusUnauthorized},
		{name: "ordinary user", bearer: "user", identity: adminauth.Identity{UserID: 7, Role: "user", Status: "active"}, want: http.StatusForbidden},
		{name: "disabled admin", bearer: "disabled", identity: adminauth.Identity{UserID: 8, Role: "admin", Status: "disabled"}, want: http.StatusForbidden},
		{name: "active admin", bearer: "admin", identity: adminauth.Identity{UserID: 9, Role: "admin", Status: "active"}, want: http.StatusOK},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newAccountingTestServer(t, &fakeAccounting{}, test.identity)
			request := httptest.NewRequest(http.MethodGet, "/relay-ops/accounting", nil)
			if test.bearer != "" {
				request.Header.Set("Authorization", "Bearer "+test.bearer)
			}
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestAccountingRoutesAreNotMountedWhenDisabled(t *testing.T) {
	t.Parallel()

	server := newAccountingTestServer(t, nil, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	for _, target := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/relay-ops/accounting"},
		{method: http.MethodGet, path: "/relay-ops/api/accounting/daily?date=2026-08-02"},
		{method: http.MethodPost, path: "/relay-ops/api/accounting/cash-events"},
		{method: http.MethodGet, path: "/relay-ops/static/accounting.js"},
	} {
		request := authenticatedRequest(target.method, target.path, "")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d want=404", target.method, target.path, recorder.Code)
		}
	}
}

func TestAccountingBrowserScriptIsReachableWithoutBearerHeader(t *testing.T) {
	t.Parallel()

	server := newAccountingTestServer(t, &fakeAccounting{}, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/relay-ops/static/accounting.js", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("content type=%q", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), "cash-event-form") {
		t.Fatalf("script body missing form behavior")
	}
}

func TestCreateCashEventRejectsInvalidTransportAndInputBeforePersistence(t *testing.T) {
	t.Parallel()

	validBody := `{"event_type":"account_purchase","paid_at":"2026-08-02T10:00:00+08:00","amount_cny":"68.00","source_kind":"owned_oauth","account_id":42,"notes":"plus account purchase"}`
	tests := []struct {
		name        string
		body        string
		idempotency string
		origin      string
		contentType string
		wantStatus  int
	}{
		{name: "origin", body: validBody, idempotency: "accounting:purchase:42:20260802", origin: "https://evil.example", contentType: "application/json", wantStatus: http.StatusForbidden},
		{name: "content type", body: validBody, idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "text/plain", wantStatus: http.StatusForbidden},
		{name: "unknown field", body: strings.TrimSuffix(validBody, "}") + `,"supplier":"forbidden"}`, idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "credential-like note", body: strings.Replace(validBody, "plus account purchase", "api_key=sk-not-allowed", 1), idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "second object", body: validBody + `{}`, idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "trailing scalar", body: validBody + ` true`, idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "missing idempotency key", body: validBody, origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "short idempotency key", body: validBody, idempotency: "short", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "non ascii idempotency key", body: validBody, idempotency: "accounting:购买:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "invalid idempotency character", body: validBody, idempotency: "accounting purchase 42 20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "domain amount rule", body: strings.Replace(validBody, `"68.00"`, `"-68.00"`, 1), idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
		{name: "body over limit", body: strings.TrimSuffix(validBody, "}") + `,"notes":"` + strings.Repeat("x", 4096) + `"}`, idempotency: "accounting:purchase:42:20260802", origin: "https://api.example.com", contentType: "application/json", wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			accountingService := &fakeAccounting{}
			server := newAccountingTestServer(t, accountingService, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
			request := authenticatedRequest(http.MethodPost, "/relay-ops/api/accounting/cash-events", test.body)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Content-Type", test.contentType)
			if test.idempotency != "" {
				request.Header.Set("Idempotency-Key", test.idempotency)
			}
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if accountingService.createCalls != 0 {
				t.Fatalf("persistence called %d times for invalid request", accountingService.createCalls)
			}
		})
	}
}

func TestCreateCashEventReturnsNormalizedEventAndReplayStatus(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		created bool
		want    int
	}{
		{name: "created", created: true, want: http.StatusCreated},
		{name: "replayed", created: false, want: http.StatusOK},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			accountID := int64(42)
			paidAt := time.Date(2026, 8, 2, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
			accountingService := &fakeAccounting{
				created: test.created,
				event: accounting.CashEvent{
					ID: 17, EventType: accounting.EventTypeAccountPurchase, PaidAt: paidAt,
					AmountCNY: decimal.RequireFromString("68.00"), SourceKind: accounting.SourceKindOwnedOAuth,
					AccountID: &accountID, Notes: "plus account purchase", CreatedByUserID: 9,
					CreatedAt: time.Date(2026, 8, 2, 10, 1, 0, 0, time.UTC),
				},
			}
			server := newAccountingTestServer(t, accountingService, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
			request := authenticatedJSONRequest(http.MethodPost, "/relay-ops/api/accounting/cash-events",
				`{"event_type":"account_purchase","paid_at":"2026-08-02T10:00:00+08:00","amount_cny":"68.00","source_kind":"owned_oauth","account_id":42,"notes":"plus account purchase"}`)
			request.Header.Set("Idempotency-Key", "accounting:purchase:42:20260802")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.want, recorder.Body.String())
			}
			body := recorder.Body.String()
			for _, value := range []string{`"id":17`, `"event_type":"account_purchase"`, `"amount_cny":"68.00"`, `"source_kind":"owned_oauth"`, `"account_id":42`} {
				if !strings.Contains(body, value) {
					t.Fatalf("response missing %q: %s", value, body)
				}
			}
			if accountingService.createCalls != 1 || accountingService.actor.UserID != 9 ||
				accountingService.idempotencyKey != "accounting:purchase:42:20260802" ||
				accountingService.input.AmountCNY.StringFixed(2) != "68.00" {
				t.Fatalf("create call = %#v", accountingService)
			}
		})
	}
}

func TestCreateCashEventDoesNotExposeInternalErrors(t *testing.T) {
	t.Parallel()

	accountingService := &fakeAccounting{createErr: errors.New(`pq: password authentication failed for "secret-user"`)}
	server := newAccountingTestServer(t, accountingService, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	request := authenticatedJSONRequest(http.MethodPost, "/relay-ops/api/accounting/cash-events",
		`{"event_type":"fee","paid_at":"2026-08-02T10:00:00+08:00","amount_cny":"2.50","source_kind":"upstream_apikey","notes":"wire fee"}`)
	request.Header.Set("Idempotency-Key", "accounting:fee:20260802")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, leak := range []string{"pq:", "password", "secret-user"} {
		if strings.Contains(recorder.Body.String(), leak) {
			t.Fatalf("internal error leaked %q: %s", leak, recorder.Body.String())
		}
	}
}

func TestAccountingDailyReturnsSavedSnapshotAndHandlesMissingDate(t *testing.T) {
	t.Parallel()

	reportDate := time.Date(2026, 8, 2, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	accountingService := &fakeAccounting{
		snapshotFound: true,
		snapshot: accounting.DailySnapshot{
			ReportDate: reportDate, ExternalRevenueCNY: decimal.RequireFromString("91.20"),
			OwnedOAuthCostCNY:      decimal.RequireFromString("11.50"),
			UpstreamAPIKeyCostCNY:  decimal.RequireFromString("3.25"),
			UnlinkedCashOutflowCNY: decimal.RequireFromString("68.00"),
		},
	}
	server := newAccountingTestServer(t, accountingService, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	request := authenticatedRequest(http.MethodGet, "/relay-ops/api/accounting/daily?date=2026-08-02", "")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, value := range []string{`"report_date":"2026-08-02"`, `"external_revenue_cny":"91.20"`, `"owned_oauth_cost_cny":"11.50"`, `"upstream_apikey_cost_cny":"3.25"`, `"unlinked_cash_outflow_cny":"68.00"`} {
		if !strings.Contains(recorder.Body.String(), value) {
			t.Fatalf("daily response missing %q: %s", value, recorder.Body.String())
		}
	}

	missing := newAccountingTestServer(t, &fakeAccounting{}, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder = httptest.NewRecorder()
	missing.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/accounting/daily?date=2026-08-03", ""))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d want=404", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	missing.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/accounting/daily?date=2026-02-31", ""))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status=%d want=400", recorder.Code)
	}
}

func TestAccountingPageRendersOnlyMinimalEscapedLedgerData(t *testing.T) {
	t.Parallel()

	accountID := int64(42)
	accountingService := &fakeAccounting{
		snapshotFound: true,
		snapshot: accounting.DailySnapshot{
			ReportDate:             time.Date(2026, 7, 30, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
			ExternalRevenueCNY:     decimal.RequireFromString("91.20"),
			ResourceCostCNY:        decimal.RequireFromString("14.75"),
			OwnedOAuthCostCNY:      decimal.RequireFromString("11.50"),
			UpstreamAPIKeyCostCNY:  decimal.RequireFromString("3.25"),
			UnlinkedCashOutflowCNY: decimal.RequireFromString("68.00"),
		},
		events: []accounting.CashEvent{{
			ID: 17, EventType: accounting.EventTypeAccountPurchase,
			PaidAt:    time.Date(2026, 7, 30, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
			AmountCNY: decimal.RequireFromString("68.00"), SourceKind: accounting.SourceKindOwnedOAuth,
			AccountID: &accountID, Notes: `<script>alert("x")</script>`,
		}},
	}
	server := newAccountingTestServer(t, accountingService, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/accounting?date=2026-07-30", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, value := range []string{
		`name="event_type"`, `name="paid_at"`, `name="amount_cny"`, `name="source_kind"`, `name="account_id"`, `name="notes"`,
		"owned_oauth", "upstream_apikey", "91.20", "11.50", "3.25", "未关联现金支出", "&lt;script&gt;",
	} {
		if !strings.Contains(body, value) {
			t.Fatalf("page missing %q", value)
		}
	}
	for _, leak := range []string{
		`name="supplier"`, `name="batch"`, `name="external_ref"`, `name="group"`,
		`name="api_key"`, `name="token"`, `name="cookie"`, `name="password"`,
		`name="oauth_credential"`, `<script>alert("x")</script>`, "credentials",
	} {
		if strings.Contains(body, leak) {
			t.Fatalf("page leaked %q", leak)
		}
	}
	if got := strings.Count(body, `cash-event-field`); got != 6 {
		t.Fatalf("cash event field count=%d want=6", got)
	}
}

func newTestServer(_ fakeOps) http.Handler {
	server, err := NewServer(Dependencies{
		BaseOrigin: "https://api.example.com", Pricing: fakePricing{},
	})
	if err != nil {
		panic(err)
	}
	return server
}

type fakePricing struct{}

func (fakePricing) PublicPricing(context.Context) ([]PublicGroup, error) {
	return []PublicGroup{{Name: "GPT-Pro", UpdatedAt: "2026-07-19 12:00", Models: []PublicModel{{ModelID: "gpt-5.6-sol", Tier: ">272k", Input: "1.25", Output: "10.00", CacheRead: "0.125"}}}}, nil
}

type fakeOps struct{ view OpsView }

func (source fakeOps) Snapshot(context.Context) (OpsView, error) {
	if source.view.NativeMonitorURL == "" {
		source.view.NativeMonitorURL = "/monitor"
	}
	return source.view, nil
}

type fakeAdminVerifier struct {
	identity adminauth.Identity
}

func (verifier fakeAdminVerifier) VerifyAdminSession(context.Context, adminauth.Session) (adminauth.Identity, error) {
	return verifier.identity, nil
}

type fakeAccounting struct {
	event          accounting.CashEvent
	created        bool
	createErr      error
	createCalls    int
	actor          domain.AdminActor
	input          accounting.CashEventInput
	idempotencyKey string
	snapshot       accounting.DailySnapshot
	snapshotFound  bool
	snapshotErr    error
	events         []accounting.CashEvent
	listErr        error
}

type fakeProductionUpstreamService struct {
	input upstreams.ProductionInput
}

func (service *fakeProductionUpstreamService) List(context.Context, domain.AdminActor) ([]upstreams.Source, error) {
	return nil, nil
}

func (service *fakeProductionUpstreamService) CreateProduction(_ context.Context, _ domain.AdminActor, input upstreams.ProductionInput) (upstreams.Source, error) {
	service.input = input
	return upstreams.Source{ID: 17, Name: input.Name, Role: upstreams.RoleProduction, BaseURL: input.BaseURL, AdapterType: input.AdapterType, GroupIDs: input.GroupIDs, Enabled: true}, nil
}

func (service *fakeProductionUpstreamService) Disable(context.Context, domain.AdminActor, domain.UpstreamID) error {
	return nil
}

func (service *fakeAccounting) CreateCashEvent(_ context.Context, actor domain.AdminActor, input accounting.CashEventInput, idempotencyKey string) (accounting.CashEvent, bool, error) {
	service.createCalls++
	service.actor = actor
	service.input = input
	service.idempotencyKey = idempotencyKey
	return service.event, service.created, service.createErr
}

func (service *fakeAccounting) ReadDailySnapshot(context.Context, time.Time) (accounting.DailySnapshot, bool, error) {
	return service.snapshot, service.snapshotFound, service.snapshotErr
}

func (service *fakeAccounting) ListCashEvents(context.Context, time.Time, time.Time, int) ([]accounting.CashEvent, error) {
	return service.events, service.listErr
}

func (service *fakeAccounting) RecomputeDate(context.Context, time.Time) (accounting.DailySnapshot, error) {
	return service.snapshot, service.snapshotErr
}

func newAccountingTestServer(t *testing.T, service AccountingService, identity adminauth.Identity) http.Handler {
	t.Helper()
	server, err := NewServer(Dependencies{
		BaseOrigin: "https://api.example.com",
		Auth:       fakeAdminVerifier{identity: identity},
		Pricing:    fakePricing{},
		Accounting: service,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func authenticatedRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer browser-token")
	return request
}

func authenticatedJSONRequest(method, path, body string) *http.Request {
	request := authenticatedRequest(method, path, body)
	request.Header.Set("Origin", "https://api.example.com")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestReconciliationManualAdjustmentTargetsException(t *testing.T) {
	t.Parallel()

	service := &fakeReconciliation{created: true}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	request := authenticatedJSONRequest(http.MethodPost, "/relay-ops/api/reconciliation/exceptions/73/adjust",
		`{"amount":"0.23","notes":"confirmed by upstream"}`)
	request.Header.Set("Idempotency-Key", "browser-retry-key")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.exceptionID != 73 || service.input.AttemptID != 0 || service.input.ActorUserID != 9 ||
		!service.input.Amount.Equal(decimal.RequireFromString("0.23")) || service.input.Notes != "confirmed by upstream" ||
		service.input.IdempotencyKey != "manual:exception:73:browser-retry-key" {
		t.Fatalf("manual adjustment call = %#v", service)
	}
}

func TestOperationsRoutesReturnScopedSummaryAndDailyHistory(t *testing.T) {
	t.Parallel()

	groupID := int64(3)
	margin := decimal.RequireFromString("0.80")
	service := &fakeReconciliation{
		operations: reconciliation.OperationsSummary{
			Scope:         reconciliation.OperationsScope{GroupID: &groupID},
			TotalAttempts: 4, UpstreamCost: decimal.RequireFromString("0.80"),
			UserCharge: decimal.RequireFromString("4.00"), PaperProfit: decimal.RequireFromString("3.20"),
			ProfitMargin: &margin, UnattributedAttempts: 1, Currency: "USD",
		},
		daily: []reconciliation.OperationsDailyRow{{
			Day: "2026-08-01", TotalAttempts: 4, UpstreamCost: decimal.RequireFromString("0.80"),
			UserCharge: decimal.RequireFromString("4.00"), PaperProfit: decimal.RequireFromString("3.20"), Currency: "USD",
		}},
	}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	query := "?group_id=3&start=2026-08-01T00:00:00Z&end=2026-08-02T00:00:00Z&currency=USD&timezone=UTC"
	request := authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/operations"+query, "")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("operations status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{`"group_id":3`, `"profit_margin":"0.8"`, `"unattributed_attempts":1`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("operations response missing %s: %s", want, recorder.Body.String())
		}
	}
	if service.operationsScope.GroupID == nil || *service.operationsScope.GroupID != 3 || service.operationsScope.Timezone != "UTC" {
		t.Fatalf("operations scope = %#v", service.operationsScope)
	}

	request = authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/operations/history"+query, "")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"day":"2026-08-01"`) {
		t.Fatalf("history status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.historyScope.GroupID == nil || *service.historyScope.GroupID != 3 {
		t.Fatalf("history scope = %#v", service.historyScope)
	}
}

func TestOperationsRoutesRejectInvalidScopeAndRequireAdmin(t *testing.T) {
	t.Parallel()

	server := newReconciliationTestServer(t, &fakeReconciliation{}, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	invalid := authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/operations?group_id=0", "")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, invalid)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_OPERATIONS_SCOPE") {
		t.Fatalf("invalid scope status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	unauthenticated := httptest.NewRequest(http.MethodGet, "/relay-ops/api/reconciliation/operations", nil)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, unauthenticated)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequestCostRouteRejectsMissingExactRequestIDBeforeCallingService(t *testing.T) {
	t.Parallel()

	service := &fakeReconciliation{}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost", ""))

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_REQUEST_COST_QUERY") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.requestCostCalls != 0 {
		t.Fatalf("request cost service called %d times for invalid query", service.requestCostCalls)
	}
}

func TestRequestCostRouteReturnsOnlyNativeEvidenceForAdmin(t *testing.T) {
	t.Parallel()

	matchedAt := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	service := &fakeReconciliation{requestCostDetail: reconciliation.RequestCostDetail{
		LocalRequestID: "local-1", UpstreamRequestID: "upstream-1", SourceID: "native-log-7",
		AdapterType: reconciliation.AdapterSub2API, Model: "gpt-test", PromptTokens: 17, CompletionTokens: 9,
		UpstreamActualCost: decimal.RequireFromString("0.0821"), UpstreamStandardCost: decimal.Zero,
		CostSource: "上游逐笔账单", Confidence: "confirmed", MatchedAt: &matchedAt, Status: reconciliation.StatusMatched,
	}}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})

	unauthenticated := httptest.NewRecorder()
	server.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost?local_request_id=local-1", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost?local_request_id=+local-1+", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.requestCostQuery.LocalRequestID != "local-1" || service.requestCostQuery.UpstreamRequestID != "" {
		t.Fatalf("request cost query=%#v", service.requestCostQuery)
	}
	var response map[string]json.RawMessage
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"local_request_id": `"local-1"`, "upstream_request_id": `"upstream-1"`, "source_id": `"native-log-7"`,
		"adapter_type": `"sub2api"`, "model": `"gpt-test"`, "prompt_tokens": "17", "completion_tokens": "9",
		"upstream_actual_cost": `"0.0821"`, "upstream_standard_cost": `"0"`, "cost_source": `"上游逐笔账单"`,
		"confidence": `"confirmed"`, "status": `"matched"`,
	} {
		if got := string(response[name]); got != want {
			t.Fatalf("response[%q]=%s want=%s body=%s", name, got, want, recorder.Body.String())
		}
	}
	if _, ok := response["matched_at"]; !ok {
		t.Fatalf("matched_at missing from response: %s", recorder.Body.String())
	}
	if len(response) != 13 {
		t.Fatalf("response contains non-contract fields: %s", recorder.Body.String())
	}
}

func TestRequestCostRouteMapsNoMatchedNativeTransactionToNotFound(t *testing.T) {
	t.Parallel()

	server := newReconciliationTestServer(t, &fakeReconciliation{requestCostErr: pgx.ErrNoRows}, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost?upstream_request_id=missing", ""))
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "REQUEST_COST_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequestCostRouteReturnsNetNativeChargeAndRefundCost(t *testing.T) {
	t.Parallel()

	service := &fakeReconciliation{requestCostDetail: reconciliation.RequestCostDetail{
		LocalRequestID: "local-net", SourceID: "native-charge,native-refund", AdapterType: reconciliation.AdapterNewAPI,
		UpstreamActualCost: decimal.RequireFromString("0.06"), CostSource: reconciliation.RequestCostSourceNativeLedger,
		Confidence: "confirmed", Status: reconciliation.StatusMatched,
	}}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost?local_request_id=local-net", ""))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"source_id":"native-charge,native-refund"`) ||
		!strings.Contains(recorder.Body.String(), `"upstream_actual_cost":"0.06"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequestCostRouteRejectsConflictingExactIdentifiers(t *testing.T) {
	t.Parallel()

	service := &fakeReconciliation{}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/request-cost?local_request_id=local-1&upstream_request_id=upstream-1", ""))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_REQUEST_COST_QUERY") || service.requestCostCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.requestCostCalls, recorder.Body.String())
	}
}

func TestOperationsHistoryDefaultsToThirtyNaturalDays(t *testing.T) {
	t.Parallel()

	service := &fakeReconciliation{}
	server := newReconciliationTestServer(t, service, adminauth.Identity{UserID: 9, Role: "admin", Status: "active"})
	request := authenticatedRequest(http.MethodGet, "/relay-ops/api/reconciliation/operations/history?timezone=Asia/Shanghai", "")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.historyScope.Timezone != "Asia/Shanghai" || !service.historyScope.Start.Before(service.historyScope.End) ||
		service.historyScope.End.Sub(service.historyScope.Start) < 29*24*time.Hour ||
		service.historyScope.End.Sub(service.historyScope.Start) > 31*24*time.Hour {
		t.Fatalf("default history scope = %#v", service.historyScope)
	}
}

func newReconciliationTestServer(t *testing.T, service ReconciliationService, identity adminauth.Identity) http.Handler {
	t.Helper()
	server, err := NewServer(Dependencies{
		BaseOrigin:     "https://api.example.com",
		Auth:           fakeAdminVerifier{identity: identity},
		Pricing:        fakePricing{},
		Reconciliation: service,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

type fakeReconciliation struct {
	exceptionID       int64
	input             reconciliation.ManualAdjustmentInput
	created           bool
	requestCostDetail reconciliation.RequestCostDetail
	requestCostErr    error
	requestCostQuery  reconciliation.RequestCostQuery
	requestCostCalls  int
	operations        reconciliation.OperationsSummary
	daily             []reconciliation.OperationsDailyRow
	operationsScope   reconciliation.OperationsScope
	historyScope      reconciliation.OperationsScope
}

func (s *fakeReconciliation) ReadReconciliationSummary(context.Context, int64, time.Time, time.Time, string) (reconciliation.Summary, error) {
	return reconciliation.Summary{}, nil
}

func (s *fakeReconciliation) ReadRequestCostDetail(_ context.Context, query reconciliation.RequestCostQuery) (reconciliation.RequestCostDetail, error) {
	s.requestCostCalls++
	s.requestCostQuery = query
	return s.requestCostDetail, s.requestCostErr
}

func (s *fakeReconciliation) ListUpstreamCostExceptions(context.Context, int64, int) ([]reconciliation.Exception, error) {
	return nil, nil
}

func (s *fakeReconciliation) CreateManualUpstreamCostForException(_ context.Context, exceptionID int64, input reconciliation.ManualAdjustmentInput) (reconciliation.Transaction, bool, error) {
	s.exceptionID = exceptionID
	s.input = input
	return reconciliation.Transaction{ID: 18, SourceType: reconciliation.SourceManualAdjustment}, s.created, nil
}

func (s *fakeReconciliation) RefreshReconciliation(context.Context, int64, time.Time, time.Time, string) (reconciliation.Summary, error) {
	return reconciliation.Summary{}, nil
}

func (s *fakeReconciliation) ReadOperationsSummary(_ context.Context, scope reconciliation.OperationsScope) (reconciliation.OperationsSummary, error) {
	s.operationsScope = scope
	return s.operations, nil
}

func (s *fakeReconciliation) ListOperationsDaily(_ context.Context, scope reconciliation.OperationsScope) ([]reconciliation.OperationsDailyRow, error) {
	s.historyScope = scope
	return s.daily, nil
}
