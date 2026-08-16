# OAuth 图片编辑上传 MIME 兼容热修实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. The implementation task must use superpowers:test-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 仅在 OAuth Images Responses 请求构造路径中，把空 MIME 或规范化后的 `application/octet-stream` 图片上传按文件字节识别为真实 `image/*`，并拒绝无法识别或非图片内容。

**Architecture:** 保留现有共享 `openAIImageUploadToDataURL`，避免改变 Grok 等其他链路；在 `openai_images_responses.go` 内新增 OAuth Responses 私有 helper，并只让 `buildOpenAIImagesResponsesRequest` 的 image/mask 上传调用它。标准库 `mime.ParseMediaType` 仅用于比较基础 MIME，显式 `image/*` 的原字符串继续写入 Data URL。

**Tech Stack:** Go、`mime`、`net/http`、`encoding/base64`、`testing`、`testify/require`、`gjson`。

## Global Constraints

- 基线固定为 `main@44095897d1bba3302c877431ba9bb5b6e356ab46`；不得从旧 `origin/main` 或其他候选派生。
- 只修改 `upstream/sub2api/backend/internal/service/openai_images_responses.go`、最小相关 service 单测和本任务规格/计划/复审报告。
- 不修改共享 helper 的 Grok 行为、multipart 解析、API-key 链路、其他上传链路、错误码、错误文案/中文提示、`ErrorPassthroughRule` 或客户端表现。
- 不修改依赖、配置、迁移、前端、GitHub Actions、发布脚本、生产数据或账号分组策略。
- 仓库文档不得写个人电子邮件地址；必要事故上下文仅保留 `user_id=34`、API key id `50`、`group_id=19`。
- 仅运行相关 service 单测、后端必要构建/类型检查、`gofmt`、`git diff --check` 和范围检查；不运行无关全仓测试、mutation、压力、soak 或浏览器验证。
- 候选完成后只进入 `READY_FOR_ROOT_REVIEW`；不得合并根 `main`、推送或部署。

---

### Task 1: OAuth Responses 图片上传 MIME 规范化与拒绝门禁

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_images_responses.go:320-410`
- Create: `upstream/sub2api/backend/internal/service/openai_images_responses_mime_test.go`
- Create: `docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-implementation.md`

**Interfaces:**
- Consumes: `OpenAIImagesUpload{FileName string, ContentType string, Data []byte}` and existing `buildOpenAIImagesResponsesRequest(parsed *OpenAIImagesRequest, toolModel string) ([]byte, error)`.
- Produces: unexported `openAIResponsesImageUploadToDataURL(upload OpenAIImagesUpload) (string, error)` used only by OAuth Responses request construction.
- Preserves: existing `openAIImageUploadToDataURL(upload OpenAIImagesUpload) (string, error)` and its Grok callers unchanged.

- [ ] **Step 1: Add a focused failing table-driven helper test**

Create `openai_images_responses_mime_test.go` with a real 1×1 PNG byte fixture decoded from base64 and cases equivalent to:

```go
func TestOpenAIResponsesImageUploadToDataURLNormalizesFallbackMIME(t *testing.T) {
    pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
    require.NoError(t, err)

    tests := []struct {
        name        string
        contentType string
        data        []byte
        wantPrefix  string
        wantErr     bool
    }{
        {name: "empty MIME detects PNG", data: pngBytes, wantPrefix: "data:image/png;base64,"},
        {name: "octet stream detects PNG", contentType: "application/octet-stream", data: pngBytes, wantPrefix: "data:image/png;base64,"},
        {name: "normalized octet stream detects PNG", contentType: " Application/Octet-Stream; charset=binary ", data: pngBytes, wantPrefix: "data:image/png;base64,"},
        {name: "empty MIME rejects text", data: []byte("plain text"), wantErr: true},
        {name: "octet stream rejects text", contentType: "application/octet-stream", data: []byte("plain text"), wantErr: true},
        {name: "explicit image MIME is preserved", contentType: " IMAGE/PNG; x-existing=1 ", data: []byte("not-sniffable"), wantPrefix: "data:IMAGE/PNG; x-existing=1;base64,"},
    }
    // Assert error/no error and exact prefix per row.
}
```

Also add a focused request-builder test proving both `parsed.Uploads` and `parsed.MaskUpload` use the strict helper and a non-image fallback MIME returns an error before an upstream request body can be produced.

- [ ] **Step 2: Run the focused tests and record RED evidence**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIResponsesImageUploadToDataURL|TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME' -count=1
```

Expected: FAIL because `openAIResponsesImageUploadToDataURL` does not exist and the request builder still accepts `application/octet-stream`.

- [ ] **Step 3: Implement the minimal OAuth Responses-only helper**

In `openai_images_responses.go`, add the standard-library `mime` import and implement this behavior without editing the existing shared helper:

