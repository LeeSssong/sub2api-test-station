package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
)

const syntheticIncidentID = "synthetic:relay-ops:acceptance:v1"

type IncidentRunner interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type Result struct {
	IncidentID            string `json:"incident_id"`
	State                 string `json:"state"`
	Transition            string `json:"transition"`
	AgentStatus           string `json:"agent_status"`
	Notification          string `json:"notification"`
	AlertNotification     string `json:"alert_notification"`
	DuplicateNotification string `json:"duplicate_notification"`
	RecoveryNotification  string `json:"recovery_notification"`
	ExternalUpstream      string `json:"external_upstream"`
}

type Service struct {
	Incidents IncidentRunner
	Agent     AnalysisRunner
	Notifier  MessageSender
}

// Run emits one fixed, fully redacted event. It deliberately has no upstream,
// probe, billing, routing, balance, or key dependency.
func (s Service) Run(ctx context.Context) (Result, error) {
	if s.Incidents == nil {
		return Result{}, fmt.Errorf("incident runner is required")
	}
	evidence := sha256.Sum256([]byte(fmt.Sprintf("%s|agent=%t|notifier=%t", syntheticIncidentID, s.Agent != nil, s.Notifier != nil)))
	evidenceHash := hex.EncodeToString(evidence[:])
	alertTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: true,
		EvidenceHash: evidenceHash, CurrentValue: "synthetic acceptance event",
		ConfirmationWindows: 1,
	})
	if err != nil {
		return Result{}, fmt.Errorf("record synthetic incident: %w", err)
	}
	result := Result{
		IncidentID: syntheticIncidentID, State: alertTransition.State, Transition: alertTransition.Kind,
		AgentStatus: "fallback", Notification: "not_configured",
		AlertNotification: "not_configured", DuplicateNotification: "not_run", RecoveryNotification: "not_configured",
		ExternalUpstream: "not_accessed",
	}
	if s.Agent != nil {
		result.AgentStatus = "not_run"
	}
	if s.Notifier != nil {
		result.Notification = "not_run"
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1", IncidentID: syntheticIncidentID,
		Severity: "P1", Upstream: "relay-ops synthetic", MetricName: "synthetic_acceptance",
		BaselineValue: "none", CurrentValue: "synthetic acceptance event", Samples: 1,
		EvidenceRefs:   []string{"synthetic:relay-ops-incident-v1"},
		AllowedActions: []string{"observe", "request_human_review"},
	}
	analysis := agent.Fallback(contract)
	if alertTransition.Notify && s.Agent != nil {
		if generated, analyzeErr := s.Agent.AnalyzeOnce(ctx, contract); analyzeErr == nil {
			analysis = generated
			result.AgentStatus = "completed"
		} else {
			result.AgentStatus = "fallback_after_error"
		}
	}
	alertMessage := notify.RenderFeishu(notify.IncidentView{
		Title:    "relay-ops 合成告警验收",
		Severity: "P1", Current: alertTransition.State, Baseline: "recovered", Analysis: analysis.Summary,
		WhatWasDone: []string{
			"生成固定脱敏事件并写入本地事件状态机",
			"调用 Agent 分析链路（如已配置）",
			"未访问任何上游、未产生请求费用",
		},
		Results: []string{"状态：" + alertTransition.State, "分析：" + analysis.Summary, "外部上游：未访问"},
		Change:  "这是一次人工触发的合成事件，不代表真实上游故障",
		Focus:   "确认飞书消息已收到；无需修改价格、路由、余额或 Key",
		Links:   []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
	if alertTransition.Notify && s.Notifier != nil {
		if err := s.Notifier.SendIncident(ctx, syntheticIncidentID, evidenceHash, alertMessage); err != nil {
			result.AlertNotification = "failed"
		} else {
			result.AlertNotification = "delivered"
		}
	} else if !alertTransition.Notify {
		result.AlertNotification = "suppressed"
	}

	duplicateTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: true,
		EvidenceHash: evidenceHash, CurrentValue: "synthetic acceptance event",
		ConfirmationWindows: 1,
	})
	if err != nil {
		return Result{}, fmt.Errorf("record duplicate synthetic incident: %w", err)
	}
	if duplicateTransition.Notify {
		result.DuplicateNotification = "unexpected_notify"
	} else {
		result.DuplicateNotification = "suppressed"
	}

	recoveryHash := digest(evidenceHash + ":recovered")
	recoveryTransition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: syntheticIncidentID, Severity: "P1", Failing: false,
		EvidenceHash: recoveryHash, CurrentValue: "synthetic acceptance recovered",
	})
	if err != nil {
		return Result{}, fmt.Errorf("recover synthetic incident: %w", err)
	}
	result.State = recoveryTransition.State
	result.Transition = recoveryTransition.Kind
	if recoveryTransition.Notify && s.Notifier != nil {
		recoveryMessage := notify.RenderRecoveryCard(notify.RecoveryCardView{
			Title:   "relay-ops 合成告警已恢复",
			Summary: "合成事件已恢复",
			Detail:  "合成事件已从告警恢复，真实服务未受影响",
			Metrics: []notify.RecoveryMetric{
				{Label: "当前状态", Value: "已恢复"},
				{Label: "重复事件", Value: "已抑制"},
				{Label: "真实服务影响", Value: "无"},
			},
			Basis:      []string{"固定事件已转为恢复状态，重复告警未再次发送", analysis.Summary},
			Source:     "relay-ops 合成验收（未访问上游）",
			Focus:      "确认飞书只收到一条告警和一条恢复通知",
			Links:      []notify.Link{{Label: "运维后台", URL: "/ops"}},
			Suppressed: true,
		})
		if err := s.Notifier.SendIncident(ctx, syntheticIncidentID, recoveryHash, recoveryMessage); err != nil {
			result.RecoveryNotification = "failed"
		} else {
			result.RecoveryNotification = "delivered"
		}
	} else if !recoveryTransition.Notify {
		result.RecoveryNotification = "suppressed"
	}

	result.Notification = overallNotification(result)
	return result, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func overallNotification(result Result) string {
	if result.AlertNotification == "failed" || result.RecoveryNotification == "failed" || result.DuplicateNotification == "unexpected_notify" {
		return "failed"
	}
	if result.AlertNotification == "not_configured" || result.RecoveryNotification == "not_configured" {
		return "not_configured"
	}
	return "delivered"
}
