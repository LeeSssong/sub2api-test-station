package registration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"example.invalid/internal-test-service/internal/authproxy"
	"example.invalid/internal-test-service/internal/store"
	"example.invalid/internal-test-service/internal/sub2api"
)

type JoinLink struct {
	JoinID  string `json:"join_id"`
	Code    string `json:"code"`
	AffCode string `json:"aff_code"`
}
type JoinState struct {
	State           string `json:"state"`
	RegisteredCount int    `json:"registered_count"`
	MaxUsers        int    `json:"max_users"`
	Code            string `json:"code,omitempty"`
	AffCode         string `json:"aff_code,omitempty"`
}

type Forwarder func(ctx context.Context, body []byte, headers http.Header) (status int, responseHeaders http.Header, responseBody []byte, err error)
type BudgetGate func(ctx context.Context) (bool, error)

type Service struct {
	Store            *store.Store
	Provider         sub2api.Client
	InvitationCipher *InvitationCipher
	MaxUsers         int
	Mode             string
	CanReserve       BudgetGate
	Forward          Forwarder
	RegistrationOpen bool
	AuthForward      authproxy.Forwarder
	GrantDailyLogin  func(context.Context, int64, time.Time) error
	CanGrantDaily    BudgetGate
	GrantTimeout     time.Duration
	Now              func() time.Time
	mu               sync.Mutex
}

const defaultGrantTimeout = 2 * time.Second

func (s *Service) mode(ctx context.Context) string {
	if v, _ := s.Store.GetSetting(ctx, "mode"); v != "" {
		return v
	}
	return s.Mode
}
func (s *Service) setMode(ctx context.Context, mode string) error {
	s.Mode = mode
	return s.Store.SetSetting(ctx, "mode", mode)
}

func (s *Service) EffectiveRegistrationOpen(ctx context.Context) (bool, error) {
	if s.mode(ctx) != "write" || !s.RegistrationOpen {
		return false, nil
	}
	if reason, err := s.Store.GetReadOnlyReason(ctx); err != nil {
		return false, err
	} else if reason != "" {
		return false, nil
	}
	count, err := s.Store.CountLaunchCapacity(ctx)
	if err != nil {
		return false, err
	}
	if count >= s.MaxUsers {
		return false, nil
	}
	if s.CanGrantDaily != nil {
		return s.CanGrantDaily(ctx)
	}
	return true, nil
}

