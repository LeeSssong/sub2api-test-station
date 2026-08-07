package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hostContractVersion is the argument contract this updater speaks. The host
// executor refuses any other value so a stale binary or script names the drift.
const hostContractVersion = "1"

// HostExecutor invokes the root-owned update script with an immutable image reference.
type HostExecutor struct {
	path     string
	traceDir string
	runner   CommandRunner
}

// NewHostExecutor wires the executor script; a non-empty traceDir makes the
// script append its step trace to trace-<operation_id>.log for status readers.
func NewHostExecutor(path, traceDir string, runner CommandRunner) *HostExecutor {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &HostExecutor{path: path, traceDir: traceDir, runner: runner}
}

// TracePath returns the step trace location for an operation, or "" when
// tracing is disabled.
func TracePath(traceDir, operationID string) string {
	if traceDir == "" || operationID == "" {
		return ""
	}
	return filepath.Join(traceDir, "trace-"+operationID+".log")
}

func (e *HostExecutor) Run(ctx context.Context, op Operation) (ExecutionResult, error) {
	if e == nil || e.path == "" || e.runner == nil {
		return ExecutionResult{}, errors.New("host update executor is not configured")
	}
	var env []string
	if tracePath := TracePath(e.traceDir, op.OperationID); tracePath != "" {
		_ = os.Remove(tracePath)
		env = []string{"RELEASE_EVENT_LOG=" + tracePath}
	}
	stdout, stderr, err := e.runner.Run(
		ctx,
		env,
		e.path,
		"--contract-version", hostContractVersion,
		"--image", op.Image,
		"--version", op.TargetVersion,
		"--operation-id", op.OperationID,
	)
	if err != nil {
		// The script exits non-zero when rollback itself fails, and that verdict must
		// reach the operator instead of collapsing into a generic failure.
		if result, parseErr := parseTerminalResult(stdout); parseErr == nil && result.RollbackFailed {
			return result, nil
		}
		if result, parseErr := parseInterventionResult(stdout); parseErr == nil {
			return result, nil
		}
		return ExecutionResult{}, commandFailure(stderr, err)
	}
	result, err := parseTerminalResult(stdout)
	if err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
}

func parseInterventionResult(stdout string) (ExecutionResult, error) {
	var gate struct {
		SchemaVersion int    `json:"schema_version"`
		Downtime      bool   `json:"downtime_required"`
		ReasonCode    string `json:"reason_code"`
		Reason        string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &gate); err != nil {
		return ExecutionResult{}, err
	}
	if gate.SchemaVersion != 1 || !gate.Downtime || !validReasonCode(gate.ReasonCode) || len(gate.Reason) > 512 {
		return ExecutionResult{}, errors.New("invalid administrator-intervention result")
	}
	return ExecutionResult{
		Stage: "intervention_required", Result: gate.ReasonCode, Error: gate.Reason,
		InterventionRequired: true,
	}, nil
}

func validReasonCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func parseTerminalResult(stdout string) (ExecutionResult, error) {
	lines := strings.FieldsFunc(strings.TrimSpace(stdout), func(r rune) bool { return r == '\n' || r == '\r' })
	if len(lines) != 1 {
		return ExecutionResult{}, errors.New("executor did not emit exactly one terminal result")
	}
	switch strings.TrimSpace(lines[0]) {
	case "result=promoted":
		return ExecutionResult{Stage: "promoted", Result: "promoted", Promoted: true}, nil
	case "result=rolled_back":
		return ExecutionResult{Stage: "rolled_back", Result: "rolled_back", RolledBack: true}, nil
	case "result=rollback_failed":
		return ExecutionResult{Stage: "rollback_failed", Result: "rollback_failed", RollbackFailed: true}, nil
	default:
		return ExecutionResult{}, fmt.Errorf("unrecognized executor terminal result")
	}
}
