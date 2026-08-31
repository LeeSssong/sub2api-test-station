package notify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFeishuSenderSendsInteractiveCardAndNeverCallsUrgent(t *testing.T) {
	var paths []string
	sender := NewFeishuSender("https://open.feishu.invalid", UpstreamBalanceSecrets{
		AppID: "fake-app-id", AppSecret: "fake-app-secret", ChatID: "oc_fake_team", RecipientOpenIDs: []string{"ou-fake"},
	})
	sender.httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		body := `{"code":0,"tenant_access_token":"fake-token","expire":7200}`
		if strings.Contains(request.URL.Path, "/messages") {
			requestBody, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(requestBody), `"msg_type":"interactive"`) || !strings.Contains(string(requestBody), `"receive_id":"oc_fake_team"`) {
				t.Fatalf("unexpected send body shape")
			}
			body = `{"code":0,"data":{"message_id":"om_fake"}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	messageID, err := sender.Send(context.Background(), UpstreamBalanceCardInput{
		State: UpstreamBalanceCardStateZero, ValueUSD: 0, BaseURL: "https://zero.example",
		LoginAccount: "registry-user.invalid", LoginPassword: "fake-password-value", RecipientOpenIDs: []string{"ou-fake"},
		Accounts: []UpstreamBalanceCardAccount{{ID: 7, Name: "zero-account"}},
	})
	if err != nil || messageID != "om_fake" {
		t.Fatalf("Send() = %q, %v", messageID, err)
	}
	for _, path := range paths {
		if strings.Contains(path, "urgent_app") {
			t.Fatalf("P1 called urgency endpoint: %v", paths)
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
