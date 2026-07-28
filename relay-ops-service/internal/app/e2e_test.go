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
	"time"

	"example.invalid/relay-ops-service/internal/alerting"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/collection"
	"example.invalid/relay-ops-service/internal/groupimpact"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestCandidateCollectionPriceChangeDoesNotNotifyAndPaidProbeIsExplicit(t *testing.T) {
	database := openE2EDatabase(t)
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
	notifier := &fakeMessageSender{}
	collector := &collection.Collector{
		Repository: database,
		Fetcher: pricing.Fetcher{
			Client: &http.Client{Transport: pages}, Resolver: e2eResolver{},
		},
		Extractor: pricing.CompositeExtractor{}, Probes: probe,
		Notifier: notifier, Decisions: database,
		Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{PricingNoticeEnabled: true},
		},
	}
	source := collection.Source{ID: id, Name: name, Role: collection.RoleCandidate, BaseURL: "https://" + host + "/v1", PricingURL: "https://" + host + "/pricing", UsageURL: "https://" + host + "/usage", ProbeSecretRef: record.SecretRef.SecretRef, Enabled: true}
	ctx := context.Background()
	if err := collector.Run(ctx, source, false); err != nil {
		t.Fatal(err)
	}
	if err := collector.Run(ctx, source, false); err != nil {
		t.Fatal(err)
	}
	if err := collector.Run(ctx, source, false); err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 0 || probe.calls != 0 {
		t.Fatalf("messages=%d probes=%d", len(notifier.messages), probe.calls)
	}
	if err := collector.Run(ctx, source, true); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || len(notifier.messages) != 0 {
		t.Fatalf("probes=%d messages=%d", probe.calls, len(notifier.messages))
	}
}

func TestConsolidatedGroupIncidentLifecycleWithConciseReminder(t *testing.T) {
	database := openE2EDatabase(t)
	ctx := context.Background()
	testID := randomTestID(t)
	group := sub2api.Group{
		ID: time.Now().UnixNano(), Name: "公开分组-" + testID, Status: "active",
	}
	observedAt := time.Date(2026, 7, 29, 7, 0, 0, 0, time.UTC)
	reader := &e2eRuntimeReader{
		group:    group,
		window:   e2eRuntimeSnapshot(20, 18, .10, 1200),
		baseline: e2eRuntimeSnapshot(500, 490, .02, 1000),
	}
	capacity, err := groupimpact.CapacitySignal(
		group.Name,
		groupimpact.CapacityEvidence{Available: 2, Total: 3},
		observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertGroupSignal(ctx, capacity); err != nil {
		t.Fatal(err)
	}
	client := &capturedFeishuClient{}
	sender := notify.DeliverySender{Client: client, Repository: database}
	service := groupimpact.Service{
		Reader: reader, Signals: database,
		Incidents: incidents.Machine{
			Repository: database, Policy: incidents.DefaultPolicy(),
		},
		Notifier: sender, Decisions: database,
		Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{
				GroupRuntimeEnabled:  true,
				GroupCapacityEnabled: true,
			},
		},
		Now: func() time.Time { return observedAt },
	}
	for range 2 {
		if err := service.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(client.messages) != 1 ||
		!strings.HasPrefix(client.messages[0].Card.Header.Title.Content, "P1｜") {
		t.Fatalf("initial messages=%#v", client.messages)
	}

	reader.window = e2eRuntimeSnapshot(30, 26, .13, 1250)
	observedAt = observedAt.Add(time.Minute)
	if err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.messages) != 1 {
		t.Fatalf("numeric evidence update sent=%d", len(client.messages))
	}

	escalator := alerting.Service{
		Repository: database, Sender: sender,
		Clock: func() time.Time { return time.Now().UTC().Add(16 * time.Minute) },
	}
	if err := escalator.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.messages) != 2 ||
		!strings.HasPrefix(client.messages[1].Card.Header.Title.Content, "再次提醒｜") {
		t.Fatalf("reminder messages=%#v", client.messages)
	}
	reminderText := client.messages[1].RenderedText()
	for _, want := range []string{"持续时间", "尚未有人确认接手", "30"} {
		if !strings.Contains(reminderText, want) {
			t.Fatalf("reminder missing %q in %q", want, reminderText)
		}
	}
	for _, forbidden := range []string{"第 1 次提醒", "发生了什么", "用户影响", "建议处理"} {
		if strings.Contains(reminderText, forbidden) {
			t.Fatalf("reminder cloned %q: %s", forbidden, reminderText)
		}
	}

	observedAt = observedAt.Add(time.Minute)
	healthyCapacity, err := groupimpact.CapacitySignal(
		group.Name,
		groupimpact.CapacityEvidence{Available: 3, Total: 3},
		observedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertGroupSignal(ctx, healthyCapacity); err != nil {
		t.Fatal(err)
	}
	reader.window = e2eRuntimeSnapshot(20, 20, 0, 1000)
	if err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.messages) != 2 {
		t.Fatalf("first healthy observation sent=%d", len(client.messages))
	}
	observedAt = observedAt.Add(time.Minute)
	if err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.messages) != 3 ||
		!strings.HasPrefix(client.messages[2].Card.Header.Title.Content, "恢复｜") {
		t.Fatalf("recovery messages=%#v", client.messages)
	}
}