func (s *Service) Register(ctx context.Context, body []byte, headers http.Header) (authproxy.Response, error) {
	count, err := s.Store.CountLaunchCapacity(ctx)
	if err != nil {
		return authproxy.Response{}, err
	}
	if count >= s.MaxUsers {
		return registrationResponse(http.StatusConflict, "D04_REGISTRATION_FULL", "首发计划用户已满，注册已自动关闭"), nil
	}
	open, err := s.EffectiveRegistrationOpen(ctx)
	if err != nil {
		return authproxy.Response{}, err
	}
	if !open {
		return registrationResponse(http.StatusForbidden, "D04_REGISTRATION_CLOSED", "当前未开放注册"), nil
	}
	if s.AuthForward == nil {
		return authproxy.Response{}, errors.New("registration forwarder is not configured")
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	slotID := randomID("launch-registration-")
	if err := s.Store.ReserveRegistrationSlot(ctx, slotID, s.MaxUsers, now); err != nil {
		if errors.Is(err, store.ErrLaunchFull) {
			return registrationResponse(http.StatusConflict, "D04_REGISTRATION_FULL", "首发计划用户已满，注册已自动关闭"), nil
		}
		return authproxy.Response{}, err
	}
	releaseReservation := true
	defer func() {
		if releaseReservation {
			_ = s.Store.ReleaseRegistrationSlot(context.WithoutCancel(ctx), slotID)
		}
	}()
	response, err := s.AuthForward(ctx, authproxy.RegisterEndpoint, body, headers)
	if err != nil || response.Status < http.StatusOK || response.Status >= http.StatusMultipleChoices {
		return response, err
	}
	userID := authproxy.ExtractUserID(response.Body)
	if userID == 0 {
		releaseReservation = false
		_ = s.Store.SetReadOnlyReason(ctx, "registration response missing user id")
		return response, nil
	}
	if _, err := s.Store.CompleteRegistrationSlot(ctx, slotID, store.InternalUser{UserID: userID, JoinedAt: now}); err != nil {
		releaseReservation = false
		_ = s.Store.SetReadOnlyReason(ctx, "uncertain launch roster write")
		return response, nil
	}
	releaseReservation = false
	if s.GrantDailyLogin != nil {
		timeout := s.GrantTimeout
		if timeout <= 0 {
			timeout = defaultGrantTimeout
		}
		grantCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		_ = s.GrantDailyLogin(grantCtx, userID, now)
		cancel()
	}
	return response, nil
}

func registrationResponse(status int, code, message string) authproxy.Response {
	body, _ := json.Marshal(map[string]string{"code": code, "message": message})
	return authproxy.Response{Status: status, Header: jsonHeader(), Body: body}
}

func (s *Service) CreateInvitation(ctx context.Context, issuerUserID int64) (JoinLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode(ctx) != "write" {
		return JoinLink{}, fmt.Errorf("internal test is not writable")
	}
	count, err := s.Store.CountRegisteredUsers(ctx)
	if err != nil {
		return JoinLink{}, err
	}
	if count >= s.MaxUsers {
		return JoinLink{}, ErrFull
	}
	if s.CanReserve != nil {
		ok, err := s.CanReserve(ctx)
		if err != nil {
			return JoinLink{}, err
		}
		if !ok {
			return JoinLink{}, ErrBudgetFull
		}
	}
	joinID := randomID("join-")
	idem := randomID(fmt.Sprintf("d04-invite-%d-", issuerUserID))
	codes, err := s.Provider.GenerateInvitation(ctx, 1, nil, idem)
	if err != nil {
		return JoinLink{}, err
	}
	if len(codes) != 1 {
		return JoinLink{}, fmt.Errorf("provider returned no invitation")
	}
	code := codes[0]
	if s.InvitationCipher == nil {
		return JoinLink{}, fmt.Errorf("invitation encryption is not configured")
	}
	ciphertext, err := s.InvitationCipher.Encrypt(joinID, code.Code)
	if err != nil {
		return JoinLink{}, err
	}
	id, err := s.Store.RegisterInvitation(ctx, store.Invitation{JoinID: joinID, ProviderCodeID: code.ID, CodeCiphertext: ciphertext, CodeHash: hash(code.Code), IssuerUserID: issuerUserID, AffCode: code.AffCode, CreatedAt: time.Now()})
	if err != nil {
		return JoinLink{}, err
	}
	_ = id
	return JoinLink{JoinID: joinID, Code: code.Code, AffCode: code.AffCode}, nil
}

func (s *Service) JoinState(ctx context.Context, joinID string) (JoinState, error) {
	inv, err := s.Store.GetInvitation(ctx, joinID)
	if err != nil {
		return JoinState{}, err
	}
	count, err := s.Store.CountRegisteredUsers(ctx)
	if err != nil {
		return JoinState{}, err
	}
	state := "open"
	switch s.mode(ctx) {
	case "closed":
		state = "closed"
	case "read_only":
		state = "read_only"
	}
	if inv.ClosedAt.Valid {
		state = "closed"
	}
	if count >= s.MaxUsers && state == "open" {
		state = "full"
	}
	if state == "open" && s.CanReserve != nil {
		if ok, budgetErr := s.CanReserve(ctx); budgetErr == nil && !ok {
			state = "budget_full"
		}
	}
	if s.InvitationCipher == nil {
		return JoinState{}, fmt.Errorf("invitation encryption is not configured")
	}
	code, err := s.InvitationCipher.Decrypt(inv.JoinID, inv.CodeCiphertext)
	if err != nil {
		return JoinState{}, err
	}
	return JoinState{State: state, RegisteredCount: count, MaxUsers: s.MaxUsers, Code: code, AffCode: inv.AffCode}, nil
}

func (s *Service) GateRegistration(ctx context.Context, body []byte, headers http.Header) (int, http.Header, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode(ctx) != "write" {
		return http.StatusForbidden, jsonHeader(), errorJSON("D04_REGISTRATION_CLOSED", "首发计划注册暂未开放"), nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return http.StatusBadRequest, jsonHeader(), errorJSON("INVALID_JSON", "注册信息格式错误"), nil
	}
	code, _ := payload["invitation_code"].(string)
	if strings.TrimSpace(code) == "" {
		code, _ = payload["invite_code"].(string)
	}
	if code == "" {
		code = headers.Get("X-D04-Invitation-Code")
	}
	if strings.TrimSpace(code) == "" {
		return http.StatusBadRequest, jsonHeader(), errorJSON("INVITATION_REQUIRED", "请输入首发计划邀请码"), nil
	}
	delete(payload, "invite_code")
	payload["invitation_code"] = code
	count, err := s.Store.CountRegisteredUsers(ctx)
	if err != nil {
		return 0, nil, nil, err
	}
	if count >= s.MaxUsers {
		return http.StatusConflict, jsonHeader(), errorJSON("INTERNAL_TEST_FULL", "首发计划用户已满，请等待站长增加名额"), nil
	}
	inv, err := s.Store.GetInvitationByHash(ctx, hash(code))
	if err != nil || inv.UsedBy.Valid || inv.ClosedAt.Valid {
		return http.StatusBadRequest, jsonHeader(), errorJSON("INVITATION_INVALID", "邀请码无效或已使用"), nil
	}
	if s.CanReserve != nil {
		ok, err := s.CanReserve(ctx)
		if err != nil {
			return 0, nil, nil, err
		}
		if !ok {
			return http.StatusConflict, jsonHeader(), errorJSON("INTERNAL_TEST_BUDGET_FULL", "首发计划预算已用尽，请等待站长调整"), nil
		}
	}
	forwardBody, _ := json.Marshal(payload)
	status, rh, response, err := s.Forward(ctx, forwardBody, headers)
	if err != nil {
		return 0, nil, nil, err
	}
	if status < 200 || status >= 300 {
		return status, rh, response, nil
	}
	userID := extractUserID(response)
	if userID == 0 {
		return 0, nil, nil, fmt.Errorf("registration response missing user id")
	}
	if err := s.completeRegistration(ctx, inv, userID, time.Now()); err != nil {
		return 0, nil, nil, err
	}
	return status, rh, response, nil
}

func (s *Service) ReconcileRegistrations(ctx context.Context) (int, error) {
	if s.mode(ctx) != "write" {
		return 0, nil
	}
	local, err := s.Store.ListInvitationUses(ctx)
	if err != nil {
		return 0, err
	}
	provider, err := s.Provider.ListInvitationCodes(ctx)
	if err != nil {
		return 0, err
	}
	byID := make(map[int64]sub2api.InvitationCode, len(provider))
	for _, code := range provider {
		byID[code.ID] = code
	}
	recovered := 0
	for _, inv := range local {
		if inv.UsedBy.Valid {
			continue
		}
		remote, ok := byID[inv.ProviderCodeID]
		if !ok || remote.UsedBy == nil || *remote.UsedBy <= 0 {
			continue
		}
		if _, err := s.Provider.GetUser(ctx, *remote.UsedBy); err != nil {
			return recovered, err
		}
		if err := s.completeRegistration(ctx, inv, *remote.UsedBy, time.Now()); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func (s *Service) completeRegistration(ctx context.Context, inv store.Invitation, userID int64, now time.Time) error {
	var referral *store.Grant
	if _, inviterErr := s.Store.GetInternalUser(ctx, inv.IssuerUserID); inviterErr == nil {
		referral = &store.Grant{UserID: inv.IssuerUserID, Kind: "referral_reward", Amount: 5_000_000, InviteeUserID: nullable(userID), IdempotencyKey: fmt.Sprintf("d04-referral-%d", userID), Status: "reserved", CreatedAt: now, Note: "reserved at referred registration"}
	}
	return s.Store.CompleteRegistration(ctx, inv.CodeHash, store.InternalUser{UserID: userID, InviterUserID: nullable(inv.IssuerUserID), InvitationID: nullable(inv.ID), JoinedAt: now}, referral)
}

func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.setMode(ctx, "closed"); err != nil {
		return err
	}
	invs, err := s.Store.ListInvitationUses(ctx)
	if err != nil {
		return err
	}
	for _, inv := range invs {
		if !inv.UsedBy.Valid && inv.ClosedAt.Valid == false {
			if err := s.Provider.ExpireInvitation(ctx, inv.ProviderCodeID, randomID("d04-expire-")); err != nil {
				return err
			}
		}
	}
	return s.Store.CloseUnusedInvitations(ctx, time.Now())
}

var ErrFull = fmt.Errorf("internal test full")
var ErrBudgetFull = fmt.Errorf("internal test budget full")

func nullable(v int64) (n sql.NullInt64) { n.Int64 = v; n.Valid = true; return }
func hash(value string) string           { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func randomID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return prefix + fmt.Sprint(time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b)
}
func jsonHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return h
}
func errorJSON(code, message string) []byte {
	b, _ := json.Marshal(map[string]string{"code": code, "message": message})
	return b
}
func extractUserID(data []byte) int64 {
	var v struct {
		ID   int64 `json:"id"`
		Data struct {
			ID   int64 `json:"id"`
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	if json.Unmarshal(data, &v) != nil {
		return 0
	}
	if v.ID != 0 {
		return v.ID
	}
	if v.Data.ID != 0 {
		return v.Data.ID
	}
	if v.Data.User.ID != 0 {
		return v.Data.User.ID
	}
	return v.User.ID
}
