package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/config"
	"example.invalid/internal-test-service/internal/credits"
	httpserver "example.invalid/internal-test-service/internal/http"
	"example.invalid/internal-test-service/internal/ops"
	"example.invalid/internal-test-service/internal/publicsettings"
	"example.invalid/internal-test-service/internal/registration"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
)

type App struct {
	Store     *store.Store
	Handler   http.Handler
	Scheduler *ops.Scheduler
	close     func() error
}

const maxNativeResponseBytes = 1 << 20

func New(ctx context.Context, cfg config.Config) (*App, error) {
	st, err := store.Open(ctx, cfg.DataPath)
	if err != nil {
		return nil, err
	}
	provider, err := sub2api.NewHTTPClient(cfg.Sub2APIURL, cfg.AdminAPIKeyFile)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	credit := &credits.Service{
		Store: st, Provider: provider, Timezone: cfg.Timezone, TotalBudget: cfg.TotalBudget,
		DailyLoginCredit:  cfg.DailyLoginCredit,
		CostMultiplierBPS: cfg.BudgetCostBPS, CostPolicyID: cfg.CostPolicyID,
		CostPolicyQualified: cfg.CostPolicyQualified, Mode: cfg.Mode,
	}
	forward := forwardNative(cfg.Sub2APIURL)
	grantDaily := func(ctx context.Context, userID int64, now time.Time) error {
		_, grantErr := credit.GrantDailyLogin(ctx, userID, now)
		return grantErr
	}
	reg := &registration.Service{
		Store: st, MaxUsers: cfg.MaxUsers, Mode: cfg.Mode,
		RegistrationOpen: cfg.RegistrationOpen, AuthForward: forward,
		GrantDailyLogin: grantDaily, CanGrantDaily: credit.CanGrantDailyLogin,
		Now: time.Now,
	}
	auth := &authproxy.Service{
		Forward: forward,
		IsLaunchUser: func(ctx context.Context, userID int64) (bool, error) {
			_, memberErr := st.GetInternalUser(ctx, userID)
			if memberErr == nil {
				return true, nil
			}
			return false, nil
		},
		GrantDailyLogin: grantDaily,
		Now:             time.Now,
	}
	settings := &publicsettings.Service{
		Forward:                   forwardPublicSettings(cfg.Sub2APIURL),
		EffectiveRegistrationOpen: reg.EffectiveRegistrationOpen,
	}
	var reporter *ops.Reporter
	if cfg.CostPolicyQualified {
		reporter = &ops.Reporter{Store: st, Credits: credit, Timezone: cfg.Timezone}
	}
	scheduler := &ops.Scheduler{Store: st, Credits: credit, Reporter: reporter, Timezone: cfg.Timezone}
	if cfg.FeishuAppIDFile != "" {
		alerter, alertErr := ops.NewAppBotAlerter(cfg.FeishuBaseURL, cfg.FeishuAppIDFile, cfg.FeishuAppSecretFile, cfg.FeishuAlertChatIDFile)
		if alertErr != nil {
			_ = st.Close()
			return nil, alertErr
		}
		scheduler.Alerter = alerter
	}
	srv, err := httpserver.NewServer(reg, auth, settings)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	srv.SchedulerStatus = func() (time.Time, bool) {
		status := scheduler.Status()
		return status.LastTick, status.LastTickOK
	}
	return &App{Store: st, Handler: srv, Scheduler: scheduler, close: st.Close}, nil
}
func (a *App) Close() error {
	if a.close != nil {
		return a.close()
	}
	return nil
}

func forwardNative(baseURL string) authproxy.Forwarder {
	return func(ctx context.Context, endpoint string, body []byte, headers http.Header) (authproxy.Response, error) {
		switch endpoint {
		case authproxy.RegisterEndpoint, authproxy.LoginEndpoint, authproxy.Login2FAEndpoint:
		default:
			return authproxy.Response{}, authproxy.ErrEndpointNotAllowed
		}
		return forwardRequest(ctx, baseURL, http.MethodPost, endpoint, body, headers)
	}
}

func forwardPublicSettings(baseURL string) publicsettings.Forwarder {
	return func(ctx context.Context, headers http.Header) (authproxy.Response, error) {
		return forwardRequest(ctx, baseURL, http.MethodGet, "/api/v1/settings/public", nil, headers)
	}
}

func forwardRequest(ctx context.Context, baseURL, method, endpoint string, body []byte, headers http.Header) (authproxy.Response, error) {
	if len(body) > maxNativeResponseBytes {
		return authproxy.Response{}, errors.New("native request exceeds 1 MiB")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return authproxy.Response{}, err
	}
	for _, key := range []string{
		"Accept",
		"Accept-Language",
		"Authorization",
		"Content-Type",
		"Cookie",
		"Origin",
		"User-Agent",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
	} {
		for _, value := range headers.Values(key) {
			req.Header.Add(key, value)
		}
	}
	resp, err := (&http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}).Do(req)
	if err != nil {
		return authproxy.Response{}, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxNativeResponseBytes+1))
	if readErr != nil {
		return authproxy.Response{}, readErr
	}
	if len(data) > maxNativeResponseBytes {
		return authproxy.Response{}, errors.New("native response exceeds 1 MiB")
	}
	out := http.Header{}
	for _, key := range []string{"Set-Cookie", "Authorization", "Content-Type"} {
		for _, value := range resp.Header.Values(key) {
			out.Add(key, value)
		}
	}
	return authproxy.Response{Status: resp.StatusCode, Header: out, Body: data}, nil
}

func Healthcheck(ctx context.Context, address string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("healthcheck HTTP %d", resp.StatusCode)
	}
	var body map[string]any
	return json.NewDecoder(resp.Body).Decode(&body)
}
