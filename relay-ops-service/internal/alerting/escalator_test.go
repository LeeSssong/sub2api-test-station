package alerting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/notify"
)

func TestNextEscalationAtUsesBoundedOriginalIncidentClock(t *testing.T) {
	first := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		severity  string
		level     int
		want      time.Time
		wantFound bool
	}{
		{name: "P0 initial", severity: "P0", level: 0, want: first.Add(5 * time.Minute), wantFound: true},
		{name: "P0 first escalation", severity: "P0", level: 1, want: first.Add(15 * time.Minute), wantFound: true},
		{name: "P0 complete", severity: "P0", level: 2},
		{name: "P1 initial", severity: "P1", level: 0, want: first.Add(15 * time.Minute), wantFound: true},
		{name: "P1 complete", severity: "P1", level: 1},
		{name: "P2", severity: "P2", level: 0},
		{name: "recovery", severity: "", level: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, found := NextEscalationAt(test.severity, test.level, first)
			if found != test.wantFound || found && !got.Equal(test.want) {
				t.Fatalf("NextEscalationAt(%q, %d) = %s, %v; want %s, %v",
					test.severity, test.level, got, found, test.want, test.wantFound)
			}
		})
	}
}

func TestServiceResendsAuditedCardWithSeverityElapsedTimeAndNextLevel(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 5, 0, 0, time.UTC)
	first := now.Add(-5 * time.Minute)
	payload, err := notify.RenderAlert(notify.IncidentView{Title: "公开分组不可用", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeEscalationRepository{claims: []*Incident{{
		Key: "group:GPT-Plus:availability", Severity: "P0", OccurrenceNo: 3,
		EscalationLevel: 0, FirstDeliveredAt: first, MessagePayload: payload,
	}}}
	sender := &fakeEscalationSender{}
	service := Service{Repository: repository, Sender: sender, Clock: func() time.Time { return now }}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent=%#v", sender.sent)
	}
	sent := sender.sent[0]
	if sent.key != "group:GPT-Plus:availability" || sent.evidence != "occurrence:3:escalation:1" {
		t.Fatalf("delivery identity=%#v", sent)
	}
	if sent.message.Severity != "P0" || sent.message.OccurrenceNo != 3 || sent.message.Transition != "escalation_1" {
		t.Fatalf("message identity=%#v", sent.message)
	}
	if !strings.HasPrefix(sent.message.Card.Header.Title.Content, "再次提醒｜") ||
		!strings.Contains(sent.message.RenderedText(), "持续时间") ||
		!strings.Contains(sent.message.RenderedText(), "5 分钟") {
		t.Fatalf("message=%s", sent.message.RenderedText())
	}
	if len(repository.results) != 1 || !repository.results[0].Succeeded ||
		repository.results[0].Level != 1 || repository.results[0].NextEscalationAt == nil ||
		!repository.results[0].NextEscalationAt.Equal(first.Add(15*time.Minute)) {
		t.Fatalf("results=%#v", repository.results)
	}
	firstEscalationPayload, err := sent.message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondEscalation, err := escalationMessage(Incident{
		Key: "group:GPT-Plus:availability", Severity: "P0", OccurrenceNo: 3,
		EscalationLevel: 1, FirstDeliveredAt: first, MessagePayload: firstEscalationPayload,
	}, 2, first.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(secondEscalation.RenderedText(), "**持续时间**") != 1 ||
		strings.Count(secondEscalation.Card.Header.Title.Content, "再次提醒｜") != 1 {
		t.Fatalf("escalation metadata accumulated: %s", secondEscalation.RenderedText())
	}
}

func TestServiceRetriesFailedLevelAfterOneMinuteWithoutAdvancing(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 15, 0, 0, time.UTC)
	payload, err := notify.RenderAlert(notify.IncidentView{Title: "错误率升高", Severity: "P1"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeEscalationRepository{claims: []*Incident{{
		Key: "site:group:2:error_rate", Severity: "P1", OccurrenceNo: 1,
		EscalationLevel: 0, FirstDeliveredAt: now.Add(-15 * time.Minute), MessagePayload: payload,
	}}}
	sender := &fakeEscalationSender{err: errors.New("send failed")}
	service := Service{Repository: repository, Sender: sender, Clock: func() time.Time { return now }}
	if err := service.Run(context.Background()); err == nil {
		t.Fatal("expected send failure")
	}
	if len(repository.results) != 1 || repository.results[0].Succeeded ||
		repository.results[0].Level != 1 || !repository.results[0].RetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("results=%#v", repository.results)
	}
}

func TestServiceDoesNothingWhenRepositoryHasNoUnacknowledgedActiveClaim(t *testing.T) {
	sender := &fakeEscalationSender{}
	service := Service{
		Repository: &fakeEscalationRepository{},
		Sender:     sender,
		Clock:      func() time.Time { return time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC) },
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sent=%#v", sender.sent)
	}
}

type fakeEscalationRepository struct {
	claims  []*Incident
	results []Result
}

func (repository *fakeEscalationRepository) ClaimDueEscalation(_ context.Context, _ time.Time) (*Incident, error) {
	if len(repository.claims) == 0 {
		return nil, nil
	}
	claim := repository.claims[0]
	repository.claims = repository.claims[1:]
	return claim, nil
}

func (repository *fakeEscalationRepository) FinishEscalation(_ context.Context, result Result) error {
	repository.results = append(repository.results, result)
	return nil
}

type sentEscalation struct {
	key      string
	evidence string
	message  notify.FeishuMessage
}

type fakeEscalationSender struct {
	sent []sentEscalation
	err  error
}

func (sender *fakeEscalationSender) SendIncident(_ context.Context, key, evidence string, message notify.FeishuMessage) error {
	sender.sent = append(sender.sent, sentEscalation{key: key, evidence: evidence, message: message})
	return sender.err
}
