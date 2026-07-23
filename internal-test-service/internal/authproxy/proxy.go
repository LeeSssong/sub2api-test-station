package authproxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const (
	RegisterEndpoint = "/api/v1/auth/register"
	LoginEndpoint    = "/api/v1/auth/login"
	Login2FAEndpoint = "/api/v1/auth/login/2fa"
)

var ErrEndpointNotAllowed = errors.New("authentication endpoint is not allowed")

type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

type Forwarder func(context.Context, string, []byte, http.Header) (Response, error)

type Service struct {
	Forward         Forwarder
	IsLaunchUser    func(context.Context, int64) (bool, error)
	GrantDailyLogin func(context.Context, int64, time.Time) error
	GrantTimeout    time.Duration
	Now             func() time.Time
}

const defaultGrantTimeout = 2 * time.Second

func (s *Service) Authenticate(ctx context.Context, endpoint string, body []byte, headers http.Header) (Response, error) {
	if !allowedEndpoint(endpoint) {
		return Response{}, ErrEndpointNotAllowed
	}
	response, err := s.Forward(ctx, endpoint, body, headers)
	if err != nil || response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices {
		return response, err
	}
	userID := ExtractUserID(response.Body)
	if userID == 0 || s.IsLaunchUser == nil || s.GrantDailyLogin == nil {
		return response, nil
	}
	member, memberErr := s.IsLaunchUser(ctx, userID)
	if memberErr != nil || !member {
		return response, nil
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	timeout := s.GrantTimeout
	if timeout <= 0 {
		timeout = defaultGrantTimeout
	}
	grantCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	_ = s.GrantDailyLogin(grantCtx, userID, now)
	return response, nil
}

func allowedEndpoint(endpoint string) bool {
	switch endpoint {
	case RegisterEndpoint, LoginEndpoint, Login2FAEndpoint:
		return true
	default:
		return false
	}
}

func ExtractUserID(data []byte) int64 {
	var value struct {
		ID   int64 `json:"id"`
		Data struct {
			ID   int64 `json:"id"`
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	if json.Unmarshal(data, &value) != nil {
		return 0
	}
	switch {
	case value.ID != 0:
		return value.ID
	case value.Data.ID != 0:
		return value.Data.ID
	case value.Data.User.ID != 0:
		return value.Data.User.ID
	default:
		return value.User.ID
	}
}
