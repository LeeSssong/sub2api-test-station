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
		{name: "P0 second reminder", severity: "P0", level: 2, want: first.Add(45 * time.Minute), wantFound: true},
		{name: "P0 recurring reminder", severity: "P0", level: 3, want: first.Add(75 * time.Minute), wantFound: true},
		{name: "P1 initial", severity: "P1", level: 0, want: first.Add(15 * time.Minute), wantFound: true},
		{name: "P1 recurring reminder", severity: "P1", level: 1, want: first.Add(75 * time.Minute), wantFound: true},
		{name: "P1 second recurring reminder", severity: "P1", level: 2, want: first.Add(135 * time.Minute), wantFound: true},
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

func TestServiceRendersCurrentSnapshotWithSeverityElapsedTimeAndNextLevel(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 5, 0, 0, time.UTC)
	first := now.Add(-5 * time.Minute)
	repository := &fakeEscalationRepository{claims: []*Incident{{
		Key: "group:7:user-impact", Severity: "P0", OccurrenceNo: 3,
		EscalationLevel: 0, ClaimToken: "claim-1", FirstDeliveredAt: first,
		CurrentValue: `{"group_name":"GPT-Plus","headline":"全部请求持续失败",` +
			`"latest_fact":"最近 15 分钟 31 次请求全部失败。",` +
			`"capacity":"当前可用账号 1 / 3。","observed_at":"2026-07-28T04:05:00Z"}`,
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
	if sent.key != "group:7:user-impact" || sent.evidence != "occurrence:3:escalation:1" {
		t.Fatalf("delivery identity=%#v", sent)
	}
	if sent.message.Severity != "P0" || sent.message.OccurrenceNo != 3 || sent.message.Transition != "escalation_1" {
		t.Fatalf("message identity=%#v", sent.message)
	}
	if !strings.HasPrefix(sent.message.Card.Header.Title.Content, "再次提醒｜") ||
		!strings.Contains(sent.message.RenderedText(), "持续时间") ||
		!strings.Contains(sent.message.RenderedText(), "5 分钟") ||
		!strings.Contains(sent.message.RenderedText(), "31 次请求全部失败") ||
		!strings.Contains(sent.message.RenderedText(), "可用账号 1 / 3") {
		t.Fatalf("message=%s", sent.message.RenderedText())
	}
	for _, forbidden := range []string{
		"第 1 次提醒", "发生了什么", "用户影响", "已知线索",
	} {
		if strings.Contains(sent.message.RenderedText(), forbidden) {
			t.Fatalf("reminder contains cloned section %q: %s",
				forbidden, sent.message.RenderedText())
		}
	}
	if len(repository.results) != 1 || !repository.results[0].Succeeded ||
		repository.results[0].Level != 1 || repository.results[0].NextEscalationAt == nil ||
		repository.results[0].ClaimToken != "claim-1" ||
		!repository.results[0].NextEscalationAt.Equal(first.Add(15*time.Minute)) {
		t.Fatalf("results=%#v", repository.results)
	}
	secondEscalation, err := escalationMessage(Incident{
		Key: "group:7:user-impact", Severity: "P0", OccurrenceNo: 3,
		EscalationLevel: 1, ClaimToken: "claim-2", FirstDeliveredAt: first,
		CurrentValue: `{"group_name":"GPT-Plus","headline":"全部请求持续失败",` +
			`"latest_fact":"最新窗口仍全部失败。","capacity":"当前可用账号 1 / 3。"}`,
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
	repository := &fakeEscalationRepository{claims: []*Incident{{
		Key: "group:2:user-impact", Severity: "P1", OccurrenceNo: 1,
		EscalationLevel: 0, ClaimToken: "claim-1",
		FirstDeliveredAt: now.Add(-15 * time.Minute),
		CurrentValue: `{"group_name":"Public","headline":"部分请求持续失败",` +
			`"latest_fact":"错误仍在持续。","capacity":"当前可用账号 2 / 3。"}`,
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

func TestServiceRetriesInvalidLegacySnapshotWithoutSendingRawValue(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 15, 0, 0, time.UTC)
	repository := &fakeEscalationRepository{claims: []*Incident{{
		Key: "site:group:2:error_rate", Severity: "P1", OccurrenceNo: 1,
		EscalationLevel: 0, ClaimToken: "claim-legacy",
		FirstDeliveredAt: now.Add(-15 * time.Minute),
		CurrentValue:     `error_rate=0.2`,
	}}}
	sender := &fakeEscalationSender{}
	service := Service{
		Repository: repository, Sender: sender, Clock: func() time.Time { return now },
	}
	if err := service.Run(context.Background()); err == nil {
		t.Fatal("expected legacy snapshot decode failure")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("legacy value was sent: %#v", sender.sent)
	}
	if len(repository.results) != 1 ||
		repository.results[0].Succeeded ||
		!repository.results[0].RetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("results=%#v", repository.results)
	}
}

func TestServiceDoesNothingWhenRepositoryHasNoActiveClaim(t *testing.T) {
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
