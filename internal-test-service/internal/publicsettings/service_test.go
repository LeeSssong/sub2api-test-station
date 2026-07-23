package publicsettings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"example.invalid/internal-test-service/internal/authproxy"
)

func assertPublicSettings(t *testing.T, body []byte, registration, invitation, affiliate bool) {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"registration_enabled":    registration,
		"invitation_code_enabled": invitation,
		"affiliate_enabled":       affiliate,
	} {
		if got, ok := envelope.Data[key].(bool); !ok || got != want {
			t.Fatalf("%s=%v want=%v body=%s", key, envelope.Data[key], want, body)
		}
	}
}

func containsString(body []byte, value string) bool {
	return bytes.Contains(body, []byte(value))
}

func TestOverlaySynchronizesLaunchSwitchesWithoutChangingOtherSettings(t *testing.T) {
	upstream := []byte(`{"code":0,"message":"success","data":{"registration_enabled":true,"invitation_code_enabled":true,"affiliate_enabled":true,"site_name":"Relay","large_native_value":9007199254740993}}`)
	svc := &Service{
		Forward: func(context.Context, http.Header) (authproxy.Response, error) {
			return authproxy.Response{
				Status: http.StatusOK,
				Header: http.Header{"Content-Type": {"application/json"}},
				Body:   upstream,
			}, nil
		},
		EffectiveRegistrationOpen: func(context.Context) (bool, error) { return true, nil },
	}

	response, err := svc.Get(context.Background(), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	assertPublicSettings(t, response.Body, true, false, false)
	if !containsString(response.Body, `"site_name":"Relay"`) {
		t.Fatalf("unrelated settings changed: %s", response.Body)
	}
	if !containsString(response.Body, `"large_native_value":9007199254740993`) {
		t.Fatalf("unrelated numeric setting lost precision: %s", response.Body)
	}
}

func TestOverlayClosesRegistrationWhenD04IsClosedOrFull(t *testing.T) {
	svc := &Service{
		Forward: func(context.Context, http.Header) (authproxy.Response, error) {
			return authproxy.Response{
				Status: http.StatusOK,
				Body:   []byte(`{"code":0,"data":{"registration_enabled":true,"invitation_code_enabled":false,"affiliate_enabled":false}}`),
			}, nil
		},
		EffectiveRegistrationOpen: func(context.Context) (bool, error) { return false, nil },
	}
	response, err := svc.Get(context.Background(), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	assertPublicSettings(t, response.Body, false, false, false)
}

func TestOverlayNeverReopensNativeClosedRegistration(t *testing.T) {
	svc := &Service{
		Forward: func(context.Context, http.Header) (authproxy.Response, error) {
			return authproxy.Response{
				Status: http.StatusOK,
				Body:   []byte(`{"code":0,"data":{"registration_enabled":false}}`),
			}, nil
		},
		EffectiveRegistrationOpen: func(context.Context) (bool, error) { return true, nil },
	}
	response, err := svc.Get(context.Background(), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	assertPublicSettings(t, response.Body, false, false, false)
}

func TestOverlayPreservesFailedOrMalformedUpstreamResponse(t *testing.T) {
	for _, response := range []authproxy.Response{
		{Status: http.StatusServiceUnavailable, Body: []byte(`{"message":"unavailable"}`)},
		{Status: http.StatusOK, Body: []byte(`{`)},
	} {
		svc := &Service{
			Forward: func(context.Context, http.Header) (authproxy.Response, error) {
				return response, nil
			},
			EffectiveRegistrationOpen: func(context.Context) (bool, error) { return false, nil },
		}
		got, err := svc.Get(context.Background(), http.Header{})
		if err != nil || got.Status != response.Status || string(got.Body) != string(response.Body) {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	}
}
