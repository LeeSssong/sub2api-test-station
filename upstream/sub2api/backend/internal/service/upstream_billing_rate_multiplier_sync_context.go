package service

import "context"

const (
	UpstreamBillingRateMultiplierSyncTriggerScheduled = "scheduled"
	UpstreamBillingRateMultiplierSyncTriggerLifecycle = "lifecycle"
)

type upstreamBillingRateMultiplierSyncTriggerContextKey struct{}

// WithUpstreamBillingRateMultiplierSyncTrigger marks the source that caused a
// native billing probe. Scheduled is the safe default for existing callers;
// lifecycle callers opt in explicitly.
func WithUpstreamBillingRateMultiplierSyncTrigger(ctx context.Context, trigger string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, upstreamBillingRateMultiplierSyncTriggerContextKey{}, trigger)
}

// UpstreamBillingRateMultiplierSyncTriggerFromContext returns a validated
// audit trigger without making the persistence path trust arbitrary strings.
func UpstreamBillingRateMultiplierSyncTriggerFromContext(ctx context.Context) string {
	if ctx == nil {
		return UpstreamBillingRateMultiplierSyncTriggerScheduled
	}
	trigger, _ := ctx.Value(upstreamBillingRateMultiplierSyncTriggerContextKey{}).(string)
	if trigger == UpstreamBillingRateMultiplierSyncTriggerLifecycle {
		return trigger
	}
	return UpstreamBillingRateMultiplierSyncTriggerScheduled
}
