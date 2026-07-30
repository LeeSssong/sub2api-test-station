package notify

import (
	"context"
	"strings"
	"testing"
)

func TestOneShotSenderDoesNotAddAcknowledgementOrIncidentIdentity(t *testing.T) {
	repository := &fakeOneShotRepository{}
	client := &oneShotClient{}
	sender := OneShotSender{Client: client, Repository: repository}
	message := RenderPricingNotice(PricingNoticeView{
		Upstream: "Neko",
		Change:   "公开价格由 0.07 元变为 0.08 元。",
		Review:   "核对售价与毛利。",
	})

	err := sender.SendOneShot(context.Background(), OneShotIdentity{
		Key: "pricing:7:hash", Family: "pricing_notice",
		PolicyVersion: 1, SourceKind: "public_pricing",
	}, message)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || len(repository.reservations) != 1 || len(repository.outcomes) != 1 {
		t.Fatalf("client=%d reservations=%d outcomes=%d",
			client.calls, len(repository.reservations), len(repository.outcomes))
	}
	reservation := repository.reservations[0]
	if reservation.NotificationKey != "pricing:7:hash" ||
		reservation.Family != "pricing_notice" ||
		reservation.PolicyVersion != 1 ||
		reservation.SourceKind != "public_pricing" ||
		reservation.DedupKey == "" || reservation.MessageHash == "" {
		t.Fatalf("reservation = %#v", reservation)
	}
	if strings.Contains(string(reservation.Payload), "确认并接手") ||
		strings.Contains(client.message.RenderedText(), "确认并接手") {
		t.Fatal("one-shot notification gained an incident acknowledgement")
	}
	assertReminderOnlyCard(t, client.message)
	if client.message.OccurrenceNo != 0 || client.message.Transition != "" {
		t.Fatalf("one-shot gained incident identity: %#v", client.message)
	}
}

func TestOneShotSenderSkipsExistingReservation(t *testing.T) {
	repository := &fakeOneShotRepository{alreadyExists: true}
	client := &oneShotClient{}
	sender := OneShotSender{Client: client, Repository: repository}

	err := sender.SendOneShot(context.Background(), OneShotIdentity{
		Key: "daily-digest:2026-07-29", Family: "daily_digest",
		PolicyVersion: 1, SourceKind: "daily_report",
	}, RenderFeishu(IncidentView{Title: "日报", Severity: "P2"}))
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 || len(repository.outcomes) != 0 {
		t.Fatalf("duplicate sent: client=%d outcomes=%d", client.calls, len(repository.outcomes))
	}
}

type fakeOneShotRepository struct {
	alreadyExists bool
	reservations  []OneShotReservation
	outcomes      []DeliveryOutcome
}

func (repository *fakeOneShotRepository) ReserveOneShot(_ context.Context, reservation OneShotReservation) (int64, bool, error) {
	repository.reservations = append(repository.reservations, reservation)
	return 17, !repository.alreadyExists, nil
}

func (repository *fakeOneShotRepository) FinishOneShot(_ context.Context, _ int64, outcome DeliveryOutcome) error {
	repository.outcomes = append(repository.outcomes, outcome)
	return nil
}

type oneShotClient struct {
	calls   int
	message FeishuMessage
}

func (client *oneShotClient) Send(_ context.Context, message FeishuMessage) error {
	client.calls++
	client.message = message
	return nil
}
