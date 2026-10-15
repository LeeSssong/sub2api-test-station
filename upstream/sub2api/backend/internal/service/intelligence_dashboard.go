package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type IntelligenceResultRepository interface {
	ListIntelligenceResults(context.Context, []int64, int, time.Time) ([]*PelicanGroupTestResult, error)
}
type IntelligenceResult struct {
	ValidUntil      *time.Time `json:"valid_until,omitempty"`
	ID              int64      `json:"id"`
	GroupID         int64      `json:"group_id"`
	ModelID         string     `json:"model_id"`
	Kind            string     `json:"kind"`
	Verdict         string     `json:"verdict"`
	ErrorKind       string     `json:"error_kind,omitempty"`
	Prompt          string     `json:"prompt,omitempty"`
	ExpectedAnswer  string     `json:"expected_answer,omitempty"`
	ResponseText    string     `json:"response_text,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort"`
	LatencyMs       int64      `json:"latency_ms"`
	Attempts        int        `json:"attempts"`
	StartedAt       time.Time  `json:"started_at"`
	*IntelligenceResultMetadata
}
type IntelligenceGroup struct {
	CandyCronExpression   string                `json:"candy_cron_expression"`
	CandyRunningStartedAt *time.Time            `json:"candy_running_started_at,omitempty"`
	CandyModelIDs         []string              `json:"candy_model_ids"`
	QualityTemplateID     int64                 `json:"quality_template_id,omitempty"`
	QualitySourceStatus   string                `json:"quality_source_status,omitempty"`
	NextRunAt             *time.Time            `json:"next_run_at,omitempty"`
	RunningStartedAt      *time.Time            `json:"running_started_at,omitempty"`
	ID                    int64                 `json:"id"`
	Name                  string                `json:"name"`
	Description           string                `json:"description"`
	Platform              string                `json:"platform"`
	RateMultiplier        float64               `json:"rate_multiplier"`
	ModelID               string                `json:"model_id"`
	ReasoningEffort       string                `json:"reasoning_effort"`
	ExpectedAnswer        string                `json:"expected_answer"`
	Results               []*IntelligenceResult `json:"results"`
}
type IntelligenceDashboard struct {
	Enabled     bool                 `json:"enabled"`
	GeneratedAt time.Time            `json:"generated_at"`
	Groups      []*IntelligenceGroup `json:"groups"`
}

