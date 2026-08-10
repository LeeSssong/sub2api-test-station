package compare

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	ErrCutoverPredecessor         = errors.New("cutover predecessor is not external")
	ErrCutoverEvidence            = errors.New("cutover evidence is not eligible")
	ErrCutoverIdempotencyConflict = errors.New("cutover idempotency key conflicts with an existing request")
)

type CutoverAuditRecord struct {
	AuditID          string              `json:"audit_id"`
	IdempotencyKey   string              `json:"idempotency_key"`
	Page             Page                `json:"page"`
	RequestedMode    ReadMode            `json:"requested_mode"`
	EffectiveMode    ReadMode            `json:"effective_mode"`
	PreviousMode     ReadMode            `json:"previous_mode"`
	ReportSetID      string              `json:"report_set_id,omitempty"`
	RunID            string              `json:"run_id,omitempty"`
	EvidenceOperator string              `json:"evidence_operator,omitempty"`
	ComparedAt       time.Time           `json:"compared_at,omitempty"`
	ActorID          int64               `json:"actor_id"`
	RecordedAt       time.Time           `json:"recorded_at"`
	Result           string              `json:"result"`
	Retirement       *RetirementEvidence `json:"retirement,omitempty"`
}

type JSONLCutoverAuthority struct {
	path    string
	reports ReportSetRepository
	now     func() time.Time
	maxAge  time.Duration
	mu      sync.Mutex
}

func NewJSONLCutoverAuthority(path string, reports ReportSetRepository, now func() time.Time, maxAge time.Duration) *JSONLCutoverAuthority {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &JSONLCutoverAuthority{path: path, reports: reports, now: now, maxAge: maxAge}
}

