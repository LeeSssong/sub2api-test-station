package billing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/upstreams"
)

func TestProvisioningServicePersistsValidatedBillingSourceWithoutCredential(t *testing.T) {
	t.Parallel()
	repository := &fakeBillingProvisionRepository{}
	service := ProvisioningService{
		Repository: repository,
		Upstreams: fakeProductionPreparer{record: upstreams.ProductionRecord{Source: upstreams.Source{
			Name: "billing primary", BaseURL: "https://billing.example/v1", AdapterType: "sub2api", Enabled: true,
		}}},
		Sessions: fakeBillingReadPreparer{record: BillingReadSessionRecord{
			LoginURL: "https://billing.example/login", BillingAccountID: 8123,
			Secret: SessionSecretRef{SecretRef: "file:/run/secrets/upstream-sessions/billing-primary", Fingerprint: "fingerprint", LastFour: "cret"},
		}},
	}

	result, err := service.Provision(context.Background(), domain.AdminActor{UserID: 42}, BillingProvisionInput{
		Production:       upstreams.ProductionInput{Name: "billing primary", AdapterType: "sub2api"},
		BearerSecretFile: "/run/secrets/upstream-sessions/billing-primary", LoginURL: "https://billing.example/login", BillingAccountID: 8123,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UpstreamID != 17 || result.BillingAccountID != 8123 || result.AlreadyConfigured {
		t.Fatalf("result=%#v", result)
	}
	if repository.record.Production.Source.Name != "billing primary" || repository.record.Session.BillingAccountID != 8123 {
		t.Fatalf("record=%#v", repository.record)
	}
	if repository.record.Session.Secret.SecretRef != "file:/run/secrets/upstream-sessions/billing-primary" {
		t.Fatalf("secret reference=%#v", repository.record.Session.Secret)
	}
	if strings.Contains(repository.record.Session.Secret.SecretRef, "credential-value") {
		t.Fatal("credential leaked into provision record")
	}
}

func TestProvisioningServiceReturnsDeterministicConflict(t *testing.T) {
	t.Parallel()
	service := ProvisioningService{
		Repository: &fakeBillingProvisionRepository{err: ErrBillingProvisionConflict},
		Upstreams:  fakeProductionPreparer{record: upstreams.ProductionRecord{}},
		Sessions:   fakeBillingReadPreparer{record: BillingReadSessionRecord{}},
	}
	_, err := service.Provision(context.Background(), domain.AdminActor{UserID: 42}, BillingProvisionInput{})
	if !errors.Is(err, ErrBillingProvisionConflict) {
		t.Fatalf("error=%v", err)
	}
}

type fakeProductionPreparer struct {
	record upstreams.ProductionRecord
	err    error
}

func (p fakeProductionPreparer) PrepareProduction(context.Context, domain.AdminActor, upstreams.ProductionInput) (upstreams.ProductionRecord, error) {
	return p.record, p.err
}

type fakeBillingReadPreparer struct {
	record BillingReadSessionRecord
	err    error
}

func (p fakeBillingReadPreparer) PrepareBillingRead(context.Context, domain.AdminActor, string, string, int64) (BillingReadSessionRecord, error) {
	return p.record, p.err
}

type fakeBillingProvisionRepository struct {
	record BillingProvisionRecord
	err    error
}

func (r *fakeBillingProvisionRepository) ProvisionBillingSource(_ context.Context, record BillingProvisionRecord) (BillingProvisionResult, error) {
	r.record = record
	if r.err != nil {
		return BillingProvisionResult{}, r.err
	}
	return BillingProvisionResult{UpstreamID: 17, BillingAccountID: record.Session.BillingAccountID}, nil
}
