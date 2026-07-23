package candidates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/domain"
)

func TestCreateStoresOnlyCandidateSecretReference(t *testing.T) {
	t.Parallel()

	keyFile := writeKey(t, 0o600, "sk-upstream-secret-1234")
	repo := &fakeRepository{}
	service := Service{Repository: repo, Resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}}
	created, err := service.Create(context.Background(), domain.AdminActor{UserID: 42}, CandidateInput{
		Name: "wawazz", BaseURL: "https://wawazz.example/v1/", PricingURL: "https://wawazz.example/pricing",
		UsageURL: "https://wawazz.example/usage", PerformanceURL: "https://wawazz.example/monitor", ProbeKeyFile: keyFile,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.BaseURL != "https://wawazz.example/v1" || created.ProbeSecretRef == "" {
		t.Fatalf("created = %#v", created)
	}
	if repo.input.SecretRef.LastFour != "1234" || repo.input.SecretRef.SecretRef != created.ProbeSecretRef {
		t.Fatalf("secret ref = %#v", repo.input.SecretRef)
	}
	wantFingerprint := sha256.Sum256([]byte("sk-upstream-secret-1234"))
	if repo.input.SecretRef.Fingerprint != hex.EncodeToString(wantFingerprint[:]) {
		t.Fatalf("fingerprint = %q", repo.input.SecretRef.Fingerprint)
	}
	serialized := repo.serialized()
	if strings.Contains(serialized, "sk-upstream-secret-1234") {
		t.Fatalf("repository input leaked key: %s", serialized)
	}
	if repo.input.Audit.ActorUserID != 42 || repo.input.Audit.Action != "candidate.create" {
		t.Fatalf("audit = %#v", repo.input.Audit)
	}
}

func TestCreateRejectsUnsafeURLsAndSecretFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    CandidateInput
		resolver Resolver
	}{
		{name: "http", input: validInput(t), resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}},
		{name: "literal private", input: validInput(t), resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}},
		{name: "resolved private", input: validInput(t), resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.4")}}}},
		{name: "writable key", input: validInput(t), resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}},
	}
	tests[0].input.BaseURL = "http://wawazz.example/v1"
	tests[1].input.BaseURL = "https://127.0.0.1/v1"
	if err := os.Chmod(tests[3].input.ProbeKeyFile, 0o666); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := Service{Repository: &fakeRepository{}, Resolver: test.resolver}
			if _, err := service.Create(context.Background(), domain.AdminActor{UserID: 1}, test.input); err == nil {
				t.Fatal("unsafe candidate unexpectedly accepted")
			}
		})
	}
}

func TestCreateReturnsRepositoryConflict(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{err: ErrConflict}
	service := Service{Repository: repo, Resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}}
	_, err := service.Create(context.Background(), domain.AdminActor{UserID: 1}, validInput(t))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestCreateClassifiesRepositoryFailureWithoutExposingDetails(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{err: errors.New("database unavailable at /sensitive/socket")}
	service := Service{Repository: repo, Resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}}
	_, err := service.Create(context.Background(), domain.AdminActor{UserID: 1}, validInput(t))
	if !errors.Is(err, ErrCreateFailed) {
		t.Fatalf("error = %v, want ErrCreateFailed", err)
	}
	if err.Error() != "candidate create failed" {
		t.Fatalf("error text = %q", err.Error())
	}
}

func TestCreateWithKeyInstallsSecretAndStoresOnlyMetadata(t *testing.T) {
	t.Parallel()

	directory := secureDirectory(t)
	repo := &fakeRepository{}
	service := Service{
		Repository:  repo,
		Resolver:    fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}},
		SecretStore: FileSecretStore{Directory: directory},
	}
	key := []byte("candidate-secret-9012")
	created, err := service.CreateWithKey(context.Background(), domain.AdminActor{UserID: 42}, CandidateIntakeInput{
		Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing",
		UsageURL: "https://candidate.example/usage", PerformanceURL: "https://candidate.example/performance", ProbeKey: key,
	})
	if err != nil {
		t.Fatalf("CreateWithKey: %v", err)
	}
	if created.ID != 17 || !strings.HasPrefix(created.ProbeSecretRef, "file:"+directory+string(filepath.Separator)) {
		t.Fatalf("created = %#v", created)
	}
	if repo.input.SecretRef.LastFour != "9012" || repo.input.SecretRef.Fingerprint == "" {
		t.Fatalf("secret metadata = %#v", repo.input.SecretRef)
	}
	if strings.Contains(repo.serialized(), "candidate-secret-9012") {
		t.Fatal("repository input leaked candidate key")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("managed files = %d, %v", len(entries), err)
	}
	assertZeroed(t, key)
}

