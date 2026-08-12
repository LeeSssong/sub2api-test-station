package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	// OpenAIUpstreamHTTP2StreamErrorCode is returned to OpenAI-compatible clients
	// when an upstream HTTP/2 response stream is reset after the request started.
	OpenAIUpstreamHTTP2StreamErrorCode = "upstream_http2_stream_error"
	OpenAIUpstreamStreamReadErrorCode  = "upstream_stream_read_error"
)

type openAIUpstreamStreamReadError struct {
	cause         error
	clientCode    string
	clientMessage string
	recovery      *OpenAIStreamRecoveryInfo
}

func (e *openAIUpstreamStreamReadError) Error() string {
	return fmt.Sprintf("stream usage incomplete: %v", e.cause)
}

func (e *openAIUpstreamStreamReadError) Unwrap() error { return e.cause }

func newOpenAIUpstreamStreamReadError(err error) error {
	code, message := classifyOpenAIUpstreamStreamReadError(err)
	return &openAIUpstreamStreamReadError{
		cause:         err,
		clientCode:    code,
		clientMessage: message,
	}
}

// OpenAIStreamRecoveryInfo carries post-output stream metadata without
// changing the legacy read-error identity.  In particular, callers can still
// use OpenAIUpstreamStreamReadErrorDetails/errors.As while handlers decide
// whether to append a structured recovery SSE event.
type OpenAIStreamRecoveryInfo struct {
	OutputStarted bool
	ResponseID    string
	UsageKnown    bool
	Payload       OpenAIStreamRecoveryPayload
}

func (e *openAIUpstreamStreamReadError) OpenAIStreamRecoveryInfo() OpenAIStreamRecoveryInfo {
	if e == nil || e.recovery == nil {
		return OpenAIStreamRecoveryInfo{}
	}
	return *e.recovery
}

// newOpenAIUpstreamStreamReadRecoveryError preserves the existing stream-read
// error contract while attaching the recovery payload needed after semantic
// output has already been sent to the client.
func newOpenAIUpstreamStreamReadRecoveryError(cause error, responseID string, usageKnown bool) error {
	if cause == nil {
		cause = errors.New("missing terminal event")
	}
	code, message := classifyOpenAIUpstreamStreamReadError(cause)
	responseID = strings.TrimSpace(responseID)
	return &openAIUpstreamStreamReadError{
		cause:         cause,
		clientCode:    code,
		clientMessage: message,
		recovery: &OpenAIStreamRecoveryInfo{
			OutputStarted: true,
			ResponseID:    responseID,
			UsageKnown:    usageKnown,
			Payload: OpenAIStreamRecoveryPayload{
				Type:              "upstream_stream_error",
				Message:           "Upstream response stream was interrupted",
				Retryable:         true,
				ResumeSupported:   responseID != "",
				RetryAfterSeconds: 10,
				ResponseID:        responseID,
			},
		},
	}
}

// OpenAIStreamRecoveryDetails extracts optional post-output recovery metadata
// from an error while preserving the wrapped error's legacy identity.
func OpenAIStreamRecoveryDetails(err error) (OpenAIStreamRecoveryInfo, bool) {
	if err == nil {
		return OpenAIStreamRecoveryInfo{}, false
	}
	var provider interface {
		OpenAIStreamRecoveryInfo() OpenAIStreamRecoveryInfo
	}
	if !errors.As(err, &provider) {
		return OpenAIStreamRecoveryInfo{}, false
	}
	info := provider.OpenAIStreamRecoveryInfo()
	return info, info.OutputStarted
}

// shouldClassifyOpenAIUpstreamStreamReadError excludes cancellation and
// response-size enforcement from upstream retry.
func shouldClassifyOpenAIUpstreamStreamReadError(err error, contexts ...context.Context) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
		return false
	}
	for _, ctx := range contexts {
		if ctx != nil && ctx.Err() != nil {
			return false
		}
	}
	return true
}

// OpenAIUpstreamStreamReadErrorDetails returns the stable, sanitized client
// classification attached to an upstream stream read failure.
func OpenAIUpstreamStreamReadErrorDetails(err error) (code, message string, ok bool) {
	var streamErr *openAIUpstreamStreamReadError
	if !errors.As(err, &streamErr) || streamErr == nil {
		return "", "", false
	}
	return streamErr.clientCode, streamErr.clientMessage, true
}

func classifyOpenAIUpstreamStreamReadError(err error) (code, message string) {
	if err != nil {
		lower := strings.ToLower(err.Error())
		// net/http's HTTP/2 stream error is unexported. Its stable text contains
		// "stream error: stream ID ..."; match only the transport signature and
		// never pass the original text to the client.
		if strings.Contains(lower, "stream error: stream id ") ||
			(strings.Contains(lower, "http2:") && strings.Contains(lower, "stream")) {
			return OpenAIUpstreamHTTP2StreamErrorCode, "Upstream HTTP/2 stream failed"
		}
	}
	return OpenAIUpstreamStreamReadErrorCode, "Upstream response stream was interrupted"
}
