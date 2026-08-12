package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMapUserErrorCategory(t *testing.T) {
	cases := []struct {
		phase, etype, want string
	}{
		{"auth", "authentication_error", "auth"},
		{"request", "rate_limit_error", "rate_limit"},
		{"request", "billing_error", "quota"},
		{"request", "subscription_error", "quota"},
		{"request", "invalid_request_error", "invalid_request"},
		{"routing", "api_error", "service_unavailable"},
		{"account_auth", "upstream_error", "upstream"},
		{"upstream", "upstream_error", "upstream"},
		{"network", "api_error", "upstream"},
		{"internal", "api_error", "internal"},
		{"weird", "weird", "other"},
	}
	for _, c := range cases {
		if got := MapUserErrorCategory(c.phase, c.etype); got != c.want {
			t.Errorf("MapUserErrorCategory(%q,%q)=%q want %q", c.phase, c.etype, got, c.want)
		}
	}
}

func TestCategoryToFilter(t *testing.T) {
	phases, types := CategoryToFilter("rate_limit")
	if len(types) != 1 || types[0] != "rate_limit_error" || len(phases) != 0 {
		t.Fatalf("rate_limit => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("auth")
	if len(phases) != 1 || phases[0] != "auth" || len(types) != 0 {
		t.Fatalf("auth => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("service_unavailable")
	if len(phases) != 1 || phases[0] != "routing" || len(types) != 0 {
		t.Fatalf("service_unavailable => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("upstream")
	if len(phases) != 3 || phases[0] != "account_auth" || phases[1] != "upstream" || phases[2] != "network" || len(types) != 0 {
		t.Fatalf("upstream => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("internal")
	if len(phases) != 1 || phases[0] != "internal" || len(types) != 0 {
		t.Fatalf("internal => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("quota")
	if len(types) != 2 || types[0] != "billing_error" || types[1] != "subscription_error" || len(phases) != 0 {
		t.Fatalf("quota => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("invalid_request")
	if len(types) != 1 || types[0] != "invalid_request_error" || len(phases) != 0 {
		t.Fatalf("invalid_request => phases=%v types=%v", phases, types)
	}
	phases, types = CategoryToFilter("other")
	if len(phases) != 0 || len(types) != 0 {
		t.Fatalf("other => phases=%v types=%v", phases, types)
	}
}

func TestToUserErrorRequest_RedactsSensitiveFields(t *testing.T) {
	src := &OpsErrorLog{
		ID:              123,
		CreatedAt:       time.Unix(0, 0).UTC(),
		Model:           "m",
		RequestedModel:  "rm",
		InboundEndpoint: "/v1/chat/completions",
		StatusCode:      429,
		Platform:        "openai",
		Phase:           "request",
		Type:            "rate_limit_error",
		Message:         "rate limit exceeded with internal limiter name",
		Owner:           "client",
		APIKeyName:      "my-key",
		APIKeyDeleted:   true,
	}
	out := ToUserErrorRequest(src)
	if out.ID != 123 {
		t.Errorf("want ID=123, got %d", out.ID)
	}
	if out.Model != "rm" {
		t.Errorf("want requested_model preferred, got %q", out.Model)
	}
	if out.Category != "rate_limit" {
		t.Errorf("category=%q", out.Category)
	}
	if out.InboundEndpoint != "/v1/chat/completions" {
		t.Errorf("basic fields wrong: %+v", out)
	}
	require.Equal(t, "local_limit", out.ErrorClass)
	require.Equal(t, "请求过于频繁", out.Meaning)
	require.Equal(t, "请稍后重试或降低并发", out.Suggestion)
	require.Equal(t, out.Meaning, out.Message)
	require.NotContains(t, out.Message, "internal limiter")
	if out.KeyName != "my-key" {
		t.Errorf("want key_name=my-key, got %q", out.KeyName)
	}
	if !out.KeyDeleted {
		t.Error("want key_deleted=true")
	}
}

func TestToUserErrorRequestDetail_WhitelistAndRedacts(t *testing.T) {
	uid := int64(42)
	accountID := int64(7)
	upstreamStatus := 503
	src := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			ID:               999,
			CreatedAt:        time.Unix(1000, 0).UTC(),
			Model:            "gpt-4",
			RequestedModel:   "gpt-4-turbo",
			InboundEndpoint:  "/v1/chat/completions",
			StatusCode:       502,
			Platform:         "openai",
			Phase:            "upstream",
			Type:             "api_error",
			Message:          "upstream error",
			UserID:           &uid,
			UserEmail:        "secret@example.com",
			AccountID:        &accountID,
			RequestID:        "req-gateway",
			ClientRequestID:  "req-client",
			ClientIP:         func() *string { s := "1.2.3.4"; return &s }(),
			UpstreamEndpoint: "https://api.openai.com/v1/chat/completions",
			UpstreamModel:    "gpt-4-upstream",
			UserAgent:        "codex_cli_rs/0.125.0",
			GroupName:        "grp-a",
			Stream:           true,
		},
		ErrorBody:          `{"error":{"message":"upstream failed","type":"server_error"}}`,
		UpstreamStatusCode: &upstreamStatus,
	}

	out := ToUserErrorRequestDetail(src)
	if out == nil {
		t.Fatal("expected non-nil detail")
	}

	// 基础字段正确映射
	if out.ID != 999 {
		t.Errorf("want ID=999, got %d", out.ID)
	}
	require.Equal(t, "upstream_failed", out.ErrorClass)
	require.Equal(t, "上游请求失败", out.Meaning)
	require.Equal(t, "请稍后重试；持续失败请联系管理员并提供请求 ID", out.Suggestion)
	require.Equal(t, out.Meaning, out.Message)

	// client_ip / user_agent / stream 是该用户自己的请求属性。
	if out.ClientIP != "1.2.3.4" {
		t.Errorf("want client_ip=1.2.3.4, got %q", out.ClientIP)
	}
	if out.UserAgent != "codex_cli_rs/0.125.0" {
		t.Errorf("want user_agent=codex_cli_rs/0.125.0, got %q", out.UserAgent)
	}
	if !out.Stream {
		t.Errorf("want stream=true")
	}

	// 序列化后不含敏感字段
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	raw := string(b)
	for _, forbidden := range []string{
		"account_id",
		"client_request_id",
		"error_body",
		"group_name",
		"status_code",
		"upstream_status_code",
		"upstream_endpoint",
		"upstream_model",
		"user_email",
		"upstream failed",
		"server_error",
	} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("sensitive field %q leaked in JSON output: %s", forbidden, raw)
		}
	}
}

func TestToUserErrorRequestDetailSelectsOwnedRequestID(t *testing.T) {
	out := ToUserErrorRequestDetail(&OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			RequestID:       " req-gateway ",
			ClientRequestID: "req-client",
		},
	})
	require.Equal(t, "req-gateway", out.RequestID)

	fallback := ToUserErrorRequestDetail(&OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{ClientRequestID: " req-client "},
	})
	require.Equal(t, "req-client", fallback.RequestID)
}

func TestToUserErrorRequestDetailJSONWhitelistKeepsListAndAdminFieldsRedacted(t *testing.T) {
	listJSON, err := json.Marshal(UserErrorRequest{})
	require.NoError(t, err)
	require.NotContains(t, string(listJSON), "request_id")

	detailJSON, err := json.Marshal(UserErrorRequestDetail{RequestID: "req-owned"})
	require.NoError(t, err)
	require.Contains(t, string(detailJSON), `"request_id":"req-owned"`)
	for _, forbidden := range []string{
		"account_id",
		"client_request_id",
		"error_body",
		"group_name",
		"status_code",
		"upstream_status_code",
		"upstream_endpoint",
		"upstream_model",
	} {
		require.NotContains(t, string(detailJSON), forbidden)
	}
}

func TestToUserErrorRequestDetail_Nil(t *testing.T) {
	if out := ToUserErrorRequestDetail(nil); out != nil {
		t.Errorf("expected nil for nil input, got %+v", out)
	}
}
