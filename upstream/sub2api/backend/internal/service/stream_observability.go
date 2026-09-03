package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

type StreamLifecycleStage string

const (
	StreamLifecycleStageAccepted           StreamLifecycleStage = "accepted"
	StreamLifecycleStageUpstreamRequest    StreamLifecycleStage = "upstream_request_started"
	StreamLifecycleStageResponseHeaders    StreamLifecycleStage = "response_headers_received"
	StreamLifecycleStageFirstEvent         StreamLifecycleStage = "first_event_received"
	StreamLifecycleStageFirstVisibleOutput StreamLifecycleStage = "first_visible_output"
	StreamLifecycleStageTerminalEvent      StreamLifecycleStage = "terminal_event_received"
	StreamLifecycleStageDecoderError       StreamLifecycleStage = "decoder_error"
	StreamLifecycleStageClientDisconnected StreamLifecycleStage = "client_disconnected"
	StreamLifecycleStageCompleted          StreamLifecycleStage = "completed"
	StreamLifecycleStageFailed             StreamLifecycleStage = "failed"
)

type StreamErrorClass string

const (
	StreamErrorClassClientDisconnected      StreamErrorClass = "client_disconnected"
	StreamErrorClassUpstreamExplicitError   StreamErrorClass = "upstream_explicit_error"
	StreamErrorClassUpstreamEOF             StreamErrorClass = "upstream_eof"
	StreamErrorClassUpstreamConnectionReset StreamErrorClass = "upstream_connection_reset"
	StreamErrorClassUpstreamTimeout         StreamErrorClass = "upstream_timeout"
	StreamErrorClassUpstreamSSEMalformed    StreamErrorClass = "upstream_sse_malformed"
	StreamErrorClassProxyOrDNSFailure       StreamErrorClass = "proxy_or_dns_failure"
	StreamErrorClassEdgeResponseInterrupted StreamErrorClass = "edge_response_interrupted"
	StreamErrorClassUnknownTransport        StreamErrorClass = "unknown_transport"
)

type StreamFailureStage string

const (
	StreamFailureStageClientRequestRead StreamFailureStage = "client_request_read"
	StreamFailureStageUpstreamConnect   StreamFailureStage = "upstream_connect"
	StreamFailureStageUpstreamHeaders   StreamFailureStage = "upstream_headers"
	StreamFailureStageUpstreamBodyRead  StreamFailureStage = "upstream_body_read"
	StreamFailureStageSSEDecode         StreamFailureStage = "sse_decode"
	StreamFailureStageClientWrite       StreamFailureStage = "client_write"
	StreamFailureStagePostStreamUsage   StreamFailureStage = "post_stream_usage"
	StreamFailureStageUnknown           StreamFailureStage = "unknown"
)

type StreamObservationInput struct {
	RequestID         string
	LogicalRequestID  string
	AttemptID         string
	ClientRequestID   string
	ThreadID          string
	WindowID          string
	SessionID         string
	Environment       string
	DeploymentCommit  string
	ContainerSlot     string
	ContainerID       string
	AccountID         int64
	AccountName       string
	Platform          string
	Model             string
	MappedModel       string
	UpstreamRequestID string
}

type StreamHeaders struct {
	HTTPStatus       int
	ContentType      string
	ContentEncoding  string
	TransferEncoding string
	Protocol         string
	EndpointClass    string
}

