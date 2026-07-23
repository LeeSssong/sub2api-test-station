package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/credits"
	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/registration"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
	"example.invalid/internal-test-service/internal/testsupport"
)

func TestPublicLaunchRegistrationAndDailyLoginEndToEnd(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	fake := testsupport.NewFake()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	credit := &credits.Service{
		Store: st, Provider: fake, Timezone: loc, TotalBudget: 500_000_000,
		DailyLoginCredit: domain.DailyLoginCredit, CostMultiplierBPS: 700,
		CostPolicyID: "test-policy", CostPolicyQualified: true, Mode: "write",
	}
	grant := func(ctx context.Context, userID int64, now time.Time) error {
		_, grantErr := credit.GrantDailyLogin(ctx, userID, now)
		return grantErr
	}
	forward := func(_ context.Context, _ string, body []byte, _ http.Header) (authproxy.Response, error) {
		var request struct {
			UserID int64 `json:"fixture_user_id"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return authproxy.Response{}, err
		}
		fake.AddUser(sub2api.User{ID: request.UserID})
		response, _ := json.Marshal(map[string]any{"data": map[string]any{"user": map[string]any{"id": request.UserID}}})
		return authproxy.Response{Status: http.StatusCreated, Header: http.Header{"Content-Type": {"application/json"}}, Body: response}, nil
	}
	now := time.Date(2026, 7, 21, 2, 0, 0, 0, time.UTC)
	reg := &registration.Service{
		Store: st, MaxUsers: 15, Mode: "write", RegistrationOpen: true,
		AuthForward: forward, GrantDailyLogin: grant, CanGrantDaily: credit.CanGrantDailyLogin,
		Now: func() time.Time { return now },
	}
	for id := int64(1); id <= 15; id++ {
		response, err := reg.Register(ctx, []byte(fmt.Sprintf(`{"fixture_user_id":%d}`, id)), http.Header{})
		if err != nil || response.Status != http.StatusCreated {
			t.Fatalf("register %d: response=%+v err=%v", id, response, err)
		}
		balance, _ := fake.GetBalance(ctx, id)
		if balance.Balance != domain.DailyLoginCredit || len(balance.History) != 1 {
			t.Fatalf("registration credit %d: %+v", id, balance)
		}
	}
	full, err := reg.Register(ctx, []byte(`{"fixture_user_id":16}`), http.Header{})
	if err != nil || full.Status != http.StatusConflict || !contains(string(full.Body), "D04_REGISTRATION_FULL") {
		t.Fatalf("cap response=%+v err=%v", full, err)
	}

	auth := &authproxy.Service{
		Forward: func(context.Context, string, []byte, http.Header) (authproxy.Response, error) {
			return authproxy.Response{Status: http.StatusOK, Body: []byte(`{"data":{"user":{"id":1}}}`)}, nil
		},
		IsLaunchUser: func(ctx context.Context, userID int64) (bool, error) {
			_, memberErr := st.GetInternalUser(ctx, userID)
			return memberErr == nil, nil
		},
		GrantDailyLogin: grant,
		Now:             func() time.Time { return now },
	}
	if _, err := auth.Authenticate(ctx, authproxy.LoginEndpoint, nil, http.Header{}); err != nil {
		t.Fatal(err)
	}
	balance, _ := fake.GetBalance(ctx, 1)
	if balance.Balance != domain.DailyLoginCredit || len(balance.History) != 1 {
		t.Fatalf("same-day login was not idempotent: %+v", balance)
	}
	auth.Now = func() time.Time { return now.Add(24 * time.Hour) }
	if _, err := auth.Authenticate(ctx, authproxy.LoginEndpoint, nil, http.Header{}); err != nil {
		t.Fatal(err)
	}
	balance, _ = fake.GetBalance(ctx, 1)
	if balance.Balance != 2*domain.DailyLoginCredit || len(balance.History) != 2 {
		t.Fatalf("next-day login credit missing: %+v", balance)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if count, _ := restarted.CountRegisteredUsers(ctx); count != 15 {
		t.Fatalf("restart count=%d", count)
	}
}

func contains(s, n string) bool {
	for i := 0; i+len(n) <= len(s); i++ {
		if s[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
