package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountMultiplierNativeProbeStub struct {
	snapshot *UpstreamBillingProbeSnapshot
	err      error
	calls    int
}

func (s *accountMultiplierNativeProbeStub) ProbeAccount(
	context.Context,
	int64,
) (*UpstreamBillingProbeSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type accountMultiplierMeasurementStub struct {
	calls []bool
	err   error
}

func (s *accountMultiplierMeasurementStub) RefreshAccount(
	_ context.Context,
	_ int64,
	force bool,
) (*UpstreamMultiplierMeasurementSnapshot, error) {
	s.calls = append(s.calls, force)
	return nil, s.err
}

func TestAccountMultiplierRefreshSkipsFreshDeclaration(t *testing.T) {
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	freshUntil := now.Add(time.Hour)
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status:     UpstreamBillingProbeStatusOK,
				Data:       accountMonitorDeclaredMultiplierData(0.08, observedAt),
				ReceivedAt: &observedAt,
				FreshUntil: &freshUntil,
			},
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	native := &accountMultiplierNativeProbeStub{}
	measured := &accountMultiplierMeasurementStub{}
	service := NewAccountMultiplierService(repo, native, measured)
	service.now = func() time.Time { return now }

	err := service.RefreshAccount(context.Background(), 17, false)

	require.NoError(t, err)
	require.Zero(t, native.calls)
	require.Empty(t, measured.calls)
}

func TestAccountMultiplierRefreshFallsBackOnlyForUnsupportedDeclaration(t *testing.T) {
	tests := []struct {
		name         string
		nativeStatus string
		force        bool
		wantMeasured bool
	}{
		{
			name:         "unsupported declaration",
			nativeStatus: UpstreamBillingProbeStatusUnsupported,
			wantMeasured: true,
		},
		{
			name:         "failed declaration",
			nativeStatus: UpstreamBillingProbeStatusFailed,
		},
		{
			name:         "successful declaration",
			nativeStatus: UpstreamBillingProbeStatusOK,
		},
		{
			name:         "forced unsupported declaration",
			nativeStatus: UpstreamBillingProbeStatusUnsupported,
			force:        true,
			wantMeasured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				ID:       17,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			}
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
			native := &accountMultiplierNativeProbeStub{
				snapshot: &UpstreamBillingProbeSnapshot{Status: tt.nativeStatus},
			}
			measured := &accountMultiplierMeasurementStub{}
			service := NewAccountMultiplierService(repo, native, measured)

			err := service.RefreshAccount(context.Background(), 17, tt.force)

			require.NoError(t, err)
			require.Equal(t, 1, native.calls)
			if tt.wantMeasured {
				require.Equal(t, []bool{tt.force}, measured.calls)
			} else {
				require.Empty(t, measured.calls)
			}
		})
	}
}

func TestAccountMultiplierRefreshReusesUnsupportedBackoffAndFreshMeasurement(t *testing.T) {
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	nextProbeAt := now.Add(time.Hour)
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status:      UpstreamBillingProbeStatusUnsupported,
				NextProbeAt: nextProbeAt,
			},
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	native := &accountMultiplierNativeProbeStub{}
	measured := &accountMultiplierMeasurementStub{}
	service := NewAccountMultiplierService(repo, native, measured)
	service.now = func() time.Time { return now }

	err := service.RefreshAccount(context.Background(), 17, false)

	require.NoError(t, err)
	require.Zero(t, native.calls)
	require.Equal(t, []bool{false}, measured.calls)
}

func TestAccountMultiplierRefreshPropagatesMeasurementError(t *testing.T) {
	account := &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: account}}
	native := &accountMultiplierNativeProbeStub{
		snapshot: &UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
	}
	measured := &accountMultiplierMeasurementStub{err: errors.New("measurement failed")}
	service := NewAccountMultiplierService(repo, native, measured)

	err := service.RefreshAccount(context.Background(), 17, false)

	require.EqualError(t, err, "measurement failed")
}