type StreamObservationSnapshot struct {
	Event                  string               `json:"event"`
	Stage                  StreamLifecycleStage `json:"stage"`
	RequestID              string               `json:"request_id,omitempty"`
	LogicalRequestID       string               `json:"logical_request_id,omitempty"`
	AttemptID              string               `json:"attempt_id,omitempty"`
	ClientRequestID        string               `json:"client_request_id,omitempty"`
	ThreadID               string               `json:"thread_id,omitempty"`
	WindowID               string               `json:"window_id,omitempty"`
	SessionID              string               `json:"session_id,omitempty"`
	AccountID              int64                `json:"account_id,omitempty"`
	AccountName            string               `json:"account_name,omitempty"`
	Platform               string               `json:"platform,omitempty"`
	Model                  string               `json:"model,omitempty"`
	MappedModel            string               `json:"mapped_model,omitempty"`
	UpstreamRequestID      string               `json:"upstream_request_id,omitempty"`
	ResponseID             string               `json:"response_id,omitempty"`
	Environment            string               `json:"environment,omitempty"`
	DeploymentCommit       string               `json:"deployment_commit,omitempty"`
	ContainerSlot          string               `json:"container_slot,omitempty"`
	ContainerID            string               `json:"container_id,omitempty"`
	ElapsedMS              int64                `json:"elapsed_ms"`
	HTTPStatus             int                  `json:"http_status,omitempty"`
	ContentType            string               `json:"content_type,omitempty"`
	ContentEncoding        string               `json:"content_encoding,omitempty"`
	TransferEncoding       string               `json:"transfer_encoding,omitempty"`
	Protocol               string               `json:"protocol,omitempty"`
	EndpointClass          string               `json:"upstream_endpoint_class,omitempty"`
	LastEventType          string               `json:"last_event_type,omitempty"`
	EventIndex             int                  `json:"event_index,omitempty"`
	TerminalEventType      string               `json:"terminal_event_type,omitempty"`
	TerminalEventValid     bool                 `json:"terminal_event_valid"`
	SawTerminalEvent       bool                 `json:"saw_terminal_event"`
	SawFailedEvent         bool                 `json:"saw_failed_event"`
	SemanticOutputSeen     bool                 `json:"semantic_output_seen"`
	UsageKnown             bool                 `json:"usage_known"`
	ClientOutputStarted    bool                 `json:"client_output_started"`
	ClientDisconnected     bool                 `json:"client_disconnected"`
	BytesRead              int64                `json:"bytes_read"`
	ResponseBytesForwarded int64                `json:"response_bytes_forwarded"`
	InputTokens            int                  `json:"input_tokens,omitempty"`
	OutputTokens           int                  `json:"output_tokens,omitempty"`
	CacheReadTokens        int                  `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens    int                  `json:"cache_creation_tokens,omitempty"`
	FailureStage           StreamFailureStage   `json:"failure_stage,omitempty"`
	ErrorClass             StreamErrorClass     `json:"error_class,omitempty"`
	ErrorType              string               `json:"error_type,omitempty"`
	ErrorChain             string               `json:"error_chain,omitempty"`
	RootCause              string               `json:"root_cause"`
	CorrelationDegraded    bool                 `json:"correlation_degraded"`
	EventBody              string               `json:"-"`
}

type StreamDiagnosticEntry struct {
	Environment      string `json:"environment,omitempty"`
	Host             string `json:"host,omitempty"`
	Route            string `json:"route,omitempty"`
	ActiveSlot       string `json:"active_slot,omitempty"`
	DeploymentCommit string `json:"deployment_commit,omitempty"`
	ContainerID      string `json:"container_id,omitempty"`
}

type StreamDiagnosticResponse struct {
	RequestID        string                      `json:"request_id,omitempty"`
	LogicalRequestID string                      `json:"logical_request_id,omitempty"`
	Environment      string                      `json:"environment,omitempty"`
	Entry            StreamDiagnosticEntry       `json:"entry"`
	Attempts         []StreamObservationSnapshot `json:"attempts"`
	Final            StreamObservationSnapshot   `json:"final"`
	EvidenceMissing  []string                    `json:"evidence_missing,omitempty"`
}

func ProjectStreamDiagnostic(details []*OpsErrorLogDetail, requestID, logicalRequestID string) StreamDiagnosticResponse {
	out := StreamDiagnosticResponse{RequestID: strings.TrimSpace(requestID), LogicalRequestID: strings.TrimSpace(logicalRequestID), Attempts: []StreamObservationSnapshot{}}
	for _, detail := range details {
		if detail == nil || strings.TrimSpace(detail.UpstreamErrors) == "" {
			continue
		}
		var events []*OpsUpstreamErrorEvent
		if err := json.Unmarshal([]byte(detail.UpstreamErrors), &events); err != nil {
			continue
		}
		for _, event := range events {
			if event == nil || event.StreamObservation == nil {
				continue
			}
			snapshot := *event.StreamObservation
			if out.RequestID == "" {
				out.RequestID = snapshot.RequestID
			}
			if out.LogicalRequestID == "" {
				out.LogicalRequestID = snapshot.LogicalRequestID
			}
			if out.Environment == "" {
				out.Environment = snapshot.Environment
			}
			if out.Entry.Environment == "" {
				out.Entry = StreamDiagnosticEntry{Environment: snapshot.Environment, ActiveSlot: snapshot.ContainerSlot, DeploymentCommit: snapshot.DeploymentCommit, ContainerID: snapshot.ContainerID}
			}
			out.Attempts = append(out.Attempts, snapshot)
		}
	}
	if len(out.Attempts) == 0 {
		out.EvidenceMissing = []string{"stream_lifecycle"}
		out.Final = StreamObservationSnapshot{Event: "openai.stream.lifecycle", RootCause: "insufficient_evidence", CorrelationDegraded: true}
		return out
	}
	out.Final = out.Attempts[len(out.Attempts)-1]
	return out
}

type StreamObservation struct {
	mu        sync.Mutex
	startedAt time.Time
	snapshot  StreamObservationSnapshot
}

func NewStreamObservation(input StreamObservationInput) *StreamObservation {
	s := StreamObservationSnapshot{
		Event:             "openai.stream.lifecycle",
		Stage:             StreamLifecycleStageAccepted,
		RequestID:         strings.TrimSpace(input.RequestID),
		LogicalRequestID:  strings.TrimSpace(input.LogicalRequestID),
		AttemptID:         strings.TrimSpace(input.AttemptID),
		ClientRequestID:   strings.TrimSpace(input.ClientRequestID),
		ThreadID:          strings.TrimSpace(input.ThreadID),
		WindowID:          strings.TrimSpace(input.WindowID),
		SessionID:         strings.TrimSpace(input.SessionID),
		Environment:       strings.TrimSpace(input.Environment),
		DeploymentCommit:  strings.TrimSpace(input.DeploymentCommit),
		ContainerSlot:     strings.TrimSpace(input.ContainerSlot),
		ContainerID:       strings.TrimSpace(input.ContainerID),
		AccountID:         input.AccountID,
		AccountName:       strings.TrimSpace(input.AccountName),
		Platform:          strings.TrimSpace(input.Platform),
		Model:             strings.TrimSpace(input.Model),
		MappedModel:       strings.TrimSpace(input.MappedModel),
		UpstreamRequestID: strings.TrimSpace(input.UpstreamRequestID),
	}
	s.CorrelationDegraded = s.RequestID == "" || s.LogicalRequestID == "" || s.AttemptID == ""
	return &StreamObservation{startedAt: time.Now(), snapshot: s}
}

func (o *StreamObservation) RecordHeaders(headers StreamHeaders) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.Stage = StreamLifecycleStageResponseHeaders
	o.snapshot.HTTPStatus = headers.HTTPStatus
	o.snapshot.ContentType = strings.TrimSpace(headers.ContentType)
	o.snapshot.ContentEncoding = strings.TrimSpace(headers.ContentEncoding)
	o.snapshot.TransferEncoding = strings.TrimSpace(headers.TransferEncoding)
	o.snapshot.Protocol = strings.TrimSpace(headers.Protocol)
	o.snapshot.EndpointClass = strings.TrimSpace(headers.EndpointClass)
}

func (o *StreamObservation) RecordEvent(eventType string, index int, bytesRead int64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.LastEventType = strings.TrimSpace(eventType)
	o.snapshot.EventIndex = index
	if bytesRead > o.snapshot.BytesRead {
		o.snapshot.BytesRead = bytesRead
	}
	if o.snapshot.Stage == StreamLifecycleStageAccepted || o.snapshot.Stage == StreamLifecycleStageResponseHeaders {
		o.snapshot.Stage = StreamLifecycleStageFirstEvent
	}
	if strings.HasPrefix(o.snapshot.LastEventType, "response.failed") {
		o.snapshot.SawFailedEvent = true
	}
}

func (o *StreamObservation) RecordVisibleOutput(bytesForwarded int64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.SemanticOutputSeen = true
	o.snapshot.ClientOutputStarted = true
	o.snapshot.Stage = StreamLifecycleStageFirstVisibleOutput
	if bytesForwarded > o.snapshot.ResponseBytesForwarded {
		o.snapshot.ResponseBytesForwarded = bytesForwarded
	}
	if bytesForwarded > o.snapshot.BytesRead {
		o.snapshot.BytesRead = bytesForwarded
	}
}

func (o *StreamObservation) RecordTerminal(eventType, responseID string, inputTokens, outputTokens, totalTokens int, bytesForwarded int64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.Stage = StreamLifecycleStageTerminalEvent
	o.snapshot.TerminalEventType = strings.TrimSpace(eventType)
	o.snapshot.TerminalEventValid = strings.TrimSpace(eventType) != ""
	o.snapshot.SawTerminalEvent = o.snapshot.TerminalEventValid
	o.snapshot.ResponseID = strings.TrimSpace(responseID)
	o.snapshot.InputTokens = inputTokens
	o.snapshot.OutputTokens = outputTokens
	o.snapshot.UsageKnown = totalTokens >= 0 && (inputTokens > 0 || outputTokens > 0 || totalTokens > 0)
	if bytesForwarded > o.snapshot.ResponseBytesForwarded {
		o.snapshot.ResponseBytesForwarded = bytesForwarded
	}
	if bytesForwarded > o.snapshot.BytesRead {
		o.snapshot.BytesRead = bytesForwarded
	}
}

func (o *StreamObservation) RecordFailure(stage StreamFailureStage, err error, clientDisconnected bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.FailureStage = stage
	o.snapshot.ErrorClass = ClassifyStreamError(stage, err, clientDisconnected)
	o.snapshot.ErrorType = errorType(err)
	o.snapshot.ErrorChain = SanitizeStreamErrorChain(err)
	o.snapshot.ClientDisconnected = clientDisconnected
	if !o.snapshot.SawTerminalEvent && !clientDisconnected {
		o.snapshot.Event = "openai.stream_incomplete"
	}
	if clientDisconnected {
		o.snapshot.Stage = StreamLifecycleStageClientDisconnected
	} else if stage == StreamFailureStageSSEDecode {
		o.snapshot.Stage = StreamLifecycleStageDecoderError
	} else {
		o.snapshot.Stage = StreamLifecycleStageFailed
	}
	if o.snapshot.ErrorClass == StreamErrorClassEdgeResponseInterrupted || o.snapshot.SawTerminalEvent {
		o.snapshot.RootCause = "insufficient_evidence"
	} else {
		o.snapshot.RootCause = "insufficient_evidence"
	}
}

func (o *StreamObservation) Snapshot() StreamObservationSnapshot {
	if o == nil {
		return StreamObservationSnapshot{Event: "openai.stream.lifecycle", RootCause: "insufficient_evidence", CorrelationDegraded: true}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.snapshot
	out.ElapsedMS = time.Since(o.startedAt).Milliseconds()
	if out.ElapsedMS < 0 {
		out.ElapsedMS = 0
	}
	if out.RootCause == "" {
		out.RootCause = "insufficient_evidence"
	}
	return out
}

var requestSecretPattern = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;]+|([?&](?:api[_-]?key|token|key|secret|password)=[^&\s]+)|\b(?:token|secret|password|api[_-]?key)\s*[:=]\s*[^\s,;]+`)

