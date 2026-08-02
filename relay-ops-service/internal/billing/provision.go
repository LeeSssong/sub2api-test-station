package billing

import (
	"context"
	"errors"
	"fmt"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/upstreams"
)

var ErrBillingProvisionConflict = errors.New("billing provision conflicts with existing configuration")

type BillingProvisionInput struct {
	Production       upstreams.ProductionInput
	BearerSecretFile string
	LoginURL         string
	BillingAccountID int64
}

type BillingProvisionRecord struct {
	Production upstreams.ProductionRecord
	Session    BillingReadSessionRecord
}

type BillingProvisionResult struct {
	UpstreamID        domain.UpstreamID
	BillingAccountID  int64
	AlreadyConfigured bool
}

type ProductionPreparer interface {
	PrepareProduction(context.Context, domain.AdminActor, upstreams.ProductionInput) (upstreams.ProductionRecord, error)
}

type BillingReadPreparer interface {
	PrepareBillingRead(context.Context, domain.AdminActor, string, string, int64) (BillingReadSessionRecord, error)
}

type BillingProvisionRepository interface {
	ProvisionBillingSource(context.Context, BillingProvisionRecord) (BillingProvisionResult, error)
}

// ProvisioningService validates the declaration with the existing upstream and
// billing services, then delegates the state transition to one transaction.
type ProvisioningService struct {
	Repository BillingProvisionRepository
	Upstreams  ProductionPreparer
	Sessions   BillingReadPreparer
}

func (s ProvisioningService) Provision(ctx context.Context, actor domain.AdminActor, input BillingProvisionInput) (BillingProvisionResult, error) {
	if s.Repository == nil || s.Upstreams == nil || s.Sessions == nil {
		return BillingProvisionResult{}, fmt.Errorf("billing provision dependencies are required")
	}
	production, err := s.Upstreams.PrepareProduction(ctx, actor, input.Production)
	if err != nil {
		return BillingProvisionResult{}, err
	}
	session, err := s.Sessions.PrepareBillingRead(ctx, actor, input.BearerSecretFile, input.LoginURL, input.BillingAccountID)
	if err != nil {
		return BillingProvisionResult{}, err
	}
	return s.Repository.ProvisionBillingSource(ctx, BillingProvisionRecord{Production: production, Session: session})
}
