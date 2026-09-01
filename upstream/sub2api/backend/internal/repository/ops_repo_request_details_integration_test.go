//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetailsUsesCurrentOpsErrorSchema(t *testing.T) {
	ctx := context.Background()
	repo := NewOpsRepository(integrationDB)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	items, total, err := repo.ListRequestDetails(ctx, &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Page:      1,
		PageSize:  10,
	})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
}
