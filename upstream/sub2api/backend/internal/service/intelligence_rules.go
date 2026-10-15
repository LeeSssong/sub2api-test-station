package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const IntelligenceDrawingPrompt = "请直接返回完整的单文件HTML代码：使用内联SVG绘制山姆奥特曼{动作}在{场景}的2D动画。SVG必须设置viewBox，并用SVG或CSS实现动画；不要使用canvas、JavaScript、外部资源或Markdown代码块。你不需要任何测试"

const IntelligenceCandyPrompt = CandyPrompt + "\n允许靠手感按形状挑选糖果，但不能辨别口味；可以预先确定两种形状各取多少个。"

type IntelligenceCandySchedule struct {
	GroupID        int64  `json:"group_id"`
	CronExpression string `json:"cron_expression"`
}

type IntelligenceRuleConfig struct {
	DrawingJudge        *QualityJudgeConfig `json:"drawing_judge,omitempty"`
	CandyCronExpression string              `json:"candy_cron_expression"`
	CandyNextRunAt      *time.Time          `json:"candy_next_run_at,omitempty"`
	DrawingNextRunAt    *time.Time          `json:"drawing_next_run_at,omitempty"`
	RunningKinds        []string            `json:"running_kinds,omitempty"`
	QualityTemplateID   int64               `json:"quality_template_id"`
	CandyModels         []string            `json:"candy_models"`
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Candy               *PelicanTestConfig  `json:"candy"`
	Actions             []string            `json:"actions"`
	Scenes              []string            `json:"scenes"`
}

type IntelligenceResultMetadata struct {
	DrawingJudgment  *DrawingJudgment `json:"drawing_judgment,omitempty"`
	Source           string           `json:"source,omitempty"`
	SourceResultID   int64            `json:"source_result_id,omitempty"`
	SourceTemplateID int64            `json:"source_template_id,omitempty"`
	SourceStartedAt  *time.Time       `json:"source_started_at,omitempty"`
	SourceFinishedAt *time.Time       `json:"source_finished_at,omitempty"`
	Verdict          string           `json:"quality_verdict,omitempty"`
	Action           string           `json:"action,omitempty"`
	Scene            string           `json:"scene,omitempty"`
	FirstTokenMs     *int64           `json:"first_token_ms,omitempty"`
	InputTokens      *int             `json:"input_tokens,omitempty"`
	OutputTokens     *int             `json:"output_tokens,omitempty"`
}

type IntelligenceRuleInput struct {
	DrawingJudge         *QualityJudgeConfig         `json:"drawing_judge,omitempty"`
	CandySchedules       []IntelligenceCandySchedule `json:"candy_schedules"`
	CandyReasoningEffort string                      `json:"candy_reasoning_effort"`
	QualitySources       []IntelligenceQualitySource `json:"quality_sources"`
	CandyModels          []string                    `json:"candy_models"`
	Name                 string                      `json:"name"`
	GroupIDs             []int64                     `json:"group_ids"`
	ModelID              string                      `json:"model_id"`
	ReasoningEffort      string                      `json:"reasoning_effort"`
	CronExpression       string                      `json:"cron_expression"`
	Enabled              bool                        `json:"enabled"`
	CandyPrompt          string                      `json:"candy_prompt"`
	ExpectedAnswer       string                      `json:"expected_answer"`
	Judge                *QualityJudgeConfig         `json:"judge,omitempty"`
	DrawingPrompt        string                      `json:"drawing_prompt"`
	Actions              []string                    `json:"actions"`
	Scenes               []string                    `json:"scenes"`
}

// Rules are transactionally materialized into the existing per-group schedules.
type IntelligenceRuleRepository interface {
	SaveIntelligenceRule(context.Context, string, []*PelicanGroupTestPlan) error
	DeleteIntelligenceRule(context.Context, string) error
}

