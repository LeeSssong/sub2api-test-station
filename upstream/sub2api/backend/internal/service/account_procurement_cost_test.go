package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type procurementCostAccountRepoStub struct {
	AccountRepository
	account     *Account
	updateCalls int
}

func (r *procurementCostAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *procurementCostAccountRepoStub) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

func TestUpdateAccountProcurementCostTransitions(t *testing.T) {
	firstEffectiveAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	initialCost := 8.25
	repo := &procurementCostAccountRepoStub{account: &Account{
		ID:                         91,
		Platform:                   PlatformAnthropic,
		Type:                       AccountTypeOAuth,
		Status:                     StatusActive,
		ProcurementCostCNY:         &initialCost,
		ProcurementCostEffectiveAt: &firstEffectiveAt,
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	t.Run("omitted preserves cost and effective time", func(t *testing.T) {
		updated, err := svc.UpdateAccount(context.Background(), 91, &UpdateAccountInput{})
		require.NoError(t, err)
		require.NotNil(t, updated.ProcurementCostCNY)
		require.Equal(t, 8.25, *updated.ProcurementCostCNY)
		require.Equal(t, firstEffectiveAt, *updated.ProcurementCostEffectiveAt)
	})

	t.Run("positive amount records server effective time", func(t *testing.T) {
		amount := 12.50
		before := time.Now().UTC()
		updated, err := svc.UpdateAccount(context.Background(), 91, &UpdateAccountInput{
			ProcurementCost: &ProcurementCostUpdate{Value: &amount},
		})
		after := time.Now().UTC()
		require.NoError(t, err)
		require.Equal(t, 12.50, *updated.ProcurementCostCNY)
		require.NotNil(t, updated.ProcurementCostEffectiveAt)
		require.False(t, updated.ProcurementCostEffectiveAt.Before(before))
		require.False(t, updated.ProcurementCostEffectiveAt.After(after))
	})

	t.Run("zero amount is retained", func(t *testing.T) {
		amount := 0.0
		updated, err := svc.UpdateAccount(context.Background(), 91, &UpdateAccountInput{
			ProcurementCost: &ProcurementCostUpdate{Value: &amount},
		})
		require.NoError(t, err)
		require.NotNil(t, updated.ProcurementCostCNY)
		require.Equal(t, 0.0, *updated.ProcurementCostCNY)
		require.NotNil(t, updated.ProcurementCostEffectiveAt)
	})

	t.Run("explicit null clears cost and effective time", func(t *testing.T) {
		updated, err := svc.UpdateAccount(context.Background(), 91, &UpdateAccountInput{
			ProcurementCost: &ProcurementCostUpdate{},
		})
		require.NoError(t, err)
		require.Nil(t, updated.ProcurementCostCNY)
		require.Nil(t, updated.ProcurementCostEffectiveAt)
	})
}
