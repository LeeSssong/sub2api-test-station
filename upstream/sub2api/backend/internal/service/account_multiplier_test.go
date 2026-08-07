package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

var legacyMultiplierMeasurementKey = strings.Join([]string{"account", "monitor", "multiplier", "measurement"}, "_")

func TestRateMultiplierSingleSourceUsesNativeAccountValue(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)
	nativeRate := 0.25
	account := &Account{
		RateMultiplier: &nativeRate,
		Extra: map[string]any{
			UpstreamBillingRateSyncEnabledExtraKey: true,
			UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
				Status: UpstreamBillingProbeStatusOK,
				Data: map[string]any{
					"effective_rate_multiplier": 0.5,
				},
				ReceivedAt: probeTimePtr(now.Add(-time.Minute)),
				FreshUntil: probeTimePtr(now.Add(time.Hour)),
			},
			legacyMultiplierMeasurementKey: map[string]any{
				"version": 1,
				"status":  "ok",
				"source":  "measured",
				"value":   0.75,
			},
		},
	}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceDeclared {
		t.Fatalf("Resolve() = %#v, want automatic native multiplier", got)
	}
	if got.Value == nil || math.Abs(*got.Value-0.25) > 1e-9 {
		t.Fatalf("Resolve().Value = %#v, want 0.25", got.Value)
	}
}

func TestRateMultiplierSingleSourceDerivesManualSourceFromOfficialSyncSwitch(t *testing.T) {
	nativeRate := 0.08
	account := &Account{
		RateMultiplier: &nativeRate,
		Extra: map[string]any{
			UpstreamBillingRateSyncEnabledExtraKey: false,
		},
	}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, time.Now())

	if got.Status != AccountMonitorMultiplierStatusOK || got.Source != AccountMonitorMultiplierSourceManual {
		t.Fatalf("Resolve() = %#v, want manual source from official sync switch", got)
	}
	if got.Value == nil || math.Abs(*got.Value-0.08) > 1e-9 {
		t.Fatalf("Resolve().Value = %#v, want 0.08", got.Value)
	}
}

func TestRateMultiplierAutomaticUsesProbeFreshnessMetadata(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)
	nativeRate := 0.25
	account := &Account{RateMultiplier: &nativeRate, Extra: map[string]any{
		UpstreamBillingRateSyncEnabledExtraKey: true,
		UpstreamBillingProbeExtraKey: UpstreamBillingProbeSnapshot{
			Status:     UpstreamBillingProbeStatusOK,
			ReceivedAt: probeTimePtr(now.Add(-2 * time.Hour)),
			FreshUntil: probeTimePtr(now.Add(-time.Minute)),
		},
	}}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, now)
	if got.Status != AccountMonitorMultiplierStatusStale || got.Value == nil || *got.Value != nativeRate {
		t.Fatalf("Resolve() = %#v, want stale native multiplier", got)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("Resolve().ObservedAt = %#v, want probe received_at", got.ObservedAt)
	}
}

func TestRateMultiplierAutomaticPreservesNativeValueOnProbeFailure(t *testing.T) {
	nativeRate := 0.4
	account := &Account{RateMultiplier: &nativeRate, Extra: map[string]any{
		UpstreamBillingRateSyncEnabledExtraKey: true,
		UpstreamBillingProbeExtraKey:           UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusFailed},
	}}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, time.Now())
	if got.Status != AccountMonitorMultiplierStatusFailed || got.Value == nil || *got.Value != nativeRate {
		t.Fatalf("Resolve() = %#v, want failed status with native multiplier", got)
	}
}

func TestRateMultiplierAutomaticWithoutProbeSnapshotIsUnavailable(t *testing.T) {
	nativeRate := 0.4
	account := &Account{RateMultiplier: &nativeRate, Extra: map[string]any{
		UpstreamBillingRateSyncEnabledExtraKey: true,
	}}

	got := NewAccountMultiplierService(nil, nil, nil).Resolve(account, time.Now())
	if got.Status != AccountMonitorMultiplierStatusUnavailable || got.Value == nil || *got.Value != nativeRate {
		t.Fatalf("Resolve() = %#v, want unavailable status with native multiplier", got)
	}
}

func TestRateMultiplierSingleSourceUsesBillingDefaultForLegacyNilColumn(t *testing.T) {
	got := NewAccountMultiplierService(nil, nil, nil).Resolve(&Account{}, time.Now())
	if got.Value == nil || *got.Value != 1 || got.Source != AccountMonitorMultiplierSourceManual || got.Status != AccountMonitorMultiplierStatusOK {
		t.Fatalf("Resolve() = %#v, want native BillingRateMultiplier default 1", got)
	}
}

func TestAccountMultiplierRefreshContinuesBalanceAfterDeclarationFailure(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	probeErr := errors.New("probe failed")
	probe := &accountMultiplierDeclarationProbeStub{err: probeErr}
	service := NewAccountMultiplierService(nil, nil, nil)
	service.SetDeclarationProbe(probe)

	err := service.Refresh(context.Background(), account, AccountMonitorRefreshOptions{RefreshDeclaration: true})

	if !errors.Is(err, probeErr) {
		t.Fatalf("Refresh() error = %v, want declaration failure", err)
	}
	if !reflect.DeepEqual(probe.calls, []int64{9}) {
		t.Fatalf("probe calls = %#v, want account 9", probe.calls)
	}
}

type accountMultiplierRepoStub struct {
	*upstreamBillingProbeAccountRepo
}

func (r *accountMultiplierRepoStub) UpdateAccountMonitorBalance(
	_ context.Context,
	expected *Account,
	snapshot *AccountMonitorBalance,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil || account.Platform != expected.Platform || account.Type != expected.Type ||
		!reflect.DeepEqual(account.Credentials, expected.Credentials) || !reflect.DeepEqual(account.ProxyID, expected.ProxyID) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[AccountMonitorBalanceExtraKey] = snapshot
	return nil
}

type accountMultiplierDeclarationProbeStub struct {
	calls    []int64
	snapshot *UpstreamBillingProbeSnapshot
	err      error
}

func (s *accountMultiplierDeclarationProbeStub) ProbeAccount(_ context.Context, accountID int64) (*UpstreamBillingProbeSnapshot, error) {
	s.calls = append(s.calls, accountID)
	return s.snapshot, s.err
}

func accountMultiplierJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func errorsForAccountMultiplierTest(message string) error {
	return errors.New(message)
}
