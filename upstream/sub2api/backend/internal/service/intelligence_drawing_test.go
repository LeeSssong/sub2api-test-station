package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDrawingJudgeSingleRequest(t *testing.T) {
	for _, tc := range []struct{ reply, want string }{
		{`{"subject":{"verdict":"correct","reason":"人物清晰"},"action":{"verdict":"correct","reason":"手持桨划船"},"scene":{"verdict":"correct","reason":"背景有沙丘"},"structure":{"verdict":"correct","reason":"人船完整"}}`, "correct"},
		{`{"subject":{"verdict":"correct","reason":"人物清晰"},"action":{"verdict":"unknown","reason":"动作不可辨认"},"scene":{"verdict":"correct","reason":"背景有沙丘"},"structure":{"verdict":"incorrect","reason":"人物与船身分离"}}`, "incorrect"},
		{`broken`, "unknown"},
	} {
		svc, cfg, accounts, slots := judgeFixture()
		for i := range accounts.accounts {
			accounts.accounts[i].Platform = PlatformOpenAI
		}
		calls := 0
		svc.requestImage = func(_ context.Context, id int64, model, prompt, image string) (string, error) {
			calls++
			require.Contains(t, prompt, "划着小船")
			require.Contains(t, prompt, "沙漠里")
			require.Equal(t, "data:image/png;base64,fixture", image)
			return tc.reply, nil
		}
		cfg.Prompt = "请画山姆奥特曼划着小船在沙漠里"
		cfg.IntelligenceResult = &IntelligenceResultMetadata{Action: "划着小船", Scene: "沙漠里"}
		got := svc.JudgeDrawing(context.Background(), 99, cfg, "data:image/png;base64,fixture")
		require.Equal(t, 1, calls)
		require.Len(t, slots.released, 1)
		require.Equal(t, tc.want, got.Verdict)
	}
}
func TestDrawingJudgeDoesNotRetryPaidFailure(t *testing.T) {
	svc, cfg, accounts, _ := judgeFixture()
	for i := range accounts.accounts {
		accounts.accounts[i].Platform = PlatformOpenAI
	}
	calls := 0
	svc.requestImage = func(context.Context, int64, string, string, string) (string, error) {
		calls++
		return "", errors.New("timeout")
	}
	got := svc.JudgeDrawing(context.Background(), 99, cfg, "data:image/png;base64,fixture")
	require.Equal(t, 1, calls)
	require.Equal(t, "unknown", got.Verdict)
	require.Equal(t, "judge_request_failed", got.Reason)
}

func TestDrawingJudgeRequiresVisualEvidence(t *testing.T) {
	// Only the external image request is replaced; account selection and result
	// validation run normally. A bare overall "correct" must never authorize a pass.
	for _, tc := range []struct {
		name, dimension, verdict, reason, want string
		omit                                   bool
	}{
		{name: "all four dimensions supported", want: "correct"},
		{name: "severe obstruction", dimension: "structure", verdict: "incorrect", reason: "大块绿色图形严重遮挡巴士和人物", want: "incorrect"},
		{name: "unidentifiable character", dimension: "subject", verdict: "unknown", reason: "只有通用小人，无法确认题目人物", want: "unknown"},
		{name: "wrong character", dimension: "subject", verdict: "incorrect", reason: "画面主体是动物", want: "incorrect"},
		{name: "unclear action", dimension: "action", verdict: "unknown", reason: "人物与载具关系不可辨认", want: "unknown"},
		{name: "wrong action", dimension: "action", verdict: "incorrect", reason: "人物在行走，没有划船", want: "incorrect"},
		{name: "unclear scene", dimension: "scene", verdict: "unknown", reason: "背景没有可辨认的场景特征", want: "unknown"},
		{name: "wrong scene", dimension: "scene", verdict: "incorrect", reason: "背景是竹林而非沙漠", want: "incorrect"},
		{name: "unclear structure", dimension: "structure", verdict: "unknown", reason: "遮挡处的结构无法确认", want: "unknown"},
		{name: "missing structure", dimension: "structure", omit: true, want: "unknown"},
		{name: "missing visual reason", dimension: "subject", verdict: "correct", reason: " ", want: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks := map[string]any{
				"subject":   map[string]string{"verdict": "correct", "reason": "人物面容和发型具有题目人物的卡通化特征"},
				"action":    map[string]string{"verdict": "correct", "reason": "人物坐在船内，双手持桨"},
				"scene":     map[string]string{"verdict": "correct", "reason": "背景有沙丘和仙人掌"},
				"structure": map[string]string{"verdict": "correct", "reason": "人物、船和船桨完整，主体清晰可见"},
			}
			if tc.dimension != "" {
				if tc.omit {
					delete(checks, tc.dimension)
				} else {
					checks[tc.dimension] = map[string]string{"verdict": tc.verdict, "reason": tc.reason}
				}
			}
			body, err := json.Marshal(checks)
			require.NoError(t, err)
			svc, cfg, accounts, _ := judgeFixture()
			for i := range accounts.accounts {
				accounts.accounts[i].Platform = PlatformOpenAI
			}
			svc.requestImage = func(context.Context, int64, string, string, string) (string, error) {
				return string(body), nil
			}
			got := svc.JudgeDrawing(context.Background(), 99, cfg, "data:image/png;base64,fixture")
			require.Equal(t, tc.want, got.Verdict)
			if tc.dimension != "" && !tc.omit && strings.TrimSpace(tc.reason) != "" {
				require.Contains(t, got.Reason, tc.reason)
			}
		})
	}
	t.Run("legacy overall correct has no visual evidence", func(t *testing.T) {
		svc, cfg, accounts, _ := judgeFixture()
		for i := range accounts.accounts {
			accounts.accounts[i].Platform = PlatformOpenAI
		}
		svc.requestImage = func(context.Context, int64, string, string, string) (string, error) {
			return `{"verdict":"correct","reason":"符合动作和场景"}`, nil
		}
		got := svc.JudgeDrawing(context.Background(), 99, cfg, "data:image/png;base64,fixture")
		require.Equal(t, "unknown", got.Verdict)
	})
}

