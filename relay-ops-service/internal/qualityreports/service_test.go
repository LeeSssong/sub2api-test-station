package qualityreports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/probes"
)

func TestBuildFromFastRunKeepsHardGatesAheadOfScores(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"schema_version":1,"run_id":"fast-1","channel_id":"candidate-17","profile_id":"quality-first-fast-v1","job_kind":"health_pulse","recorded_at":"2026-07-22T03:00:00Z","status":"failed","metrics":{"selected_models":["gpt-a"],"direct":{"request_count":2,"success_count":1,"success_rate":0.5},"gateway":{"status":"unknown"}},"errors":[{"stage":"gpt-a.stream","category":"incomplete_sse"}]}`)

	report, err := Build(domain.UpstreamID(17), "Candidate", probes.FastResult{
		RunID: "fast-1", JobKind: "health_pulse", Status: "failed", RecordedAt: now, Record: raw,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantHash := sha256.Sum256(raw)
	if report.Status != "blocked" || report.QualityScore != 0 || report.ReportHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("report = %#v", report)
	}
	if report.ExpiresAt != now.Add(30*time.Minute) || report.Gateway != "unknown" {
		t.Fatalf("report freshness/gateway = %#v", report)
	}
}

func TestServicePreviewIsHashBoundFreshAndWriteFree(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 10, 0, 0, time.UTC)
	repository := &fakeRepository{report: Report{
		ReportID: "fast-1", ReportHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status: "needs_evidence", ExpiresAt: now.Add(time.Minute),
	}}
	service := Service{Repository: repository, Clock: func() time.Time { return now }}

	preview, err := service.Preview(context.Background(), domain.AdminActor{UserID: 1}, "fast-1", repository.report.ReportHash)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Status != "dry_run" || preview.Writes != 0 || preview.ReportID != "fast-1" {
		t.Fatalf("preview = %#v", preview)
	}
	if repository.puts != 0 {
		t.Fatalf("preview performed %d writes", repository.puts)
	}

	_, err = service.Preview(context.Background(), domain.AdminActor{UserID: 1}, "fast-1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if !errors.Is(err, ErrStale) {
		t.Fatalf("mismatch error = %v", err)
	}
}

type fakeRepository struct {
	report Report
	puts   int
}

func (f *fakeRepository) Get(context.Context, string) (Report, bool, error) {
	return f.report, true, nil
}

func (f *fakeRepository) Put(context.Context, Report) error {
	f.puts++
	return nil
}
