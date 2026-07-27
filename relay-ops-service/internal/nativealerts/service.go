package nativealerts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type IncidentRunner interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type AnalysisRunner interface {
	AnalyzeOnce(context.Context, agent.IncidentContractV1) (agent.Analysis, error)
}

type MessageSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type Observation struct {
	Monitor sub2api.ChannelMonitor
	History sub2api.MonitorHistory
}

type Result struct {
	IncidentID   string `json:"incident_id"`
	State        string `json:"state"`
	Transition   string `json:"transition"`
	AgentStatus  string `json:"agent_status"`
	Notification string `json:"notification"`
}

type Service struct {
	Incidents IncidentRunner
	Agent     AnalysisRunner
	Notifier  MessageSender
}

func (s Service) ObserveMonitor(ctx context.Context, monitor sub2api.ChannelMonitor, history sub2api.MonitorHistory) error {
	_, err := s.Observe(ctx, Observation{Monitor: monitor, History: history})
	return err
}

func (s Service) Observe(ctx context.Context, observation Observation) (Result, error) {
	if s.Incidents == nil {
		return Result{}, fmt.Errorf("native alert incident runner is required")
	}
	key := incidentKey(observation.Monitor, observation.History)
	status := strings.ToLower(strings.TrimSpace(observation.History.Status))
	if status == "" {
		status = "error"
	}
	failing := status != "operational"
	evidence := evidenceHash(observation.Monitor, observation.History)
	transition, err := s.Incidents.Observe(ctx, incidents.Observation{
		Key: key, Severity: "P1", Failing: failing, EvidenceHash: evidence,
		CurrentValue: monitorValue(status, observation.History.LatencyMS),
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{
		IncidentID: key, State: transition.State, Transition: transition.Kind,
		AgentStatus: "not_run", Notification: "suppressed",
	}
	if !transition.Notify {
		return result, nil
	}
	contract := agent.IncidentContractV1{
		ContractVersion: "relay-ops-incident-v1",
		IncidentID:      key + ":" + transition.Kind + ":" + evidence[:16],
		Severity:        "P1",
		Upstream:        "Sub2API native monitor",
		PublicGroup:     observation.Monitor.GroupName,
		Model:           observation.History.Model,
		MetricName:      "native_monitor_status",
		BaselineValue:   "operational",
		CurrentValue:    monitorValue(status, observation.History.LatencyMS),
		Samples:         1,
		EvidenceRefs:    []string{"native-monitor:" + strconv.FormatInt(observation.Monitor.ID, 10)},
		AllowedActions:  []string{"observe", "request_human_review"},
	}
	analysis := agent.Fallback(contract)
	if s.Agent != nil {
		if generated, analyzeErr := s.Agent.AnalyzeOnce(ctx, contract); analyzeErr == nil {
			analysis = generated
			result.AgentStatus = "completed"
		} else {
			result.AgentStatus = "fallback_after_error"
		}
	} else {
		result.AgentStatus = "fallback"
	}
	if s.Notifier == nil {
		result.Notification = "not_configured"
		return result, nil
	}
	message := renderMessage(observation, transition, analysis)
	deliveryEvidence := transition.Kind + ":" + evidence
	if err := s.Notifier.SendIncident(ctx, key, deliveryEvidence, message); err != nil {
		result.Notification = "failed"
		return result, err
	}
	result.Notification = "delivered"
	return result, nil
}

func incidentKey(monitor sub2api.ChannelMonitor, history sub2api.MonitorHistory) string {
	model := history.Model
	if model == "" {
		model = monitor.PrimaryModel
	}
	return "native-monitor:" + strconv.FormatInt(monitor.ID, 10) + ":" + model
}

func evidenceHash(monitor sub2api.ChannelMonitor, history sub2api.MonitorHistory) string {
	value := fmt.Sprintf("%d|%s|%s|%s", monitor.ID, history.Model, strings.ToLower(strings.TrimSpace(history.Status)), monitor.GroupName)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func monitorValue(status string, latency int64) string {
	if latency <= 0 {
		return status
	}
	return status + " (" + strconv.FormatInt(latency, 10) + "ms)"
}

func renderMessage(observation Observation, transition incidents.Transition, analysis agent.Analysis) notify.FeishuMessage {
	status := strings.ToLower(strings.TrimSpace(observation.History.Status))
	title := "上游原生监控异常：" + observation.Monitor.GroupName
	change := "监控状态变为 " + status
	focus := "人工查看原生 Monitor 和用户完整链路；不要自动改路由、价格或 Key"
	if transition.Kind == "recovered" {
		return renderRecoveryMessage(observation)
	}
	return notify.RenderFeishu(notify.IncidentView{
		Title: title, Severity: "P1", Recovery: transition.Kind == "recovered",
		Current: monitorValue(status, observation.History.LatencyMS), Baseline: "operational", Analysis: analysis.Summary,
		WhatWasDone: []string{
			"读取 Sub2API 原生 Channel Monitor 最新状态",
			"按两窗口规则确认事件并执行只读分析",
		},
		Results: []string{
			"模型：" + observation.History.Model,
			"状态：" + monitorValue(status, observation.History.LatencyMS),
			"分析：" + analysis.Summary,
		},
		Change: change,
		Focus:  focus,
		Links:  []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
}

func renderRecoveryMessage(observation Observation) notify.FeishuMessage {
	metrics := []notify.RecoveryMetric{
		{Label: "当前状态", Value: "运行正常"},
		{Label: "监控模型", Value: observation.History.Model},
	}
	if observation.History.LatencyMS > 0 {
		metrics = append(metrics, notify.RecoveryMetric{
			Label: "最新延迟",
			Value: strconv.FormatInt(observation.History.LatencyMS, 10) + "ms",
		})
	}
	if checkedAt, err := time.Parse(time.RFC3339, observation.History.CheckedAt); err == nil {
		metrics = append(metrics, notify.RecoveryMetric{
			Label: "证据时间",
			Value: checkedAt.UTC().Format("15:04 UTC"),
		})
	}
	return notify.RenderRecoveryCard(notify.RecoveryCardView{
		Title:   "上游原生监控已恢复：" + observation.Monitor.GroupName,
		Summary: "原生监控已恢复",
		Detail:  "监控状态已回到正常基线",
		Metrics: metrics,
		Basis:   []string{"当前状态为运行正常"},
		Source:  "Sub2API 原生 Channel Monitor 最新状态",
		Focus:   "继续观察恢复后的成功率和首字延迟",
		Links:   []notify.Link{{Label: "运维后台", URL: "/ops"}},
	})
}