func TestDrawingJudgmentRejectsAmbiguousResponses(t *testing.T) {
	const check = `{"verdict":"correct","reason":"可辨认的视觉证据"}`
	const valid = `{"subject":` + check + `,"action":` + check + `,"scene":` + check + `,"structure":` + check + `}`
	for _, tc := range []struct{ name, response string }{
		{"duplicate dimension", strings.TrimSuffix(valid, "}") + `,"subject":` + check + `}`},
		{"unexpected overall verdict", strings.TrimSuffix(valid, "}") + `,"verdict":"correct"}`},
		{"duplicate nested verdict", strings.Replace(valid, check, `{"verdict":"incorrect","verdict":"correct","reason":"遮挡主体"}`, 1)},
		{"unknown nested field", strings.Replace(valid, check, `{"verdict":"correct","reason":"人物清晰","extra":true}`, 1)},
		{"invalid verdict", strings.Replace(valid, check, `{"verdict":"passed","reason":"人物清晰"}`, 1)},
		{"missing reason", strings.Replace(valid, check, `{"verdict":"correct"}`, 1)},
		{"null dimension", strings.Replace(valid, check, `null`, 1)},
		{"extra JSON", valid + `{}`},
		{"truncated JSON", strings.TrimSuffix(valid, "}")},
		{"oversized response", strings.Repeat(" ", 8001) + valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDrawingJudgment(tc.response)
			require.Error(t, err)
			require.Nil(t, got, "ambiguous evidence must not authorize a pass")
		})
	}
}

func TestDrawingPublicVerdictPreservesGeneration(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		state, display string
		expiry         time.Time
	}{{"passed", "passed", now.Add(time.Minute)}, {"incorrect", "incorrect", now.Add(time.Minute)}, {"unknown", "unknown", now.Add(time.Minute)}, {"pending", "pending", now.Add(time.Minute)}, {"pending", "unknown", now.Add(-time.Minute)}} {
		r := &PelicanGroupTestResult{Status: "success", ResponseText: "<svg></svg>", PelicanConfig: &PelicanTestConfig{QuestionKind: "pelican", IntelligenceResult: &IntelligenceResultMetadata{DrawingJudgment: &DrawingJudgment{Status: tc.state, Deadline: tc.expiry}}}}
		got := intelligencePublicResult(r, true)
		require.Equal(t, "passed", got.Verdict, "visual judgment must not change generation timeline or group status")
		require.Equal(t, tc.display, got.DrawingJudgment.Status)
		require.Equal(t, "<svg></svg>", got.ResponseText)
		require.Equal(t, "success", r.Status)
		require.Equal(t, tc.state, r.PelicanConfig.IntelligenceResult.DrawingJudgment.Status)
	}
	old := intelligencePublicResult(&PelicanGroupTestResult{Status: "success", PelicanConfig: &PelicanTestConfig{QuestionKind: "pelican"}}, true)
	require.Equal(t, "passed", old.Verdict)
}
func TestDrawingImagePayload(t *testing.T) {
	for _, mode := range []string{"responses", "chat", "anthropic"} {
		var payload map[string]any
		switch mode {
		case "responses":
			payload = createPelicanOpenAIPayload("model", false, "judge", "low")
		case "chat":
			payload = createOpenAIChatCompletionsTestPayload("model", "judge")
		case "anthropic":
			payload, _ = createPelicanClaudePayload("claude-sonnet-4-6", "judge", "low")
		}
		require.NoError(t, addDrawingJudgeImage(payload, mode, "data:image/png;base64,aGVsbG8="))
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		require.Equal(t, 1, strings.Count(string(data), "aGVsbG8="))
		require.Contains(t, string(data), "judge")
	}
}

