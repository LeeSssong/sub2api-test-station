package publicsettings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"example.invalid/internal-test-service/internal/authproxy"
)

type Forwarder func(context.Context, http.Header) (authproxy.Response, error)

type Service struct {
	Forward                   Forwarder
	EffectiveRegistrationOpen func(context.Context) (bool, error)
}

func (s *Service) Get(ctx context.Context, headers http.Header) (authproxy.Response, error) {
	response, err := s.Forward(ctx, headers)
	if err != nil || response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices {
		return response, err
	}
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return response, nil
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return response, nil
	}
	nativeOpen, _ := data["registration_enabled"].(bool)
	effectiveOpen := false
	if s.EffectiveRegistrationOpen != nil {
		if value, effectiveErr := s.EffectiveRegistrationOpen(ctx); effectiveErr == nil {
			effectiveOpen = value
		}
	}
	data["registration_enabled"] = nativeOpen && effectiveOpen
	data["invitation_code_enabled"] = false
	data["affiliate_enabled"] = false
	body, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		return response, nil
	}
	response.Body = body
	if response.Header == nil {
		response.Header = http.Header{}
	}
	response.Header.Set("Content-Type", "application/json")
	return response, nil
}
