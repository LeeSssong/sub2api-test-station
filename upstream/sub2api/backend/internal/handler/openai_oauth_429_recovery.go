package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// recoverOpenAIOAuth429GroupOnce gives a group one fresh candidate pass after
// transient 429 failover exhaustion. It is deliberately request-local and
// bounded so a real upstream outage cannot turn into an infinite retry loop.
func recoverOpenAIOAuth429GroupOnce(
	ctx context.Context,
	gateway *service.OpenAIGatewayService,
	groupID *int64,
	excludedIDs map[int64]struct{},
	lastErr *service.UpstreamFailoverError,
	streamStarted bool,
	consumed *bool,
	scope *service.OpenAIRecoveryScope,
) bool {
	if gateway == nil || groupID == nil || *groupID <= 0 || len(excludedIDs) == 0 || streamStarted ||
		lastErr == nil || lastErr.StatusCode != 429 || consumed == nil || *consumed {
		return false
	}
	*consumed = true
	clearedIDs, err := gateway.RefreshOpenAIOAuth429Group(ctx, *groupID, excludedIDs)
	if err != nil || len(clearedIDs) == 0 {
		return false
	}
	for accountID := range clearedIDs {
		delete(excludedIDs, accountID)
	}
	if scope != nil {
		gateway.ClearOpenAIRecoveryExcludedAccountIDs(*scope, clearedIDs)
	}
	return true
}
