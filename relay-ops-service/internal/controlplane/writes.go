package controlplane

import "context"

type CommandSender interface {
	SendAccountUpdate(context.Context, int64, string) error
}
type CommandRefresher struct{ Sender CommandSender }

func (r CommandRefresher) RefreshAccount(ctx context.Context, id int64, key string) error {
	if r.Sender == nil {
		return context.Canceled
	}
	return r.Sender.SendAccountUpdate(ctx, id, key)
}
