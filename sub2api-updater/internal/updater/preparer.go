package updater

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// CandidatePreparerRunner is the narrow host-command boundary used by
// CommandCandidatePreparer. The command owns release credentials and the
// reviewed local/host release chain; updater only passes the requested target.
type CandidatePreparerRunner interface {
	Run(context.Context, string, ...string) (stdout string, stderr string, err error)
}

type execCandidatePreparerRunner struct{}

func (execCandidatePreparerRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

// CommandCandidatePreparer invokes a root-owned, explicitly configured host
// command. It does not invoke a shell and deliberately discards command
// output so credentials or raw stderr cannot enter updater state/API errors.
type CommandCandidatePreparer struct {
	path   string
	runner CandidatePreparerRunner
}

func NewCommandCandidatePreparer(path string, runners ...CandidatePreparerRunner) *CommandCandidatePreparer {
	runner := CandidatePreparerRunner(execCandidatePreparerRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &CommandCandidatePreparer{path: strings.TrimSpace(path), runner: runner}
}

func (p *CommandCandidatePreparer) Prepare(ctx context.Context, targetVersion string) error {
	if p == nil || p.path == "" || !strings.HasPrefix(p.path, "/") {
		return errors.New("candidate preparer command is not configured")
	}
	if strings.TrimSpace(targetVersion) == "" || strings.ContainsAny(targetVersion, "\r\n\x00") {
		return errors.New("candidate target version is invalid")
	}
	version, err := normalizeVersion(targetVersion)
	if err != nil {
		return errors.New("candidate target version is invalid")
	}
	stdout, stderr, err := p.runner.Run(ctx, p.path, "prepare", version)
	if err != nil {
		if strings.TrimSpace(stdout) == "target_changed" || strings.TrimSpace(stderr) == "target_changed" {
			return ErrTargetChanged
		}
		return ErrCandidatePreparationFailed
	}
	return nil
}
