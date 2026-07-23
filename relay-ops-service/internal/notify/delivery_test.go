package notify

import (
	"context"
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

type fakeDeliveryRepository struct {
	reserveCalls int
	delivered    int
	seen         map[string]bool
}

func (r *fakeDeliveryRepository) ReserveNotification(_ context.Context, _ string, dedupKey, _ string) (int64, bool, error) {
	r.reserveCalls++
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[dedupKey] {
		return 1, false, nil
	}
	r.seen[dedupKey] = true
	return 1, true, nil
}
func (r *fakeDeliveryRepository) FinishNotification(_ context.Context, _ int64, status string, _ int) error {
	if status == "delivered" {
		r.delivered++
	}
	if status == "failed" {
		// A failed reservation may be retried; the real repository updates its row.
		for key := range r.seen {
			delete(r.seen, key)
		}
	}
	return nil
}
