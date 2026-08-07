package billing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestNewAPIAdapterReadsTokenLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-value" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/log/token":
			if r.URL.Query().Get("cursor") == "next" {
				fmt.Fprint(w, `{"data":[{"id":92,"type":6,"quota":25000,"request_id":"refund-req","created_at":"2026-08-01T10:01:00Z"}]}`)
				return
			}
			fmt.Fprint(w, `{"data":[{"id":91,"type":2,"quota":125000,"request_id":"newapi-req","upstream_request_id":"provider-req","model_name":"gpt-test","prompt_tokens":11,"completion_tokens":7,"created_at":1785578400}],"next_cursor":"next"}`)
		case "/api/status":
			fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewNewAPIAdapter(server.URL, "token-value", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	rows, cursor, err := adapter.ListTransactions(context.Background(), CostQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "next" || len(rows) != 1 {
		t.Fatalf("rows=%#v cursor=%q", rows, cursor)
	}
	if rows[0].Cost != domain.MicroUSD(250000) || rows[0].RequestID != "newapi-req" || rows[0].UpstreamRequestID != "provider-req" {
		t.Fatalf("charge=%#v", rows[0])
	}
	refunds, _, err := adapter.ListTransactions(context.Background(), CostQuery{Cursor: "next", Limit: 100})
	if err != nil || len(refunds) != 1 || refunds[0].Type != "refund" || refunds[0].Cost != domain.MicroUSD(-50000) {
		t.Fatalf("refunds=%#v err=%v", refunds, err)
	}
	snapshot, err := adapter.ReadSnapshot(context.Background())
	if err != nil || snapshot.ActualCost != 200000 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
}

func TestSub2APIAdapterReadsTransactionsAndSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-sub2api" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/usage/records":
			if r.URL.Query().Get("cursor") != "cursor-1" {
				t.Fatalf("cursor = %q", r.URL.Query().Get("cursor"))
			}
			fmt.Fprint(w, `{"items":[{"id":123,"request_id":"local-1","upstream_request_id":"up-1","actual_cost":0.082100,"model":"claude-test","input_tokens":10,"output_tokens":4,"created_at":"2026-08-01T11:00:00Z"}],"next_cursor":"cursor-2"}`)
		case "/v1/usage":
			fmt.Fprint(w, `{"usage":{"total":{"actual_cost":18.250001}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewSub2APIAdapter(server.URL, "sk-sub2api", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	rows, cursor, err := adapter.ListTransactions(context.Background(), CostQuery{Cursor: "cursor-1"})
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "cursor-2" || len(rows) != 1 || rows[0].Cost != 82100 || rows[0].UpstreamRequestID != "up-1" {
		t.Fatalf("rows=%#v cursor=%q", rows, cursor)
	}
	snapshot, err := adapter.ReadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActualCost != 18250001 || time.Since(snapshot.ObservedAt) > time.Minute {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestNewAPIAdapterPreservesMissingUpstreamRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/log/token":
			fmt.Fprint(w, `{"data":[{"id":91,"type":2,"quota":125000,"request_id":"newapi-req","model_name":"gpt-test","created_at":"2026-08-01T10:00:00Z"}]}`)
		case "/api/status":
			fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewNewAPIAdapter(server.URL, "token-value", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := adapter.ListTransactions(context.Background(), CostQuery{Limit: 1})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	if rows[0].RequestID != "newapi-req" || rows[0].UpstreamRequestID != "" {
		t.Fatalf("transaction=%#v", rows[0])
	}
}

func TestNewAPIAdapterRejectsInvalidQuotaPerUnit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/log/token":
			fmt.Fprint(w, `{"data":[{"id":91,"type":2,"quota":125000,"request_id":"newapi-req","created_at":"2026-08-01T10:00:00Z"}]}`)
		case "/api/status":
			fmt.Fprint(w, `{"data":{"quota_per_unit":0}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewNewAPIAdapter(server.URL, "token-value", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := adapter.ListTransactions(context.Background(), CostQuery{Limit: 1}); err == nil {
		t.Fatal("invalid quota_per_unit accepted")
	}
}

func TestBillingAdapterRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewNewAPIAdapter("not-a-url", "token", nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewSub2APIAdapter("https://example.com", "", nil); err == nil {
		t.Fatal("expected empty token error")
	}
}
