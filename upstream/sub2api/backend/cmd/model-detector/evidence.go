package main

import (
	"sort"
	"strings"
)

const (
	modelDetectionEvidenceVersion = "model-detection-evidence-v1"
	maxEvidenceModels             = 10
	maxEvidenceModelIDLength      = 128

	evidenceMatch       = "match"
	evidenceMismatch    = "mismatch"
	evidenceMissing     = "missing"
	evidenceUnavailable = "unavailable"

	verdictVerified             = "verified"
	verdictSuspectedMapping     = "suspected_mapping"
	verdictSuspectedReplacement = "suspected_replacement"
	verdictHighRiskInconsistent = "high_risk_inconsistent"
	verdictInsufficient         = "insufficient"
)

type catalogEvidence struct {
	Status         string
	ReturnedCount  int
	ReturnedModels []string
}

type activeResponseEvidence struct {
	Status        string
	ReturnedModel string
}

type fingerprintEvidence struct {
	Status     string
	Candidate  string
	Similarity float64
}

type detectionEvidence struct {
	RequestedModel string
	Catalog        catalogEvidence
	ActiveResponse activeResponseEvidence
	Fingerprint    fingerprintEvidence
	Verdict        string
}

func classifyEvidence(evidence detectionEvidence) string {
	activeMismatch := evidence.ActiveResponse.Status == evidenceMismatch
	fingerprintMismatch := evidence.Fingerprint.Status == evidenceMismatch

	if activeMismatch && fingerprintMismatch {
		returned := strings.TrimSpace(evidence.ActiveResponse.ReturnedModel)
		candidate := strings.TrimSpace(evidence.Fingerprint.Candidate)
		if evidence.Catalog.Status == evidenceMissing || (returned != "" && candidate != "" && !strings.EqualFold(returned, candidate)) {
			return verdictHighRiskInconsistent
		}
		return verdictSuspectedReplacement
	}
	if activeMismatch || fingerprintMismatch {
		return verdictSuspectedReplacement
	}
	if evidence.Catalog.Status == evidenceMissing && evidence.ActiveResponse.Status == evidenceMatch {
		return verdictSuspectedMapping
	}
	if evidence.Catalog.Status == evidenceMatch && evidence.ActiveResponse.Status == evidenceMatch && evidence.Fingerprint.Status != evidenceMismatch {
		return verdictVerified
	}
	return verdictInsufficient
}

func evidenceSummary(evidence detectionEvidence) map[string]any {
	verdict := strings.TrimSpace(evidence.Verdict)
	if verdict == "" {
		verdict = classifyEvidence(evidence)
	}
	return map[string]any{
		"evidence_version": modelDetectionEvidenceVersion,
		"requested_model":  boundedEvidenceModelID(evidence.RequestedModel),
		"catalog": map[string]any{
			"status":          evidence.Catalog.Status,
			"returned_count":  evidence.Catalog.ReturnedCount,
			"returned_models": boundedEvidenceModels(evidence.Catalog.ReturnedModels),
		},
		"active_response": map[string]any{
			"status":         evidence.ActiveResponse.Status,
			"returned_model": boundedEvidenceModelID(evidence.ActiveResponse.ReturnedModel),
		},
		"fingerprint": map[string]any{
			"status":     evidence.Fingerprint.Status,
			"candidate":  boundedEvidenceModelID(evidence.Fingerprint.Candidate),
			"similarity": evidence.Fingerprint.Similarity,
		},
		"verdict": verdict,
	}
}

func boundedEvidenceModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, min(len(models), maxEvidenceModels))
	for _, model := range models {
		model = boundedEvidenceModelID(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	sort.Strings(result)
	if len(result) > maxEvidenceModels {
		result = result[:maxEvidenceModels]
	}
	return result
}

func boundedEvidenceModelID(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > maxEvidenceModelIDLength {
		model = model[:maxEvidenceModelIDLength]
	}
	return model
}
