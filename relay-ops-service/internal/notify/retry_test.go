package notify

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDeliveryRetryServiceSendsClaimedSafePayloadWithOriginalIdentity(t *testing.T) {
	payload, err := RenderAlert(IncidentView{Title: "公开分组不可用", Severity: "P0"}).CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &retryRepository{claims: []*RetryDelivery{{
		Kind: "incident", ID: 41,
		IncidentKey: "group:GPT-Plus:availability", Severity: "P0",
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
		Kind: "incident", ID: 42,
		IncidentKey: "group:GPT-Plus:availability", Severity: "P0",
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

func TestDeliveryRetryServiceRetriesOneShotWithoutIncidentIdentity(t *testing.T) {
	message := RenderPricingNotice(PricingNoticeView{
		Upstream: "Neko",
		Change:   "公开计费倍率由 0.07x 变为 0.10x。",
		Review:   "核对售价与毛利。",
	})
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	repository := &retryRepository{claims: []*RetryDelivery{{
		Kind: "one_shot", ID: 51,
		NotificationKey: "pricing:7:semantic-change",
		Severity:        "P2", Payload: payload,
	}}}
	client := resultClientFunc(func(_ context.Context, retried FeishuMessage) (SendResult, error) {
		if retried.OccurrenceNo != 0 || retried.Transition != "" ||
			strings.Contains(retried.RenderedText(), "确认并接手") {
			t.Fatalf("one-shot gained incident lifecycle: %#v", retried)
		}
		return SendResult{
			MessageID: "om-pricing-retry", ResponseCode: http.StatusOK,
			UrgentStatus: "not_supported",
		}, nil
	})
	service := DeliveryRetryService{Repository: repository, Client: client}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.outcomes) != 0 ||
		len(repository.oneShotOutcomes) != 1 ||
		repository.oneShotOutcomes[0].id != 51 ||
		repository.oneShotOutcomes[0].outcome.Status != "delivered" {
		t.Fatalf("incident=%#v one-shot=%#v",
			repository.outcomes, repository.oneShotOutcomes)
	}
}

type retryOutcome struct {
	id      int64
	outcome DeliveryOutcome
}

type retryRepository struct {
	claims          []*RetryDelivery
	outcomes        []retryOutcome
	oneShotOutcomes []retryOutcome
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

func (r *retryRepository) FinishOneShot(_ context.Context, id int64, outcome DeliveryOutcome) error {
	r.oneShotOutcomes = append(r.oneShotOutcomes, retryOutcome{id: id, outcome: outcome})
	return nil
}
