package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	UnregisteredValue = "未登记"

	SecretErrorUnavailable        = "secret_unavailable"
	SecretErrorUnsafeFile         = "secret_unsafe_file"
	SecretErrorUnsafeParent       = "secret_unsafe_parent"
	SecretErrorInvalidJSON        = "secret_invalid_json"
	SecretErrorRegistryConflict   = "registry_baseurl_conflict"
	SecretErrorRegistryInvalidKey = "registry_baseurl_invalid"
	SecretErrorInvalidValue       = "secret_invalid_value"

	maxSmallSecretBytes = 64 << 10
	maxRegistryBytes    = 1 << 20
)

type secretLoadError struct{ code string }

func (e *secretLoadError) Error() string {
	return "upstream balance notification secret is unavailable"
}

func SecretErrorCode(err error) string {
	var target *secretLoadError
	if errors.As(err, &target) {
		return target.code
	}
	return ""
}

func newSecretError(code string) error { return &secretLoadError{code: code} }

type UpstreamBalanceSecretPaths struct {
	AppID         string
	AppSecret     string
	ChatID        string
	Recipients    string
	Registry      string
	CallbackToken string
}

type UpstreamBalanceSecrets struct {
	AppID            string
	AppSecret        string
	ChatID           string
	RecipientOpenIDs []string
	Registry         LoginRegistry
	CallbackToken    string
}

type LoginRegistryEntry struct {
	LoginAccount  string `json:"login_account"`
	LoginPassword string `json:"login_password"`
}

type LoginRegistry struct {
	entries map[string]LoginRegistryEntry
}

func (r LoginRegistry) Lookup(normalizedBaseURL string) (string, string, bool) {
	entry, ok := r.entries[normalizedBaseURL]
	if !ok {
		return UnregisteredValue, UnregisteredValue, false
	}
	account := strings.TrimSpace(entry.LoginAccount)
	password := strings.TrimSpace(entry.LoginPassword)
	if account == "" {
		account = UnregisteredValue
	}
	if password == "" {
		password = UnregisteredValue
	}
	return account, password, true
}

func LoadUpstreamBalanceSecrets(paths UpstreamBalanceSecretPaths) (UpstreamBalanceSecrets, error) {
	appID, err := readProtectedText(paths.AppID, maxSmallSecretBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	appSecret, err := readProtectedText(paths.AppSecret, maxSmallSecretBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	chatID, err := readProtectedText(paths.ChatID, maxSmallSecretBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	recipientsRaw, err := readProtectedFile(paths.Recipients, maxSmallSecretBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	defer clearBytes(recipientsRaw)
	recipients, err := decodeRecipients(recipientsRaw)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	registryRaw, err := readProtectedFile(paths.Registry, maxRegistryBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	defer clearBytes(registryRaw)
	registry, err := decodeLoginRegistry(registryRaw)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	callbackToken, err := readOptionalProtectedText(paths.CallbackToken, maxSmallSecretBytes)
	if err != nil {
		return UpstreamBalanceSecrets{}, err
	}
	return UpstreamBalanceSecrets{
		AppID: appID, AppSecret: appSecret, ChatID: chatID,
		RecipientOpenIDs: recipients, Registry: registry, CallbackToken: callbackToken,
	}, nil
}

func readProtectedText(path string, maxBytes int64) (string, error) {
	raw, err := readProtectedFile(path, maxBytes)
	if err != nil {
		return "", err
	}
	defer clearBytes(raw)
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", newSecretError(SecretErrorInvalidValue)
	}
	return value, nil
}

func readOptionalProtectedText(path string, maxBytes int64) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	return readProtectedText(path, maxBytes)
}

func readProtectedFile(path string, maxBytes int64) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, newSecretError(SecretErrorUnavailable)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, newSecretError(SecretErrorUnavailable)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > maxBytes {
		return nil, newSecretError(SecretErrorUnsafeFile)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode().Perm() != 0o700 {
		return nil, newSecretError(SecretErrorUnsafeParent)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, newSecretError(SecretErrorUnavailable)
	}
	if int64(len(raw)) > maxBytes {
		clearBytes(raw)
		return nil, newSecretError(SecretErrorUnsafeFile)
	}
	return raw, nil
}

func decodeRecipients(raw []byte) ([]string, error) {
	var document struct {
		OpenIDs []string `json:"open_ids"`
	}
	if err := decodeStrictJSON(raw, &document); err != nil {
		return nil, newSecretError(SecretErrorInvalidJSON)
	}
	if len(document.OpenIDs) < 1 || len(document.OpenIDs) > 20 {
		return nil, newSecretError(SecretErrorInvalidValue)
	}
	seen := make(map[string]struct{}, len(document.OpenIDs))
	values := make([]string, 0, len(document.OpenIDs))
	for _, value := range document.OpenIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, newSecretError(SecretErrorInvalidValue)
		}
		if _, exists := seen[value]; exists {
			return nil, newSecretError(SecretErrorInvalidValue)
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values, nil
}

func decodeLoginRegistry(raw []byte) (LoginRegistry, error) {
	var document struct {
		Version int                           `json:"version"`
		Entries map[string]LoginRegistryEntry `json:"entries"`
	}
	if err := decodeStrictJSON(raw, &document); err != nil || document.Version != 1 || document.Entries == nil {
		return LoginRegistry{}, newSecretError(SecretErrorInvalidJSON)
	}
	entries := make(map[string]LoginRegistryEntry, len(document.Entries))
	for rawKey, entry := range document.Entries {
		key, err := NormalizeBaseURL(rawKey)
		if err != nil {
			return LoginRegistry{}, newSecretError(SecretErrorRegistryInvalidKey)
		}
		if _, exists := entries[key]; exists {
			return LoginRegistry{}, newSecretError(SecretErrorRegistryConflict)
		}
		entries[key] = LoginRegistryEntry{
			LoginAccount: strings.TrimSpace(entry.LoginAccount), LoginPassword: strings.TrimSpace(entry.LoginPassword),
		}
	}
	return LoginRegistry{entries: entries}, nil
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty base URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid base URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.EscapedPath(), "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func clearBytes(raw []byte) {
	for i := range raw {
		raw[i] = 0
	}
}
