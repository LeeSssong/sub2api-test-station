package feishuevents

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxClockSkew = 5 * time.Minute
	maxTextBytes = 4 << 10
	maxIDBytes   = 256
)

var (
	ErrUnauthorized = errors.New("Feishu callback authentication failed")
	ErrExpired      = errors.New("Feishu callback timestamp expired")
	ErrTooLarge     = errors.New("Feishu callback exceeds size limit")
	ErrMalformed    = errors.New("Feishu callback is malformed")
)

type Verifier struct {
	verificationToken string
	encryptKey        string
	now               func() time.Time
}

type Envelope struct {
	Challenge string
	Event     *MessageEvent
}

type MessageEvent struct {
	EventID      string
	EventType    string
	AppID        string
	MessageID    string
	ChatID       string
	ChatType     string
	MessageType  string
	Content      string
	SenderOpenID string
	SenderType   string
	Mentions     []Mention
}

type Mention struct {
	Key           string
	OpenID        string
	MentionedType string
	Name          string
}

func NewVerifier(verificationToken, encryptKey string, now func() time.Time) (*Verifier, error) {
	if verificationToken == "" || encryptKey == "" || now == nil {
		return nil, ErrMalformed
	}
	return &Verifier{verificationToken: verificationToken, encryptKey: encryptKey, now: now}, nil
}

func (v *Verifier) Decode(req *http.Request, maxBodyBytes int64) (Envelope, error) {
	if req == nil || req.Body == nil || maxBodyBytes <= 0 {
		return Envelope{}, ErrMalformed
	}
	rawBody, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes+1))
	if err != nil {
		return Envelope{}, ErrMalformed
	}
	if int64(len(rawBody)) > maxBodyBytes {
		return Envelope{}, ErrTooLarge
	}

	var wrapper struct {
		Encrypt string `json:"encrypt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || wrapper.Encrypt == "" {
		return Envelope{}, ErrMalformed
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Envelope{}, ErrMalformed
	}

	timestamp := req.Header.Get("X-Lark-Request-Timestamp")
	nonce := req.Header.Get("X-Lark-Request-Nonce")
	signature := req.Header.Get("X-Lark-Signature")
	unsigned := timestamp == "" && nonce == "" && signature == ""
	if !unsigned {
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || nonce == "" || len(signature) != sha256.Size*2 {
			return Envelope{}, ErrUnauthorized
		}
		delta := v.now().Sub(time.Unix(seconds, 0))
		if delta < -maxClockSkew || delta > maxClockSkew {
			return Envelope{}, ErrExpired
		}
		digest := sha256.Sum256([]byte(timestamp + nonce + v.encryptKey + string(rawBody)))
		expected := hex.EncodeToString(digest[:])
		if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
			return Envelope{}, ErrUnauthorized
		}
	}
	plain, err := decrypt(wrapper.Encrypt, v.encryptKey)
	if err != nil {
		return Envelope{}, ErrMalformed
	}
	envelope, err := v.decodePlain(plain)
	if err != nil {
		return Envelope{}, err
	}
	if unsigned && envelope.Challenge == "" {
		return Envelope{}, ErrUnauthorized
	}
	return envelope, nil
}

func (v *Verifier) decodePlain(plain []byte) (Envelope, error) {
	var payload struct {
		Schema    string `json:"schema"`
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Header    struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
			AppID     string `json:"app_id"`
			Token     string `json:"token"`
		} `json:"header"`
		Event struct {
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
				SenderType string `json:"sender_type"`
			} `json:"sender"`
			Message struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				ChatType    string `json:"chat_type"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"`
				Mentions    []struct {
					Key           string `json:"key"`
					MentionedType string `json:"mentioned_type"`
					Name          string `json:"name"`
					ID            struct {
						OpenID string `json:"open_id"`
					} `json:"id"`
				} `json:"mentions"`
			} `json:"message"`
		} `json:"event"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return Envelope{}, ErrMalformed
	}
	if payload.Type == "url_verification" {
		if !sameSecret(payload.Token, v.verificationToken) {
			return Envelope{}, ErrUnauthorized
		}
		if !validID(payload.Challenge) {
			return Envelope{}, ErrMalformed
		}
		return Envelope{Challenge: payload.Challenge}, nil
	}
	if payload.Schema != "2.0" || payload.Header.EventType != "im.message.receive_v1" {
		return Envelope{}, ErrMalformed
	}
	if !sameSecret(payload.Header.Token, v.verificationToken) {
		return Envelope{}, ErrUnauthorized
	}
	if !validID(payload.Header.EventID) || !validID(payload.Event.Message.MessageID) || !validID(payload.Event.Message.ChatID) {
		return Envelope{}, ErrMalformed
	}
	if payload.Event.Sender.SenderType == "user" && !validID(payload.Event.Sender.SenderID.OpenID) {
		return Envelope{}, ErrMalformed
	}
	if payload.Event.Message.MessageType == "text" {
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(payload.Event.Message.Content), &content); err != nil {
			return Envelope{}, ErrMalformed
		}
		if len([]byte(content.Text)) > maxTextBytes {
			return Envelope{}, ErrTooLarge
		}
	}
	event := &MessageEvent{
		EventID:      payload.Header.EventID,
		EventType:    payload.Header.EventType,
		AppID:        payload.Header.AppID,
		MessageID:    payload.Event.Message.MessageID,
		ChatID:       payload.Event.Message.ChatID,
		ChatType:     payload.Event.Message.ChatType,
		MessageType:  payload.Event.Message.MessageType,
		Content:      payload.Event.Message.Content,
		SenderOpenID: payload.Event.Sender.SenderID.OpenID,
		SenderType:   payload.Event.Sender.SenderType,
		Mentions:     make([]Mention, 0, len(payload.Event.Message.Mentions)),
	}
	for _, mention := range payload.Event.Message.Mentions {
		if len(mention.Key) > maxIDBytes || len(mention.ID.OpenID) > maxIDBytes {
			return Envelope{}, ErrMalformed
		}
		event.Mentions = append(event.Mentions, Mention{
			Key:           mention.Key,
			OpenID:        mention.ID.OpenID,
			MentionedType: mention.MentionedType,
			Name:          mention.Name,
		})
	}
	return Envelope{Event: event}, nil
}

func decrypt(encoded, encryptKey string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(ciphertext) < 2*aes.BlockSize || (len(ciphertext)-aes.BlockSize)%aes.BlockSize != 0 {
		return nil, ErrMalformed
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrMalformed
	}
	iv := ciphertext[:aes.BlockSize]
	plain := append([]byte(nil), ciphertext[aes.BlockSize:]...)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, plain)
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return nil, ErrMalformed
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return nil, ErrMalformed
		}
	}
	return plain[:len(plain)-padding], nil
}

func sameSecret(got, want string) bool {
	return len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func validID(value string) bool {
	return value != "" && len(value) <= maxIDBytes
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrMalformed
	}
	return nil
}
