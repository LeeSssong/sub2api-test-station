package billing

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const billingSecretRoot = "/run/secrets/upstream-sessions"
const maxBillingSecretBytes = 64 << 10

var ErrBillingSecretUnavailable = errors.New("billing credential is unavailable")

// BillingSource is the non-secret configuration required to collect one
// account's authoritative upstream bill. SecretRef is a reference only.
type BillingSource struct {
	AccountID   int64
	BaseURL     string
	AdapterType string
	SecretRef   string
}

// NewCostAdapter selects the immutable-billing API implementation for a
// configured source. The token is deliberately not retained in BillingSource.
func NewCostAdapter(adapterType, baseURL, token string, client *http.Client) (CostAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(adapterType)) {
	case "newapi":
		return NewNewAPIAdapter(baseURL, token, client)
	case "sub2api":
		return NewSub2APIAdapter(baseURL, token, client)
	default:
		return nil, fmt.Errorf("unsupported billing adapter type")
	}
}

// NewBillingAdapter reads a source's credential only while creating the
// adapter. The temporary byte slice is cleared before this function returns.
func NewBillingAdapter(source BillingSource, client *http.Client) (CostAdapter, error) {
	if source.AccountID <= 0 || strings.TrimSpace(source.BaseURL) == "" || strings.TrimSpace(source.AdapterType) == "" {
		return nil, fmt.Errorf("billing source configuration is incomplete")
	}
	secret, err := ReadBillingSecret(source.SecretRef)
	if err != nil {
		return nil, err
	}
	defer ClearBillingSecret(secret)
	return NewCostAdapter(source.AdapterType, source.BaseURL, string(secret), client)
}

// ReadBillingSecret resolves a file: secret reference rooted at
// /run/secrets/upstream-sessions. It never returns filesystem details in an
// error, avoiding leakage of secret paths through scheduler logs.
func ReadBillingSecret(reference string) ([]byte, error) {
	return readBillingSecret(reference, billingSecretRoot)
}

// ClearBillingSecret overwrites a transient credential after use.
func ClearBillingSecret(secret []byte) {
	for i := range secret {
		secret[i] = 0
	}
}

func readBillingSecret(reference, root string) ([]byte, error) {
	path, ok := billingSecretPath(reference, root)
	if !ok {
		return nil, ErrBillingSecretUnavailable
	}
	info, err := os.Lstat(path)
	if err != nil || !validBillingSecretFile(info) {
		return nil, ErrBillingSecretUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrBillingSecretUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !validBillingSecretFile(opened) {
		return nil, ErrBillingSecretUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBillingSecretBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxBillingSecretBytes {
		ClearBillingSecret(raw)
		return nil, ErrBillingSecretUnavailable
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		ClearBillingSecret(raw)
		return nil, ErrBillingSecretUnavailable
	}
	secret := bytes.Clone(trimmed)
	ClearBillingSecret(raw)
	return secret, nil
}

func billingSecretPath(reference, root string) (string, bool) {
	if !strings.HasPrefix(reference, "file:") {
		return "", false
	}
	path := filepath.Clean(strings.TrimPrefix(reference, "file:"))
	root = filepath.Clean(root)
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return resolvedPath, true
}

func validBillingSecretFile(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBillingSecretBytes {
		return false
	}
	permissions := info.Mode().Perm()
	return permissions == 0o600 || permissions == 0o640
}
