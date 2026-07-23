package ops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppBotAlerterSendsInteractiveCardToConfiguredChat(t *testing.T) {
	var messageCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["app_id"] != "test-app-id" || body["app_secret"] != "test-app-secret" {
				t.Fatalf("unexpected token request: %#v", body)
			}
			fmt.Fprint(w, `{"code":0,"tenant_access_token":"test-tenant-token","expire":7200}`)
		case "/open-apis/im/v1/messages":
			messageCalls++
			if r.URL.Query().Get("receive_id_type") != "chat_id" || r.Header.Get("Authorization") != "Bearer test-tenant-token" {
				t.Fatalf("unexpected message request: %s %#v", r.URL.String(), r.Header)
			}
			var body struct {
				ReceiveID string `json:"receive_id"`
				MsgType   string `json:"msg_type"`
				Content   string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ReceiveID != "test-alert-chat" || body.MsgType != "interactive" || !json.Valid([]byte(body.Content)) {
				t.Fatalf("unexpected message body: %#v", body)
			}
			if contains(body.Content, "test-app-secret") || !contains(body.Content, "scheduler_tick_failed") {
				t.Fatalf("unsafe or incomplete card: %s", body.Content)
			}
			fmt.Fprint(w, `{"code":0,"data":{"message_id":"om-d04-alert"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	appID := writeAlertSecret(t, dir, "app-id", "test-app-id")
	appSecret := writeAlertSecret(t, dir, "app-secret", "test-app-secret")
	chatID := writeAlertSecret(t, dir, "chat-id", "test-alert-chat")
	alerter, err := NewAppBotAlerter(server.URL, appID, appSecret, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if err := alerter.Send(t.Context(), Alert{Severity: "high", Code: "scheduler_tick_failed", Message: "must not be rendered", At: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if messageCalls != 1 {
		t.Fatalf("message calls = %d", messageCalls)
	}
}

func writeAlertSecret(t *testing.T, dir, name, value string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
