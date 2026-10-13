//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIntelligenceDrawingClaimAndCompletion(t *testing.T) {
	ctx := context.Background()
	repo := NewPelicanGroupTestRepository(integrationDB)
	group := createPelicanGroupTestGroups(t, "drawing-judge")[0]
	plan, err := repo.CreatePlan(ctx, newPelicanGroupTestPlan(group.ID, true, time.Now()))
	require.NoError(t, err)
	cfg := *plan.PelicanConfig
	cfg.IntelligenceResult = &service.IntelligenceResultMetadata{Action: "划着小船", Scene: "沙漠里", DrawingJudgment: &service.DrawingJudgment{Status: "pending", Deadline: time.Now().Add(time.Minute)}}
	saved, err := repo.CreateResult(ctx, &service.PelicanGroupTestResult{PlanID: plan.ID, Status: "success", ResponseText: "<svg/>", PelicanConfig: &cfg, StartedAt: time.Now(), FinishedAt: time.Now()})
	require.NoError(t, err)
	jrepo := repo.(*pelicanGroupTestRepository)
	won, err := jrepo.ClaimDrawingJudgment(ctx, saved.ID)
	require.NoError(t, err)
	require.True(t, won)
	won, err = jrepo.ClaimDrawingJudgment(ctx, saved.ID)
	require.NoError(t, err)
	require.False(t, won)
	require.NoError(t, jrepo.CompleteDrawingJudgment(ctx, saved.ID, &service.DrawingJudgment{Status: "incorrect", Reason: "人车分离"}))
	require.NoError(t, jrepo.CompleteDrawingJudgment(ctx, saved.ID, &service.DrawingJudgment{Status: "passed"}))
	got, err := repo.GetResult(ctx, saved.ID)
	require.NoError(t, err)
	require.Equal(t, "success", got.Status)
	require.Equal(t, "<svg/>", got.ResponseText)
	require.Equal(t, "划着小船", got.PelicanConfig.IntelligenceResult.Action)
	require.Equal(t, "incorrect", got.PelicanConfig.IntelligenceResult.DrawingJudgment.Status)
}
