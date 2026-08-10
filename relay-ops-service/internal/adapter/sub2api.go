package adapter

import (
	"context"
	"errors"

	"example.invalid/relay-ops-service/internal/billing"
)

type OfficialClient interface {
	RefreshAccount(context.Context, int64, string) error
}

type OfficialAccountUpdater interface {
	UpdateAccount(context.Context, billing.AccountUpdateCommand) error
}
type Sub2API struct{ Client OfficialClient }

func (a Sub2API) SendAccountUpdate(ctx context.Context, id int64, key string) error {
	if a.Client == nil {
		return context.Canceled
	}
	return a.Client.RefreshAccount(ctx, id, key)
}

func (a Sub2API) SendAccountUpdateCommand(ctx context.Context, command billing.AccountUpdateCommand) error {
	if a.Client == nil {
		return context.Canceled
	}
	if err := command.Validate(); err != nil {
		return err
	}
	updater, ok := a.Client.(OfficialAccountUpdater)
	if !ok {
		return errors.New("official account update API is unavailable")
	}
	return updater.UpdateAccount(ctx, command)
}
