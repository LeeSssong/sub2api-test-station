package feishuapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// sendText is what a caller has to do for a plain-text message now that the
// client exposes only SendMessage. Text delivery still has to keep working —
// nothing in relay-ops sends it today, but the API accepts it.
func sendText(ctx context.Context, client *Client, chatID, text string) (string, error) {
	content, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return "", err
	}
	return client.SendMessage(ctx, chatID, OutboundMessage{MsgType: "text", Content: content})
}

func TestClientCachesTokenAndSendsTextToExactChat(t *testing.T) {
	var mu sync.Mutex
	tokenCalls := 0
	messageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCalls++
			if r.Method != http.MethodPost {
				t.Errorf("token method = %s", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 2 || body["app_id"] != "cli-test" || body["app_secret"] != "app-secret-value" {
				t.Errorf("token body = %#v", body)
			}
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"tenant-token-value","expire":7200}`)
		case "/open-apis/im/v1/messages":
			messageCalls++
			if r.Method != http.MethodPost || r.URL.Query().Get("receive_id_type") != "chat_id" {
				t.Errorf("message request = %s %s", r.Method, r.URL.String())
			}
			if r.Header.Get("Authorization") != "Bearer tenant-token-value" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			var body struct {
				ReceiveID string `json:"receive_id"`
				MsgType   string `json:"msg_type"`
				Content   string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ReceiveID != "chat-secret-id" || body.MsgType != "text" || body.Content != `{"text":"切换已完成"}` {
				t.Errorf("message body = %#v", body)
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"message_id":"om-%d"}}`, messageCalls)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	for i, want := range []string{"om-1", "om-2"} {
		messageID, err := sendText(t.Context(), client, "chat-secret-id", "切换已完成")
		if err != nil || messageID != want {
			t.Fatalf("call %d = %q, %v", i, messageID, err)
		}
	}
	if tokenCalls != 1 || messageCalls != 2 {
		t.Fatalf("calls token=%d message=%d", tokenCalls, messageCalls)
	}
}

func TestClientSendsInteractiveCardAsJSONString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"tenant_access_token":"tenant-token-value","expire":7200}`)
			return
		}
		var body struct {
			MsgType string `json:"msg_type"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.MsgType != "interactive" {
			t.Errorf("msg_type = %q", body.MsgType)
		}
		var card map[string]any
		if err := json.Unmarshal([]byte(body.Content), &card); err != nil || card["header"] == nil || card["elements"] == nil {
			t.Errorf("content is not card JSON: %q err=%v", body.Content, err)
		}
		fmt.Fprint(w, `{"code":0,"data":{"message_id":"om-card"}}`)
	}))
	defer server.Close()
	messageID, err := newTestClient(t, server.URL).SendMessage(t.Context(), "chat-secret-id", OutboundMessage{MsgType: "interactive", Content: json.RawMessage(`{"header":{},"elements":[]}`)})
	if err != nil || messageID != "om-card" {
		t.Fatalf("message=%q err=%v", messageID, err)
	}
}

func TestClientUrgentsMessageForExactOpenIDs(t *testing.T) {
	var urgentCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"tenant_access_token":"tenant-token-value","expire":7200}`)
		case "/open-apis/im/v1/messages/om-alert/urgent_app":
			urgentCalls++
			if r.Method != http.MethodPatch || r.URL.Query().Get("user_id_type") != "open_id" {
				t.Errorf("urgent request = %s %s", r.Method, r.URL.String())
			}
			if r.Header.Get("Authorization") != "Bearer tenant-token-value" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			var body struct {
				UserIDList []string `json:"user_id_list"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(body.UserIDList) != "[ou-a ou-b]" {
				t.Errorf("urgent recipients = %#v", body.UserIDList)
			}
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	status, err := newTestClient(t, server.URL).UrgentMessage(t.Context(), "om-alert", []string{"ou-a", "ou-b"})
	if err != nil || status != http.StatusOK || urgentCalls != 1 {
		t.Fatalf("urgent result = status %d calls %d err %v", status, urgentCalls, err)
	}
}

