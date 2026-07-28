package groupimpact

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateAppliesExactImpactBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		runtime      *RuntimeEvidence
		capacity     *CapacityEvidence
		wantFailing  bool
		wantSeverity string
		wantPrimary  string
	}{
		{
			name:        "P0 zero capacity",
			capacity:    &CapacityEvidence{Available: 0, Total: 2},
			wantFailing: true, wantSeverity: "P0", wantPrimary: "no_available_accounts",
		},
		{
			name:        "P0 all requests failed",
			runtime:     &RuntimeEvidence{Requests: 31, Successes: 0, ErrorRate: 1},
			wantFailing: true, wantSeverity: "P0", wantPrimary: "all_requests_failed",
		},
		{
			name:        "P1 errors",
			runtime:     &RuntimeEvidence{Requests: 20, Successes: 19, ErrorRate: .05},
			wantFailing: true, wantSeverity: "P1", wantPrimary: "partial_request_failures",
		},
		{
			name:        "P1 redundancy",
			capacity:    &CapacityEvidence{Available: 1, Total: 3},
			wantFailing: true, wantSeverity: "P1", wantPrimary: "lost_redundancy",
		},
		{
			name: "P1 ttft",
			runtime: &RuntimeEvidence{
				Requests: 20, Successes: 20, TTFTP95MS: 3900, TTFTBaselineP95MS: 3000,
			},
			wantFailing: true, wantSeverity: "P1", wantPrimary: "ttft_degraded",
		},
		{
			name:     "two accounts retain their designed redundancy",
			capacity: &CapacityEvidence{Available: 1, Total: 2},
		},
		{
			name: "ttft exactly thirty percent slower meets boundary",
			runtime: &RuntimeEvidence{
				Requests: 20, Successes: 20, TTFTP95MS: 3900, TTFTBaselineP95MS: 3000,
			},
			wantFailing: true, wantSeverity: "P1", wantPrimary: "ttft_degraded",
		},
		{
			name: "ttft at three seconds is healthy",
			runtime: &RuntimeEvidence{
				Requests: 20, Successes: 20, TTFTP95MS: 3000, TTFTBaselineP95MS: 2000,
			},
		},
		{
			name:    "error rate below five percent is healthy",
			runtime: &RuntimeEvidence{Requests: 20, Successes: 19, ErrorRate: .0499},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			impact := Evaluate(Snapshot{
				GroupID: 7, GroupName: "GPT PLUS 内测", ObservedAt: time.Now(),
				Runtime: test.runtime, Capacity: test.capacity,
			})
			if impact.Failing != test.wantFailing ||
				impact.Severity != test.wantSeverity ||
				impact.Primary != test.wantPrimary {
				t.Fatalf("impact = %#v", impact)
			}
		})
	}
}

func TestEvaluateUsesApprovedPrimarySignalPriority(t *testing.T) {
	t.Parallel()
	base := Snapshot{
		GroupID: 7, GroupName: "GPT PLUS 内测", ObservedAt: time.Now(),
		Runtime: &RuntimeEvidence{
			Requests: 31, Successes: 0, ErrorRate: 1,
			TTFTP95MS: 5000, TTFTBaselineP95MS: 2000,
		},
		Capacity: &CapacityEvidence{Available: 0, Total: 3},
	}
	if got := Evaluate(base).Primary; got != "no_available_accounts" {
		t.Fatalf("zero capacity priority = %q", got)
	}
	base.Capacity.Available = 2
	if got := Evaluate(base).Primary; got != "all_requests_failed" {
		t.Fatalf("all failed priority = %q", got)
	}
	base.Runtime.Successes = 28
	base.Runtime.ErrorRate = .1
	if got := Evaluate(base).Primary; got != "partial_request_failures" {
		t.Fatalf("partial failures priority = %q", got)
	}
	base.Runtime.ErrorRate = 0
	base.Capacity.Available = 1
	if got := Evaluate(base).Primary; got != "lost_redundancy" {
		t.Fatalf("redundancy priority = %q", got)
	}
	base.Capacity.Available = 3
	if got := Evaluate(base).Primary; got != "ttft_degraded" {
		t.Fatalf("ttft priority = %q", got)
	}
}

func TestEvaluateSeparatesEvidenceAndMaterialHashes(t *testing.T) {
	t.Parallel()
	snapshot := Snapshot{
		GroupID: 7, GroupName: "GPT PLUS 内测", ObservedAt: time.Now(),
		Runtime:  &RuntimeEvidence{Requests: 100, Successes: 90, ErrorRate: .1},
		Capacity: &CapacityEvidence{Available: 2, Total: 3},
	}
	first := Evaluate(snapshot)
	snapshot.Runtime.Requests = 120
	snapshot.Runtime.Successes = 108
	second := Evaluate(snapshot)
	if first.EvidenceHash == second.EvidenceHash {
		t.Fatal("numeric evidence change did not change EvidenceHash")
	}
	if first.MaterialHash != second.MaterialHash {
		t.Fatal("same user impact changed MaterialHash")
	}
	snapshot.Runtime.Successes = 0
	snapshot.Runtime.ErrorRate = 1
	third := Evaluate(snapshot)
	if second.MaterialHash == third.MaterialHash || third.Primary != "all_requests_failed" {
		t.Fatalf("material impact did not change: %#v", third)
	}
}

func TestEvaluateUsesHumanLanguageOnly(t *testing.T) {
	t.Parallel()
	impact := Evaluate(Snapshot{
		GroupID: 7, GroupName: "GPT PLUS 内测", ObservedAt: time.Now(),
		Runtime: &RuntimeEvidence{Requests: 20, Successes: 18, ErrorRate: .1},
		Capacity: &CapacityEvidence{
			Available: 2, Total: 3,
			Unavailable: []UnavailableAccount{{Name: "Wawazz", Reason: "余额已耗尽"}},
		},
	})
	text := impact.Headline + "\n" + impact.UserImpact + "\n" + impact.Action
	for _, fact := range append(impact.Current, impact.Clues...) {
		text += "\n" + fact.Label + "\n" + fact.Value
	}
	for _, forbidden := range []string{"error_rate", "ttft_p95", "balance_exhausted", "active but paused"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("technical term %q leaked in %s", forbidden, text)
		}
	}
}
