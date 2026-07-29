package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
)

const syntheticIncidentID = "synthetic:relay-ops:acceptance:v1"

type IncidentRunner interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type Result struct {
	IncidentID            string `json:"incident_id"`
	State                 string `json:"state"`
	Transition            string `json:"transition"`
	AgentStatus           string `json:"agent_status"`
	Notification          string `json:"notification"`
	AlertNotification     string `json:"alert_notification"`
	DuplicateNotification string `json:"duplicate_notification"`
	RecoveryNotification  string `json:"recovery_notification"`
	ExternalUpstream      string `json:"external_upstream"`
}

type Service struct {
	Incidents IncidentRunner
	Agent     AnalysisRunner
}

// Run records one fixed, fully redacted event. It deliberately has no upstream,
// probe, billing, routing, balance, or key dependency.
func (s Service) Run(ctx context.Context) (Result, error) {
	if s.Incidents == nil {
		return Result{}, fmt.Errorf("incident runner is required")
	}
	evidence := sha256.Sum256([]byte(fmt.Sprintf("%s|agent=%t", syntheticIncidentID, s.Agent != nil)))
	evidenceHash := hex.EncodeToString(evidence[:])
	alertTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: true,
		EvidenceHash: evidenceHash, CurrentValue: "synthetic acceptance event",
		ConfirmationWindows: 1,
	})
	if err != nil {
		return Result{}, fmt.Errorf("record synthetic incident: %w", err)
	}
	result := Result{
		IncidentID: syntheticIncidentID, State: alertTransition.State, Transition: alertTransition.Kind,
		AgentStatus: "fallback", Notification: "not_configured",
		AlertNotification: "not_configured", DuplicateNotification: "not_run", RecoveryNotification: "not_configured",
		ExternalUpstream: "not_accessed",
	}
	if s.Agent != nil {
		result.AgentStatus = "not_run"
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1", IncidentID: syntheticIncidentID,
		Severity: "P1", Upstream: "relay-ops synthetic", MetricName: "synthetic_acceptance",
		BaselineValue: "none", CurrentValue: "synthetic acceptance event", Samples: 1,
		EvidenceRefs:   []string{"synthetic:relay-ops-incident-v1"},
		AllowedActions: []string{"observe", "request_human_review"},
	}
	if alertTransition.Notify && s.Agent != nil {
		if _, analyzeErr := s.Agent.AnalyzeOnce(ctx, contract); analyzeErr == nil {
			result.AgentStatus = "completed"
		} else {
			result.AgentStatus = "fallback_after_error"
		}
	}
	if !alertTransition.Notify {
		result.AlertNotification = "suppressed"
	}

	duplicateTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: true,
		EvidenceHash: evidenceHash, CurrentValue: "synthetic acceptance event",
		ConfirmationWindows: 1,
	})
	if err != nil {
		return Result{}, fmt.Errorf("record duplicate synthetic incident: %w", err)
	}
	if duplicateTransition.Notify {
		result.DuplicateNotification = "unexpected_notify"
	} else {
		result.DuplicateNotification = "suppressed"
	}

	recoveryHash := digest(evidenceHash + ":recovered")
	recoveryTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: false,
		EvidenceHash: recoveryHash, CurrentValue: "synthetic acceptance recovered",
	})
	if err != nil {
		return Result{}, fmt.Errorf("recover synthetic incident: %w", err)
	}
	result.State = recoveryTransition.State
	result.Transition = recoveryTransition.Kind
	if !recoveryTransition.Notify {
		result.RecoveryNotification = "suppressed"
	}

	result.Notification = overallNotification(result)
	return result, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func overallNotification(result Result) string {
	if result.AlertNotification == "failed" || result.RecoveryNotification == "failed" || result.DuplicateNotification == "unexpected_notify" {
		return "failed"
	}
	if result.AlertNotification == "not_configured" || result.RecoveryNotification == "not_configured" {
		return "not_configured"
	}
	return "delivered"
}