type drawingRepoFake struct {
	*groupTestRepoFake
	claimed bool
	final   *DrawingJudgment
}

func (r *drawingRepoFake) ClaimDrawingJudgment(context.Context, int64) (bool, error) {
	if r.claimed {
		return false, nil
	}
	r.claimed = true
	return true, nil
}
func (r *drawingRepoFake) CompleteDrawingJudgment(_ context.Context, _ int64, j *DrawingJudgment) error {
	r.final = j
	return nil
}
func TestDrawingSavedBeforeJudgeAndNeverJudgesTwice(t *testing.T) {
	repo := &drawingRepoFake{groupTestRepoFake: newGroupTestRepoFake()}
	svc, _ := newGroupTestService(repo.groupTestRepoFake, nil, nil)
	svc.repo = repo
	calls := 0
	svc.renderDrawing = func(_ context.Context, raw string) (string, error) {
		require.Len(t, repo.results, 1)
		require.Equal(t, "pending", repo.results[0].PelicanConfig.IntelligenceResult.DrawingJudgment.Status)
		return "image", nil
	}
	svc.judgeDrawing = func(context.Context, int64, *PelicanTestConfig, string) *QualityJudgment {
		calls++
		return &QualityJudgment{Verdict: "incorrect", Reason: "人车分离"}
	}
	r := &PelicanGroupTestResult{Status: "success", ResponseText: "<svg/>", PelicanConfig: &PelicanTestConfig{QuestionKind: "pelican", DrawingJudge: &QualityJudgeConfig{GroupID: 1, ModelID: "vision"}, IntelligenceResult: &IntelligenceResultMetadata{}}}
	prepareDrawingJudgment(r)
	saved, err := repo.CreateResult(context.Background(), r)
	require.NoError(t, err)
	svc.completeDrawingJudgment(saved)
	svc.completeDrawingJudgment(saved)
	require.Equal(t, 1, calls)
	require.Equal(t, "incorrect", repo.final.Status)
	require.Equal(t, "success", saved.Status)
	require.Equal(t, "<svg/>", saved.ResponseText)
}
func TestDrawingConfigInheritsJudgeWithoutCandyRubric(t *testing.T) {
	svc, _ := newGroupTestService(newGroupTestRepoFake(), nil, nil)
	p := groupTestPlan(1)
	p.PelicanConfig.Intelligence = &IntelligenceRuleConfig{Actions: []string{"划着小船"}, Scenes: []string{"沙漠里"}, Candy: &PelicanTestConfig{Quality: &QualityPolicy{Judge: &QualityJudgeConfig{GroupID: 8, ModelID: "vision", Prompt: "compare 21"}}}}
	plans := svc.intelligenceSamplePlans(p)
	require.Equal(t, "vision", plans[0].PelicanConfig.DrawingJudge.ModelID)
	require.NotContains(t, drawingJudgePrompt(plans[0].PelicanConfig), "compare 21")
	p.PelicanConfig.Intelligence.DrawingJudge = &QualityJudgeConfig{GroupID: 9, ModelID: "other"}
	plans = svc.intelligenceSamplePlans(p)
	require.Equal(t, "other", plans[0].PelicanConfig.DrawingJudge.ModelID)
}

func TestDrawingNativeTransportCarriesExactlyOneImage(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		for _, mode := range []string{"responses", "chat"} {
			if platform == PlatformAnthropic && mode == "chat" {
				continue
			}
			account := &Account{ID: 123, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://fixture.example"}, Extra: map[string]any{"openai_responses_supported": mode == "responses"}}
			body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"verdict\\\":\\\"correct\\\",\\\"reason\\\":\\\"ok\\\"}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
			if mode == "chat" {
				body = "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"verdict\\\":\\\"correct\\\",\\\"reason\\\":\\\"ok\\\"}\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n"
			}
			if platform == PlatformAnthropic {
				body = "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"{\\\"verdict\\\":\\\"correct\\\",\\\"reason\\\":\\\"ok\\\"}\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}}
			svc := pelicanClaudeTestService(account, upstream)
			model := "gpt-6-astra"
			if platform == PlatformAnthropic {
				model = "claude-sonnet-4-6"
			}
			output, err := svc.runDrawingJudgeRequest(context.Background(), account.ID, model, "judge", "data:image/png;base64,aGVsbG8=")
			require.NoError(t, err, platform+mode)
			require.Contains(t, output, "correct")
			require.Len(t, upstream.requests, 1)
			require.Equal(t, 1, strings.Count(string(upstream.lastBody), "aGVsbG8="), platform+mode)
		}
	}
}
