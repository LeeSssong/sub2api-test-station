package controlplane

import (
	"context"
	"errors"
)

type CommandSender interface {
	SendAccountUpdate(context.Context, int64, string) error
}
type AuditWriter interface {
	RecordExternalizationCommand(context.Context, int64, int64, string, string, int) error
}
type CommandRefresher struct {
	Sender CommandSender
	Audit  AuditWriter
}

func (r CommandRefresher) RefreshAccount(ctx context.Context, id int64, key string) error {
	if r.Sender == nil {
		return context.Canceled
	}
	identity, ok := IdentityFromContext(ctx)
	if !ok || identity.UserID <= 0 {
		return errors.New("admin identity is required")
	}
	if r.Audit != nil {
		if err := r.Audit.RecordExternalizationCommand(ctx, identity.UserID, id, key, "pending", 1); err != nil {
			return err
		}
	}
	err := r.Sender.SendAccountUpdate(ctx, id, key)
	result := "accepted"
	if err != nil {
		result = "failed"
	}
	if r.Audit != nil {
		if auditErr := r.Audit.RecordExternalizationCommand(ctx, identity.UserID, id, key, result, 1); auditErr != nil && err == nil {
			err = auditErr
		}
	}
	return err
}
