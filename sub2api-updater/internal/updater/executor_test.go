package updater

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHostExecutorMapsTerminalResultAndPassesImmutableOperation(t *testing.T) {
	runner := &recordedCommandRunner{results: []commandResult{{stdout: "result=rolled_back\n"}}}
	executor := NewHostExecutor("/opt/sub2api/production/ops/update-sub2api-host.sh", runner)
	op := sampleOperation()
	result, err := executor.Run(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RolledBack || result.Stage != "rolled_back" {
		t.Fatalf("result = %#v", result)
	}
	call := strings.Join(runner.calls[0], " ")
	want := "/opt/sub2api/production/ops/update-sub2api-host.sh" +
		" --contract-version " + hostContractVersion +
		" --image " + op.Image +
		" --version " + op.TargetVersion +
		" --operation-id " + op.OperationID
	if call != want {
		t.Fatalf("call = %q, want %q", call, want)
	}
}

func TestHostExecutorRejectsUnexpectedTerminalOutput(t *testing.T) {
	runner := &recordedCommandRunner{results: []commandResult{{stdout: "result=unknown\n"}}}
	_, err := NewHostExecutor("/host-executor", runner).Run(context.Background(), sampleOperation())
	if err == nil {
		t.Fatal("expected invalid terminal output error")
	}
}

func TestHostExecutorReportsRollbackFailureDespiteNonZeroExit(t *testing.T) {
	runner := &recordedCommandRunner{results: []commandResult{{
		stdout: "result=rollback_failed\n",
		stderr: "host update failed: rollback did not restore the previous image\n",
		err:    errors.New("exit status 1"),
	}}}
	result, err := NewHostExecutor("/host-executor", runner).Run(context.Background(), sampleOperation())
	if err != nil {
		t.Fatalf("err = %v, want the rollback failure reported as a terminal result", err)
	}
	if !result.RollbackFailed || result.Stage != "rollback_failed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHostExecutorReportsErrorWhenNonZeroExitHasNoTerminalResult(t *testing.T) {
	runner := &recordedCommandRunner{results: []commandResult{{
		stderr: "host update failed: --version must be an application version such as 0.1.165\n",
		err:    errors.New("exit status 1"),
	}}}
	_, err := NewHostExecutor("/host-executor", runner).Run(context.Background(), sampleOperation())
	if err == nil {
		t.Fatal("expected the preflight failure to surface as an error")
	}
	if !strings.Contains(err.Error(), "--version must be an application version") {
		t.Fatalf("err = %v, want the host stderr preserved", err)
	}
}
