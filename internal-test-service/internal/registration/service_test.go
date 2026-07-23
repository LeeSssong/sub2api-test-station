package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
	"example.invalid/internal-test-service/internal/testsupport"
)

func newRegistrationService(t *testing.T, max int) (*Service, *testsupport.Fake) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fake := testsupport.NewFake()
	var next int64 = 100
	cipher, err := NewInvitationCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Provider: fake, MaxUsers: max, Mode: "write", InvitationCipher: cipher}
	svc.Forward = func(ctx context.Context, body []byte, h http.Header) (int, http.Header, []byte, error) {
		id := atomic.AddInt64(&next, 1)
		b, _ := json.Marshal(map[string]any{"id": id})
		return 201, http.Header{"Content-Type": []string{"application/json"}}, b, nil
	}
	return svc, fake
}

func newPublicRegistrationService(t *testing.T, max int) (*Service, *int64, *int64) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "public.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var forwarded int64
	var granted int64
	svc := &Service{
		Store:            st,
		MaxUsers:         max,
		Mode:             "write",
		RegistrationOpen: true,
		Now:              time.Now,
	}
	svc.AuthForward = func(_ context.Context, endpoint string, body []byte, _ http.Header) (authproxy.Response, error) {
		if endpoint != authproxy.RegisterEndpoint {
			t.Fatalf("endpoint=%s", endpoint)
		}
		var request struct {
			UserID int64 `json:"fixture_user_id"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		atomic.AddInt64(&forwarded, 1)
		response, _ := json.Marshal(map[string]any{"data": map[string]any{"user": map[string]any{"id": request.UserID}}})
		return authproxy.Response{Status: http.StatusCreated, Header: http.Header{"Content-Type": {"application/json"}}, Body: response}, nil
	}
	svc.GrantDailyLogin = func(_ context.Context, _ int64, _ time.Time) error {
		atomic.AddInt64(&granted, 1)
		return nil
	}
	return svc, &forwarded, &granted
}

func TestPublicRegistrationRequiresOpenAndGrantsImmediately(t *testing.T) {
	svc, forwarded, granted := newPublicRegistrationService(t, 15)
	body := []byte(`{"fixture_user_id":1,"email":"user@example.com"}`)
	response, err := svc.Register(context.Background(), body, http.Header{})
	if err != nil || response.Status != http.StatusCreated {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if atomic.LoadInt64(forwarded) != 1 || atomic.LoadInt64(granted) != 1 {
		t.Fatalf("forwarded=%d granted=%d", *forwarded, *granted)
	}
	if _, err := svc.Store.GetInternalUser(context.Background(), 1); err != nil {
		t.Fatal(err)
	}

	svc.RegistrationOpen = false
	closed, err := svc.Register(context.Background(), []byte(`{"fixture_user_id":2}`), http.Header{})
	if err != nil || closed.Status != http.StatusForbidden || !containsJSONCode(closed.Body, "D04_REGISTRATION_CLOSED") {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	if atomic.LoadInt64(forwarded) != 1 {
		t.Fatal("closed registration was forwarded")
	}
}

func TestPublicRegistrationHardCapClosesEffectiveSwitch(t *testing.T) {
	svc, forwarded, granted := newPublicRegistrationService(t, 15)
	for id := int64(1); id <= 15; id++ {
		response, err := svc.Register(context.Background(), []byte(fmt.Sprintf(`{"fixture_user_id":%d}`, id)), http.Header{})
		if err != nil || response.Status != http.StatusCreated {
			t.Fatalf("id=%d response=%+v err=%v", id, response, err)
		}
	}
	if open, err := svc.EffectiveRegistrationOpen(context.Background()); err != nil || open {
		t.Fatalf("effective open=%v err=%v", open, err)
	}
	full, err := svc.Register(context.Background(), []byte(`{"fixture_user_id":16}`), http.Header{})
	if err != nil || full.Status != http.StatusConflict || !containsJSONCode(full.Body, "D04_REGISTRATION_FULL") {
		t.Fatalf("full=%+v err=%v", full, err)
	}
	if atomic.LoadInt64(forwarded) != 15 || atomic.LoadInt64(granted) != 15 {
		t.Fatalf("forwarded=%d granted=%d", *forwarded, *granted)
	}
}

func TestConcurrentPublicRegistrationNeverExceedsCap(t *testing.T) {
	svc, forwarded, granted := newPublicRegistrationService(t, 15)
	start := make(chan struct{})
	results := make(chan int, 20)
	for id := int64(1); id <= 20; id++ {
		go func(userID int64) {
			<-start
			response, _ := svc.Register(context.Background(), []byte(fmt.Sprintf(`{"fixture_user_id":%d}`, userID)), http.Header{})
			results <- response.Status
		}(id)
	}
	close(start)
	success := 0
	for i := 0; i < 20; i++ {
		if <-results == http.StatusCreated {
			success++
		}
	}
	if success != 15 || atomic.LoadInt64(forwarded) != 15 || atomic.LoadInt64(granted) != 15 {
		t.Fatalf("success=%d forwarded=%d granted=%d", success, *forwarded, *granted)
	}
	if count, _ := svc.Store.CountRegisteredUsers(context.Background()); count != 15 {
		t.Fatalf("count=%d", count)
	}
}

func TestPublicRegistrationReservesCapacityAcrossServiceInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	firstStore, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()

	forwardStarted := make(chan struct{})
	releaseForward := make(chan struct{})
	first := &Service{Store: firstStore, MaxUsers: 1, Mode: "write", RegistrationOpen: true, Now: time.Now}
	first.AuthForward = func(context.Context, string, []byte, http.Header) (authproxy.Response, error) {
		close(forwardStarted)
		<-releaseForward
		return authproxy.Response{Status: http.StatusCreated, Body: []byte(`{"data":{"user":{"id":1}}}`)}, nil
	}
	secondForwarded := false
	second := &Service{Store: secondStore, MaxUsers: 1, Mode: "write", RegistrationOpen: true, Now: time.Now}
	second.AuthForward = func(context.Context, string, []byte, http.Header) (authproxy.Response, error) {
		secondForwarded = true
		return authproxy.Response{Status: http.StatusCreated, Body: []byte(`{"data":{"user":{"id":2}}}`)}, nil
	}

	firstResult := make(chan authproxy.Response, 1)
	go func() {
		response, _ := first.Register(context.Background(), nil, http.Header{})
		firstResult <- response
	}()
	<-forwardStarted
	secondResponse, err := second.Register(context.Background(), nil, http.Header{})
	if err != nil || secondResponse.Status != http.StatusConflict || secondForwarded {
		t.Fatalf("second=%+v forwarded=%v err=%v", secondResponse, secondForwarded, err)
	}
	close(releaseForward)
	if response := <-firstResult; response.Status != http.StatusCreated {
		t.Fatalf("first=%+v", response)
	}
}

func containsJSONCode(body []byte, code string) bool {
	var value map[string]any
	return json.Unmarshal(body, &value) == nil && value["code"] == code
}

func TestInvitationCodeIsEncryptedAtRestAndDecryptedForJoinState(t *testing.T) {
	svc, _ := newRegistrationService(t, 2)
	link, err := svc.CreateInvitation(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := svc.Store.GetInvitation(context.Background(), link.JoinID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.CodeCiphertext == link.Code || inv.CodeCiphertext == "" {
		t.Fatalf("invitation code not encrypted: %q", inv.CodeCiphertext)
	}
	state, err := svc.JoinState(context.Background(), link.JoinID)
	if err != nil || state.Code != link.Code {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

func TestMultipleInvitationsAndHardCap(t *testing.T) {
	svc, _ := newRegistrationService(t, 2)
	ctx := context.Background()
	one, err := svc.CreateInvitation(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := svc.CreateInvitation(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if one.Code == two.Code {
		t.Fatal("codes reused")
	}
	status, _, _, err := svc.GateRegistration(ctx, []byte(`{"email":"one@example.com","password":"redacted","invitation_code":"`+one.Code+`"}`), http.Header{})
	if err != nil || status != 201 {
		t.Fatalf("first %d %v", status, err)
	}
	status, _, _, err = svc.GateRegistration(ctx, []byte(`{"email":"two@example.com","password":"redacted","invitation_code":"`+two.Code+`"}`), http.Header{})
	if err != nil || status != 201 {
		t.Fatalf("second %d %v", status, err)
	}
	three, err := svc.CreateInvitation(ctx, 1)
	if err == nil || three.Code != "" {
		t.Fatalf("expected cap: %+v %v", three, err)
	}
	third, _, _, err := svc.GateRegistration(ctx, []byte(`{"email":"three@example.com","password":"redacted","invitation_code":"bad"}`), http.Header{})
	if err != nil || third != http.StatusConflict {
		t.Fatalf("third %d %v", third, err)
	}
}

func TestRegistrationConcurrencyCannotExceedCap(t *testing.T) {
	svc, _ := newRegistrationService(t, 2)
	ctx := context.Background()
	codes := make([]string, 5)
	for i := range codes {
		link, err := svc.CreateInvitation(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		codes[i] = link.Code
	}
	results := make(chan int, 5)
	for _, code := range codes {
		go func(code string) {
			body := []byte(`{"email":"x","password":"redacted","invitation_code":"` + code + `"}`)
			status, _, _, _ := svc.GateRegistration(ctx, body, http.Header{})
			results <- status
		}(code)
	}
	success := 0
	for range codes {
		if <-results == 201 {
			success++
		}
	}
	if success != 2 {
		t.Fatalf("successes %d", success)
	}
}

func TestGateRegistrationForwardsNativeInvitationCode(t *testing.T) {
	svc, _ := newRegistrationService(t, 2)
	link, err := svc.CreateInvitation(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var forwarded map[string]any
	svc.Forward = func(_ context.Context, body []byte, _ http.Header) (int, http.Header, []byte, error) {
		if err := json.Unmarshal(body, &forwarded); err != nil {
			t.Fatal(err)
		}
		return 201, http.Header{}, []byte(`{"data":{"user":{"id":101}}}`), nil
	}
	status, _, _, err := svc.GateRegistration(context.Background(), []byte(`{"email":"user@example.com","password":"redacted","invitation_code":"`+link.Code+`"}`), http.Header{})
	if err != nil || status != 201 {
		t.Fatalf("registration status=%d err=%v", status, err)
	}
	if got, _ := forwarded["invitation_code"].(string); got != link.Code {
		t.Fatalf("forwarded invitation_code = %q", got)
	}
	if _, exists := forwarded["invite_code"]; exists {
		t.Fatal("legacy invite_code was forwarded")
	}
}

func TestReconcileRegistrationsAdoptsProviderUsedInvitation(t *testing.T) {
	svc, fake := newRegistrationService(t, 2)
	if err := svc.Store.RegisterUser(context.Background(), store.InternalUser{UserID: 1, JoinedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	link, err := svc.CreateInvitation(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := svc.Store.GetInvitation(context.Background(), link.JoinID)
	if err != nil {
		t.Fatal(err)
	}
	fake.AddUser(sub2api.User{ID: 2})
	fake.RedeemInvitation(inv.ProviderCodeID, 2)
	recovered, err := svc.ReconcileRegistrations(context.Background())
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	if _, err := svc.Store.GetInternalUser(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	grants, err := svc.Store.ListAllGrants(context.Background())
	if err != nil || len(grants) != 1 || grants[0].InviteeUserID.Int64 != 2 {
		t.Fatalf("grants=%+v err=%v", grants, err)
	}
	recovered, err = svc.ReconcileRegistrations(context.Background())
	if err != nil || recovered != 0 {
		t.Fatalf("second recovered=%d err=%v", recovered, err)
	}
}
