package notify

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDeliveryRetryServiceSendsClaimedSafePayloadWithOriginalIdentity(t *testing.T) {
	payload, err := RenderAlert(IncidentView{Title: "公开分组不可用", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &retryRepository{claims: []*RetryDelivery{{
		ID: 41, IncidentKey: "group:GPT-Plus:availability", Severity: "P0",
		OccurrenceNo: 3, Transition: "confirmed", Payload: payload,
	}}}
	client := resultClientFunc(func(_ context.Context, message FeishuMessage) (SendResult, error) {
		if message.OccurrenceNo != 3 || message.Transition != "confirmed" || message.Severity != "P0" {
			t.Fatalf("retried message=%#v", message)
		}
		return SendResult{
			MessageID: "om-retry", ResponseCode: http.StatusOK,
			Payload:      []byte(`{"content":"<at id=ou-secret></at>"}`),
			UrgentStatus: "delivered", UrgentResponseCode: http.StatusOK,
		}, nil
	})
	service := DeliveryRetryService{
		Repository: repository,
		Client:     client,
		Clock:      func() time.Time { return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC) },
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.outcomes) != 1 || repository.outcomes[0].id != 41 ||
		repository.outcomes[0].outcome.Status != "delivered" ||
		!bytes.Equal(repository.outcomes[0].outcome.Payload, payload) {
		t.Fatalf("outcomes=%#v", repository.outcomes)
	}
}

func TestDeliveryRetryServiceRecordsFailureWithoutPersistingClientPayload(t *testing.T) {
	payload, err := RenderAlert(IncidentView{Title: "公开分组不可用", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &retryRepository{claims: []*RetryDelivery{{
		ID: 42, IncidentKey: "group:GPT-Plus:availability", Severity: "P0",
		OccurrenceNo: 1, Transition: "confirmed", Payload: payload,
	}}}
	service := DeliveryRetryService{
		Repository: repository,
		Client: resultClientFunc(func(context.Context, FeishuMessage) (SendResult, error) {
			return SendResult{}, errors.New("temporary Feishu failure")
		}),
	}

	if err := service.Run(context.Background()); err == nil {
		t.Fatal("expected retry failure")
	}
	if len(repository.outcomes) != 1 || repository.outcomes[0].outcome.Status != "failed" ||
		!bytes.Equal(repository.outcomes[0].outcome.Payload, payload) {
		t.Fatalf("outcomes=%#v", repository.outcomes)
	}
}

type retryOutcome struct {
	id      int64
	outcome DeliveryOutcome
}

type retryRepository struct {
	claims   []*RetryDelivery
	outcomes []retryOutcome
}

func (r *retryRepository) ClaimNotificationRetry(context.Context, time.Time) (*RetryDelivery, error) {
	if len(r.claims) == 0 {
		return nil, nil
	}
	claim := r.claims[0]
	r.claims = r.claims[1:]
	return claim, nil
}

func (r *retryRepository) FinishNotification(_ context.Context, id int64, outcome DeliveryOutcome) error {
	r.outcomes = append(r.outcomes, retryOutcome{id: id, outcome: outcome})
	return nil
}
