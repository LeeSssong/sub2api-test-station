package handler

import "testing"

func TestProvideHandlersWiresMonitorV4(t *testing.T) {
	monitorV4 := NewMonitorV4Handler(nil)
	handlers := ProvideHandlers(
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, monitorV4, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
	)
	if handlers == nil {
		t.Fatal("ProvideHandlers() returned nil")
	}
	if handlers.MonitorV4 != monitorV4 {
		t.Fatalf("MonitorV4 handler = %p, want %p", handlers.MonitorV4, monitorV4)
	}
}
