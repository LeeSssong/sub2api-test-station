package sub2api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"example.invalid/internal-test-service/internal/domain"
)

type User struct {
	ID       int64   `json:"id"`
	Username string  `json:"username,omitempty"`
	Email    string  `json:"email,omitempty"`
	AffCode  string  `json:"aff_code,omitempty"`
	GroupID  int64   `json:"group_id,omitempty"`
	Balance  float64 `json:"balance,omitempty"`
}

type InvitationCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	AffCode   string     `json:"aff_code,omitempty"`
	UsedBy    *int64     `json:"used_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Usage struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	AmountUSD  string  `json:"amount_usd,omitempty"`
	TotalCost  float64 `json:"total_cost,omitempty"`
	ActualCost float64 `json:"actual_cost,omitempty"`
	Status     string  `json:"status,omitempty"`
	Successful bool    `json:"successful"`
	CreatedAt  string  `json:"created_at,omitempty"`
}

type Balance struct {
	UserID  int64
	Balance domain.MicroUSD
	History []BalanceEntry
}

type BalanceEntry struct {
	ID             int64   `json:"id,omitempty"`
	Code           string  `json:"code,omitempty"`
	Value          float64 `json:"value,omitempty"`
	AmountUSD      string  `json:"amount_usd,omitempty"`
	Operation      string  `json:"operation,omitempty"`
	Notes          string  `json:"notes,omitempty"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
	CreatedAt      string  `json:"created_at,omitempty"`
}

type CurrentUserResponse struct {
	ID   int64 `json:"id"`
	User User  `json:"user"`
	Data User  `json:"data"`
}

type InvitationListResponse struct {
	Data  []InvitationCode
	Items []InvitationCode
	Page  int
	Pages int
	Total int64
}

type UsageListResponse struct {
	Data        []Usage
	Items       []Usage
	NextAfterID int64 `json:"next_after_id,omitempty"`
	Page        int
	Pages       int
	Total       int64
}

type UserListResponse struct {
	Data  []User
	Items []User
	Page  int
	Pages int
	Total int64
}

type BalanceHistoryResponse struct {
	Data  []BalanceEntry
	Items []BalanceEntry
	Page  int
	Pages int
}

func (r *InvitationListResponse) UnmarshalJSON(data []byte) error {
	return decodeListResponse(data, &r.Data, &r.Items, &r.Page, &r.Pages, &r.Total)
}

func (r *UsageListResponse) UnmarshalJSON(data []byte) error {
	return decodeListResponse(data, &r.Data, &r.Items, &r.Page, &r.Pages, &r.Total)
}

func (r *UserListResponse) UnmarshalJSON(data []byte) error {
	return decodeListResponse(data, &r.Data, &r.Items, &r.Page, &r.Pages, &r.Total)
}

func (r *BalanceHistoryResponse) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Items []BalanceEntry  `json:"items"`
		Page  int             `json:"page"`
		Pages int             `json:"pages"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &r.Data); err == nil {
			r.Page, r.Pages = 1, 1
			return nil
		}
		var page struct {
			Items []BalanceEntry `json:"items"`
			Page  int            `json:"page"`
			Pages int            `json:"pages"`
		}
		if err := json.Unmarshal(envelope.Data, &page); err != nil {
			return err
		}
		r.Items = page.Items
		r.Page, r.Pages = page.Page, page.Pages
		return nil
	}
	r.Items = envelope.Items
	r.Page, r.Pages = envelope.Page, envelope.Pages
	if r.Page == 0 {
		r.Page, r.Pages = 1, 1
	}
	return nil
}

func decodeListResponse[T any](data []byte, direct, nested *[]T, page, pages *int, total *int64) error {
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Items []T             `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, direct); err == nil {
			*page, *pages = 1, 1
			return nil
		}
		var paginated struct {
			Items []T   `json:"items"`
			Page  int   `json:"page"`
			Pages int   `json:"pages"`
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(envelope.Data, &paginated); err != nil {
			return err
		}
		*nested = paginated.Items
		*page, *pages, *total = paginated.Page, paginated.Pages, paginated.Total
		return nil
	}
	*nested = envelope.Items
	*page, *pages = 1, 1
	return nil
}

func (u Usage) Normalize() Usage {
	if u.AmountUSD == "" && u.ActualCost != 0 {
		u.AmountUSD = fmt.Sprintf("%.6f", u.ActualCost)
	}
	if !u.Successful && u.ActualCost > 0 {
		u.Successful = true
	}
	return u
}

type Client interface {
	GetCurrentUser(ctx context.Context, bearer string) (User, error)
	GenerateInvitation(ctx context.Context, count int, expiresAt *time.Time, idem string) ([]InvitationCode, error)
	ListInvitationCodes(ctx context.Context) ([]InvitationCode, error)
	ExpireInvitation(ctx context.Context, codeID int64, idem string) error
	AddBalance(ctx context.Context, userID int64, amount domain.MicroUSD, idem, note string) error
	GetBalance(ctx context.Context, userID int64) (Balance, error)
	ListUsage(ctx context.Context, userID int64, afterID int64) ([]Usage, error)
	GetUser(ctx context.Context, userID int64) (User, error)
}
