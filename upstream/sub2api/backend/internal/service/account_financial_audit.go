package service

import (
	"context"
	"time"
)

type AccountFinancialAuditEvent struct {
	Action             string
	ActorUserID        int64
	RequestID          string
	AccountID          int64
	BusinessDate       string
	MutationKind       string
	OldValue, NewValue *float64
	Cutoff             int64
	Result             map[string]int64
	Sensitive          map[string]string
}
type financialAuditRecorder interface{ Record(*AuditLog) }
type AccountFinancialAudit struct {
	recorder financialAuditRecorder
	now      func() time.Time
}

func NewAccountFinancialAudit(recorder financialAuditRecorder) *AccountFinancialAudit {
	return NewAccountFinancialAuditWithClock(recorder, time.Now)
}
func NewAccountFinancialAuditWithClock(recorder financialAuditRecorder, now func() time.Time) *AccountFinancialAudit {
	if recorder == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &AccountFinancialAudit{recorder: recorder, now: now}
}
func (a *AccountFinancialAudit) Record(_ context.Context, event AccountFinancialAuditEvent) {
	if a == nil || a.recorder == nil {
		return
	}
	extra := map[string]any{"account_id": event.AccountID, "business_date": event.BusinessDate, "cutoff": event.Cutoff}
	if event.MutationKind != "" {
		extra["mutation_kind"] = event.MutationKind
	}
	if event.OldValue != nil {
		extra["old_value"] = *event.OldValue
	}
	if event.NewValue != nil {
		extra["new_value"] = *event.NewValue
	}
	for k, v := range event.Result {
		extra[k] = v
	}
	actor := event.ActorUserID
	a.recorder.Record(&AuditLog{CreatedAt: a.now(), ActorUserID: &actor, Action: event.Action, RequestID: event.RequestID, Method: "LOCAL", Path: "/admin/account-financial", StatusCode: 200, Extra: extra})
}
