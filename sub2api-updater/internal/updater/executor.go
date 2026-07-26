package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// hostContractVersion is the argument contract this updater speaks. The host
// executor refuses any other value so a stale binary or script names the drift.
const hostContractVersion = "1"

// HostExecutor invokes the root-owned update script with an immutable image reference.
type HostExecutor struct {
	path   string
	runner CommandRunner
}

func NewHostExecutor(path string, runner CommandRunner) *HostExecutor {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &HostExecutor{path: path, runner: runner}
}

func (e *HostExecutor) Run(ctx context.Context, op Operation) (ExecutionResult, error) {
	if e == nil || e.path == "" || e.runner == nil {
		return ExecutionResult{}, errors.New("host update executor is not configured")
	}
	stdout, stderr, err := e.runner.Run(
		ctx,
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
		return ExecutionResult{}, commandFailure(stderr, err)
	}
	result, err := parseTerminalResult(stdout)
	if err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
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
