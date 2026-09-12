package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexModelsGroupAccountRepo struct {
	AccountRepository
	accounts []Account
	err      error
}

func (r *codexModelsGroupAccountRepo) ListModelAvailabilityCandidates(_ context.Context, _ *int64, _ []string, _ bool) ([]Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]Account(nil), r.accounts...), nil
}

func TestCodexModelsManifestAggregationMergesDeterministically(t *testing.T) {
	candidates := []codexModelsManifestCandidate{
		{
			accountID: 20,
			manifest:  &OpenAIModelsResponse{Body: []byte(`{"models":[{"slug":"gpt-5.6","display_name":"GPT 5.6"},{"slug":"shared","source":"second"}],"metadata":{"version":1}}`)},
		},
		{
			accountID: 10,
			manifest:  &OpenAIModelsResponse{Body: []byte(`{"models":[{"slug":"gpt-5.5","display_name":"GPT 5.5"},{"slug":"shared","source":"first"}],"metadata":{"version":1}}`)},
		},
	}

	manifest, err := aggregateCodexModelsManifests(candidates)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"gpt-5.5","display_name":"GPT 5.5"},{"slug":"gpt-5.6","display_name":"GPT 5.6"},{"slug":"shared","source":"first"}],"metadata":{"version":1}}`, string(manifest.Body))
	require.Equal(t, codexModelsManifestBodyETag(manifest.Body), manifest.ETag)
}

func TestCodexModelsManifestAggregationRejectsNoSuccessfulManifest(t *testing.T) {
	_, err := aggregateCodexModelsManifests(nil)
	require.Error(t, err)
}

func TestFetchCodexModelsManifestForGroupMergesAccountManifests(t *testing.T) {
	accounts := []Account{
		accountWithID(newCodexModelsAPIKeyTestAccount("https://account-20.example/v1"), 20),
		accountWithID(newCodexModelsAPIKeyTestAccount("https://account-10.example/v1"), 10),
	}
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
		body := `{"models":[{"slug":"gpt-5.5"}]}`
		if accountID == 20 {
			body = `{"models":[{"slug":"gpt-5.6"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}}
	repo := &codexModelsGroupAccountRepo{accounts: accounts}
	s := newCodexModelsAPIKeyTestService(upstream)
	s.accountRepo = repo

	manifest, err := s.FetchCodexModelsManifestForGroup(context.Background(), 42, "0.145.0", "")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"gpt-5.5"},{"slug":"gpt-5.6"}]}`, string(manifest.Body))
}

func TestFetchCodexModelsManifestForGroupFailsClosedWhenAllAccountsFail(t *testing.T) {
	accounts := []Account{accountWithID(newCodexModelsAPIKeyTestAccount("https://account-10.example/v1"), 10)}
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"down"}`))}, nil
	}}
	s := newCodexModelsAPIKeyTestService(upstream)
	s.accountRepo = &codexModelsGroupAccountRepo{accounts: accounts}

	_, err := s.FetchCodexModelsManifestForGroup(context.Background(), 42, "0.145.0", "")
	require.Error(t, err)
}

func TestBuildCodexModelsGroupManifestCacheKeyIgnoresAccountOrder(t *testing.T) {
	first := []Account{
		accountWithID(newCodexModelsAPIKeyTestAccount("https://account-10.example/v1"), 10),
		accountWithID(newCodexModelsAPIKeyTestAccount("https://account-20.example/v1"), 20),
	}
	second := []Account{first[1], first[0]}

	require.Equal(t,
		buildCodexModelsGroupManifestCacheKey(42, "0.145.0", first),
		buildCodexModelsGroupManifestCacheKey(42, "0.145.0", second),
	)
	second[0].Credentials = map[string]any{"api_key": "sk-changed", "base_url": "https://account-20.example/v1"}
	require.NotEqual(t,
		buildCodexModelsGroupManifestCacheKey(42, "0.145.0", first),
		buildCodexModelsGroupManifestCacheKey(42, "0.145.0", second),
	)
}

func TestFetchCodexModelsManifestForGroupKeepsStaleAggregateAfterRefreshFailure(t *testing.T) {
	var calls atomic.Int32
	refreshFinished := make(chan struct{})
	var refreshOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"models":[{"slug":"old"}]}`))
			return
		}
		refreshOnce.Do(func() { close(refreshFinished) })
		http.Error(w, `{"error":"refresh failed"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	originalURL := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	t.Cleanup(func() { chatgptCodexModelsURL = originalURL })

	account := newCodexModelsTestAccount()
	repo := &codexModelsGroupAccountRepo{accounts: []Account{*account}}
	s := &OpenAIGatewayService{accountRepo: repo}
	initial, err := s.FetchCodexModelsManifestForGroup(context.Background(), 42, "0.145.0", "")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"old"}]}`, string(initial.Body))

	s.openAIModelsCache.mu.Lock()
	for key, entry := range s.openAIModelsCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		s.openAIModelsCache.entries[key] = entry
	}
	s.openAIModelsCache.mu.Unlock()

	stale, err := s.FetchCodexModelsManifestForGroup(context.Background(), 42, "0.145.0", "")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"old"}]}`, string(stale.Body))
	select {
	case <-refreshFinished:
	case <-time.After(time.Second):
		t.Fatal("stale refresh did not run")
	}

	stillStale, err := s.FetchCodexModelsManifestForGroup(context.Background(), 42, "0.145.0", "")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"old"}]}`, string(stillStale.Body))
}

func accountWithID(account *Account, id int64) Account {
	clone := *account
	clone.ID = id
	return clone
}
