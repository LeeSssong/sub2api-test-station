package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndReadinessBoundaries(t *testing.T) {
	t.Parallel()
	state := &Readiness{Database: fakePinger{}}
	handler := HealthHandler(state)
	for _, path := range []string{"/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		want := http.StatusOK
		if path == "/readyz" {
			want = http.StatusServiceUnavailable
		}
		if recorder.Code != want {
			t.Fatalf("%s status=%d want=%d", path, recorder.Code, want)
		}
	}
	state.MarkNativeSuccess()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready status=%d", recorder.Code)
	}
}

func TestReadinessFailsWhenDatabaseUnavailable(t *testing.T) {
	t.Parallel()
	state := &Readiness{Database: fakePinger{err: errors.New("down")}}
	state.MarkNativeSuccess()
	recorder := httptest.NewRecorder()
	HealthHandler(state).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestBootstrapNativeReadinessMarksOnlySuccessfulSync(t *testing.T) {
	t.Parallel()
	ready := &Readiness{Database: fakePinger{}}
	if err := BootstrapNativeReadiness(context.Background(), func(context.Context) error { return nil }, ready); err != nil {
		t.Fatal(err)
	}
	if !ready.Ready(context.Background()) {
		t.Fatal("successful bootstrap did not mark readiness")
	}

	notReady := &Readiness{Database: fakePinger{}}
	wantErr := errors.New("native unavailable")
	if err := BootstrapNativeReadiness(context.Background(), func(context.Context) error { return wantErr }, notReady); !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
	if notReady.Ready(context.Background()) {
		t.Fatal("failed bootstrap marked readiness")
	}
}

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }
