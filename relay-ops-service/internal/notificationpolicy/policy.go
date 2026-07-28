package notificationpolicy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Family string

type DeliveryMode string

const (
	FamilyGroupRuntime          Family = "group_runtime"
	FamilyGroupCapacity         Family = "group_capacity"
	FamilyAccountImpact         Family = "account_impact"
	FamilyNativeMonitorEvidence Family = "native_monitor_evidence"
	FamilyPricingNotice         Family = "pricing_notice"
	FamilyDailyDigest           Family = "daily_digest"
	FamilyIncidentEscalation    Family = "incident_escalation"

	ModeDisabled DeliveryMode = "disabled"
	ModeShadow   DeliveryMode = "shadow"
	ModeEnabled  DeliveryMode = "enabled"
)

type FeishuPolicy struct {
	GroupRuntimeEnabled          bool `json:"group_runtime_enabled"`
	GroupCapacityEnabled         bool `json:"group_capacity_enabled"`
	AccountImpactEnabled         bool `json:"account_impact_enabled"`
	NativeMonitorEvidenceEnabled bool `json:"native_monitor_evidence_enabled"`
	PricingNoticeEnabled         bool `json:"pricing_notice_enabled"`
	DailyDigestEnabled           bool `json:"daily_digest_enabled"`
	IncidentEscalationEnabled    bool `json:"incident_escalation_enabled"`
}

type Policy struct {
	Version int          `json:"version"`
	Mode    DeliveryMode `json:"delivery_mode"`
	Feishu  FeishuPolicy `json:"feishu_notifications"`
}

type rawFeishuPolicy struct {
	GroupRuntimeEnabled          *bool `json:"group_runtime_enabled"`
	GroupCapacityEnabled         *bool `json:"group_capacity_enabled"`
	AccountImpactEnabled         *bool `json:"account_impact_enabled"`
	NativeMonitorEvidenceEnabled *bool `json:"native_monitor_evidence_enabled"`
	PricingNoticeEnabled         *bool `json:"pricing_notice_enabled"`
	DailyDigestEnabled           *bool `json:"daily_digest_enabled"`
	IncidentEscalationEnabled    *bool `json:"incident_escalation_enabled"`
}

type rawPolicy struct {
	Version int             `json:"version"`
	Mode    DeliveryMode    `json:"delivery_mode"`
	Feishu  rawFeishuPolicy `json:"feishu_notifications"`
}

func Load(path string) (Policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return Policy{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	var raw rawPolicy
	if err := decoder.Decode(&raw); err != nil {
		return Policy{}, fmt.Errorf("decode notification policy: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Policy{}, err
	}
	if raw.Version != 1 {
		return Policy{}, fmt.Errorf("notification policy version must be 1")
	}
	if raw.Mode != ModeDisabled && raw.Mode != ModeShadow && raw.Mode != ModeEnabled {
		return Policy{}, fmt.Errorf("notification delivery mode must be disabled, shadow, or enabled")
	}
	fields := map[string]*bool{
		"group_runtime_enabled":           raw.Feishu.GroupRuntimeEnabled,
		"group_capacity_enabled":          raw.Feishu.GroupCapacityEnabled,
		"account_impact_enabled":          raw.Feishu.AccountImpactEnabled,
		"native_monitor_evidence_enabled": raw.Feishu.NativeMonitorEvidenceEnabled,
		"pricing_notice_enabled":          raw.Feishu.PricingNoticeEnabled,
		"daily_digest_enabled":            raw.Feishu.DailyDigestEnabled,
		"incident_escalation_enabled":     raw.Feishu.IncidentEscalationEnabled,
	}
	for name, value := range fields {
		if value == nil {
			return Policy{}, fmt.Errorf("notification policy field %s is required", name)
		}
	}
	return Policy{
		Version: raw.Version,
		Mode:    raw.Mode,
		Feishu: FeishuPolicy{
			GroupRuntimeEnabled:          *raw.Feishu.GroupRuntimeEnabled,
			GroupCapacityEnabled:         *raw.Feishu.GroupCapacityEnabled,
			AccountImpactEnabled:         *raw.Feishu.AccountImpactEnabled,
			NativeMonitorEvidenceEnabled: *raw.Feishu.NativeMonitorEvidenceEnabled,
			PricingNoticeEnabled:         *raw.Feishu.PricingNoticeEnabled,
			DailyDigestEnabled:           *raw.Feishu.DailyDigestEnabled,
			IncidentEscalationEnabled:    *raw.Feishu.IncidentEscalationEnabled,
		},
	}, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("notification policy contains trailing JSON")
		}
		return fmt.Errorf("decode notification policy trailing data: %w", err)
	}
	return nil
}

func ApprovedFamilies() []Family {
	return []Family{
		FamilyGroupRuntime,
		FamilyGroupCapacity,
		FamilyAccountImpact,
		FamilyNativeMonitorEvidence,
		FamilyPricingNotice,
		FamilyDailyDigest,
		FamilyIncidentEscalation,
	}
}

func (p Policy) Enabled(family Family) bool {
	switch family {
	case FamilyGroupRuntime:
		return p.Feishu.GroupRuntimeEnabled
	case FamilyGroupCapacity:
		return p.Feishu.GroupCapacityEnabled
	case FamilyAccountImpact:
		return p.Feishu.AccountImpactEnabled
	case FamilyNativeMonitorEvidence:
		return p.Feishu.NativeMonitorEvidenceEnabled
	case FamilyPricingNotice:
		return p.Feishu.PricingNoticeEnabled
	case FamilyDailyDigest:
		return p.Feishu.DailyDigestEnabled
	case FamilyIncidentEscalation:
		return p.Feishu.IncidentEscalationEnabled
	default:
		return false
	}
}

func (p Policy) ShouldDeliver(family Family) bool {
	return p.Mode == ModeEnabled && p.Enabled(family)
}
