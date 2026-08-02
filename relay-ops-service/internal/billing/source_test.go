package billing

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBillingSecretRestrictsRootPermissionsAndErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	valid := filepath.Join(root, "api-key")
	if err := os.WriteFile(valid, []byte("  secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(valid, 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := readBillingSecret("file:"+valid, root)
	if err != nil || string(secret) != "secret-value" {
		t.Fatalf("secret=%q err=%v", secret, err)
	}
	ClearBillingSecret(secret)
	for _, value := range secret {
		if value != 0 {
			t.Fatal("secret was not cleared")
		}
	}

	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("do-not-leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	writable := filepath.Join(root, "writable")
	if err := os.WriteFile(writable, []byte("do-not-leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	nestedLink := filepath.Join(root, "nested")
	if err := os.Symlink(filepath.Dir(outside), nestedLink); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{
		"file:" + outside,
		"file:" + filepath.Join(root, "..", filepath.Base(outside)),
		"file:" + writable,
		"file:" + link,
		"file:" + filepath.Join(nestedLink, filepath.Base(outside)),
		"literal-secret-value",
	} {
		_, err := readBillingSecret(reference, root)
		if !errors.Is(err, ErrBillingSecretUnavailable) {
			t.Fatalf("reference %q error=%v", reference, err)
		}
		if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), reference) {
			t.Fatalf("credential or path leaked in error %q", err)
		}
	}
}

func TestNewCostAdapterRoutesSupportedTypes(t *testing.T) {
	t.Parallel()
	newAPI, err := NewCostAdapter("newapi", "https://newapi.example", "token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := newAPI.(*NewAPIAdapter); !ok {
		t.Fatalf("New API adapter=%T", newAPI)
	}
	sub2API, err := NewCostAdapter(" SUB2API ", "https://sub2api.example", "token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub2API.(*Sub2APIAdapter); !ok {
		t.Fatalf("Sub2API adapter=%T", sub2API)
	}
	if _, err := NewCostAdapter("openai", "https://example.com", "token", nil); err == nil {
		t.Fatal("unsupported adapter type accepted")
	}
}
