package feishuapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
		messageID, err := client.SendText(t.Context(), "chat-secret-id", "切换已完成")
		if err != nil || messageID != want {
			t.Fatalf("call %d = %q, %v", i, messageID, err)
		}
	}
	if tokenCalls != 1 || messageCalls != 2 {
		t.Fatalf("calls token=%d message=%d", tokenCalls, messageCalls)
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

	messageID, err := newTestClient(t, server.URL).SendText(t.Context(), "chat-secret-id", "result")
	if err != nil || messageID != "om-refreshed" || tokenCalls != 2 || messageCalls != 2 {
		t.Fatalf("result=%q err=%v token=%d message=%d", messageID, err, tokenCalls, messageCalls)
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
			_, err := newTestClient(t, server.URL).SendText(t.Context(), "chat-secret-id", "result")
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
