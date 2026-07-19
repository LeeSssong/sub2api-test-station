package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/store"
)

func TestCandidateCollectionPriceChangeIsNotifiedOnceAndPaidProbeIsExplicit(t *testing.T) {
	databaseURL := os.Getenv("RELAY_OPS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("RELAY_OPS_TEST_DATABASE_URL is not set")
	}
	secret := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(secret, []byte(databaseURL), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), secret)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	testID := randomTestID(t)
	name := "e2e-candidate-" + testID
	host := "candidate-" + testID + ".example"
	record := candidates.CreateRecord{Candidate: candidates.Candidate{Name: name, BaseURL: "https://" + host + "/v1", PricingURL: "https://" + host + "/pricing", UsageURL: "https://" + host + "/usage", ProbeSecretRef: "file:/run/secrets/e2e"}, SecretRef: candidates.SecretRef{SecretRef: "file:/run/secrets/e2e-" + name, Kind: "candidate_probe_key", OwnerScope: name, Fingerprint: name, LastFour: "e2e1"}, Audit: candidates.AuditEvent{ActorUserID: 1, Action: "candidate.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": name}}}
	record.Candidate.ProbeSecretRef = record.SecretRef.SecretRef
	id, err := database.CreateCandidate(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	pages := &pageSequence{bodies: []string{`{"multiplier":"0.07x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`, `{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`, `{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`, `{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`}}
	probe := &fakeProbeRunner{runID: "e2e-probe-" + testID}
	analysis := &fakeAnalysisRunner{}
	notifier := &fakeMessageSender{}
	collector := &candidateCollector{Store: database, Fetcher: pricing.Fetcher{Client: &http.Client{Transport: pages}, Resolver: e2eResolver{}}, Extractor: pricing.CompositeExtractor{}, Probes: probe, Incidents: &incidents.Machine{Repository: database, Policy: incidents.DefaultPolicy()}, Agent: analysis, Notifier: notifier}
	ctx := context.Background()
	if err := collector.Run(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if err := collector.Run(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if err := collector.Run(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if analysis.calls != 1 || len(notifier.messages) != 1 || probe.calls != 0 {
		t.Fatalf("analysis=%d messages=%d probes=%d", analysis.calls, len(notifier.messages), probe.calls)
	}
	if !strings.Contains(notifier.messages[0].Content.Text, "0.07x") || !strings.Contains(notifier.messages[0].Content.Text, "0.1x") {
		t.Fatalf("message=%s", notifier.messages[0].Content.Text)
	}
	if err := collector.Run(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || len(notifier.messages) != 1 {
		t.Fatalf("probes=%d messages=%d", probe.calls, len(notifier.messages))
	}
}

type pageSequence struct {
	mu     sync.Mutex
	bodies []string
	index  int
}

func (s *pageSequence) RoundTrip(*http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.index
	if index >= len(s.bodies) {
		index = len(s.bodies) - 1
	}
	s.index++
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(s.bodies[index]))}, nil
}

type e2eResolver struct{}

func (e2eResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.50")}}, nil
}

type fakeProbeRunner struct {
	calls int
	runID string
}

func (p *fakeProbeRunner) Watch(context.Context, candidates.Candidate) (probes.ProbeRun, error) {
	p.calls++
	return probes.ProbeRun{SchemaVersion: 1, RunID: p.runID, ChannelID: "candidate", Status: "passed"}, nil
}

type fakeAnalysisRunner struct{ calls int }

func (a *fakeAnalysisRunner) AnalyzeOnce(_ context.Context, contract agent.IncidentContractV1) (agent.Analysis, error) {
	a.calls++
	result := agent.Fallback(contract)
	result.Change = "公开倍率发生变化"
	result.Focus = "人工核对利润空间"
	return result, nil
}

type fakeMessageSender struct{ messages []notify.FeishuMessage }

func (s *fakeMessageSender) Send(_ context.Context, message notify.FeishuMessage) error {
	s.messages = append(s.messages, message)
	return nil
}

var _ domain.UpstreamID

func randomTestID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value)
}
