package updater

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakePreparerRunner struct {
	stdout string
	stderr string
	err    error
	name   string
	args   []string
}

func (r *fakePreparerRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.stdout, r.stderr, r.err
}

func TestCommandCandidatePreparerInvokesAbsoluteHostCommand(t *testing.T) {
	runner := &fakePreparerRunner{stdout: `{"status":"ready"}`}
	preparer := NewCommandCandidatePreparer("/usr/local/libexec/sub2api-candidate-preparer", runner)

	if err := preparer.Prepare(context.Background(), "0.1.172"); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if runner.name != "/usr/local/libexec/sub2api-candidate-preparer" {
		t.Fatalf("command = %q", runner.name)
	}
	if strings.Join(runner.args, " ") != "prepare 0.1.172" {
		t.Fatalf("args = %#v", runner.args)
	}
}

func TestCommandCandidatePreparerFailsClosedOnRelativePathAndCommandFailure(t *testing.T) {
	if err := NewCommandCandidatePreparer("candidate-preparer", &fakePreparerRunner{}).Prepare(context.Background(), "0.1.172"); err == nil {
		t.Fatal("relative command path was accepted")
	}
	runner := &fakePreparerRunner{stderr: "secret token must not be exposed", err: errors.New("exit status 1")}
	err := NewCommandCandidatePreparer("/usr/local/libexec/sub2api-candidate-preparer", runner).Prepare(context.Background(), "0.1.172")
	if err == nil || !strings.Contains(err.Error(), "candidate preparation command failed") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret token") {
		t.Fatalf("error leaked command stderr: %v", err)
	}
}

func TestCommandCandidatePreparerRecognizesStableTargetChangedMarker(t *testing.T) {
	runner := &fakePreparerRunner{stdout: "target_changed\n", err: errors.New("exit status 1")}
	err := NewCommandCandidatePreparer("/usr/local/libexec/sub2api-candidate-preparer", runner).Prepare(context.Background(), "0.1.172")
	if !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("error = %v, want ErrTargetChanged", err)
	}
}
