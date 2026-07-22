package registration

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestInvitationCipherRoundTripAndTamperDetection(t *testing.T) {
	cipher, err := NewInvitationCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := cipher.Encrypt("join-one", "provider-secret-code")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "provider-secret-code" || strings.Contains(encoded, "provider-secret-code") {
		t.Fatalf("plaintext persisted: %q", encoded)
	}
	decoded, err := cipher.Decrypt("join-one", encoded)
	if err != nil || decoded != "provider-secret-code" {
		t.Fatalf("decoded=%q err=%v", decoded, err)
	}
	if _, err := cipher.Decrypt("join-two", encoded); err == nil {
		t.Fatal("expected associated-data mismatch")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 1
	tampered := "v1:" + base64.RawURLEncoding.EncodeToString(sealed)
	if _, err := cipher.Decrypt("join-one", tampered); err == nil {
		t.Fatal("expected tamper rejection")
	}
}

func TestInvitationCipherRejectsWrongKeyLength(t *testing.T) {
	if _, err := NewInvitationCipher([]byte("too-short")); err == nil {
		t.Fatal("expected 32-byte key requirement")
	}
}
