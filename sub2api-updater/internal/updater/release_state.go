package updater

import (
	"errors"
	"strings"
	"time"
)

type ReleaseStage string

const (
	StageChecking   ReleaseStage = "checking"
	StageQualifying ReleaseStage = "qualifying"
	StageReady      ReleaseStage = "ready"
	StageBlocked    ReleaseStage = "blocked"
	StagePromoting  ReleaseStage = "promoting"
)

type ReleaseState struct {
	Tag        string              `json:"tag"`
	Commit     string              `json:"commit"`
	Asset      string              `json:"asset"`
	Checksum   string              `json:"checksum"`
	Stage      ReleaseStage        `json:"stage"`
	Report     QualificationReport `json:"report"`
	ReadyUntil time.Time           `json:"ready_until,omitempty"`
}

func (s ReleaseState) Validate(now time.Time) error {
	if strings.TrimSpace(s.Tag) == "" || strings.TrimSpace(s.Commit) == "" || s.Stage == "" {
		return errors.New("release state is incomplete")
	}
	if s.Stage == StageReady && !s.ReadyUntil.After(now) {
		return errors.New("release readiness expired")
	}
	return nil
}
func (s ReleaseState) CanPromote(now time.Time) bool {
	return s.Stage == StageReady && s.ReadyUntil.After(now) && s.Report.Passed
}
