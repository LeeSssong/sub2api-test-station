package pricingevents

import (
	"context"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreampricing"
)

func TestServiceInitializesBaselineThenSendsChangedMultiplierOnce(t *testing.T) {
	t.Parallel()
	value := .05
	source := &multiplierSource{projection: multiplierProjection(20, &value, "ok")}
	baselines := &baselineRepository{items: map[string]store.Baseline{}}
	sender := &eventSender{}
	decisions := &decisionRecorder{}
	service := multiplierService(source, baselines, sender, decisions)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.identities) != 0 {
		t.Fatalf("first value sent: %#v", sender.identities)
	}
	value = .10
	source.projection = multiplierProjection(20, &value, "ok")
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.identities) != 1 {
		t.Fatalf("events = %#v", sender.identities)
	}
	identity := sender.identities[0]
	if identity.Family != "pricing_notice" ||
		identity.SourceKind != "account_multiplier" ||
		!strings.HasPrefix(identity.Key, "pricing:account:20:") {
		t.Fatalf("identity = %#v", identity)
	}
	if sender.messages[0].Severity != "P2" ||
		sender.messages[0].OccurrenceNo != 0 ||
		sender.messages[0].Transition != "" {
		t.Fatalf("message = %#v", sender.messages[0])
	}
}

func TestServiceSuppressesMappedMultiplierCoveredByProductionPricing(t *testing.T) {
	t.Parallel()
	value := .05
	source := &multiplierSource{projection: multiplierProjection(20, &value, "ok")}
	baselines := &baselineRepository{items: map[string]store.Baseline{}}
	sender := &eventSender{}
	decisions := &decisionRecorder{}
	service := multiplierService(source, baselines, sender, decisions)
	service.Resolver = fixedResolver{resolution: upstreampricing.Resolution{
		PricingURL: "https://neko.example/pricing", Multiplier: .10,
	}, found: true}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	value = .10
	source.projection = multiplierProjection(20, &value, "ok")
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.identities) != 0 {
		t.Fatalf("covered multiplier sent duplicate event: %#v", sender.identities)
	}
	last := decisions.records[len(decisions.records)-1]
	if last.Decision != "suppressed" || last.Reason != "covered_by_production_pricing" {
		t.Fatalf("decision = %#v", last)
	}
	if got := baselines.items["multiplier:account:20"].CurrentValue; got != "0.1" {
		t.Fatalf("baseline = %q", got)
	}
}

func TestServiceShadowRecordsWouldDeliverWithoutSending(t *testing.T) {
	t.Parallel()
	value := .05
	source := &multiplierSource{projection: multiplierProjection(20, &value, "ok")}
	baselines := &baselineRepository{items: map[string]store.Baseline{}}
	sender := &eventSender{}
	decisions := &decisionRecorder{}
	service := multiplierService(source, baselines, sender, decisions)
	service.Policy.Mode = notificationpolicy.ModeShadow
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	value = .10
	source.projection = multiplierProjection(20, &value, "ok")
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.identities) != 0 {
		t.Fatalf("shadow sent event: %#v", sender.identities)
	}
	last := decisions.records[len(decisions.records)-1]
	if last.Decision != "shadow_would_deliver" ||
		!strings.HasPrefix(last.Reason, "pricing:account:20:") {
		t.Fatalf("decision = %#v", last)
	}
}

func TestServiceDisabledDoesNotCreateBaselineOrEvent(t *testing.T) {
	t.Parallel()
	value := .05
	source := &multiplierSource{projection: multiplierProjection(20, &value, "ok")}
	baselines := &baselineRepository{items: map[string]store.Baseline{}}
	sender := &eventSender{}
	decisions := &decisionRecorder{}
	service := multiplierService(source, baselines, sender, decisions)
	service.Policy.Feishu.PricingNoticeEnabled = false
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(baselines.items) != 0 || len(sender.identities) != 0 || len(decisions.records) != 0 {
		t.Fatalf("disabled service changed state: baselines=%#v events=%#v decisions=%#v",
			baselines.items, sender.identities, decisions.records)
	}
}

func TestServiceIgnoresUntrustworthyMultiplier(t *testing.T) {
	t.Parallel()
	value := .05
	source := &multiplierSource{projection: multiplierProjection(20, &value, "failed")}
	baselines := &baselineRepository{items: map[string]store.Baseline{}}
	service := multiplierService(source, baselines, &eventSender{}, &decisionRecorder{})
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(baselines.items) != 0 {
		t.Fatalf("untrustworthy multiplier created baseline: %#v", baselines.items)
	}
}

type accountReader struct {
	accounts []sub2api.Account
}

func (reader accountReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return reader.accounts, nil
}

type multiplierSource struct {
	projection sub2api.AccountMonitorProjection
}

func (source *multiplierSource) ListAccountMonitors(
	context.Context,
) (sub2api.AccountMonitorProjection, error) {
	return source.projection, nil
}

type baselineRepository struct {
	items map[string]store.Baseline
}

func (repository *baselineRepository) GetOperationalBaseline(
	_ context.Context,
	key string,
) (store.Baseline, bool, error) {
	baseline, found := repository.items[key]
	return baseline, found, nil
}

func (repository *baselineRepository) PutOperationalBaseline(
	_ context.Context,
	baseline store.Baseline,
) error {
	repository.items[baseline.Key] = baseline
	return nil
}

type eventSender struct {
	identities []notify.OneShotIdentity
	messages   []notify.FeishuMessage
}

func (sender *eventSender) SendOneShot(
	_ context.Context,
	identity notify.OneShotIdentity,
	message notify.FeishuMessage,
) error {
	sender.identities = append(sender.identities, identity)
	sender.messages = append(sender.messages, message)
	return nil
}

type decisionRecorder struct {
	records []store.DecisionRecord
}

func (recorder *decisionRecorder) RecordNotificationDecision(
	_ context.Context,
	record store.DecisionRecord,
) error {
	recorder.records = append(recorder.records, record)
	return nil
}

type fixedResolver struct {
	resolution upstreampricing.Resolution
	found      bool
}

func (resolver fixedResolver) Resolve(
	context.Context,
	string,
) (upstreampricing.Resolution, bool) {
	return resolver.resolution, resolver.found
}

func multiplierService(
	source *multiplierSource,
	baselines *baselineRepository,
	sender *eventSender,
	decisions *decisionRecorder,
) Service {
	return Service{
		Accounts: accountReader{accounts: []sub2api.Account{{
			ID: 20, Name: "Plus-A", Status: "active", Schedulable: true,
		}}},
		Multipliers: source, Baselines: baselines, Notifier: sender,
		Decisions: decisions,
		Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{PricingNoticeEnabled: true},
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
		},
	}
}

func multiplierProjection(
	accountID int64,
	value *float64,
	status string,
) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: accountID, Name: "Plus-A",
			Multiplier: sub2api.AccountMonitorMultiplier{
				Value: value, Source: "declared", Status: status,
			},
		}},
	}
}
