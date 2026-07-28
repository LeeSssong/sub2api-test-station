package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliverySenderDeduplicatesSuccessfulEvidenceAndRetriesFailure(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "webhook")
	if err := os.WriteFile(secret, []byte("https://open.feishu.example/hook/test"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository := &fakeDeliveryRepository{}
	status := http.StatusInternalServerError
	client := Client{WebhookFile: secret, Resolver: notifyResolver{}, HTTP: &http.Client{Transport: notifyTransport(func(request *http.Request) *http.Response {
		if strings.Contains(request.URL.String(), "test") {
			return notifyResponse(status, "")
		}
		return notifyResponse(http.StatusNoContent, "")
	})}}
	sender := DeliverySender{Client: client, Repository: repository}
	message := RenderFeishu(IncidentView{Title: "倍率变化", Results: []string{"0.07x -> 0.10x"}})
	if err := sender.SendIncident(context.Background(), "upstream:7:pricing", "hash-1", message); err == nil {
		t.Fatal("expected first delivery failure")
	}
	status = http.StatusNoContent
	if err := sender.SendIncident(context.Background(), "upstream:7:pricing", "hash-1", message); err != nil {
		t.Fatal(err)
	}
	if err := sender.SendIncident(context.Background(), "upstream:7:pricing", "hash-1", message); err != nil {
		t.Fatal(err)
	}
	if repository.reserveCalls != 3 || repository.delivered != 1 {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestDeliverySenderSeparatesOccurrencesAndPersistsAuditedOutcome(t *testing.T) {
	repository := &fakeDeliveryRepository{}
	client := &auditedFakeClient{}
	sender := DeliverySender{Client: client, Repository: repository}
	base := RenderAlert(IncidentView{Title: "P0｜公开分组不可用", Severity: "P0"})

	first := WithDeliveryIdentity(base, 1, "confirmed")
	if err := sender.SendIncident(context.Background(), "group:GPT-Plus:availability", "available:0/1", first); err != nil {
		t.Fatal(err)
	}
	if err := sender.SendIncident(context.Background(), "group:GPT-Plus:availability", "available:0/1", first); err != nil {
		t.Fatal(err)
	}
	second := WithDeliveryIdentity(base, 2, "confirmed")
	if err := sender.SendIncident(context.Background(), "group:GPT-Plus:availability", "available:0/1", second); err != nil {
		t.Fatal(err)
	}

	if client.calls != 2 || len(repository.outcomes) != 2 {
		t.Fatalf("client calls = %d outcomes = %#v", client.calls, repository.outcomes)
	}
	if len(repository.reservations) != 3 ||
		repository.reservations[0].OccurrenceNo != 1 ||
		repository.reservations[1].OccurrenceNo != 1 ||
		repository.reservations[2].OccurrenceNo != 2 {
		t.Fatalf("reservations = %#v", repository.reservations)
	}
	for _, outcome := range repository.outcomes {
		if outcome.Status != "delivered" || outcome.MessageID == "" ||
			outcome.UrgentStatus != "failed" || !json.Valid(outcome.Payload) {
			t.Fatalf("outcome = %#v", outcome)
		}
	}
}

type fakeDeliveryRepository struct {
	reserveCalls int
	delivered    int
	seen         map[string]bool
	reservations []Reservation
	outcomes     []DeliveryOutcome
}

func (r *fakeDeliveryRepository) ReserveNotification(_ context.Context, reservation Reservation) (int64, bool, error) {
	r.reserveCalls++
	r.reservations = append(r.reservations, reservation)
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[reservation.DedupKey] {
		return 1, false, nil
	}
	r.seen[reservation.DedupKey] = true
	return 1, true, nil
}
func (r *fakeDeliveryRepository) FinishNotification(_ context.Context, _ int64, outcome DeliveryOutcome) error {
	r.outcomes = append(r.outcomes, outcome)
	if outcome.Status == "delivered" {
		r.delivered++
	}
	if outcome.Status == "failed" {
		// A failed reservation may be retried; the real repository updates its row.
		for key := range r.seen {
			delete(r.seen, key)
		}
	}
	return nil
}

type auditedFakeClient struct {
	calls int
}

func (c *auditedFakeClient) Send(_ context.Context, _ FeishuMessage) error {
	c.calls++
	return nil
}

func (c *auditedFakeClient) SendWithResult(_ context.Context, message FeishuMessage) (SendResult, error) {
	c.calls++
	payload, err := message.CardJSON()
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{
		MessageID: "om-" + string(rune('0'+c.calls)), ResponseCode: http.StatusOK,
		Payload: payload, UrgentStatus: "failed",
	}, nil
}
