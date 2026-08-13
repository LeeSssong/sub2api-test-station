package service

import (
	"context"
	"testing"
	"time"
)

type auditRecorder struct{ entries []*AuditLog }

func (r *auditRecorder) Record(entry *AuditLog) { r.entries = append(r.entries, entry) }

func TestAccountFinancialAuditRedactsAndRecordsMutationContext(t *testing.T) {
	recorder := &auditRecorder{}
	audit := NewAccountFinancialAudit(recorder)
	audit.Record(context.Background(), AccountFinancialAuditEvent{
		Action: "admin.account_financial.review", ActorUserID: 9, RequestID: "req-1", AccountID: 2,
		BusinessDate: "2026-08-13", OldValue: nil, NewValue: floatPtr(3), Cutoff: 88,
		Result:    map[string]int64{"matched": 4, "updated": 3, "skipped": 1},
		Sensitive: map[string]string{"api_key": "secret", "upstream_error": "raw"},
	})
	if len(recorder.entries) != 1 {
		t.Fatalf("entries=%d", len(recorder.entries))
	}
	entry := recorder.entries[0]
	if entry.ActorUserID == nil || *entry.ActorUserID != 9 || entry.RequestID != "req-1" || entry.Action != "admin.account_financial.review" {
		t.Fatalf("audit identity missing: %#v", entry)
	}
	if entry.RequestBody != "" {
		t.Fatalf("financial audit must not retain request body: %q", entry.RequestBody)
	}
	if _, ok := entry.Extra["sensitive"]; ok {
		t.Fatalf("sensitive fields leaked: %#v", entry.Extra)
	}
	if entry.CreatedAt.IsZero() || entry.StatusCode != 200 {
		t.Fatalf("audit result incomplete: %#v", entry)
	}
}

func TestAccountFinancialAuditUsesFixedCreatedAt(t *testing.T) {
	when := time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	recorder := &auditRecorder{}
	NewAccountFinancialAuditWithClock(recorder, func() time.Time { return when }).Record(context.Background(), AccountFinancialAuditEvent{Action: "test"})
	if !recorder.entries[0].CreatedAt.Equal(when) {
		t.Fatalf("created_at=%v", recorder.entries[0].CreatedAt)
	}
}