func (s *PelicanGroupTestService) SaveIntelligenceRule(ctx context.Context, id string, input IntelligenceRuleInput) (string, error) {
	if id == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		id = hex.EncodeToString(b[:])
	}
	if len(id) > 64 {
		return "", infraerrors.BadRequest("INVALID_INTELLIGENCE_RULE", "invalid rule id")
	}
	plans, err := s.intelligencePlans(ctx, id, input)
	if err != nil {
		return "", err
	}
	repo, ok := s.repo.(IntelligenceRuleRepository)
	if !ok {
		return "", fmt.Errorf("intelligence rule repository unavailable")
	}
	if err = repo.SaveIntelligenceRule(ctx, id, plans); err != nil {
		return "", err
	}
	return id, nil
}
func (s *PelicanGroupTestService) DeleteIntelligenceRule(ctx context.Context, id string) error {
	repo, ok := s.repo.(IntelligenceRuleRepository)
	if !ok {
		return fmt.Errorf("intelligence rule repository unavailable")
	}
	return repo.DeleteIntelligenceRule(ctx, id)
}

func (s *PelicanGroupTestService) intelligencePlans(ctx context.Context, id string, in IntelligenceRuleInput) ([]*PelicanGroupTestPlan, error) {
	invalid := func(message string) ([]*PelicanGroupTestPlan, error) {
		return nil, infraerrors.BadRequest("INVALID_INTELLIGENCE_RULE", message)
	}
	in.Name = strings.TrimSpace(in.Name)
	in.CandyPrompt = strings.TrimSpace(in.CandyPrompt)
	in.ExpectedAnswer = strings.TrimSpace(in.ExpectedAnswer)
	if in.Name == "" || len(in.Name) > 200 {
		return invalid("rule name is required (maximum 200 bytes)")
	}
	if len(in.GroupIDs) == 0 || len(in.GroupIDs) > 100 {
		return invalid("select 1–100 groups")
	}
	if in.CandyPrompt == "" {
		in.CandyPrompt = IntelligenceCandyPrompt
	}
	if in.ExpectedAnswer == "" {
		in.ExpectedAnswer = "21"
	}
	if len(in.CandyPrompt) > 32000 {
		return invalid("candy prompt exceeds 32000 bytes")
	}
	if in.Judge == nil && (!intelligenceBuiltinCandy(in.CandyPrompt) || in.ExpectedAnswer != "21") {
		return invalid("custom candy question requires a judge")
	}
	if in.CandyReasoningEffort == "" {
		in.CandyReasoningEffort = "high"
	}
	effort := normalizePelicanReasoningEffort(in.CandyReasoningEffort)
	if effort == "" {
		return invalid("invalid candy reasoning effort")
	}
	if in.DrawingJudge != nil {
		if in.DrawingJudge.GroupID <= 0 || strings.TrimSpace(in.DrawingJudge.ModelID) == "" || len(in.DrawingJudge.ModelID) > 100 {
			return invalid("drawing judge group and model are required")
		}
	}
	candy := &PelicanTestConfig{QuestionKind: "candy", Prompt: in.CandyPrompt, ReasoningEffort: effort, ParallelCount: 1, Quality: &QualityPolicy{ExpectedAnswer: in.ExpectedAnswer, Judge: in.Judge, Action: QualityActionObserveOnly}}
	if err := validateQualityPolicy(&ScheduledTestPlan{PelicanConfig: candy}); err != nil {
		return invalid(err.Error())
	}
	models, err := normalizeIntelligenceModels(in.CandyModels)
	if err != nil {
		return invalid(err.Error())
	}
	if in.CronExpression != "" && in.CronExpression != "*/30 * * * *" {
		return invalid("drawing schedule must run every 30 minutes")
	}
	in.CronExpression = "*/30 * * * *"
	schedules := map[int64]string{}
	for _, schedule := range in.CandySchedules {
		if schedule.GroupID <= 0 || schedules[schedule.GroupID] != "" || !slices.Contains(in.GroupIDs, schedule.GroupID) {
			return invalid("invalid or duplicate candy schedule group")
		}
		expression := strings.TrimSpace(schedule.CronExpression)
		if _, err := intelligenceNextRun(expression, s.now()); err != nil {
			return invalid("invalid candy cron: " + err.Error())
		}
		schedules[schedule.GroupID] = expression
	}
	if !strings.Contains(in.DrawingPrompt, "{动作}") || !strings.Contains(in.DrawingPrompt, "{场景}") {
		return invalid("drawing template must contain {动作} and {场景}")
	}
	validateChoices := func(items []string) bool {
		if len(items) == 0 || len(items) > 100 {
			return false
		}
		for _, v := range items {
			if strings.TrimSpace(v) == "" || len(v) > 200 || strings.ContainsAny(v, "{}\r\n") {
				return false
			}
		}
		return true
	}
	if !validateChoices(in.Actions) || !validateChoices(in.Scenes) {
		return invalid("provide 1–100 nonempty actions and scenes (maximum 200 bytes each)")
	}
	seen := map[int64]bool{}
	plans := make([]*PelicanGroupTestPlan, 0, len(in.GroupIDs))
	for _, gid := range in.GroupIDs {
		if seen[gid] {
			return invalid("duplicate group")
		}
		seen[gid] = true
		p, err := s.planFromInput(ctx, PelicanGroupTestPlanInput{GroupID: gid, ModelID: in.ModelID, ReasoningEffort: in.ReasoningEffort, CronExpression: in.CronExpression, Enabled: in.Enabled, Prompt: in.DrawingPrompt, ParallelCount: 1}, nil)
		if err != nil {
			return nil, err
		}
		expression := schedules[gid]
		if expression == "" {
			expression = pelicanGroupTestDefaultCron
		}
		candyNext, err := intelligenceNextRun(expression, s.now())
		if err != nil {
			return invalid(err.Error())
		}
		drawingNext := *p.NextRunAt
		p.PelicanConfig.Intelligence = &IntelligenceRuleConfig{DrawingJudge: in.DrawingJudge, ID: id, Name: in.Name, Candy: candy, CandyModels: models, CandyCronExpression: expression, CandyNextRunAt: &candyNext, DrawingNextRunAt: &drawingNext, Actions: append([]string(nil), in.Actions...), Scenes: append([]string(nil), in.Scenes...)}
		if candyNext.Before(drawingNext) {
			p.NextRunAt = &candyNext
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func intelligenceChoice(items []string) string {
	if len(items) == 0 {
		return ""
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(items))))
	if err != nil {
		return items[0]
	}
	return items[n.Int64()]
}

func (s *PelicanGroupTestService) intelligenceSamplePlans(plan *PelicanGroupTestPlan) []*PelicanGroupTestPlan {
	cfg := plan.PelicanConfig
	if cfg == nil || cfg.Intelligence == nil {
		return []*PelicanGroupTestPlan{plan}
	}
	rule := cfg.Intelligence
	drawingPlan := *plan
	drawing := *cfg
	drawing.Intelligence = nil
	drawing.QuestionKind = "pelican"
	drawing.DrawingJudge = rule.DrawingJudge
	if drawing.DrawingJudge == nil && rule.Candy != nil && rule.Candy.Quality != nil {
		drawing.DrawingJudge = rule.Candy.Quality.Judge
	}
	drawing.IntelligenceResult = &IntelligenceResultMetadata{Action: intelligenceChoice(rule.Actions), Scene: intelligenceChoice(rule.Scenes)}
	drawing.Prompt = strings.NewReplacer("{动作}", drawing.IntelligenceResult.Action, "{场景}", drawing.IntelligenceResult.Scene).Replace(cfg.Prompt)
	drawingPlan.PelicanConfig = &drawing
	plans := []*PelicanGroupTestPlan{}
	if plan.intelligenceRunKinds == nil || containsString(plan.intelligenceRunKinds, "pelican") {
		plans = append(plans, &drawingPlan)
	}
	if plan.intelligenceRunKinds == nil || containsString(plan.intelligenceRunKinds, "candy") {
		models, _ := normalizeIntelligenceModels(rule.CandyModels)
		for _, model := range models {
			candyPlan := *plan
			candyPlan.ModelID = model
			candy := intelligenceCandyConfig(rule)
			candy.ModelID = model
			candy.IntelligenceResult = &IntelligenceResultMetadata{Source: "intelligence_live"}
			candyPlan.PelicanConfig = candy
			plans = append(plans, &candyPlan)
		}
	}
	return plans
}

func (s *PelicanGroupTestService) gradeIntelligence(ctx context.Context, accountID int64, cfg *PelicanTestConfig, r *ScheduledTestResult) {
	if cfg == nil || cfg.QuestionKind != "candy" || cfg.Quality == nil || r.Status != "success" {
		return
	}
	var judgment *QualityJudgment
	if cfg.Quality.Judge == nil && intelligenceBuiltinCandy(cfg.Prompt) && cfg.Quality.ExpectedAnswer == "21" {
		judgment = &QualityJudgment{Verdict: "incorrect", Reason: "builtin_candy"}
		if CandyAnswerCorrect(r.ResponseText) {
			judgment.Verdict = "correct"
		}
	} else if s.judgeQuality != nil {
		judgment = s.judgeQuality(ctx, accountID, cfg, r.ResponseText)
	}
	applyQualityJudgment(r, judgment)
}

func intelligenceBuiltinCandy(prompt string) bool {
	return strings.TrimSpace(prompt) == strings.TrimSpace(CandyPrompt) || strings.TrimSpace(prompt) == strings.TrimSpace(IntelligenceCandyPrompt)
}

// Legacy rules without a candy block receive standalone defaults; no quality source is read.
func intelligenceCandyConfig(rule *IntelligenceRuleConfig) *PelicanTestConfig {
	cfg := PelicanTestConfig{Prompt: IntelligenceCandyPrompt, ReasoningEffort: "high"}
	if rule.Candy != nil {
		cfg = *rule.Candy
	}
	cfg.QuestionKind, cfg.ParallelCount, cfg.Intelligence = "candy", 1, nil
	if cfg.Prompt == "" {
		cfg.Prompt = IntelligenceCandyPrompt
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "high"
	}
	policy := QualityPolicy{ExpectedAnswer: "21"}
	if cfg.Quality != nil {
		policy.ExpectedAnswer, policy.Judge = cfg.Quality.ExpectedAnswer, cfg.Quality.Judge
	}
	policy.Action = QualityActionObserveOnly
	cfg.Quality = &policy
	return &cfg
}

func intelligenceNextRun(expression string, now time.Time) (time.Time, error) {
	if len(strings.Fields(expression)) != 5 {
		return time.Time{}, fmt.Errorf("use five cron fields: minute hour day month weekday")
	}
	next, err := computeNextRun(expression, now)
	if err == nil && next.IsZero() {
		err = fmt.Errorf("cron has no future occurrence")
	}
	return next, err
}

// Clone the execution snapshot. Persist both independent cursors with the lease so
// another worker cannot duplicate a due kind, and manual runs never move either cursor.
func prepareIntelligenceRun(plan *PelicanGroupTestPlan, now time.Time) (*PelicanGroupTestPlan, time.Time, error) {
	p := *plan
	cfg := *plan.PelicanConfig
	rule := *cfg.Intelligence
	cfg.Intelligence, p.PelicanConfig = &rule, &cfg
	p.intelligenceRunKinds = []string{}
	if rule.CandyCronExpression == "" {
		rule.CandyCronExpression = pelicanGroupTestDefaultCron
	}
	fallback := plan.NextRunAt
	if fallback == nil {
		fallback = &now
	}
	if rule.CandyNextRunAt == nil {
		rule.CandyNextRunAt = fallback
	}
	if rule.DrawingNextRunAt == nil {
		rule.DrawingNextRunAt = fallback
	}
	for _, kind := range []string{"candy", "pelican"} {
		cursor, expression := &rule.CandyNextRunAt, rule.CandyCronExpression
		if kind == "pelican" {
			cursor, expression = &rule.DrawingNextRunAt, pelicanGroupTestDefaultCron
		}
		if !(**cursor).After(now) {
			next, err := intelligenceNextRun(expression, now)
			if err != nil {
				return nil, time.Time{}, err
			}
			*cursor = &next
			p.intelligenceRunKinds = append(p.intelligenceRunKinds, kind)
		}
	}
	rule.RunningKinds = p.intelligenceRunKinds
	next := *rule.DrawingNextRunAt
	if rule.CandyNextRunAt.Before(next) {
		next = *rule.CandyNextRunAt
	}
	return &p, next, nil
}
