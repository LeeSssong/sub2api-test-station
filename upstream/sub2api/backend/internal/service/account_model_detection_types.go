package service

import "time"

const (
	AccountModelDetectionStatusUntested            = "untested"
	AccountModelDetectionStatusQueued              = "queued"
	AccountModelDetectionStatusRunning             = "running"
	AccountModelDetectionStatusNormal              = "normal"
	AccountModelDetectionStatusAbnormal            = "abnormal"
	AccountModelDetectionStatusInsufficient        = "insufficient"
	AccountModelDetectionStatusFailed              = "failed"
	AccountModelDetectionStatusUnsupported         = "unsupported"
	AccountModelDetectionStatusServiceUnconfigured = "service_unconfigured"
	AccountModelDetectionStatusServiceUnavailable  = "service_unavailable"
)

const (
	AccountModelDetectionProfileLow     = "low"
	AccountModelDetectionProfileMedium  = "medium"
	AccountModelDetectionProfileHigh    = "high"
	AccountModelDetectionProfileUnknown = "unknown"

	AccountModelDetectionModeMonitor    = "monitor"
	AccountModelDetectionModeManual     = "manual"
	AccountModelDetectionModeEscalation = "escalation"
	AccountModelDetectionModeHistorical = "historical"

	AccountModelDetectionTriggerScheduled           = "scheduled"
	AccountModelDetectionTriggerManual              = "manual"
	AccountModelDetectionTriggerFirstRun            = "first_run"
	AccountModelDetectionTriggerConsecutiveAbnormal = "consecutive_abnormal"
	AccountModelDetectionTriggerInsufficient        = "insufficient"
	AccountModelDetectionTriggerModelConflict       = "model_conflict"
)

const (
	AccountModelDetectionEvidenceComplete     = "complete"
	AccountModelDetectionEvidenceInsufficient = "insufficient"
	AccountModelDetectionEvidenceUnavailable  = "unavailable"
	AccountModelDetectionEvidenceHistorical   = "historical"
)

type AccountModelDetectorState string

const (
	AccountModelDetectorStateReady        AccountModelDetectorState = "ready"
	AccountModelDetectorStateUnconfigured AccountModelDetectorState = "unconfigured"
	AccountModelDetectorStateUnavailable  AccountModelDetectorState = "unavailable"
)

type AccountModelDetectionModelOption struct {
	ID        string `json:"id"`
	Supported bool   `json:"supported"`
	Selected  bool   `json:"selected"`
	Reason    string `json:"reason,omitempty"`
}

type AccountModelDetectionSettings struct {
	AccountID            int64     `json:"account_id"`
	ConnectionProbeModel string    `json:"connection_probe_model"`
	ModelDetectionModel  string    `json:"model_detection_model"`
	UpdatedBy            int64     `json:"updated_by"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AccountModelDetectionSummary struct {
	Status                string         `json:"status"`
	ModelID               string         `json:"model_id,omitempty"`
	ClaimedModel          string         `json:"claimed_model,omitempty"`
	JuiceStatus           string         `json:"juice_status,omitempty"`
	JuiceSummary          map[string]any `json:"juice_summary,omitempty"`
	FingerprintCandidate  string         `json:"fingerprint_candidate,omitempty"`
	FingerprintSimilarity map[string]any `json:"fingerprint_similarity,omitempty"`
	DetectorVersion       string         `json:"detector_version,omitempty"`
	ErrorCode             string         `json:"error_code,omitempty"`
	ErrorMessage          string         `json:"error_message,omitempty"`
	QueuedAt              *time.Time     `json:"queued_at,omitempty"`
	StartedAt             *time.Time     `json:"started_at,omitempty"`
	FinishedAt            *time.Time     `json:"finished_at,omitempty"`
	RunID                 string         `json:"run_id,omitempty"`
	Source                string         `json:"source,omitempty"`
	Profile               string         `json:"profile,omitempty"`
	Mode                  string         `json:"mode,omitempty"`
	TriggerReason         string         `json:"trigger_reason,omitempty"`
	PlannedRequests       int            `json:"planned_requests,omitempty"`
	ValidSamples          int            `json:"valid_samples,omitempty"`
	EvidenceState         string         `json:"evidence_state,omitempty"`
	FingerprintStatus     string         `json:"fingerprint_status,omitempty"`
}

type AccountModelDetectionProjection struct {
	Status        string                             `json:"status"`
	DetectorState AccountModelDetectorState          `json:"detector_state"`
	Settings      AccountModelDetectionSettings      `json:"settings"`
	ModelOptions  []AccountModelDetectionModelOption `json:"model_options"`
	Recent        *AccountModelDetectionSummary      `json:"recent,omitempty"`
	Current       *AccountModelDetectionSummary      `json:"current,omitempty"`
}

type AccountModelDetectionRun struct {
	ID                    string
	AccountID             int64
	SlotKey               *string
	TriggerKind           string
	ModelID               string
	ClaimedModel          string
	Status                string
	JuiceStatus           string
	JuiceSummary          map[string]any
	FingerprintCandidate  string
	FingerprintSimilarity map[string]any
	DetectorVersion       string
	ErrorCode             string
	ErrorMessage          string
	QueuedAt              time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	CreatedAt             time.Time
	Profile               string
	Mode                  string
	TriggerReason         string
	PlannedRequests       int
	ValidSamples          int
	EvidenceState         string
	FingerprintStatus     string
}

type AccountModelDetectionSidecarCatalog struct {
	Version string
	Models  []string
	State   AccountModelDetectorState
}

type AccountModelDetectionRequest struct {
	RunID         string `json:"run_id"`
	DeclaredModel string `json:"declared_model"`
	RequestModel  string `json:"request_model"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	Profile       string `json:"profile"`
	Mode          string `json:"mode"`
	TriggerReason string `json:"trigger_reason"`
}

type AccountModelDetectionResponse struct {
	Status                string         `json:"status"`
	JuiceStatus           string         `json:"juice_status,omitempty"`
	JuiceSummary          map[string]any `json:"juice_summary,omitempty"`
	FingerprintCandidate  string         `json:"fingerprint_candidate,omitempty"`
	FingerprintSimilarity map[string]any `json:"fingerprint_similarity,omitempty"`
	DetectorVersion       string         `json:"detector_version,omitempty"`
	ErrorCode             string         `json:"error_code,omitempty"`
	Profile               string         `json:"profile,omitempty"`
	PlannedRequests       int            `json:"planned_requests,omitempty"`
	ValidSamples          int            `json:"valid_samples,omitempty"`
	EvidenceState         string         `json:"evidence_state,omitempty"`
	FingerprintStatus     string         `json:"fingerprint_status,omitempty"`
}

type AccountModelDetectionHistoryPage struct {
	Items      []AccountModelDetectionRun
	NextCursor string
}
