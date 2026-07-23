package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/store"
)

func TestProductionCollectionDetectsMultiplierOnceWithoutPaidProbe(t *testing.T) {
	pages := &sequenceTransport{bodies: []string{
		`{"multiplier":"0.07x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
		`{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
		`{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
	}}
	repository := &fakeRepository{}
	notifier := &fakeNotifier{}
	probe := &fakeProbe{}
	collector := Collector{
		Repository: repository,
		Fetcher:    pricing.Fetcher{Client: &http.Client{Transport: pages}, Resolver: publicResolver{}},
		Extractor:  pricing.CompositeExtractor{},
		Incidents:  &fakeIncidents{},
		Notifier:   notifier,
		Probes:     probe,
	}
	source := Source{ID: 7, Name: "Neko", Role: RoleProduction, PricingURL: "https://neko.example/pricing", Enabled: true}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := collector.Run(ctx, source, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(repository.snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2 changed hashes", len(repository.snapshots))
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("notifications = %d, want one semantic change", len(notifier.messages))
	}
	if probe.calls != 0 {
		t.Fatalf("production probe calls = %d, want 0", probe.calls)
	}
}

func TestCandidateCollectionOnlyProbesWhenExplicitlyAllowed(t *testing.T) {
	repository := &fakeRepository{}
	probe := &fakeProbe{}
	collector := Collector{
		Repository: repository,
		Fetcher: pricing.Fetcher{Client: &http.Client{Transport: &sequenceTransport{bodies: []string{
			`{"multiplier":"0.05x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
		}}}, Resolver: publicResolver{}},
		Extractor: pricing.CompositeExtractor{}, Probes: probe,
	}
	source := Source{ID: 8, Name: "Candidate", Role: RoleCandidate, BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing", ProbeSecretRef: "file:/run/secrets/candidate", Enabled: true}
	if err := collector.Run(context.Background(), source, false); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 0 {
		t.Fatalf("read-only probe calls = %d", probe.calls)
	}
	if err := collector.Run(context.Background(), source, true); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("explicit probe calls = %d, want 1", probe.calls)
	}
}

func TestCollectionReparsesLegacySnapshotAndRecordsUnparseableEvidence(t *testing.T) {
	body := `<html><body><div>bash - 80x24</div><script>window.payload = "0x14x66x"</script></body></html>`
	hash := sha256.Sum256([]byte(body))
	repository := &fakeRepository{snapshots: []store.PricingSnapshot{{
		UpstreamID:     7,
		ContentHash:    hex.EncodeToString(hash[:]),
		NormalizedJSON: []byte(`{"models":null,"advertised_multiplier_bps":800000,"confidence":"common_html"}`),
	}}}
	collector := Collector{
		Repository: repository,
		Fetcher: pricing.Fetcher{Client: &http.Client{Transport: &sequenceTransport{
			bodies:      []string{body},
			contentType: "text/html",
		}}, Resolver: publicResolver{}},
		Extractor: pricing.CompositeExtractor{},
	}
	if err := collector.Run(context.Background(), Source{ID: 7, Name: "Neko", Role: RoleProduction, PricingURL: "https://neko.example/pricing", Enabled: true}, false); err != nil {
		t.Fatal(err)
	}
	if len(repository.snapshots) != 2 {
		t.Fatalf("snapshots = %d, want a corrected append-only snapshot", len(repository.snapshots))
	}
	var evidence map[string]any
	if err := json.Unmarshal(repository.snapshots[1].NormalizedJSON, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence["schema_version"] != "pricing-evidence-v2" || evidence["confidence"] != "unparseable" {
		t.Fatalf("corrected evidence = %#v", evidence)
	}
	if _, exists := evidence["advertised_multiplier_bps"]; exists || evidence["models"] != nil {
		t.Fatalf("unparseable page produced pricing evidence: %#v", evidence)
	}
}

type sequenceTransport struct {
	bodies      []string
	index       int
	contentType string
}

func (s *sequenceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := s.bodies[s.index]
	if s.index < len(s.bodies)-1 {
		s.index++
	}
	contentType := s.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

type publicResolver struct{}

func (publicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.11")}}, nil
}

type fakeRepository struct {
	snapshots []store.PricingSnapshot
	runs      int
}

func (r *fakeRepository) LatestPricingSnapshot(context.Context, domain.UpstreamID) (store.PricingSnapshot, bool, error) {
	if len(r.snapshots) == 0 {
		return store.PricingSnapshot{}, false, nil
	}
	return r.snapshots[len(r.snapshots)-1], true, nil
}
func (r *fakeRepository) AppendPricingSnapshot(_ context.Context, snapshot store.PricingSnapshot) (int64, error) {
	r.snapshots = append(r.snapshots, snapshot)
	return int64(len(r.snapshots)), nil
}
func (r *fakeRepository) AppendProbeRun(context.Context, domain.UpstreamID, probes.ProbeRun, time.Time) error {
	r.runs++
	return nil
}

type fakeIncidents struct{}

func (fakeIncidents) Observe(context.Context, incidents.Observation) (incidents.Transition, error) {
	return incidents.Transition{Notify: true, State: "confirmed", Kind: "confirmed"}, nil
}

type fakeNotifier struct{ messages []notify.FeishuMessage }

func (n *fakeNotifier) SendIncident(_ context.Context, _, _ string, message notify.FeishuMessage) error {
	n.messages = append(n.messages, message)
	return nil
}

type fakeProbe struct{ calls int }

func (p *fakeProbe) Watch(context.Context, candidates.Candidate) (probes.ProbeRun, error) {
	p.calls++
	return probes.ProbeRun{RunID: "probe", Status: "passed"}, nil
}
