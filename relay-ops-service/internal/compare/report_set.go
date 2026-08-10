package compare

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"
)

var requiredWindowDefinitions = []WindowDefinition{
	{Kind: WindowMinimum, Duration: 15 * time.Minute},
	{Kind: WindowDefault, Duration: 24 * time.Hour},
	{Kind: WindowMaximum, Duration: 30 * 24 * time.Hour},
}

func RequiredWindowDefinitions() []WindowDefinition {
	return append([]WindowDefinition(nil), requiredWindowDefinitions...)
}

type JSONLReportSetRepository struct {
	path string
	mu   sync.Mutex
}

func NewJSONLReportSetRepository(path string) *JSONLReportSetRepository {
	return &JSONLReportSetRepository{path: path}
}

func (r *JSONLReportSetRepository) SaveReportSet(ctx context.Context, set CompareReportSet) error {
	if r == nil || r.path == "" {
		return fmt.Errorf("comparison report-set path is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		return fmt.Errorf("encode comparison report set: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.loadLocked(ctx, "")
	if err != nil {
		return err
	}
	for _, item := range existing {
		if item.ID == set.ID {
			return fmt.Errorf("comparison report set %q already exists", set.ID)
		}
	}
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open comparison report set: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat comparison report set: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("comparison report set must be a regular 0600 file")
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("append comparison report set: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync comparison report set: %w", err)
	}
	return nil
}

func (r *JSONLReportSetRepository) LoadReportSets(ctx context.Context, page Page) ([]CompareReportSet, error) {
	if r == nil || r.path == "" {
		return nil, fmt.Errorf("comparison report-set path is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked(ctx, page)
}

func (r *JSONLReportSetRepository) loadLocked(ctx context.Context, page Page) ([]CompareReportSet, error) {
	file, err := os.Open(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open comparison report set: %w", err)
	}
	defer file.Close()
	sets := make([]CompareReportSet, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var set CompareReportSet
		if err := json.NewDecoder(bytes.NewReader(scanner.Bytes())).Decode(&set); err != nil {
			return nil, fmt.Errorf("decode comparison report set: %w", err)
		}
		if page == "" || set.Page == page {
			sets = append(sets, set)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan comparison report set: %w", err)
	}
	return sets, nil
}

type ReportSetService struct {
	repository ReportSetRepository
}

func NewReportSetService(repository ReportSetRepository) *ReportSetService {
	return &ReportSetService{repository: repository}
}

func (s *ReportSetService) CompareAndPersistSet(ctx context.Context, input ReportSetInput) (CompareReportSet, error) {
	if s == nil || s.repository == nil {
		return CompareReportSet{}, fmt.Errorf("compare report-set repository is required")
	}
	if input.SetID == "" || input.RunID == "" || input.Page == "" || input.Operator == "" || input.EvidenceLineage == "" || input.ComparedAt.IsZero() {
		return CompareReportSet{}, fmt.Errorf("report-set identity, page, operator, lineage, and timestamp are required")
	}
	if len(input.Comparisons) != len(requiredWindowDefinitions) {
		return CompareReportSet{}, fmt.Errorf("report set requires exactly three comparison windows")
	}
	byWindow := make(map[WindowKind]ComparisonInput, len(input.Comparisons))
	for _, comparison := range input.Comparisons {
		if comparison.Legacy.Page != input.Page || comparison.External.Page != input.Page ||
			comparison.Legacy.RunID != input.RunID || comparison.External.RunID != input.RunID ||
			comparison.Legacy.EvidenceLineage != input.EvidenceLineage || comparison.External.EvidenceLineage != input.EvidenceLineage ||
			comparison.Operator != input.Operator || !comparison.ComparedAt.Equal(input.ComparedAt) {
			return CompareReportSet{}, fmt.Errorf("comparison does not belong to report set")
		}
		if _, duplicate := byWindow[comparison.Legacy.Window]; duplicate {
			return CompareReportSet{}, fmt.Errorf("duplicate comparison window %q", comparison.Legacy.Window)
		}
		byWindow[comparison.Legacy.Window] = comparison
	}
	set := CompareReportSet{ID: input.SetID, RunID: input.RunID, Page: input.Page, Operator: input.Operator, EvidenceLineage: input.EvidenceLineage, ComparedAt: input.ComparedAt.UTC(), PersistedAt: time.Now().UTC()}
	for _, definition := range requiredWindowDefinitions {
		comparison, ok := byWindow[definition.Kind]
		if !ok || !comparison.Legacy.WindowEnd.Equal(input.ComparedAt) || !comparison.External.WindowEnd.Equal(input.ComparedAt) ||
			!comparison.Legacy.WindowStart.Equal(input.ComparedAt.Add(-definition.Duration)) || !comparison.External.WindowStart.Equal(input.ComparedAt.Add(-definition.Duration)) {
			return CompareReportSet{}, fmt.Errorf("window %q does not match its definition", definition.Kind)
		}
		report, err := buildCompareReport(comparison)
		if err != nil {
			return CompareReportSet{}, err
		}
		report.ReportSetID = set.ID
		report.RunID = set.RunID
		report.EvidenceLineage = set.EvidenceLineage
		report.PersistedAt = set.PersistedAt
		set.Reports = append(set.Reports, report)
	}
	if err := s.repository.SaveReportSet(ctx, set); err != nil {
		return CompareReportSet{}, fmt.Errorf("persist compare report set: %w", err)
	}
	return set, nil
}

func EvaluateLatestPageCutover(requested ReadMode, page Page, sets []CompareReportSet, now time.Time, maxAge time.Duration, retirement *RetirementEvidence) CutoverDecision {
	decision := CutoverDecision{Page: page, RequestedMode: requested, EffectiveMode: LegacyOnly, Reason: "legacy_default"}
	if requested == LegacyOnly {
		return decision
	}
	if requested == ShadowBuilding || requested == DualReadComparing {
		decision.EffectiveMode = requested
		decision.Reason = "legacy_visible_during_comparison"
		return decision
	}
	if requested != ExternalPrimary && requested != LegacyRetired {
		decision.Degraded = true
		decision.Reason = "invalid_mode"
		return decision
	}
	slices.SortFunc(sets, func(left, right CompareReportSet) int { return right.ComparedAt.Compare(left.ComparedAt) })
	for _, set := range sets {
		if !validReportSetAt(set, page, now, maxAge) {
			continue
		}
		if requested == LegacyRetired && !retirement.valid() {
			decision.Degraded = true
			decision.Reason = "retirement_evidence_missing"
			return decision
		}
		decision.EffectiveMode = requested
		decision.UseExternal = true
		decision.Reason = "comparison_gate_passed"
		decision.ReportSetID = set.ID
		decision.RunID = set.RunID
		decision.Operator = set.Operator
		decision.ComparedAt = set.ComparedAt
		return decision
	}
	decision.Degraded = true
	decision.Reason = "comparison_gate_failed"
	return decision
}

func validReportSetAt(set CompareReportSet, page Page, now time.Time, maxAge time.Duration) bool {
	if set.Page != page || !set.Eligible() || maxAge <= 0 || now.Before(set.ComparedAt) || now.Sub(set.ComparedAt) > maxAge {
		return false
	}
	definitions := make(map[WindowKind]time.Duration, len(requiredWindowDefinitions))
	for _, definition := range requiredWindowDefinitions {
		definitions[definition.Kind] = definition.Duration
	}
	for _, report := range set.Reports {
		duration, ok := definitions[report.Window]
		if !ok || !report.WindowEnd.Equal(set.ComparedAt) || !report.WindowStart.Equal(set.ComparedAt.Add(-duration)) {
			return false
		}
		for _, freshness := range []FreshnessEvidence{report.Freshness.Legacy, report.Freshness.External} {
			if !freshnessPassed(freshness, set.ComparedAt) || now.After(freshness.FreshUntil) || set.ComparedAt.Sub(freshness.GeneratedAt) > maxAge {
				return false
			}
		}
	}
	return true
}
