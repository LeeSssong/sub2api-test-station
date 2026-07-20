package feishuevents

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testVerificationToken = "verification-token"
	testEncryptKey        = "encrypt-key"
	maxTestBodyBytes      = int64(256 << 10)
)

var testNow = time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

func TestVerifierDecodesEncryptedChallenge(t *testing.T) {
	verifier := newTestVerifier(t)
	req := encryptedRequest(t, testNow, `{"challenge":"challenge-value","token":"verification-token","type":"url_verification"}`)

	envelope, err := verifier.Decode(req, maxTestBodyBytes)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if envelope.Challenge != "challenge-value" || envelope.Event != nil {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestVerifierDecodesEncryptedMessageEvent(t *testing.T) {
	verifier := newTestVerifier(t)
	req := encryptedRequest(t, testNow, validMessageJSON(`{"text":"@_user_1 切换 GPT-Pro 到灾备"}`))

	envelope, err := verifier.Decode(req, maxTestBodyBytes)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	event := envelope.Event
	if event == nil || event.EventID != "evt-1" || event.MessageID != "msg-1" || event.ChatID != "chat-1" || event.SenderOpenID != "ou-user" {
		t.Fatalf("event = %#v", event)
	}
	if len(event.Mentions) != 1 || event.Mentions[0].Key != "@_user_1" || event.Mentions[0].MentionedType != "app" {
		t.Fatalf("mentions = %#v", event.Mentions)
	}
}

func TestVerifierRejectsUntrustedOrMalformedRequests(t *testing.T) {
	tests := []struct {
		name    string
		request func(*testing.T) *http.Request
		want    error
	}{
		{
			name: "bad signature",
			request: func(t *testing.T) *http.Request {
				req := encryptedRequest(t, testNow, validMessageJSON(`{"text":"查询当前分组状态"}`))
				req.Header.Set("X-Lark-Signature", strings.Repeat("0", 64))
				return req
			},
			want: ErrUnauthorized,
		},
		{
			name: "expired timestamp",
			request: func(t *testing.T) *http.Request {
				return encryptedRequest(t, testNow.Add(-6*time.Minute), validMessageJSON(`{"text":"查询当前分组状态"}`))
			},
			want: ErrExpired,
		},
		{
			name: "invalid timestamp",
			request: func(t *testing.T) *http.Request {
				req := encryptedRequest(t, testNow, validMessageJSON(`{"text":"查询当前分组状态"}`))
				req.Header.Set("X-Lark-Request-Timestamp", "not-a-number")
				return req
			},
			want: ErrUnauthorized,
		},
		{
			name: "wrong token",
			request: func(t *testing.T) *http.Request {
				return encryptedRequest(t, testNow, strings.Replace(validMessageJSON(`{"text":"查询当前分组状态"}`), testVerificationToken, "wrong-token", 1))
			},
			want: ErrUnauthorized,
		},
		{
			name: "invalid padding",
			request: func(t *testing.T) *http.Request {
				return encryptedRequestWithCiphertext(t, testNow, append(bytes.Repeat([]byte{1}, aes.BlockSize), bytes.Repeat([]byte{0}, aes.BlockSize)...))
			},
			want: ErrMalformed,
		},
		{
			name: "unexpected outer field",
			request: func(t *testing.T) *http.Request {
				req := encryptedRequest(t, testNow, validMessageJSON(`{"text":"查询当前分组状态"}`))
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				body["extra"] = true
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				return signedRequest(testNow, data)
			},
			want: ErrMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestVerifier(t).Decode(tt.request(t), maxTestBodyBytes)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifierEnforcesBodyAndTextLimits(t *testing.T) {
	verifier := newTestVerifier(t)

	t.Run("body", func(t *testing.T) {
		data := bytes.Repeat([]byte("x"), 1025)
		req := signedRequest(testNow, data)
		if _, err := verifier.Decode(req, 1024); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("error = %v, want ErrTooLarge", err)
		}
	})

	t.Run("decoded text", func(t *testing.T) {
		content, err := json.Marshal(map[string]string{"text": strings.Repeat("x", 4097)})
		if err != nil {
			t.Fatal(err)
		}
		req := encryptedRequest(t, testNow, validMessageJSON(string(content)))
		if _, err := verifier.Decode(req, maxTestBodyBytes); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("error = %v, want ErrTooLarge", err)
		}
	})
}

func newTestVerifier(t *testing.T) *Verifier {
	t.Helper()
	verifier, err := NewVerifier(testVerificationToken, testEncryptKey, func() time.Time { return testNow })
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func validMessageJSON(content string) string {
	return fmt.Sprintf(`{
  "schema":"2.0",
  "header":{"event_id":"evt-1","event_type":"im.message.receive_v1","app_id":"cli-test","tenant_key":"tenant-1","create_time":"1784534400000","token":"%s"},
  "event":{
    "sender":{"sender_id":{"open_id":"ou-user"},"sender_type":"user","tenant_key":"tenant-1"},
    "message":{"message_id":"msg-1","chat_id":"chat-1","chat_type":"group","message_type":"text","content":%q,"mentions":[{"key":"@_user_1","id":{"open_id":"ou-bot"},"mentioned_type":"app","name":"星桥AI监控Agent"}]}
  }
}`, testVerificationToken, content)
}

func encryptedRequest(t *testing.T, timestamp time.Time, plaintext string) *http.Request {
	t.Helper()
	key := sha256.Sum256([]byte(testEncryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(plaintext)
	padding := aes.BlockSize - len(data)%aes.BlockSize
	data = append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
	iv := []byte("0123456789abcdef")
	ciphertext := make([]byte, aes.BlockSize+len(data))
	copy(ciphertext, iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext[aes.BlockSize:], data)
	return encryptedRequestWithCiphertext(t, timestamp, ciphertext)
}

func encryptedRequestWithCiphertext(t *testing.T, timestamp time.Time, ciphertext []byte) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{"encrypt": base64.StdEncoding.EncodeToString(ciphertext)})
	if err != nil {
		t.Fatal(err)
	}
	return signedRequest(timestamp, body)
}

func signedRequest(timestamp time.Time, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/relay-ops/api/feishu/events", bytes.NewReader(body))
	timestampText := fmt.Sprint(timestamp.Unix())
	nonce := "nonce-1"
	digest := sha256.Sum256([]byte(timestampText + nonce + testEncryptKey + string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lark-Request-Timestamp", timestampText)
	req.Header.Set("X-Lark-Request-Nonce", nonce)
	req.Header.Set("X-Lark-Signature", hex.EncodeToString(digest[:]))
	return req
}