func (a *JSONLCutoverAuthority) SetMode(ctx context.Context, page Page, mode ReadMode, actorID int64, idempotencyKey string, retirement *RetirementEvidence) (CutoverAuditRecord, error) {
	if a == nil || a.path == "" || a.reports == nil {
		return CutoverAuditRecord{}, fmt.Errorf("cutover authority is unavailable")
	}
	if !validPage(page) || !validReadMode(mode) || actorID <= 0 || idempotencyKey == "" {
		return CutoverAuditRecord{}, fmt.Errorf("page, mode, actor, and idempotency key are required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	records, err := a.loadLocked(ctx)
	if err != nil {
		return CutoverAuditRecord{}, err
	}
	for _, record := range records {
		if record.IdempotencyKey != idempotencyKey {
			continue
		}
		if record.Page != page || record.RequestedMode != mode || record.ActorID != actorID {
			return CutoverAuditRecord{}, ErrCutoverIdempotencyConflict
		}
		return record, nil
	}
	now := a.now().UTC()
	previous := latestRequestedMode(records, page)
	decision, err := a.evaluate(ctx, records, page, mode, now, retirement)
	if err != nil {
		return CutoverAuditRecord{}, err
	}
	result := "mode_changed"
	if mode == LegacyOnly && previous != LegacyOnly {
		result = "rolled_back"
	}
	record := CutoverAuditRecord{
		AuditID: fmt.Sprintf("cutover:%s:%d", page, now.UnixNano()), IdempotencyKey: idempotencyKey,
		Page: page, RequestedMode: mode, EffectiveMode: decision.EffectiveMode, PreviousMode: previous,
		ReportSetID: decision.ReportSetID, RunID: decision.RunID, EvidenceOperator: decision.Operator,
		ComparedAt: decision.ComparedAt, ActorID: actorID, RecordedAt: now, Result: result, Retirement: retirement,
	}
	if err := a.appendLocked(record); err != nil {
		return CutoverAuditRecord{}, err
	}
	return record, nil
}

func (a *JSONLCutoverAuthority) Decision(ctx context.Context, page Page) (CutoverDecision, error) {
	if a == nil || a.path == "" || a.reports == nil || !validPage(page) {
		return CutoverDecision{}, fmt.Errorf("cutover authority is unavailable")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	records, err := a.loadLocked(ctx)
	if err != nil {
		return CutoverDecision{}, err
	}
	mode := latestRequestedMode(records, page)
	retirement := latestRetirement(records, page)
	decision, err := a.evaluate(ctx, records, page, mode, a.now().UTC(), retirement)
	if errors.Is(err, ErrCutoverPredecessor) || errors.Is(err, ErrCutoverEvidence) {
		return CutoverDecision{Page: page, RequestedMode: mode, EffectiveMode: LegacyOnly, Degraded: true, Reason: err.Error()}, nil
	}
	return decision, err
}

func (a *JSONLCutoverAuthority) evaluate(ctx context.Context, records []CutoverAuditRecord, page Page, mode ReadMode, now time.Time, retirement *RetirementEvidence) (CutoverDecision, error) {
	return a.evaluatePage(ctx, records, page, mode, now, retirement, map[Page]bool{})
}

func (a *JSONLCutoverAuthority) evaluatePage(ctx context.Context, records []CutoverAuditRecord, page Page, mode ReadMode, now time.Time, retirement *RetirementEvidence, visited map[Page]bool) (CutoverDecision, error) {
	if visited[page] {
		return CutoverDecision{}, ErrCutoverPredecessor
	}
	visited[page] = true
	defer delete(visited, page)
	if mode == ExternalPrimary || mode == LegacyRetired {
		if predecessor, ok := predecessorPage(page); ok {
			predecessorMode := latestRequestedMode(records, predecessor)
			predecessorDecision, err := a.evaluatePage(ctx, records, predecessor, predecessorMode, now, latestRetirement(records, predecessor), visited)
			if err != nil || !predecessorDecision.UseExternal {
				return CutoverDecision{}, ErrCutoverPredecessor
			}
		}
	}
	sets, err := a.reports.LoadReportSets(ctx, page)
	if err != nil {
		return CutoverDecision{}, err
	}
	decision := EvaluateLatestPageCutover(mode, page, sets, now, a.maxAge, retirement)
	if (mode == ExternalPrimary || mode == LegacyRetired) && !decision.UseExternal {
		return CutoverDecision{}, ErrCutoverEvidence
	}
	return decision, nil
}

func (a *JSONLCutoverAuthority) AuditRecords(ctx context.Context) ([]CutoverAuditRecord, error) {
	if a == nil || a.path == "" {
		return nil, fmt.Errorf("cutover authority is unavailable")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadLocked(ctx)
}

func (a *JSONLCutoverAuthority) loadLocked(ctx context.Context) ([]CutoverAuditRecord, error) {
	file, err := os.Open(a.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open cutover audit: %w", err)
	}
	defer file.Close()
	records := make([]CutoverAuditRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var record CutoverAuditRecord
		if err := json.NewDecoder(bytes.NewReader(scanner.Bytes())).Decode(&record); err != nil {
			return nil, fmt.Errorf("decode cutover audit: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan cutover audit: %w", err)
	}
	return records, nil
}

func (a *JSONLCutoverAuthority) appendLocked(record CutoverAuditRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode cutover audit: %w", err)
	}
	file, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open cutover audit: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat cutover audit: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("cutover audit must be a regular 0600 file")
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("append cutover audit: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync cutover audit: %w", err)
	}
	return nil
}

func latestRequestedMode(records []CutoverAuditRecord, page Page) ReadMode {
	mode := LegacyOnly
	for _, record := range records {
		if record.Page == page {
			mode = record.RequestedMode
		}
	}
	return mode
}

func latestRetirement(records []CutoverAuditRecord, page Page) *RetirementEvidence {
	var result *RetirementEvidence
	for _, record := range records {
		if record.Page == page && record.Retirement != nil {
			copy := *record.Retirement
			result = &copy
		}
	}
	return result
}

func predecessorPage(page Page) (Page, bool) {
	switch page {
	case PageProfitability:
		return PageAccountMonitor, true
	case PageAccounting:
		return PageProfitability, true
	case PageReconciliation:
		return PageAccounting, true
	default:
		return "", false
	}
}

func validPage(page Page) bool {
	return page == PageAccountMonitor || page == PageProfitability || page == PageAccounting || page == PageReconciliation
}

func validReadMode(mode ReadMode) bool {
	return mode == LegacyOnly || mode == ShadowBuilding || mode == DualReadComparing || mode == ExternalPrimary || mode == LegacyRetired
}