func SanitizeStreamErrorChain(err error) string {
	if err == nil {
		return ""
	}
	value := logredact.RedactText(strings.TrimSpace(err.Error()), "api_key", "secret", "cookie", "authorization")
	value = requestSecretPattern.ReplaceAllStringFunc(value, func(match string) string {
		if strings.HasPrefix(strings.ToLower(match), "bearer") {
			return "Bearer [redacted]"
		}
		if i := strings.IndexByte(match, '='); i >= 0 {
			return match[:i+1] + "[redacted]"
		}
		return "[redacted]"
	})
	return value
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "net.DNSError"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "io.ErrUnexpectedEOF"
	}
	if errors.Is(err, io.EOF) {
		return "io.EOF"
	}
	if errors.Is(err, context.Canceled) {
		return "context.Canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context.DeadlineExceeded"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "syscall.ECONNRESET"
	}
	if errors.Is(err, syscall.EPIPE) {
		return "syscall.EPIPE"
	}
	return "error"
}

func ClassifyStreamError(stage StreamFailureStage, err error, clientDisconnected bool) StreamErrorClass {
	if clientDisconnected || errors.Is(err, context.Canceled) && stage == StreamFailureStageClientWrite {
		return StreamErrorClassClientDisconnected
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StreamErrorClassUpstreamTimeout
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || stage == StreamFailureStageUpstreamConnect && (strings.Contains(strings.ToLower(SanitizeStreamErrorChain(err)), "connection refused") || strings.Contains(strings.ToLower(SanitizeStreamErrorChain(err)), "proxy")) {
		return StreamErrorClassProxyOrDNSFailure
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return StreamErrorClassUpstreamEOF
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EPIPE) {
		return StreamErrorClassUpstreamConnectionReset
	}
	if stage == StreamFailureStageSSEDecode {
		return StreamErrorClassUpstreamSSEMalformed
	}
	return StreamErrorClassUnknownTransport
}
