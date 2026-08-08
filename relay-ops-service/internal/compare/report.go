package compare

import "time"

type ReadMode string

const (
	LegacyOnly        ReadMode = "legacy_only"
	ShadowBuilding    ReadMode = "shadow_building"
	DualReadComparing ReadMode = "dual_read_comparing"
	ExternalPrimary   ReadMode = "external_primary"
	LegacyRetired     ReadMode = "legacy_retired"
)

type CompareReport struct {
	Window           time.Duration     `json:"window"`
	Counts           map[string]int64  `json:"counts"`
	DecimalAmounts   map[string]string `json:"decimal_amounts"`
	RateVersions     map[string]string `json:"rate_versions"`
	Freshness        map[string]int64  `json:"freshness"`
	PermissionPassed bool              `json:"permission_passed"`
	ExportPassed     bool              `json:"export_passed"`
	Degraded         bool              `json:"degraded"`
	RollbackPassed   bool              `json:"rollback_passed"`
	Passed           bool              `json:"passed"`
}

func (r CompareReport) Eligible() bool {
	return r.Passed && r.PermissionPassed && r.ExportPassed && r.RollbackPassed && !r.Degraded
}
