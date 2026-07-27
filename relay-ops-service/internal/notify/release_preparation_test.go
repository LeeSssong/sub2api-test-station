package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderReleasePreparationSuccessIsFactOnly(t *testing.T) {
	message := RenderReleasePreparation(ReleasePreparationView{
		Status: "succeeded", Version: "0.1.167", ReleaseName: "v0.1.167",
		ReleaseBody: "Fix billing and compatibility.", PublishedAt: "2026-07-28T01:02:03Z",
		ReleaseURL:     "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.167",
		OfficialCommit: strings.Repeat("a", 40), SourceCommit: strings.Repeat("b", 40),
		ImageDigest: "sha256:" + strings.Repeat("c", 64), ProductionImage: "sha256:" + strings.Repeat("d", 64),
		RunningVersion: "0.1.166", RunningImage: "sha256:" + strings.Repeat("e", 64),
		RunningHealth: "healthy", RunningStartedAt: "2026-07-27T00:00:00Z",
		ComposeSHA256: strings.Repeat("f", 64), Checks: []string{"后端测试通过", "前端测试通过"},
		WorkflowURL: "https://github.com/LeeSssong/sub2api/actions/runs/1",
	})
	text := message.RenderedText()
	if message.Card.Header.Title.Content != "Sub2API 0.1.167 候选镜像已静默准备" {
		t.Fatalf("title=%q", message.Card.Header.Title.Content)
	}
	for _, want := range []string{
		"Fix billing and compatibility.", "后端测试通过", "前端测试通过",
		"0.1.166", "healthy", "未调用更新 API", "未修改 Compose", "未切换运行容器",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	for _, forbidden := range []string{"下一步", "请点击", "立即更新"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden %q in %s", forbidden, text)
		}
	}
}

func TestRenderReleasePreparationFailureUsesStableCodeAndRedactsBody(t *testing.T) {
	message := RenderReleasePreparation(ReleasePreparationView{
		Status: "failed", Stage: "prepare", Version: "0.1.167",
		ReleaseBody: "Bearer secret-value", ErrorCode: "TEST_GATE_FAILED",
		WorkflowURL: "https://github.com/LeeSssong/sub2api/actions/runs/2",
	})
	text := message.RenderedText()
	if message.Card.Header.Title.Content != "Sub2API 0.1.167 候选准备失败" {
		t.Fatalf("title=%q", message.Card.Header.Title.Content)
	}
	if !strings.Contains(text, "TEST_GATE_FAILED") || strings.Contains(text, "secret-value") {
		t.Fatalf("text=%s", text)
	}
}

func TestRenderReleasePreparationFiltersUnsafeLinks(t *testing.T) {
	message := RenderReleasePreparation(ReleasePreparationView{
		Status: "succeeded", Version: "0.1.167",
		ReleaseURL:  "javascript:alert(1)",
		WorkflowURL: "https://github.com/LeeSssong/sub2api/actions/runs/3",
	})
	if message.Card == nil {
		t.Fatal("card is nil")
	}
	var links []string
	for _, element := range message.Card.Elements {
		for _, action := range element.Actions {
			if action.MultiURL != nil {
				links = append(links, action.MultiURL.URL)
			}
		}
	}
	if len(links) != 1 || links[0] != "https://github.com/LeeSssong/sub2api/actions/runs/3" {
		t.Fatalf("links=%v", links)
	}
}

func TestRenderReleasePreparationRejectsUnstableErrorCode(t *testing.T) {
	message := RenderReleasePreparation(ReleasePreparationView{
		Status: "failed", Version: "0.1.167", ErrorCode: "failure: Bearer secret-value",
	})
	text := message.RenderedText()
	if !strings.Contains(text, "RELEASE_PREPARATION_FAILED") || strings.Contains(text, "secret-value") {
		t.Fatalf("text=%s", text)
	}
}

func TestRenderReleasePreparationBoundsLargeReleaseBody(t *testing.T) {
	message := RenderReleasePreparation(ReleasePreparationView{
		Status: "succeeded", Version: "0.1.167", ReleaseBody: strings.Repeat("修复内容", 10_000),
	})
	card, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(card) >= 30<<10 {
		t.Fatalf("card bytes=%d", len(card))
	}
	if !strings.Contains(message.RenderedText(), "...") {
		t.Fatalf("release body was not visibly truncated")
	}
	if !utf8.ValidString(message.RenderedText()) {
		t.Fatal("rendered text is not valid UTF-8")
	}
}
