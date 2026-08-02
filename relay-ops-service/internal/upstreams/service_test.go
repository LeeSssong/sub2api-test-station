package upstreams

import (
	"context"
	"errors"
	"net"
	"testing"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestCreateProductionNormalizesURLsAndRequiresNoProbeKey(t *testing.T) {
	repository := &fakeRepository{}
	service := Service{Repository: repository, Resolver: publicResolver{}}

	created, err := service.CreateProduction(context.Background(), domain.AdminActor{UserID: 42}, ProductionInput{
		Name: " Neko ", BaseURL: "https://API.NEKO.example/v1/", PricingURL: "https://api.neko.example/pricing",
		UsageURL: "https://api.neko.example/usage", PerformanceURL: "https://api.neko.example/performance",
		AdapterType: AdapterSub2API, GroupIDs: []int64{7, 3, 7}, MonitorID: 19,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Neko" || created.BaseURL != "https://api.neko.example/v1" {
		t.Fatalf("created = %#v", created)
	}
	if len(created.GroupIDs) != 2 || created.GroupIDs[0] != 3 || created.GroupIDs[1] != 7 {
		t.Fatalf("group IDs = %#v", created.GroupIDs)
	}
	if created.Role != RoleProduction || !created.Enabled || created.MonitorID != 19 {
		t.Fatalf("created = %#v", created)
	}
	if created.AdapterType != AdapterSub2API || repository.record.Source.AdapterType != AdapterSub2API {
		t.Fatalf("adapter type = created %q persisted %q", created.AdapterType, repository.record.Source.AdapterType)
	}
	if repository.record.Audit.ActorUserID != 42 || repository.record.Audit.Action != "upstream.production.create" {
		t.Fatalf("audit = %#v", repository.record.Audit)
	}
}

func TestCreateProductionRejectsUnsupportedAdapterType(t *testing.T) {
	service := Service{Repository: &fakeRepository{}, Resolver: publicResolver{}}
	_, err := service.CreateProduction(context.Background(), domain.AdminActor{UserID: 1}, ProductionInput{
		Name: "unsupported", AdapterType: "openai", BaseURL: "https://public.example/v1", PricingURL: "https://public.example/pricing", GroupIDs: []int64{1},
	})
	if !errors.Is(err, ErrAdapterTypeInvalid) {
		t.Fatalf("error = %v, want ErrAdapterTypeInvalid", err)
	}
}

func TestCreateProductionRejectsUnsafeURLAndMissingGroups(t *testing.T) {
	service := Service{Repository: &fakeRepository{}, Resolver: privateResolver{}}
	_, err := service.CreateProduction(context.Background(), domain.AdminActor{UserID: 1}, ProductionInput{
		Name: "unsafe", AdapterType: AdapterNewAPI, BaseURL: "https://private.example/v1", PricingURL: "https://private.example/pricing", GroupIDs: []int64{1},
	})
	if err == nil {
		t.Fatal("expected private URL rejection")
	}

	service.Resolver = publicResolver{}
	_, err = service.CreateProduction(context.Background(), domain.AdminActor{UserID: 1}, ProductionInput{
		Name: "missing-groups", AdapterType: AdapterNewAPI, BaseURL: "https://public.example/v1", PricingURL: "https://public.example/pricing",
	})
	if !errors.Is(err, ErrGroupRequired) {
		t.Fatalf("error = %v, want ErrGroupRequired", err)
	}
}

type fakeRepository struct {
	record ProductionRecord
}

func (r *fakeRepository) CreateProduction(_ context.Context, record ProductionRecord) (domain.UpstreamID, error) {
	r.record = record
	return 17, nil
}

func (r *fakeRepository) ResolvePublicGroupIDs(context.Context, []string) ([]int64, error) {
	return []int64{3, 7}, nil
}

func (r *fakeRepository) ListProduction(context.Context) ([]Source, error) { return nil, nil }
func (r *fakeRepository) DisableProduction(context.Context, domain.UpstreamID, AuditEvent) error {
	return nil
}

type publicResolver struct{}

func (publicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
}

type privateResolver struct{}

func (privateResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
}
