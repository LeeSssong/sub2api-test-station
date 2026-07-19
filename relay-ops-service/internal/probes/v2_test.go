package probes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
)

func TestV2WatchRunsWithScrubbedEnvironmentAndExpenseBound(t *testing.T) {
	helper := writeHelper(t, `
if env | grep -q '^SHOULD_NOT_LEAK='; then exit 41; fi
if [ -z "$RELAY_OPS_CANDIDATE_KEY" ]; then exit 42; fi
printf '%s' '{"schema_version":1,"run_id":"run-1","channel_id":"candidate-17","status":"passed","metrics":{"probed_models":["gpt-a"],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}},"errors":[]}'
`)
	keyFile := writeProbeKey(t, "sk-candidate-secret-1234")
	t.Setenv("SHOULD_NOT_LEAK", "sensitive-parent-value")
	runner := testRunner(helper)
	runner.MaxRequestCost = domain.MicroUSD(500)

	run, err := runner.Watch(context.Background(), candidate(keyFile))
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if run.RunID != "run-1" || run.ChatRequests != 2 || run.ExpenseUpperBound != 1_000 {
		t.Fatalf("run = %#v", run)
	}
}

func TestV2WatchEnforcesDeadlineOutputAndJSONContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		script  string
		timeout time.Duration
		want    error
	}{
		{name: "deadline", script: `exec sleep 2`, timeout: 20 * time.Millisecond, want: context.DeadlineExceeded},
		{name: "output cap", script: `head -c 4096 /dev/zero | tr '\000' x`, want: ErrOutputTooLarge},
		{name: "malformed json", script: `printf '%s' 'not-json'`, want: ErrMalformedOutput},
		{name: "non-zero", script: `printf '%s' 'secret response' >&2; exit 7`, want: ErrProbeFailed},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := testRunner(writeHelper(t, test.script))
			runner.MaxOutputBytes = 512
			ctx := context.Background()
			if test.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, test.timeout)
				defer cancel()
			}
			_, err := runner.Watch(ctx, candidate(writeProbeKey(t, "sk-candidate-secret-5678")))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret response") {
				t.Fatalf("error leaked stderr: %v", err)
			}
		})
	}
}

func TestV2QualifyUsesFullRunnerOnlyWhenExplicitlyCalled(t *testing.T) {
	t.Parallel()
	helper := writeHelper(t, `
if [ "$1" != "run" ]; then exit 43; fi
printf '%s' '{"schema_version":1,"run_id":"qualification-1","channel_id":"candidate-17","recorded_at":"2026-07-19T12:00:00Z","status":"passed","evidence_source":"live_direct","metrics":{"capacity":{"recommendation":{"concurrency":2,"rpm":20}}},"errors":[]}'
`)
	runner := testRunner(helper)
	result, err := runner.Qualify(context.Background(), candidate(writeProbeKey(t, "sk-candidate-secret-9876")), QualificationRequest{Reason: "promotion review"})
	if err != nil {
		t.Fatalf("Qualify: %v", err)
	}
	if result.RunID != "qualification-1" || result.Status != "passed" || !strings.Contains(string(result.Record), `"capacity"`) {
		t.Fatalf("result = %#v", result)
	}
}

func testRunner(helper string) *V2Executor {
	return &V2Executor{
		RubyPath: helper, ScriptPath: "/fixed/upstream-benchmark-v2.rb", ProfilePath: "/fixed/candidate-watch-v2.yaml",
		QualificationProfilePath: "/fixed/mvp-text-v2.yaml", MaxOutputBytes: 2 << 20,
	}
}

func candidate(keyFile string) candidates.Candidate {
	return candidates.Candidate{ID: 17, Name: "candidate", BaseURL: "https://candidate.example/v1", ProbeSecretRef: "file:" + keyFile, Enabled: true}
}

func writeProbeKey(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe-key")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeHelper(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe-helper")
	script := "#!/bin/sh\nshift\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
