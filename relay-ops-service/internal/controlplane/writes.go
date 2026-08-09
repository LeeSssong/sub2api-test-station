package controlplane

import (
	"context"
	"errors"

	"example.invalid/relay-ops-service/internal/adminauth"
)

type CommandSender interface {
	SendAccountUpdate(context.Context, int64, string) error
}
type AuditWriter interface {
	RecordExternalizationCommand(context.Context, int64, int64, string, string, int) error
}
type IdempotencyWriter interface {
	ClaimExternalizationCommand(context.Context, int64, int64, string, string, int) (bool, string, error)
	CompleteExternalizationCommand(context.Context, int64, int64, string, string, int) error
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
	actorID := identity.UserID
	if actor, adminOK := adminauth.ActorFromContext(ctx); adminOK {
		actorID = actor.UserID
	}
	if (!ok && actorID <= 0) || actorID <= 0 {
		return errors.New("admin identity is required")
	}
	if idempotency, ok := r.Audit.(IdempotencyWriter); ok {
		dispatch, _, err := idempotency.ClaimExternalizationCommand(ctx, actorID, id, key, "refresh_account", 1)
		if err != nil {
			return err
		}
		if !dispatch {
			return nil
		}
		err = r.Sender.SendAccountUpdate(ctx, id, key)
		result := "accepted"
		if err != nil {
			result = "failed"
		}
		if completeErr := idempotency.CompleteExternalizationCommand(ctx, actorID, id, key, result, 1); completeErr != nil && err == nil {
			return completeErr
		}
		return err
	}
	if r.Audit != nil {
		if err := r.Audit.RecordExternalizationCommand(ctx, actorID, id, key, "pending", 1); err != nil {
			return err
		}
	}
	err := r.Sender.SendAccountUpdate(ctx, id, key)
	result := "accepted"
	if err != nil {
		result = "failed"
	}
	if r.Audit != nil {
		if auditErr := r.Audit.RecordExternalizationCommand(ctx, actorID, id, key, result, 1); auditErr != nil && err == nil {
			err = auditErr
		}
	}
	return err
}
