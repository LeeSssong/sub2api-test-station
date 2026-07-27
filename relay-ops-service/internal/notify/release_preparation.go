package notify

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	releaseBodyMaximumBytes  = 8 << 10
	releaseValueMaximumBytes = 512
	releaseCheckMaximumBytes = 256
	releaseCheckMaximumCount = 16
)

var stableReleaseErrorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)

type ReleasePreparationView struct {
	Status           string   `json:"status"`
	Stage            string   `json:"stage"`
	Version          string   `json:"version"`
	ReleaseName      string   `json:"release_name"`
	ReleaseBody      string   `json:"release_body"`
	PublishedAt      string   `json:"published_at"`
	ReleaseURL       string   `json:"release_url"`
	OfficialCommit   string   `json:"official_commit"`
	SourceCommit     string   `json:"source_commit"`
	ImageDigest      string   `json:"image_digest"`
	ProductionImage  string   `json:"production_image_id"`
	RunningVersion   string   `json:"running_version"`
	RunningImage     string   `json:"running_image_id"`
	RunningHealth    string   `json:"running_health"`
	RunningStartedAt string   `json:"running_started_at"`
	ComposeSHA256    string   `json:"compose_sha256"`
	Checks           []string `json:"checks"`
	ErrorCode        string   `json:"error_code"`
	WorkflowURL      string   `json:"workflow_url"`
}

func RenderReleasePreparation(view ReleasePreparationView) FeishuMessage {
	version := releasePreparationDefault(view.Version)
	if strings.EqualFold(strings.TrimSpace(view.Status), "succeeded") {
		sections := []string{
			"**状态** 候选镜像已静默准备",
			"**官方版本** " + version,
			"**官方名称** " + releasePreparationDefault(view.ReleaseName),
			"**发布时间** " + releasePreparationDefault(view.PublishedAt),
			"**官方更新内容** " + releasePreparationBody(view.ReleaseBody),
			"**资格校验** " + releasePreparationChecks(view.Checks),
			"**官方 commit** " + releasePreparationDefault(view.OfficialCommit),
			"**候选源码 commit** " + releasePreparationDefault(view.SourceCommit),
			"**候选镜像 digest** " + releasePreparationDefault(view.ImageDigest),
			"**生产候选镜像** " + releasePreparationDefault(view.ProductionImage),
			"**当前运行版本** " + releasePreparationDefault(view.RunningVersion),
			"**当前运行镜像** " + releasePreparationDefault(view.RunningImage),
			"**运行健康状态** " + releasePreparationDefault(view.RunningHealth),
			"**运行启动时间** " + releasePreparationDefault(view.RunningStartedAt),
			"**Compose SHA-256** " + releasePreparationDefault(view.ComposeSHA256),
			"**生产边界** 未调用更新 API；未修改 Compose；未操作数据库；未切换运行容器",
		}
		return releasePreparationMessage(
			"Sub2API "+version+" 候选镜像已静默准备",
			"green",
			sections,
			view,
		)
	}

	errorCode := strings.TrimSpace(view.ErrorCode)
	if !stableReleaseErrorCode.MatchString(errorCode) {
		errorCode = "RELEASE_PREPARATION_FAILED"
	}
	sections := []string{
		"**状态** 候选准备失败",
		"**阶段** " + releasePreparationDefault(view.Stage),
		"**官方版本** " + version,
		"**错误码** " + errorCode,
	}
	return releasePreparationMessage(
		"Sub2API "+version+" 候选准备失败",
		"red",
		sections,
		view,
	)
}

func releasePreparationMessage(title, template string, sections []string, view ReleasePreparationView) FeishuMessage {
	links := make([]Link, 0, 2)
	if trustedReleaseLink(view.ReleaseURL, "/Wei-Shaw/sub2api/releases/") {
		links = append(links, Link{Label: "官方 Release", URL: view.ReleaseURL})
	}
	if trustedReleaseLink(view.WorkflowURL, "/LeeSssong/sub2api/actions/runs/") {
		links = append(links, Link{Label: "工作流记录", URL: view.WorkflowURL})
	}
	return resolveMessageLinks(newCardMessage(title, template, strings.Join(sections, "\n\n"), links), "")
}

func trustedReleaseLink(raw, pathPrefix string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" &&
		strings.EqualFold(parsed.Hostname(), "github.com") &&
		parsed.User == nil &&
		parsed.RawQuery == "" &&
		parsed.Fragment == "" &&
		strings.HasPrefix(parsed.EscapedPath(), pathPrefix)
}

func releasePreparationBody(value string) string {
	value = safeValue(value)
	if value == "" {
		return "无"
	}
	return trimDigestText(value, releaseBodyMaximumBytes)
}

func releasePreparationChecks(checks []string) string {
	rendered := make([]string, 0, min(len(checks), releaseCheckMaximumCount))
	for _, check := range checks {
		if len(rendered) == releaseCheckMaximumCount {
			break
		}
		value := safeValue(check)
		if value == "" {
			continue
		}
		rendered = append(rendered, trimDigestText(value, releaseCheckMaximumBytes))
	}
	if len(rendered) == 0 {
		return "无"
	}
	return strings.Join(rendered, "；")
}

func releasePreparationDefault(value string) string {
	value = safeValue(value)
	if value == "" {
		return "无"
	}
	return trimDigestText(value, releaseValueMaximumBytes)
}
