package adapter

import "context"

type OfficialClient interface {
	RefreshAccount(context.Context, int64, string) error
}
type Sub2API struct{ Client OfficialClient }

func (a Sub2API) SendAccountUpdate(ctx context.Context, id int64, key string) error {
	if a.Client == nil {
		return context.Canceled
	}
	return a.Client.RefreshAccount(ctx, id, key)
}
