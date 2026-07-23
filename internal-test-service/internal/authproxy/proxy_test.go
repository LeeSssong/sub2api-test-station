package authproxy

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestAuthenticatePreservesNativeSuccessAndCreditsMember(t *testing.T) {
	body := []byte(`{"data":{"user":{"id":7},"access_token":"fixture-token"}}`)
	granted := int64(0)
	svc := &Service{
		Forward: func(_ context.Context, endpoint string, got []byte, headers http.Header) (Response, error) {
			if endpoint != LoginEndpoint {
				t.Fatalf("endpoint=%s", endpoint)
			}
			if string(got) != `{"email":"u@example.com","password":"fixture-password"}` {
				t.Fatalf("body changed")
			}
			return Response{
				Status: http.StatusOK,
				Header: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"session=fixture"}},
				Body:   body,
			}, nil
		},
		IsLaunchUser: func(_ context.Context, userID int64) (bool, error) {
			return userID == 7, nil
		},
		GrantDailyLogin: func(_ context.Context, userID int64, _ time.Time) error {
			granted = userID
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 7, 21, 1, 0, 0, 0, time.UTC) },
	}

	response, err := svc.Authenticate(
		context.Background(),
		LoginEndpoint,
		[]byte(`{"email":"u@example.com","password":"fixture-password"}`),
		http.Header{"Origin": {"https://api.example.com"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || string(response.Body) != string(body) {
		t.Fatalf("response=%+v", response)
	}
	if response.Header.Get("Set-Cookie") != "session=fixture" {
		t.Fatalf("headers=%v", response.Header)
	}
	if granted != 7 {
		t.Fatalf("granted=%d", granted)
	}
}

func TestAuthenticateDoesNotCreditFailedOrNonMemberLogin(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		member   bool
		endpoint string
	}{
		{name: "failed", status: http.StatusUnauthorized, member: true, endpoint: LoginEndpoint},
		{name: "non-member", status: http.StatusOK, member: false, endpoint: Login2FAEndpoint},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grants := 0
			svc := &Service{
				Forward: func(context.Context, string, []byte, http.Header) (Response, error) {
					return Response{Status: test.status, Body: []byte(`{"data":{"user":{"id":7}}}`)}, nil
				},
				IsLaunchUser: func(context.Context, int64) (bool, error) { return test.member, nil },
				GrantDailyLogin: func(context.Context, int64, time.Time) error {
					grants++
					return nil
				},
				Now: time.Now,
			}
			response, err := svc.Authenticate(context.Background(), test.endpoint, nil, http.Header{})
			if err != nil || response.Status != test.status {
				t.Fatalf("response=%+v err=%v", response, err)
			}
			if grants != 0 {
				t.Fatalf("grants=%d", grants)
			}
		})
	}
}

func TestAuthenticatePreservesSuccessWhenGrantNeedsReconciliation(t *testing.T) {
	svc := &Service{
		Forward: func(context.Context, string, []byte, http.Header) (Response, error) {
			return Response{Status: http.StatusOK, Body: []byte(`{"user":{"id":9}}`)}, nil
		},
		IsLaunchUser: func(context.Context, int64) (bool, error) { return true, nil },
		GrantDailyLogin: func(context.Context, int64, time.Time) error {
			return errors.New("reconciliation required")
		},
		Now: time.Now,
	}
	response, err := svc.Authenticate(context.Background(), LoginEndpoint, nil, http.Header{})
	if err != nil || response.Status != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestAuthenticateBoundsDailyGrantWithoutChangingNativeSuccess(t *testing.T) {
	grantStarted := make(chan struct{})
	svc := &Service{
		Forward: func(context.Context, string, []byte, http.Header) (Response, error) {
			return Response{Status: http.StatusOK, Body: []byte(`{"user":{"id":9}}`)}, nil
		},
		IsLaunchUser: func(context.Context, int64) (bool, error) { return true, nil },
		GrantDailyLogin: func(ctx context.Context, _ int64, _ time.Time) error {
			close(grantStarted)
			<-ctx.Done()
			return ctx.Err()
		},
		GrantTimeout: 25 * time.Millisecond,
		Now:          time.Now,
	}
	started := time.Now()
	response, err := svc.Authenticate(context.Background(), LoginEndpoint, nil, http.Header{})
	if err != nil || response.Status != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	<-grantStarted
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("native success waited too long for balance grant: %s", elapsed)
	}
}

func TestAuthenticateRejectsUnknownEndpointBeforeForward(t *testing.T) {
	forwarded := false
	svc := &Service{
		Forward: func(context.Context, string, []byte, http.Header) (Response, error) {
			forwarded = true
			return Response{}, nil
		},
	}
	if _, err := svc.Authenticate(context.Background(), "/api/v1/admin/users", nil, http.Header{}); !errors.Is(err, ErrEndpointNotAllowed) {
		t.Fatalf("err=%v", err)
	}
	if forwarded {
		t.Fatal("unknown endpoint was forwarded")
	}
}

func TestExtractUserIDSupportsNativeAuthEnvelopes(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"data":{"user":{"id":7}}}`),
		[]byte(`{"data":{"id":8}}`),
		[]byte(`{"user":{"id":9}}`),
		[]byte(`{"id":10}`),
	} {
		if got := ExtractUserID(body); got == 0 {
			t.Fatalf("body=%s", body)
		}
	}
	if got := ExtractUserID([]byte(`{"data":{"access_token":"fixture"}}`)); got != 0 {
		t.Fatalf("unexpected id=%d", got)
	}
}
