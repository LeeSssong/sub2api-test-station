package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var errUnsafeContract = errors.New("agent incident contract contains prohibited data")

var emailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
var ipv4Pattern = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

type IncidentContractV1 struct {
	ContractVersion string   `json:"contract_version"`
	IncidentID      string   `json:"incident_id"`
	Severity        string   `json:"severity"`
	Upstream        string   `json:"upstream"`
	PublicGroup     string   `json:"public_group,omitempty"`
	Model           string   `json:"model,omitempty"`
	MetricName      string   `json:"metric_name"`
	BaselineValue   string   `json:"baseline_value,omitempty"`
	CurrentValue    string   `json:"current_value"`
	Samples         int64    `json:"samples"`
	EvidenceRefs    []string `json:"evidence_refs"`
	AllowedActions  []string `json:"allowed_actions"`
}

type Hypothesis struct {
	Cause        string   `json:"cause"`
	Confidence   float64  `json:"confidence"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type Analysis struct {
	Summary               string       `json:"summary"`
	WhatWasDone           []string     `json:"what_was_done"`
	Result                []string     `json:"result"`
	Change                string       `json:"change"`
	Focus                 string       `json:"focus"`
	Hypotheses            []Hypothesis `json:"hypotheses"`
	RecommendedAction     string       `json:"recommended_action"`
	RequiresHumanApproval bool         `json:"requires_human_approval"`
	Confidence            float64      `json:"confidence"`
}

func IsUnsafeContract(err error) bool { return errors.Is(err, errUnsafeContract) }

func validateContract(contract IncidentContractV1) error {
	if contract.ContractVersion != "relay-ops-incident-v1" || contract.IncidentID == "" || contract.Severity == "" || contract.Upstream == "" || contract.MetricName == "" || contract.CurrentValue == "" {
		return errors.New("agent incident contract is incomplete")
	}
	data, _ := json.Marshal(contract)
	lower := strings.ToLower(string(data))
	prohibited := []string{"authorization:", "bearer ", "sk-", "cookie", "password", "api_key", "jwt", "<html", "<body"}
	for _, marker := range prohibited {
		if strings.Contains(lower, marker) {
			return errUnsafeContract
		}
	}
	if emailPattern.Match(data) || ipv4Pattern.Match(data) {
		return errUnsafeContract
	}
	if len(data) > 32<<10 {
		return errors.New("agent incident contract exceeds size limit")
	}
	return nil
}

func ValidateAgentOutput(data []byte) (Analysis, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return Analysis{}, errors.New("agent output size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Analysis
	if err := decoder.Decode(&result); err != nil {
		return Analysis{}, errors.New("agent output is not valid JSON")
	}
	if result.Summary == "" || len(result.WhatWasDone) == 0 || len(result.Result) == 0 || result.Change == "" || result.Focus == "" || result.RecommendedAction == "" || result.Confidence < 0 || result.Confidence > 1 {
		return Analysis{}, errors.New("agent output schema is incomplete")
	}
	return result, nil
}

func Fallback(contract IncidentContractV1) Analysis {
	return Analysis{Summary: contract.Upstream + " 的 " + contract.MetricName + " 事件需要观察", WhatWasDone: []string{"确定性监控已记录事件"}, Result: []string{"当前值 " + contract.CurrentValue}, Change: "基线 " + contract.BaselineValue + "，当前 " + contract.CurrentValue, Focus: "查看事件证据并继续观察", Hypotheses: []Hypothesis{}, RecommendedAction: "observe", RequiresHumanApproval: false, Confidence: 0}
}