func TestReliableProductionPricingDiffDeliversOneOneShot(t *testing.T) {
	database := openE2EDatabase(t)
	ctx := context.Background()
	testID := randomTestID(t)
	host := "pricing-" + testID + ".example"
	upstreamID, err := database.CreateUpstream(ctx, store.Upstream{
		Name: "Production-" + testID, Role: "production",
		BaseURL: "https://" + host + "/v1", AdapterType: "openai", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pages := &pageSequence{bodies: []string{
		`{"multiplier":"0.07x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
		`{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
		`{"multiplier":"0.10x","models":[{"model":"gpt-a","input":"1","output":"8"}]}`,
	}}
	client := &capturedFeishuClient{}
	collector := collection.Collector{
		Repository: database,
		Fetcher: pricing.Fetcher{
			Client: &http.Client{Transport: pages}, Resolver: e2eResolver{},
		},
		Extractor: pricing.CompositeExtractor{},
		Notifier:  notify.OneShotSender{Client: client, Repository: database},
		Decisions: database,
		Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{PricingNoticeEnabled: true},
		},
	}
	source := collection.Source{
		ID: upstreamID, Name: "Production-" + testID,
		Role:       collection.RoleProduction,
		PricingURL: "https://" + host + "/pricing", Enabled: true,
	}
	for range 3 {
		if err := collector.Run(ctx, source, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(client.messages) != 1 {
		t.Fatalf("pricing messages=%#v", client.messages)
	}
	message := client.messages[0]
	if message.Severity != "P2" || message.OccurrenceNo != 0 ||
		message.Transition != "" ||
		!strings.HasPrefix(message.Card.Header.Title.Content, "价格变更｜") ||
		strings.Contains(message.RenderedText(), "确认并接手") {
		t.Fatalf("pricing one-shot=%#v", message)
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

type fakeMessageSender struct{ messages []notify.FeishuMessage }

func (s *fakeMessageSender) SendOneShot(
	_ context.Context,
	_ notify.OneShotIdentity,
	message notify.FeishuMessage,
) error {
	s.messages = append(s.messages, message)
	return nil
}

type e2eRuntimeReader struct {
	group    sub2api.Group
	window   sub2api.OpsSnapshot
	baseline sub2api.OpsSnapshot
}

func (reader *e2eRuntimeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return []sub2api.Group{reader.group}, nil
}

func (reader *e2eRuntimeReader) GetOpsSnapshot(
	_ context.Context,
	query sub2api.OpsQuery,
) (sub2api.OpsSnapshot, error) {
	if query.TimeRange == "24h" {
		return reader.baseline, nil
	}
	return reader.window, nil
}

func e2eRuntimeSnapshot(
	requests int64,
	successes int64,
	errorRate float64,
	ttftP95 float64,
) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{
		RequestCountTotal: requests,
		SuccessCount:      successes,
		ErrorRate:         errorRate,
		TTFT:              sub2api.Percentiles{P95MS: ttftP95},
	}}
}

type capturedFeishuClient struct {
	messages []notify.FeishuMessage
}

func (client *capturedFeishuClient) Send(
	_ context.Context,
	message notify.FeishuMessage,
) error {
	client.messages = append(client.messages, message)
	return nil
}

func openE2EDatabase(t *testing.T) *store.Store {
	t.Helper()
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
	t.Cleanup(database.Close)
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return database
}

func randomTestID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value)
}
