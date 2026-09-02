package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectStreamDiagnosticFromErrorDetails(t *testing.T) {
	details := []*OpsErrorLogDetail{{
		OpsErrorLog: OpsErrorLog{RequestID: "req-1"},
		UpstreamErrors: `[{"kind":"stream_failure","stream_observation":{"event":"openai.stream.lifecycle","stage":"failed","request_id":"req-1","logical_request_id":"logical-1","attempt_id":"attempt-1","environment":"production","container_slot":"blue","error_class":"upstream_eof","failure_stage":"upstream_body_read","error_chain":"unexpected EOF","root_cause":"insufficient_evidence"}}]`,
	}}

	got := ProjectStreamDiagnostic(details, "req-1", "")

	require.Equal(t, "req-1", got.RequestID)
	require.Equal(t, "logical-1", got.LogicalRequestID)
	require.Equal(t, "production", got.Environment)
	require.Len(t, got.Attempts, 1)
	require.Equal(t, StreamErrorClassUpstreamEOF, got.Final.ErrorClass)
	require.Empty(t, got.EvidenceMissing)
}

func TestProjectStreamDiagnosticReturnsInsufficientEvidenceWithoutSnapshot(t *testing.T) {
	got := ProjectStreamDiagnostic(nil, "req-missing", "")

	require.Equal(t, "req-missing", got.RequestID)
	require.Equal(t, "insufficient_evidence", got.Final.RootCause)
	require.Contains(t, got.EvidenceMissing, "stream_lifecycle")
}
