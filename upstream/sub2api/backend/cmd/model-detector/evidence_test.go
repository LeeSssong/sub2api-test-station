package main

import (
	"reflect"
	"testing"
)

func TestClassifyEvidence(t *testing.T) {
	tests := []struct {
		name     string
		evidence detectionEvidence
		want     string
	}{
		{
			name: "verified",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMatch},
				ActiveResponse: activeResponseEvidence{Status: evidenceMatch, ReturnedModel: "gpt-5.6-sol"},
				Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
			},
			want: verdictVerified,
		},
		{
			name: "suspected mapping",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMissing},
				ActiveResponse: activeResponseEvidence{Status: evidenceMatch, ReturnedModel: "gpt-5.6-sol"},
				Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
			},
			want: verdictSuspectedMapping,
		},
		{
			name: "suspected replacement from response model",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMatch},
				ActiveResponse: activeResponseEvidence{Status: evidenceMismatch, ReturnedModel: "gpt-5.4"},
				Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
			},
			want: verdictSuspectedReplacement,
		},
		{
			name: "suspected replacement from fingerprint",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMatch},
				ActiveResponse: activeResponseEvidence{Status: evidenceMatch, ReturnedModel: "gpt-5.6-sol"},
				Fingerprint:    fingerprintEvidence{Status: evidenceMismatch, Candidate: "gpt-5.4", Similarity: 0.98},
			},
			want: verdictSuspectedReplacement,
		},
		{
			name: "high risk conflicting evidence",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMissing},
				ActiveResponse: activeResponseEvidence{Status: evidenceMismatch, ReturnedModel: "gpt-5.4"},
				Fingerprint:    fingerprintEvidence{Status: evidenceMismatch, Candidate: "gpt-5.6-terra", Similarity: 0.97},
			},
			want: verdictHighRiskInconsistent,
		},
		{
			name: "insufficient when both requests are unavailable",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceUnavailable},
				ActiveResponse: activeResponseEvidence{Status: evidenceUnavailable},
				Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
			},
			want: verdictInsufficient,
		},
		{
			name: "insufficient when response omits model",
			evidence: detectionEvidence{
				RequestedModel: "gpt-5.6-sol",
				Catalog:        catalogEvidence{Status: evidenceMatch},
				ActiveResponse: activeResponseEvidence{Status: evidenceMissing},
				Fingerprint:    fingerprintEvidence{Status: evidenceUnavailable},
			},
			want: verdictInsufficient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyEvidence(tt.evidence); got != tt.want {
				t.Fatalf("classifyEvidence() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvidenceSummaryBoundsAndSeparatesSources(t *testing.T) {
	returned := []string{
		"gpt-5.6-sol", "gpt-5.4", "gpt-5.6-terra", "gpt-4.1", "gpt-4o",
		"o3", "o4-mini", "gpt-5", "gpt-5-mini", "gpt-5-nano", "z-must-be-truncated",
		"gpt-5.4",
	}
	evidence := detectionEvidence{
		RequestedModel: "gpt-5.6-sol",
		Catalog: catalogEvidence{
			Status:         evidenceMissing,
			ReturnedCount:  12,
			ReturnedModels: returned,
		},
		ActiveResponse: activeResponseEvidence{Status: evidenceMismatch, ReturnedModel: "gpt-5.4"},
		Fingerprint:    fingerprintEvidence{Status: evidenceMismatch, Candidate: "gpt-5.6-terra", Similarity: 0.97},
	}
	evidence.Verdict = classifyEvidence(evidence)

	summary := evidenceSummary(evidence)
	if summary["evidence_version"] != modelDetectionEvidenceVersion {
		t.Fatalf("evidence_version = %#v", summary["evidence_version"])
	}
	if summary["requested_model"] != "gpt-5.6-sol" {
		t.Fatalf("requested_model = %#v", summary["requested_model"])
	}
	active, ok := summary["active_response"].(map[string]any)
	if !ok || active["returned_model"] != "gpt-5.4" {
		t.Fatalf("active_response = %#v", summary["active_response"])
	}
	catalog, ok := summary["catalog"].(map[string]any)
	if !ok {
		t.Fatalf("catalog = %#v", summary["catalog"])
	}
	models, ok := catalog["returned_models"].([]string)
	if !ok {
		t.Fatalf("returned_models type = %T", catalog["returned_models"])
	}
	wantModels := []string{"gpt-4.1", "gpt-4o", "gpt-5", "gpt-5-mini", "gpt-5-nano", "gpt-5.4", "gpt-5.6-sol", "gpt-5.6-terra", "o3", "o4-mini"}
	if !reflect.DeepEqual(models, wantModels) {
		t.Fatalf("returned_models = %#v, want %#v", models, wantModels)
	}
	if catalog["returned_count"] != 12 {
		t.Fatalf("returned_count = %#v", catalog["returned_count"])
	}
	fingerprint, ok := summary["fingerprint"].(map[string]any)
	if !ok || fingerprint["candidate"] != "gpt-5.6-terra" || fingerprint["similarity"] != 0.97 {
		t.Fatalf("fingerprint = %#v", summary["fingerprint"])
	}
	if summary["verdict"] != verdictHighRiskInconsistent {
		t.Fatalf("verdict = %#v", summary["verdict"])
	}
}
