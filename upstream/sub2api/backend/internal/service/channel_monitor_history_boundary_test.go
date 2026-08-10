package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type historyBoundaryMonitorRepoStub struct {
	ChannelMonitorRepository
	monitor *ChannelMonitor
	updated *ChannelMonitor
}

func (r *historyBoundaryMonitorRepoStub) GetByID(_ context.Context, _ int64) (*ChannelMonitor, error) {
	copy := *r.monitor
	copy.GroupID = cloneInt64Pointer(r.monitor.GroupID)
	copy.TemplateID = cloneInt64Pointer(r.monitor.TemplateID)
	copy.ExtraModels = append([]string(nil), r.monitor.ExtraModels...)
	return &copy, nil
}

func (r *historyBoundaryMonitorRepoStub) Update(_ context.Context, monitor *ChannelMonitor) error {
	copy := *monitor
	copy.GroupID = cloneInt64Pointer(monitor.GroupID)
	copy.TemplateID = cloneInt64Pointer(monitor.TemplateID)
	copy.ExtraModels = append([]string(nil), monitor.ExtraModels...)
	r.updated = &copy
	return nil
}

type historyBoundaryEncryptor struct{}

func (historyBoundaryEncryptor) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (historyBoundaryEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

func TestChannelMonitorUpdateHistoryBoundary(t *testing.T) {
	oldBoundary := time.Date(2026, time.August, 9, 7, 0, 0, 0, time.UTC)
	newBoundary := time.Date(2026, time.August, 9, 8, 0, 0, 0, time.UTC)
	groupID := int64(11)
	templateID := int64(12)

	tests := []struct {
		name          string
		update        ChannelMonitorUpdateParams
		resetExpected bool
	}{
		{name: "provider", update: ChannelMonitorUpdateParams{Provider: stringPointer(MonitorProviderAnthropic)}, resetExpected: true},
		{name: "api mode", update: ChannelMonitorUpdateParams{APIMode: stringPointer(MonitorAPIModeResponses)}, resetExpected: true},
		{name: "endpoint", update: ChannelMonitorUpdateParams{Endpoint: stringPointer("https://1.1.1.1")}, resetExpected: true},
		{name: "primary model", update: ChannelMonitorUpdateParams{PrimaryModel: stringPointer("gpt-5.4-mini")}, resetExpected: true},
		{name: "api key", update: ChannelMonitorUpdateParams{APIKey: stringPointer("new-api-key")}, resetExpected: true},
		{name: "group id", update: ChannelMonitorUpdateParams{GroupID: int64Pointer(13)}, resetExpected: true},
		{name: "name", update: ChannelMonitorUpdateParams{Name: stringPointer("renamed")}},
		{name: "interval", update: ChannelMonitorUpdateParams{IntervalSeconds: intPointer(120)}},
		{name: "jitter", update: ChannelMonitorUpdateParams{JitterSeconds: intPointer(20)}},
		{name: "enabled", update: ChannelMonitorUpdateParams{Enabled: boolPointer(false)}},
		{name: "template", update: ChannelMonitorUpdateParams{TemplateID: int64Pointer(99)}},
		{name: "extra models", update: ChannelMonitorUpdateParams{ExtraModels: &[]string{"gpt-5.4-nano"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &historyBoundaryMonitorRepoStub{monitor: &ChannelMonitor{
				ID:               7,
				Name:             "original",
				Provider:         MonitorProviderOpenAI,
				APIMode:          MonitorAPIModeChatCompletions,
				Endpoint:         "https://api.example.com",
				APIKey:           "encrypted:old-api-key",
				PrimaryModel:     "gpt-5.4",
				ExtraModels:      []string{"gpt-5.4-nano"},
				GroupID:          &groupID,
				Enabled:          true,
				IntervalSeconds:  60,
				JitterSeconds:    0,
				TemplateID:       &templateID,
				HistoryStartedAt: oldBoundary,
			}}
			service := NewChannelMonitorService(repo, historyBoundaryEncryptor{})
			service.now = func() time.Time { return newBoundary }

			_, err := service.Update(context.Background(), 7, tt.update)

			require.NoError(t, err)
			require.NotNil(t, repo.updated)
			if tt.resetExpected {
				require.Equal(t, newBoundary, repo.updated.HistoryStartedAt)
			} else {
				require.Equal(t, oldBoundary, repo.updated.HistoryStartedAt)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }
func int64Pointer(value int64) *int64    { return &value }
func intPointer(value int) *int          { return &value }
func boolPointer(value bool) *bool       { return &value }
