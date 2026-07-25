package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeResolver struct {
	calls int
	image string
	err   error
}

func (f *fakeResolver) Resolve(_ context.Context, target string) (string, error) {
	if target != "1.2.3" {
		return "", ErrTargetChanged
	}
	f.calls++
	return f.image, f.err
}

type fakeExecutor struct {
	calls  int
	result ExecutionResult
	err    error
}

func (f *fakeExecutor) Run(context.Context, Operation) (ExecutionResult, error) {
	f.calls++
	return f.result, f.err
}

type fakeIdentity struct {
	id           int64
	role, status string
	err          error
	calls        int
}

func (f *fakeIdentity) Verify(context.Context, string, http.Header) (Identity, error) {
	f.calls++
	return Identity{ID: f.id, Role: f.role, Status: f.status}, f.err
}

func admissionServer(t *testing.T) (*httptest.Server, *fakeResolver, *fakeExecutor) {
	t.Helper()
	r := &fakeResolver{image: "weishaw/sub2api:1.2.3@sha256:" + strings.Repeat("a", 64)}
	e := &fakeExecutor{}
	now := func() time.Time { return time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC) }
	s := NewService(NewStore(t.TempDir()+"/state.json"), r, e, now)
	t.Cleanup(s.Close)
	h := NewHTTP(s, &fakeIdentity{id: 1, role: "admin", status: "active"}, "https://admin.example", now)
	return httptest.NewServer(h), r, e
}
func updateRequest(t *testing.T, url string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	return updateRequestBody(t, url, `{"mode":"now","target_version":"1.2.3"}`, mutate)
}

func updateRequestBody(t *testing.T, url, body string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/api/v1/admin/system/update", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("Origin", "https://admin.example")
	req.Header.Set("X-Admin-UI-Request", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if mutate != nil {
		mutate(req)
	}
	req.ContentLength = -1
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
func responseCode(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	var v struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v.Code
}

func TestHTTPUpdateRejectsAdmission(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   string
	}{
		{"missing bearer", func(r *http.Request) { r.Header.Del("Authorization") }, "UPDATE_AUTH_REQUIRED"},
		{"malformed bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer a b") }, "UPDATE_AUTH_REQUIRED"},
		{"wrong origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, "UPDATE_FORBIDDEN"},
		{"missing ui header", func(r *http.Request) { r.Header.Del("X-Admin-UI-Request") }, "UPDATE_CONFIRMATION_REQUIRED"},
		{"non json", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, "UPDATE_CONFIRMATION_REQUIRED"},
		{"cross site fetch", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, "UPDATE_FORBIDDEN"},
		{"empty mode", func(r *http.Request) { r.Body = ioNop(`{"mode":"","target_version":"1.2.3"}`) }, "UPDATE_CONFIRMATION_REQUIRED"},
		{"wrong target", func(r *http.Request) { r.Body = ioNop(`{"mode":"now","target_version":"8.8.8"}`) }, "UPDATE_TARGET_CHANGED"},
		{"too early", func(r *http.Request) {
			r.Body = ioNop(`{"mode":"schedule","target_version":"1.2.3","scheduled_at":"2026-07-25T00:01:00Z"}`)
		}, "UPDATE_INVALID_TIME"},
		{"too late", func(r *http.Request) {
			r.Body = ioNop(`{"mode":"schedule","target_version":"1.2.3","scheduled_at":"2026-08-25T00:00:00Z"}`)
		}, "UPDATE_INVALID_TIME"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, r, e := admissionServer(t)
			defer ts.Close()
			if got := responseCode(t, updateRequest(t, ts.URL, tc.mutate)); got != tc.want {
				t.Fatalf("code=%q want %q", got, tc.want)
			}
			if r.calls != 0 || e.calls != 0 {
				t.Fatalf("rejected request invoked resolver=%d executor=%d", r.calls, e.calls)
			}
		})
	}
}
func ioNop(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestHTTPSuccessStatusCancelUseOfficialEnvelope(t *testing.T) {
	ts, _, _ := admissionServer(t)
	defer ts.Close()
	res := updateRequestBody(t, ts.URL, `{"mode":"schedule","target_version":"1.2.3","scheduled_at":"2026-07-25T01:00:00Z"}`, nil)
	defer res.Body.Close()
	var created struct {
		Code int `json:"code"`
		Data struct {
			OperationID string `json:"operation_id"`
			Stage       string `json:"stage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Code != 0 || created.Data.OperationID == "" || created.Data.Stage != "scheduled" {
		t.Fatalf("created = %#v", created)
	}
	for _, methodAndPath := range []struct {
		method string
		path   string
	}{{http.MethodGet, "/api/v1/admin/system/host-update/status"}, {http.MethodDelete, "/api/v1/admin/system/host-update/schedule"}} {
		req, err := http.NewRequest(methodAndPath.method, ts.URL+methodAndPath.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer valid")
		req.Header.Set("Origin", "https://admin.example")
		req.Header.Set("X-Admin-UI-Request", "1")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if methodAndPath.method == http.MethodDelete {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Code int `json:"code"`
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || envelope.Code != 0 {
			t.Fatalf("%s %s: status=%d envelope=%#v err=%v", methodAndPath.method, methodAndPath.path, response.StatusCode, envelope, err)
		}
	}
}
func TestHTTPUpdateRejectsNonAdminIdentity(t *testing.T) {
	for _, identity := range []*fakeIdentity{{id: 0, role: "admin", status: "active"}, {id: 1, role: "user", status: "active"}, {id: 1, role: "admin", status: "disabled"}} {
		s := NewService(NewStore(t.TempDir()+"/state.json"), &fakeResolver{}, &fakeExecutor{})
		h := NewHTTP(s, identity, "https://admin.example", time.Now)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", strings.NewReader(`{"mode":"now","target_version":"1.2.3"}`))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Origin", "https://admin.example")
		req.Header.Set("X-Admin-UI-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status %d", rr.Code)
		}
		s.Close()
	}
}
func TestHTTPStatusRequiresAdminAuth(t *testing.T) {
	ts, _, _ := admissionServer(t)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/system/host-update/status", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := responseCode(t, res); got != "UPDATE_AUTH_REQUIRED" {
		t.Fatal(got)
	}
}
