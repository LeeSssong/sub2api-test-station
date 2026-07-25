package service

import (
	"testing"
	"time"
)

func TestAccountMonitorRunnerReloadSettingsResetsCadenceWithoutTriggeringProbe(t *testing.T) {
	runner := NewAccountMonitorRunner(nil)
	runner.ReloadSettings(AccountMonitorSettings{IntervalSeconds: 60})

	runner.mu.Lock()
	interval := runner.interval
	runner.mu.Unlock()
	if interval != 60*time.Second {
		t.Fatalf("interval = %s", interval)
	}

	select {
	case <-runner.trigger:
		t.Fatal("settings update must not trigger a real probe")
	default:
	}
	select {
	case <-runner.reload:
	default:
		t.Fatal("settings update did not notify the cadence loop")
	}
}
