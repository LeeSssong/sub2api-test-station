package updater

import (
	"errors"
	"strings"
	"time"
)

type MigrationClass string

const (
	MigrationExpandOnly          MigrationClass = "expand_only"
	MigrationPotentiallyBreaking MigrationClass = "potentially_breaking"
	MigrationDestructive         MigrationClass = "destructive"
)

type QualificationReport struct {
	Tag             string         `json:"tag"`
	Commit          string         `json:"commit"`
	Asset           string         `json:"asset"`
	Checksum        string         `json:"checksum"`
	AdapterVersion  string         `json:"adapter_version"`
	ContractVersion int            `json:"contract_version"`
	MigrationClass  MigrationClass `json:"migration_class"`
	Tests           []string       `json:"tests"`
	DataDiff        string         `json:"data_diff"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      time.Time      `json:"finished_at"`
	StableFailure   string         `json:"stable_failure,omitempty"`
	Passed          bool           `json:"passed"`
}

func Qualify(report QualificationReport) error {
	if strings.TrimSpace(report.Tag) == "" || strings.TrimSpace(report.Commit) == "" || report.ContractVersion <= 0 {
		return errors.New("qualification identity is incomplete")
	}
	if report.MigrationClass == MigrationDestructive {
		return errors.New("destructive migration is blocked")
	}
	if !report.Passed {
		return errors.New("qualification tests did not pass")
	}
	return nil
}
