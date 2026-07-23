package testsupport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"example.invalid/internal-test-service/internal/domain"
	"example.invalid/internal-test-service/internal/sub2api"
)

type Fake struct {
	mu                      sync.Mutex
	AdminKey                string
	Users                   map[int64]sub2api.User
	Invitations             map[int64]sub2api.InvitationCode
	Balances                map[int64]domain.MicroUSD
	BalanceHistory          map[int64][]sub2api.BalanceEntry
	Usage                   map[int64][]sub2api.Usage
	Writes                  map[string]struct{}
	NextInvitationID        int64
	NextUsageID             int64
	TimeoutAfterCommit      bool
	FailBalanceBeforeCommit bool
	MalformedJSON           bool
}

func NewFake() *Fake {
	return &Fake{AdminKey: "test-admin-key", Users: map[int64]sub2api.User{}, Invitations: map[int64]sub2api.InvitationCode{}, Balances: map[int64]domain.MicroUSD{}, BalanceHistory: map[int64][]sub2api.BalanceEntry{}, Usage: map[int64][]sub2api.Usage{}, Writes: map[string]struct{}{}, NextInvitationID: 1, NextUsageID: 1}
}
func (f *Fake) AddUser(user sub2api.User) { f.mu.Lock(); defer f.mu.Unlock(); f.Users[user.ID] = user }
func (f *Fake) AddUsage(userID int64, amount domain.MicroUSD, successful bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Usage[userID] = append(f.Usage[userID], sub2api.Usage{ID: f.NextUsageID, UserID: userID, AmountUSD: amount.String(), Successful: successful, Status: map[bool]string{true: "succeeded", false: "failed"}[successful], CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	f.NextUsageID++
}
func (f *Fake) RedeemInvitation(codeID, userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv := f.Invitations[codeID]
	inv.UsedBy = &userID
	f.Invitations[codeID] = inv
}
func (f *Fake) AddBalance(ctx context.Context, userID int64, amount domain.MicroUSD, idem, note string) error {
	f.mu.Lock()
	if f.FailBalanceBeforeCommit {
		f.mu.Unlock()
		return context.DeadlineExceeded
	}
	if _, ok := f.Writes[idem]; ok {
		f.mu.Unlock()
		return nil
	}
	f.Writes[idem] = struct{}{}
	f.Balances[userID] += amount
	f.BalanceHistory[userID] = append(f.BalanceHistory[userID], sub2api.BalanceEntry{AmountUSD: amount.String(), Value: float64(amount) / 1_000_000, Operation: "add", Notes: note, IdempotencyKey: idem, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	timeout := f.TimeoutAfterCommit
	f.mu.Unlock()
	if timeout {
		return context.DeadlineExceeded
	}
	return nil
}
func (f *Fake) GetCurrentUser(ctx context.Context, bearer string) (sub2api.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.Users {
		if bearer == fmt.Sprintf("user-%d", u.ID) {
			return u, nil
		}
	}
	return sub2api.User{}, fmt.Errorf("unauthorized")
}
func (f *Fake) GenerateInvitation(ctx context.Context, count int, expiresAt *time.Time, idem string) ([]sub2api.InvitationCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Writes[idem]; ok {
		return nil, nil
	}
	f.Writes[idem] = struct{}{}
	var result []sub2api.InvitationCode
	for i := 0; i < count; i++ {
		id := f.NextInvitationID
		f.NextInvitationID++
		code := fmt.Sprintf("D04-%06d", id)
		aff := fmt.Sprintf("aff-%d", id)
		inv := sub2api.InvitationCode{ID: id, Code: code, AffCode: aff, ExpiresAt: expiresAt}
		f.Invitations[id] = inv
		result = append(result, inv)
	}
	return result, nil
}
func (f *Fake) ListInvitationCodes(ctx context.Context) ([]sub2api.InvitationCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sub2api.InvitationCode, 0, len(f.Invitations))
	for _, i := range f.Invitations {
		out = append(out, i)
	}
	return out, nil
}
func (f *Fake) ExpireInvitation(ctx context.Context, codeID int64, idem string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Writes[idem]; ok {
		return nil
	}
	f.Writes[idem] = struct{}{}
	if inv, ok := f.Invitations[codeID]; ok {
		now := time.Now()
		inv.ExpiresAt = &now
		f.Invitations[codeID] = inv
	}
	return nil
}
func (f *Fake) GetBalance(ctx context.Context, userID int64) (sub2api.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return sub2api.Balance{UserID: userID, Balance: f.Balances[userID], History: append([]sub2api.BalanceEntry(nil), f.BalanceHistory[userID]...)}, nil
}
func (f *Fake) ListUsage(ctx context.Context, userID int64, afterID int64) ([]sub2api.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sub2api.Usage
	for _, u := range f.Usage[userID] {
		if u.ID > afterID {
			out = append(out, u)
		}
	}
	return out, nil
}
func (f *Fake) GetUser(ctx context.Context, userID int64) (sub2api.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.Users[userID]
	if !ok {
		return sub2api.User{}, fmt.Errorf("user not found")
	}
	return u, nil
}

func (f *Fake) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") && r.Header.Get("x-api-key") != f.AdminKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if f.MalformedJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{"))
			return
		}
		path := r.URL.Path
		if path == "/api/v1/auth/me" {
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			u, err := f.GetCurrentUser(r.Context(), bearer)
			if err != nil {
				http.Error(w, "unauthorized", 401)
				return
			}
			writeJSON(w, map[string]any{"data": u})
			return
		}
		if path == "/api/v1/admin/redeem-codes/generate" {
			var body struct {
				Count int    `json:"count"`
				Type  string `json:"type"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Type != "invitation" {
				http.Error(w, "invalid type", http.StatusBadRequest)
				return
			}
			list, _ := f.GenerateInvitation(r.Context(), body.Count, nil, r.Header.Get("Idempotency-Key"))
			writeJSON(w, map[string]any{"data": list})
			return
		}
		if path == "/api/v1/admin/redeem-codes" {
			list, _ := f.ListInvitationCodes(r.Context())
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": list, "total": len(list), "page": 1, "page_size": len(list), "pages": 1}})
			return
		}
		if strings.HasPrefix(path, "/api/v1/admin/redeem-codes/") && strings.HasSuffix(path, "/expire") {
			parts := strings.Split(path, "/")
			id, _ := strconv.ParseInt(parts[len(parts)-2], 10, 64)
			_ = f.ExpireInvitation(r.Context(), id, r.Header.Get("Idempotency-Key"))
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		if strings.HasSuffix(path, "/balance") {
			parts := strings.Split(path, "/")
			id, _ := strconv.ParseInt(parts[len(parts)-2], 10, 64)
			if r.Method == http.MethodPost {
				var body struct {
					Balance   float64 `json:"balance"`
					Operation string  `json:"operation"`
					Notes     string  `json:"notes"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				amt, _ := domain.ParseMicroUSD(strconv.FormatFloat(body.Balance, 'f', 6, 64))
				err := f.AddBalance(r.Context(), id, amt, r.Header.Get("Idempotency-Key"), body.Notes)
				if err == context.DeadlineExceeded {
					return
				}
				writeJSON(w, map[string]any{"ok": true})
				return
			}
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(path, "/balance-history") {
			parts := strings.Split(path, "/")
			id, _ := strconv.ParseInt(parts[len(parts)-2], 10, 64)
			b, _ := f.GetBalance(r.Context(), id)
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": b.History, "total": len(b.History), "page": 1, "page_size": 1000, "pages": 1}})
			return
		}
		if path == "/api/v1/admin/usage" {
			id, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
			after, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)
			list, _ := f.ListUsage(r.Context(), id, after)
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": list, "total": len(list), "page": 1, "page_size": len(list), "pages": 1}})
			return
		}
		if strings.HasPrefix(path, "/api/v1/admin/users/") {
			parts := strings.Split(path, "/")
			id, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
			u, err := f.GetUser(r.Context(), id)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			f.mu.Lock()
			u.Balance = float64(f.Balances[id]) / 1_000_000
			f.mu.Unlock()
			writeJSON(w, map[string]any{"code": 0, "data": u})
			return
		}
		if path == "/api/v1/admin/users" {
			f.mu.Lock()
			list := make([]sub2api.User, 0, len(f.Users))
			for _, u := range f.Users {
				list = append(list, u)
			}
			f.mu.Unlock()
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": list, "total": len(list), "page": 1, "page_size": len(list), "pages": 1}})
			return
		}
		http.NotFound(w, r)
	})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func HashCode(code string) string { h := sha256.Sum256([]byte(code)); return hex.EncodeToString(h[:]) }