func intelligenceVerdict(r *PelicanGroupTestResult) string {
	if r.PelicanConfig != nil && r.PelicanConfig.IntelligenceResult != nil && r.PelicanConfig.IntelligenceResult.Verdict != "" {
		return r.PelicanConfig.IntelligenceResult.Verdict
	}
	if r.Status == "success" {
		return "passed"
	}
	if strings.HasPrefix(r.ErrorMessage, "answer_mismatch") {
		return "incorrect"
	}
	return "abnormal"
}
func intelligencePublicResult(r *PelicanGroupTestResult, detail bool) *IntelligenceResult {
	out := &IntelligenceResult{ID: r.ID, GroupID: r.GroupID, Kind: "pelican", Verdict: intelligenceVerdict(r), LatencyMs: r.LatencyMs, Attempts: len(r.Attempts), StartedAt: r.StartedAt}
	if r.AccountID > 0 {
		out.Attempts++
	}
	if out.Verdict == "abnormal" {
		out.ErrorKind = "request_failed"
		if strings.HasPrefix(r.ErrorMessage, "no_available_account") {
			out.ErrorKind = "no_available_account"
		}
		if strings.HasPrefix(r.ErrorMessage, "judge_") {
			out.ErrorKind = "judge_inconclusive"
		} else if r.ResponseText != "" {
			out.ErrorKind = "invalid_output"
		}
	}
	if cfg := r.PelicanConfig; cfg != nil {
		out.ModelID = cfg.ModelID
		out.ReasoningEffort = cfg.ReasoningEffort
		out.IntelligenceResultMetadata = cfg.IntelligenceResult
		if cfg.IntelligenceResult != nil {
			m := *cfg.IntelligenceResult
			m.DrawingJudgment = drawingJudgmentForDisplay(cfg.IntelligenceResult)
			out.IntelligenceResultMetadata = &m
		}
		if cfg.QuestionKind != "" {
			out.Kind = cfg.QuestionKind
		}
		if cfg.Quality != nil {
			out.ExpectedAnswer = cfg.Quality.ExpectedAnswer
		}
		if detail {
			out.Prompt = intelligenceTestPrompt(cfg)
		}
	}
	if detail {
		out.ResponseText = r.ResponseText
	}
	return out
}
func (s *PelicanGroupTestService) IntelligenceDashboard(ctx context.Context) (*IntelligenceDashboard, error) {
	view := &IntelligenceDashboard{Enabled: true, GeneratedAt: s.now(), Groups: []*IntelligenceGroup{}}
	// Visibility in navigation is configured through custom menus. The independent
	// authenticated page has no second legacy-gallery switch.
	plans, err := s.repo.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	groups := map[int64]*Group{}
	blocks := map[string]*IntelligenceGroup{}
	key := func(gid int64, model string) string { return fmt.Sprint(gid) }
	for _, p := range plans {
		if p.PelicanConfig == nil || p.PelicanConfig.Intelligence == nil {
			continue
		}
		if p.GroupStatus != "" && p.GroupStatus != StatusActive {
			continue
		}
		g := groups[p.GroupID]
		if g == nil {
			g, err = s.groups.GetByID(ctx, p.GroupID)
			if err != nil {
				return nil, err
			}
			if g == nil || g.Status != StatusActive {
				continue
			}
			groups[p.GroupID] = g
		}
		ids = append(ids, p.ID)
		k := key(g.ID, p.ModelID)
		if blocks[k] == nil {
			b := &IntelligenceGroup{ID: g.ID, Name: g.Name, Description: g.Description, Platform: g.Platform, RateMultiplier: g.RateMultiplier, ModelID: p.ModelID, Results: []*IntelligenceResult{}}
			if p.PelicanConfig != nil {
				b.ReasoningEffort = p.PelicanConfig.ReasoningEffort
				rule := p.PelicanConfig.Intelligence
				b.CandyModelIDs, _ = normalizeIntelligenceModels(rule.CandyModels)
				b.CandyCronExpression = rule.CandyCronExpression
				if b.CandyCronExpression == "" {
					b.CandyCronExpression = pelicanGroupTestDefaultCron
				}
				candy := intelligenceCandyConfig(rule)
				b.ExpectedAnswer = candy.Quality.ExpectedAnswer

			}
			blocks[k] = b
			if p.Enabled {
				b.NextRunAt = p.NextRunAt
			}
			if p.RunningUntil != nil && p.RunningUntil.After(s.now()) {
				started := p.RunningUntil.Add(-pelicanGroupTestLease)
				kinds := p.PelicanConfig.Intelligence.RunningKinds
				if len(kinds) == 0 || containsString(kinds, "pelican") {
					b.RunningStartedAt = &started
				}
				if len(kinds) == 0 || containsString(kinds, "candy") {
					b.CandyRunningStartedAt = &started
				}
			}
			view.Groups = append(view.Groups, b)
		}
	}
	if len(ids) == 0 {
		return view, nil
	}
	repo, ok := s.repo.(IntelligenceResultRepository)
	if !ok {
		return nil, fmt.Errorf("intelligence result repository unavailable")
	}
	since := s.now().Add(-72 * time.Hour)
	results, err := repo.ListIntelligenceResults(ctx, ids, 5000, since)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		dto := intelligencePublicResult(r, false)
		k := key(r.GroupID, dto.ModelID)
		b := blocks[k]
		if b == nil {
			g := groups[r.GroupID]
			if g == nil {
				continue
			}
			b = &IntelligenceGroup{ID: g.ID, Name: g.Name, Description: g.Description, Platform: g.Platform, RateMultiplier: g.RateMultiplier, ModelID: dto.ModelID, ReasoningEffort: dto.ReasoningEffort, ExpectedAnswer: dto.ExpectedAnswer, Results: []*IntelligenceResult{}}
			blocks[k] = b
			view.Groups = append(view.Groups, b)
		}
		expression := pelicanGroupTestDefaultCron
		if dto.Kind == "candy" {
			expression = b.CandyCronExpression
		}
		if next, err := intelligenceNextRun(expression, r.StartedAt); err == nil {
			validUntil := next.Add(pelicanGroupTestRunTimeout)
			dto.ValidUntil = &validUntil
		}
		b.Results = append(b.Results, dto)
	}
	return view, nil
}
func (s *PelicanGroupTestService) IntelligenceResult(ctx context.Context, id int64) (*IntelligenceResult, error) {
	since := s.now().Add(-72 * time.Hour)
	if repo, ok := s.repo.(interface {
		GetIntelligenceResult(context.Context, int64, int, time.Time) (*PelicanGroupTestResult, error)
	}); ok {
		r, err := repo.GetIntelligenceResult(ctx, id, 5000, since)
		if err != nil {
			return nil, err
		}
		if r == nil {
			return nil, ErrPelicanGroupTestResultNotFound
		}
		return intelligencePublicResult(r, true), nil
	}
	// Compatibility for repository adapters without the optimized single-record read.
	view, err := s.IntelligenceDashboard(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range view.Groups {
		for _, r := range g.Results {
			if r.ID == id {
				full, err := s.GetResult(ctx, id)
				if err != nil {
					return nil, err
				}
				return intelligencePublicResult(full, true), nil
			}
		}
	}
	return nil, ErrPelicanGroupTestResultNotFound
}