func TestCreateWithKeyRemovesSecretWhenRepositoryFails(t *testing.T) {
	t.Parallel()

	directory := secureDirectory(t)
	repo := &fakeRepository{err: errors.New("database unavailable")}
	service := Service{
		Repository:  repo,
		Resolver:    fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}},
		SecretStore: FileSecretStore{Directory: directory},
	}
	key := []byte("rollback-secret-3456")
	_, err := service.CreateWithKey(context.Background(), domain.AdminActor{UserID: 42}, CandidateIntakeInput{
		Name: "rollback", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing",
		UsageURL: "https://candidate.example/usage", ProbeKey: key,
	})
	if err == nil {
		t.Fatal("CreateWithKey unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "rollback-secret-3456") || strings.Contains(err.Error(), directory) {
		t.Fatalf("error leaked secret detail: %v", err)
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rollback left managed files = %d, %v", len(entries), readErr)
	}
	assertZeroed(t, key)
}

func TestCreateWithKeyHidesOriginalErrorWhenSecretCleanupFails(t *testing.T) {
	t.Parallel()

	directory := secureDirectory(t)
	service := Service{
		Repository: &fakeRepository{err: fmt.Errorf("%w: /sensitive/repository/path", ErrConflict)},
		Resolver:   fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}},
		SecretStore: failingRemoveSecretStore{
			FileSecretStore: FileSecretStore{Directory: directory},
			err:             errors.New("remove /sensitive/managed/path"),
		},
	}
	key := []byte("cleanup-failure-secret")
	_, err := service.CreateWithKey(context.Background(), domain.AdminActor{UserID: 42}, CandidateIntakeInput{
		Name: "cleanup-failure", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing",
		UsageURL: "https://candidate.example/usage", ProbeKey: key,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if err.Error() != "candidate secret cleanup failed" {
		t.Fatalf("error text = %q", err.Error())
	}
	assertZeroed(t, key)
}

func TestCreateWithKeyRejectsMissingStoreAndClearsOwnedBuffer(t *testing.T) {
	t.Parallel()

	key := []byte("missing-store-secret")
	service := Service{Repository: &fakeRepository{}, Resolver: fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}}
	_, err := service.CreateWithKey(context.Background(), domain.AdminActor{UserID: 42}, CandidateIntakeInput{
		Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing",
		UsageURL: "https://candidate.example/usage", ProbeKey: key,
	})
	if err == nil {
		t.Fatal("CreateWithKey accepted a missing secret store")
	}
	assertZeroed(t, key)
}

func TestListReturnsCandidateMetadataWithoutSecretValues(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{candidates: []Candidate{{
		ID: 17, Name: "wawazz", BaseURL: "https://wawazz.example/v1",
		PricingURL: "https://wawazz.example/pricing", ProbeSecretRef: "file:/run/secrets/wawazz",
	}}}
	listed, err := (Service{Repository: repo}).List(context.Background(), domain.AdminActor{UserID: 42})
	if err != nil || len(listed) != 1 || listed[0].ID != 17 {
		t.Fatalf("List = %#v, %v", listed, err)
	}
	if strings.Contains(repo.serialized(), "secret-value") {
		t.Fatal("candidate list leaked a secret value")
	}
}

func TestDisableAuditsAdministratorAction(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{}
	service := Service{Repository: repo}
	if err := service.Disable(context.Background(), domain.AdminActor{UserID: 42}, 17); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if repo.disabledID != 17 || repo.disableAudit.ActorUserID != 42 || repo.disableAudit.Action != "candidate.disable" {
		t.Fatalf("disabled=%d audit=%#v", repo.disabledID, repo.disableAudit)
	}
	if err := service.Disable(context.Background(), domain.AdminActor{}, 17); err == nil {
		t.Fatal("Disable accepted missing administrator")
	}
}

func validInput(t *testing.T) CandidateInput {
	t.Helper()
	return CandidateInput{
		Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing",
		UsageURL: "https://candidate.example/usage", ProbeKeyFile: writeKey(t, 0o600, "candidate-secret-5678"),
	}
}

func writeKey(t *testing.T, mode os.FileMode, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe-key")
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertZeroed(t *testing.T, value []byte) {
	t.Helper()
	for index, item := range value {
		if item != 0 {
			t.Fatalf("byte %d was not cleared", index)
		}
	}
}

type fakeResolver struct{ ips []net.IPAddr }

func (r fakeResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return r.ips, nil }

type failingRemoveSecretStore struct {
	FileSecretStore
	err error
}

func (s failingRemoveSecretStore) Remove(string) error { return s.err }

type fakeRepository struct {
	input        CreateRecord
	err          error
	candidates   []Candidate
	disabledID   domain.UpstreamID
	disableAudit AuditEvent
}

func (r *fakeRepository) CreateCandidate(_ context.Context, input CreateRecord) (domain.UpstreamID, error) {
	r.input = input
	return 17, r.err
}

func (r *fakeRepository) ListCandidates(context.Context) ([]Candidate, error) {
	return r.candidates, r.err
}

func (r *fakeRepository) DisableCandidate(_ context.Context, id domain.UpstreamID, audit AuditEvent) error {
	r.disabledID = id
	r.disableAudit = audit
	return r.err
}

func (r *fakeRepository) serialized() string {
	return r.input.Candidate.Name + r.input.Candidate.BaseURL + r.input.SecretRef.SecretRef + r.input.SecretRef.Fingerprint + r.input.SecretRef.LastFour
}