func TestClientRejectsOversizedInteractiveCardBeforeNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("network request made for oversized card")
	}))
	defer server.Close()
	content := json.RawMessage(`{"value":"` + strings.Repeat("x", maxMessageContentBytes) + `"}`)
	_, err := newTestClient(t, server.URL).SendMessage(t.Context(), "chat-secret-id", OutboundMessage{MsgType: "interactive", Content: content})
	if err == nil {
		t.Fatal("oversized card was accepted")
	}
}

func TestClientRefreshesTokenOnceAfterUnauthorized(t *testing.T) {
	tokenCalls := 0
	messageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			tokenCalls++
			fmt.Fprintf(w, `{"code":0,"tenant_access_token":"token-%d","expire":7200}`, tokenCalls)
			return
		}
		messageCalls++
		if messageCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"secret":"response-secret"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-2" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"code":0,"data":{"message_id":"om-refreshed"}}`)
	}))
	defer server.Close()

	messageID, err := sendText(t.Context(), newTestClient(t, server.URL), "chat-secret-id", "result")
	if err != nil || messageID != "om-refreshed" || tokenCalls != 2 || messageCalls != 2 {
		t.Fatalf("result=%q err=%v token=%d message=%d", messageID, err, tokenCalls, messageCalls)
	}
}

// Feishu rejects a stale tenant token with HTTP 200 and a business code, not
// HTTP 401. Before this was handled the refresh-and-retry path was unreachable
// and a revoked token failed every delivery until the process restarted.
func TestClientRefreshesTokenAfterBusinessCodeRejection(t *testing.T) {
	tokenCalls := 0
	messageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			tokenCalls++
			fmt.Fprintf(w, `{"code":0,"tenant_access_token":"token-%d","expire":7200}`, tokenCalls)
			return
		}
		messageCalls++
		if messageCalls == 1 {
			fmt.Fprint(w, `{"code":99991663,"msg":"tenant access token invalid"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-2" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"code":0,"data":{"message_id":"om-recovered"}}`)
	}))
	defer server.Close()

	messageID, err := sendText(t.Context(), newTestClient(t, server.URL), "chat-secret-id", "result")
	if err != nil || messageID != "om-recovered" || tokenCalls != 2 || messageCalls != 2 {
		t.Fatalf("result=%q err=%v token=%d message=%d", messageID, err, tokenCalls, messageCalls)
	}
}

// A permanent rejection must report the code it actually got. Folding it into
// "schema mismatch" left no way to tell a revoked token apart from a bot that
// had been removed from the chat.
func TestClientReportsPersistentBusinessCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"tenant_access_token":"tenant-token-value","expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":230002,"msg":"bot is not in the chat: response-secret"}`)
	}))
	defer server.Close()

	_, err := sendText(t.Context(), newTestClient(t, server.URL), "chat-secret-id", "result")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 230002 {
		t.Fatalf("err = %v, want APIError code 230002", err)
	}
	if strings.Contains(err.Error(), "response-secret") {
		t.Fatalf("error leaked the Feishu msg field: %v", err)
	}
}

func TestClientCapsResponsesAndRedactsErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "large token response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, strings.Repeat("x", maxResponseBytes+1))
			},
		},
		{
			name: "message API error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "tenant_access_token") {
					fmt.Fprint(w, `{"code":0,"tenant_access_token":"tenant-token-value","expire":7200}`)
					return
				}
				fmt.Fprint(w, `{"code":999,"msg":"response-secret"}`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			_, err := sendText(t.Context(), newTestClient(t, server.URL), "chat-secret-id", "result")
			if err == nil {
				t.Fatal("error response unexpectedly accepted")
			}
			for _, secret := range []string{"app-secret-value", "tenant-token-value", "chat-secret-id", "response-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func TestClientConstructionDoesNotExposeSecretFilePaths(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "sensitive-secret-file-name")
	_, err := NewClient("https://open.feishu.cn", secretPath, secretPath)
	if err == nil {
		t.Fatal("missing secret files were accepted")
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "sensitive-secret-file-name") {
		t.Fatalf("construction error leaked secret path: %v", err)
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	dir := t.TempDir()
	appIDFile := filepath.Join(dir, "app-id")
	appSecretFile := filepath.Join(dir, "app-secret")
	if err := os.WriteFile(appIDFile, []byte("cli-test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appSecretFile, []byte("app-secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(baseURL, appIDFile, appSecretFile)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
