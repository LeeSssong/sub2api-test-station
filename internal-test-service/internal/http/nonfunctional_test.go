package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/domain"
)

func TestConcurrentSameDayLoginHasOneProviderEffect(t *testing.T) {
	fx := newHTTPTest(t)
	const requests = 12
	var start sync.WaitGroup
	start.Add(1)
	var workers sync.WaitGroup
	statuses := make(chan int, requests)
	for i := 0; i < requests; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			start.Wait()
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, authproxy.LoginEndpoint, strings.NewReader(`{"email":"user@example.com","password":"fixture"}`))
			req.Header.Set("Origin", "http://example.com")
			fx.server.ServeHTTP(rr, req)
			statuses <- rr.Code
		}()
	}
	start.Done()
	workers.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("login status=%d", status)
		}
	}
	balance, err := fx.fake.GetBalance(context.Background(), 1)
	if err != nil || balance.Balance != domain.DailyLoginCredit || len(balance.History) != 1 {
		t.Fatalf("provider effect=%+v err=%v", balance, err)
	}
}

func TestResponsesSetSecurityHeadersAndDoNotAllowCrossOriginWrites(t *testing.T) {
	fx := newHTTPTest(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, authproxy.LoginEndpoint, nil)
	req.Header.Set("Origin", "https://untrusted.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	fx.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("OPTIONS status=%d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" || rr.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("unexpected CORS headers=%v", rr.Header())
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("security headers=%v", rr.Header())
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "*") {
		t.Fatalf("unexpected CSP %q", csp)
	}
}

func TestCrossOriginAuthenticationPostIsRejectedBeforeForwarding(t *testing.T) {
	fx := newHTTPTest(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, authproxy.LoginEndpoint, strings.NewReader(`{"email":"user@example.com"}`))
	req.Header.Set("Origin", "https://untrusted.example")
	fx.server.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "ORIGIN_REJECTED") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAuthenticationPostWithoutOriginIsRejected(t *testing.T) {
	fx := newHTTPTest(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, authproxy.LoginEndpoint, strings.NewReader(`{"email":"user@example.com"}`))
	fx.server.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "ORIGIN_REJECTED") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestConcurrentLoginCompletesWithinRequestBudget(t *testing.T) {
	fx := newHTTPTest(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 4; i++ {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, authproxy.LoginEndpoint, nil)
			req.Header.Set("Origin", "http://example.com")
			fx.server.ServeHTTP(rr, req)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("same-day login did not complete within local request budget")
	}
}
