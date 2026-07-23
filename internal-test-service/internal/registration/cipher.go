package registration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

type InvitationCipher struct {
	aead cipher.AEAD
}

func NewInvitationCipher(key []byte) (*InvitationCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("invitation encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &InvitationCipher{aead: aead}, nil
}

func NewInvitationCipherFromFile(path string) (*InvitationCipher, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read invitation encryption key: %w", err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("decode invitation encryption key: invalid hex")
	}
	return NewInvitationCipher(key)
}

func (c *InvitationCipher) Encrypt(joinID, plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate invitation nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(joinID))
	return "v1:" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *InvitationCipher) Decrypt(joinID, encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "v1:") {
		return "", fmt.Errorf("unsupported invitation ciphertext version")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil || len(sealed) < c.aead.NonceSize() {
		return "", fmt.Errorf("invalid invitation ciphertext")
	}
	nonce, ciphertext := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(joinID))
	if err != nil {
		return "", fmt.Errorf("decrypt invitation ciphertext")
	}
	return string(plaintext), nil
}
