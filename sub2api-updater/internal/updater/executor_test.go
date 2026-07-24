package updater

import (
	"context"
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
	if call != "/opt/sub2api/production/ops/update-sub2api-host.sh --image "+op.Image+" --operation-id "+op.OperationID {
		t.Fatalf("call = %q", call)
	}
}

func TestHostExecutorRejectsUnexpectedTerminalOutput(t *testing.T) {
	runner := &recordedCommandRunner{results: []commandResult{{stdout: "result=unknown\n"}}}
	_, err := NewHostExecutor("/host-executor", runner).Run(context.Background(), sampleOperation())
	if err == nil {
		t.Fatal("expected invalid terminal output error")
	}
}