```go
func openAIResponsesImageUploadToDataURL(upload OpenAIImagesUpload) (string, error) {
    if len(upload.Data) == 0 {
        return "", fmt.Errorf("upload %q is empty", strings.TrimSpace(upload.FileName))
    }

    contentType := strings.TrimSpace(upload.ContentType)
    normalizedType := contentType
    if parsedType, _, err := mime.ParseMediaType(contentType); err == nil {
        normalizedType = parsedType
    }

    if contentType == "" || strings.EqualFold(normalizedType, "application/octet-stream") {
        contentType = http.DetectContentType(upload.Data)
        normalizedType = contentType
        if parsedType, _, err := mime.ParseMediaType(contentType); err == nil {
            normalizedType = parsedType
        }
    }

    if !strings.HasPrefix(strings.ToLower(normalizedType), "image/") {
        return "", fmt.Errorf("upload %q is not an image", strings.TrimSpace(upload.FileName))
    }
    return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data), nil
}
```

Change only the two OAuth Responses request-builder call sites—input images and mask upload—to call `openAIResponsesImageUploadToDataURL`. Do not change `grok_media.go` or the shared helper.

- [ ] **Step 4: Run the focused tests and record GREEN evidence**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIResponsesImageUploadToDataURL|TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME' -count=1
```

Expected: PASS with zero failures.

- [ ] **Step 5: Run the existing OAuth/API-key Images regression slice**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIGatewayServiceForwardImages_(OAuthEditsMultipartUsesResponsesAPI|APIKeyEditUsesConfiguredV1BaseURL)|TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEdit' -count=1
```

Expected: PASS, proving the OAuth edit path still constructs Responses requests and the API-key/multipart paths remain unchanged.

- [ ] **Step 6: Run formatting, necessary build/type checks, and scope guards**

Run:

```bash
gofmt -w upstream/sub2api/backend/internal/service/openai_images_responses.go upstream/sub2api/backend/internal/service/openai_images_responses_mime_test.go
cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIResponsesImageUploadToDataURL|TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME|TestOpenAIGatewayServiceForwardImages_(OAuthEditsMultipartUsesResponsesAPI|APIKeyEditUsesConfiguredV1BaseURL)|TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEdit' -count=1
cd upstream/sub2api/backend && go test ./internal/service -run '^$'
git diff --check
changed_paths=$(git diff --name-only 44095897d1bba3302c877431ba9bb5b6e356ab46...HEAD)
unexpected_paths=$(printf '%s\n' "$changed_paths" | rg -v '^(upstream/sub2api/backend/internal/service/openai_images_responses\.go|upstream/sub2api/backend/internal/service/openai_images_responses_mime_test\.go|docs/superpowers/specs/2026-08-16-oauth-images-edit-mime-compat-design\.md|docs/superpowers/plans/2026-08-16-oauth-images-edit-mime-compat\.md|docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-(implementation|task-review|final-review)\.md)$' || true)
test -z "$unexpected_paths"
forbidden_paths=$(git diff --name-only 44095897d1bba3302c877431ba9bb5b6e356ab46 -- .github upstream/sub2api/backend/go.mod upstream/sub2api/backend/go.sum upstream/sub2api/frontend upstream/sub2api/backend/migrations ops)
test -z "$forbidden_paths"
email_scan_pattern='user_''email|用户''邮箱|@''[A-Za-z0-9.-]+\.[A-Za-z]{2,}'
if rg -n "$email_scan_pattern" docs/superpowers/specs/2026-08-16-oauth-images-edit-mime-compat-design.md docs/superpowers/plans/2026-08-16-oauth-images-edit-mime-compat.md docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-*.md; then exit 1; fi
```

Expected: focused tests and compile-only package check PASS; diff check clean; changed paths limited to the declared service source/test and task-local docs; forbidden-path diff empty; email scan empty.

- [ ] **Step 7: Write implementation evidence and self-review the exact diff**

Create the implementation report with:

- baseline, branch, implementation commit/tree;
- RED and GREEN commands/results;
- exact changed files and behavior matrix;
- confirmation that shared helper/Grok/API-key/error mapping/config/migrations/production were untouched;
- validation commands/results, unverified production items, `downtime_required=false` expectation, rollback and residual risk.

Then inspect:

```bash
git diff 44095897d1bba3302c877431ba9bb5b6e356ab46 -- upstream/sub2api/backend/internal/service/openai_images_responses.go upstream/sub2api/backend/internal/service/openai_images_responses_mime_test.go docs/superpowers/specs/2026-08-16-oauth-images-edit-mime-compat-design.md docs/superpowers/plans/2026-08-16-oauth-images-edit-mime-compat.md docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-implementation.md
```

- [ ] **Step 8: Commit the implementation task**

```bash
git add upstream/sub2api/backend/internal/service/openai_images_responses.go upstream/sub2api/backend/internal/service/openai_images_responses_mime_test.go docs/superpowers/plans/2026-08-16-oauth-images-edit-mime-compat.md docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-implementation.md
git commit -m "fix: normalize OAuth image upload MIME"
```

After the task commit, the controller must dispatch a fresh read-only task reviewer for specification compliance and code quality, then a separate most-capable read-only whole-branch reviewer. Findings must be fixed and re-reviewed before `READY_FOR_ROOT_REVIEW`.
