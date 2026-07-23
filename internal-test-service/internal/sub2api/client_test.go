package sub2api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/sub2api"
	"example.invalid/internal-test-service/internal/testsupport"
)

func TestHTTPClientHeadersAndJSON(t *testing.T) {
	fake := testsupport.NewFake()
	fake.AddUser(sub2api.User{ID: 7, AffCode: "aff-7"})
	server := httptest.NewServer(fake.Handler())
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, fake.AdminKey)
	u, err := c.GetCurrentUser(context.Background(), "user-7")
	if err != nil || u.ID != 7 {
		t.Fatalf("user %+v %v", u, err)
	}
	inv, err := c.GenerateInvitation(context.Background(), 1, nil, "stable-key")
	if err != nil || len(inv) != 1 {
		t.Fatalf("invite %+v %v", inv, err)
	}
	if err := c.AddBalance(context.Background(), 7, domain.CheckinGrant, "balance-key", "checkin"); err != nil {
		t.Fatal(err)
	}
	b, err := c.GetBalance(context.Background(), 7)
	if err != nil || b.Balance != domain.CheckinGrant {
		t.Fatalf("balance %+v %v", b, err)
	}
}

func TestHTTPClientRedactsNon2xxBody(t *testing.T) {
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("secret-token-and-password"))
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "do-not-log")
	_, err := c.GetUser(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if containsText(err.Error(), "secret-token") || containsText(err.Error(), "do-not-log") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func TestHTTPClientUsageCursorQuery(t *testing.T) {
	var got string
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[{"id":3,"user_id":7,"amount_usd":"0.50","successful":true}]}`))
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "key")
	items, err := c.ListUsage(context.Background(), 7, 2)
	if err != nil || len(items) != 1 || items[0].ID != 3 {
		t.Fatalf("items %+v %v", items, err)
	}
	if got != "exact_total=true&page=1&page_size=1000&sort_by=id&sort_order=desc&user_id=7" {
		t.Fatalf("query %s", got)
	}
}

func TestHTTPClientAdminGETIncludesAPIKey(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "server-side-admin-key")
	if _, err := c.ListUsage(context.Background(), 7, 0); err != nil {
		t.Fatal(err)
	}
	if gotKey != "server-side-admin-key" {
		t.Fatalf("admin GET x-api-key = %q", gotKey)
	}
}

func TestHTTPClientUserGETDoesNotIncludeAdminAPIKey(t *testing.T) {
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Fatalf("user request carried admin key")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":{"id":7}}`))
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "server-side-admin-key")
	if _, err := c.GetCurrentUser(context.Background(), "user-token"); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPClientDecodesOfficialPaginatedAdminResponses(t *testing.T) {
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/redeem-codes":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":11,"code":"redacted","used_by":7}],"total":1,"page":1,"page_size":20,"pages":1}}`))
		case "/api/v1/admin/usage":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":12,"user_id":7,"total_cost":0.5,"actual_cost":0.25}],"total":1,"page":1,"page_size":20,"pages":1}}`))
		case "/api/v1/admin/users/7":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"username":"user","balance":1.25}}`))
		case "/api/v1/admin/users/7/balance-history":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":13,"code":"redacted","value":1.25,"notes":"D04 [idempotency_key=d04-test]"}],"total":1,"page":1,"page_size":1000,"pages":1,"total_recharged":1.25}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "admin")
	codes, err := c.ListInvitationCodes(context.Background())
	if err != nil || len(codes) != 1 || codes[0].ID != 11 || codes[0].UsedBy == nil || *codes[0].UsedBy != 7 {
		t.Fatalf("codes=%+v err=%v", codes, err)
	}
	usage, err := c.ListUsage(context.Background(), 7, 0)
	if err != nil || len(usage) != 1 || usage[0].ID != 12 || !usage[0].Successful || usage[0].AmountUSD != "0.250000" {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	user, err := c.GetUser(context.Background(), 7)
	if err != nil || user.ID != 7 {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	balance, err := c.GetBalance(context.Background(), 7)
	if err != nil || balance.Balance != 1_250_000 || len(balance.History) != 1 || !containsText(balance.History[0].Notes, "d04-test") {
		t.Fatalf("balance=%+v err=%v", balance, err)
	}
}

func TestHTTPClientPaginatesBalanceHistory(t *testing.T) {
	server := httptest.NewServer(testHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/users/7":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"balance":3.00}}`))
		case "/api/v1/admin/users/7/balance-history":
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":2,"value":2.00}],"page":2,"pages":2}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":1,"value":1.00}],"page":1,"pages":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := sub2api.NewHTTPClientWithKey(server.URL, "admin")
	balance, err := c.GetBalance(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(balance.History) != 2 || balance.History[0].ID != 1 || balance.History[1].ID != 2 {
		t.Fatalf("history=%+v", balance.History)
	}
}

func containsText(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
func testHandler(fn func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(fn)
}
