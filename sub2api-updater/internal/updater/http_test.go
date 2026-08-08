package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	ts, r, e, _ := admissionServerWithTrace(t)
	return ts, r, e
}

func admissionServerWithTrace(t *testing.T) (*httptest.Server, *fakeResolver, *fakeExecutor, string) {
	t.Helper()
	r := &fakeResolver{image: "weishaw/sub2api:1.2.3@sha256:" + strings.Repeat("a", 64)}
	e := &fakeExecutor{}
	now := func() time.Time { return time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC) }
	s := NewService(NewStore(t.TempDir()+"/state.json"), r, e, now)
	t.Cleanup(s.Close)
	traceDir := t.TempDir()
	h := NewHTTP(s, &fakeIdentity{id: 1, role: "admin", status: "active"}, "https://admin.example", traceDir, now)
	return httptest.NewServer(h), r, e, traceDir
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

func readinessRequest(t *testing.T, url, targetVersion string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url+"/api/v1/admin/system/host-update/readiness?target_version="+targetVersion, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("X-Admin-UI-Request", "1")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if mutate != nil {
		mutate(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestHTTPReadinessReportsReadyWithoutOperation(t *testing.T) {
	ts, resolver, executor := admissionServer(t)
	defer ts.Close()

	res := readinessRequest(t, ts.URL, "1.2.3", nil)
	defer res.Body.Close()
	var response struct {
		Code int       `json:"code"`
		Data Readiness `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || response.Code != 0 || !response.Data.Ready || response.Data.TargetVersion != "1.2.3" {
		t.Fatalf("status=%d response=%#v", res.StatusCode, response)
	}
	if resolver.calls != 1 || executor.calls != 0 {
		t.Fatalf("readiness invoked resolver=%d executor=%d", resolver.calls, executor.calls)
	}

	statusReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/system/host-update/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	statusReq.Header.Set("Authorization", "Bearer valid")
	statusReq.Header.Set("X-Admin-UI-Request", "1")
	statusRes, err := http.DefaultClient.Do(statusReq)
	if err != nil {
		t.Fatal(err)
	}
	defer statusRes.Body.Close()
	var status struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(statusRes.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if statusRes.StatusCode != http.StatusOK || status.Code != 0 || string(status.Data) != "null" {
		t.Fatalf("status=%d response=%#v", statusRes.StatusCode, status)
	}
}

func TestHTTPReadinessRequiresTargetVersion(t *testing.T) {
	ts, resolver, executor := admissionServer(t)
	defer ts.Close()

	res := readinessRequest(t, ts.URL, "", nil)
	if res.StatusCode != http.StatusBadRequest || responseCode(t, res) != codeConfirmationRequired {
		t.Fatalf("status=%d", res.StatusCode)
	}
	if resolver.calls != 0 || executor.calls != 0 {
		t.Fatalf("invalid request invoked resolver=%d executor=%d", resolver.calls, executor.calls)
	}
}

func TestHTTPReadinessReportsMissingCandidate(t *testing.T) {
	ts, resolver, executor := admissionServer(t)
	defer ts.Close()
	resolver.err = ErrCandidateNotReady

	res := readinessRequest(t, ts.URL, "1.2.3", nil)
	defer res.Body.Close()
	var response struct {
		Code int       `json:"code"`
		Data Readiness `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || response.Code != 0 || response.Data.Ready || response.Data.Reason != "candidate_not_ready" {
		t.Fatalf("status=%d response=%#v", res.StatusCode, response)
	}
	if resolver.calls != 1 || executor.calls != 0 {
		t.Fatalf("missing candidate invoked resolver=%d executor=%d", resolver.calls, executor.calls)
	}
}

func TestHTTPReadinessRequiresActiveAdmin(t *testing.T) {
	ts, _, _ := admissionServer(t)
	defer ts.Close()
	res := readinessRequest(t, ts.URL, "1.2.3", func(r *http.Request) { r.Header.Del("Authorization") })
	if res.StatusCode != http.StatusUnauthorized || responseCode(t, res) != codeAuthRequired {
		t.Fatalf("status=%d", res.StatusCode)
	}

	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")), &fakeResolver{}, &fakeExecutor{})
	t.Cleanup(service.Close)
	handler := NewHTTP(service, &fakeIdentity{id: 1, role: "user", status: "active"}, "https://admin.example", "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/host-update/readiness?target_version=1.2.3", nil)
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("X-Admin-UI-Request", "1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-admin status=%d", rr.Code)
	}
	if got := responseCodeFromBody(t, rr.Body); got != codeForbidden {
		t.Fatalf("non-admin code=%q", got)
	}
}

func TestHTTPUpdateReturnsCandidateNotReady(t *testing.T) {
	ts, resolver, _ := admissionServer(t)
	defer ts.Close()
	resolver.err = ErrCandidateNotReady

	res := updateRequest(t, ts.URL, nil)
	if res.StatusCode != http.StatusConflict || responseCode(t, res) != "UPDATE_CANDIDATE_NOT_READY" {
		t.Fatalf("status=%d", res.StatusCode)
	}
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
		h := NewHTTP(s, identity, "https://admin.example", t.TempDir(), time.Now)
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

func TestHTTPStatusAllowsBrowserGETWithoutOrigin(t *testing.T) {
	ts, _, _ := admissionServer(t)
	defer ts.Close()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/system/host-update/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("X-Admin-UI-Request", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d code=%s", res.StatusCode, responseCodeFromBody(t, res.Body))
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil || envelope.Code != 0 {
		t.Fatalf("envelope=%#v err=%v", envelope, err)
	}
}

func TestHTTPStatusReportsTraceEvents(t *testing.T) {
	ts, _, _, traceDir := admissionServerWithTrace(t)
	defer ts.Close()
	// Admit an operation so status has an operation ID to look up.
	res := updateRequest(t, ts.URL, nil)
	var admitted struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&admitted); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	tracePath := filepath.Join(traceDir, "trace-"+admitted.Data.OperationID+".log")
	if err := os.WriteFile(tracePath, []byte("inspect\nbackup-db\nhealth\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/system/host-update/status", nil)
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("X-Admin-UI-Request", "1")
	statusRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer statusRes.Body.Close()
	var envelope struct {
		Data struct {
			Stage  string   `json:"stage"`
			Events []string `json:"events"`
		} `json:"data"`
	}
	if err := json.NewDecoder(statusRes.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	want := []string{"inspect", "backup-db", "health"}
	if len(envelope.Data.Events) != len(want) {
		t.Fatalf("events = %#v, want %v", envelope.Data.Events, want)
	}
	for i, event := range want {
		if envelope.Data.Events[i] != event {
			t.Fatalf("events[%d] = %q, want %q", i, envelope.Data.Events[i], event)
		}
	}
}

func TestHTTPPrepareCandidateRequiresAdminAndReturnsPreparationState(t *testing.T) {
	preparer := &fakeCandidatePreparer{}
	service := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &fakeResolver{}, &fakeExecutor{}, preparer)
	t.Cleanup(service.Close)
	handler := NewHTTP(service, &fakeIdentity{id: 1, role: "admin", status: "active"}, "https://admin.example", "")
	ts := httptest.NewServer(handler)
	defer ts.Close()

	request := func(body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/system/host-update/prepare", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer valid")
		req.Header.Set("Origin", "https://admin.example")
		req.Header.Set("X-Admin-UI-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	first := request(`{"target_version":"1.2.3"}`)
	defer first.Body.Close()
	var firstEnvelope struct {
		Code int                  `json:"code"`
		Data CandidatePreparation `json:"data"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstEnvelope); err != nil {
		t.Fatal(err)
	}
	if first.StatusCode != http.StatusAccepted || firstEnvelope.Code != 0 || firstEnvelope.Data.Stage != "preparing" {
		t.Fatalf("first status=%d response=%#v", first.StatusCode, firstEnvelope)
	}
	second := request(`{"target_version":"1.2.3"}`)
	defer second.Body.Close()
	if second.StatusCode != http.StatusAccepted || preparer.count() != 1 {
		t.Fatalf("second status=%d prepare calls=%d", second.StatusCode, preparer.count())
	}
}

func TestHTTPStatusExposesCandidatePreparationFailureReason(t *testing.T) {
	preparer := &fakeCandidatePreparer{err: errors.New("candidate image pull failed")}
	service := NewServiceWithPreparer(NewStore(filepath.Join(t.TempDir(), "state.json")), &fakeResolver{}, &fakeExecutor{}, preparer)
	t.Cleanup(service.Close)
	handler := NewHTTP(service, &fakeIdentity{id: 1, role: "admin", status: "active"}, "https://admin.example", "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/host-update/prepare", strings.NewReader(`{"target_version":"1.2.3"}`))
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("Origin", "https://admin.example")
	req.Header.Set("X-Admin-UI-Request", "1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("prepare status=%d body=%s", rr.Code, rr.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for {
		statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/host-update/status", nil)
		statusReq.Header.Set("Authorization", "Bearer valid")
		statusReq.Header.Set("X-Admin-UI-Request", "1")
		statusRR := httptest.NewRecorder()
		handler.ServeHTTP(statusRR, statusReq)
		var envelope struct {
			Data struct {
				Candidate *CandidatePreparation `json:"candidate"`
			} `json:"data"`
		}
		if err := json.Unmarshal(statusRR.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.Candidate != nil && envelope.Data.Candidate.Stage == "failed" {
			if envelope.Data.Candidate.Reason != "candidate image pull failed" {
				t.Fatalf("candidate=%#v", envelope.Data.Candidate)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never exposed failure: %s", statusRR.Body.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHTTPPrepareCandidateReportsUnavailablePreparationCommand(t *testing.T) {
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")), &fakeResolver{}, &fakeExecutor{})
	t.Cleanup(service.Close)
	handler := NewHTTP(service, &fakeIdentity{id: 1, role: "admin", status: "active"}, "https://admin.example", "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/host-update/prepare", strings.NewReader(`{"target_version":"1.2.3"}`))
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("Origin", "https://admin.example")
	req.Header.Set("X-Admin-UI-Request", "1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable || responseCodeFromBody(t, rr.Body) != "UPDATE_CANDIDATE_PREPARATION_FAILED" {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func responseCodeFromBody(t *testing.T, body io.Reader) string {
	t.Helper()
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Code
}
