package updater

import (
	"testing"
	"time"
)

func TestReleaseStateRequiresUnexpiredReady(t *testing.T) {
	now := time.Now()
	r := ReleaseState{Tag: "v1", Commit: "abc", Stage: StageReady, ReadyUntil: now.Add(time.Minute), Report: QualificationReport{Passed: true}}
	if !r.CanPromote(now) {
		t.Fatal("ready release should promote")
	}
	r.ReadyUntil = now.Add(-time.Second)
	if r.CanPromote(now) {
		t.Fatal("expired release promoted")
	}
}
