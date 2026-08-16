# OAuth Images Edit MIME Compatibility — Task 1 Implementation Report

## Identity

- Date: 2026-08-16
- Baseline: `44095897d1bba3302c877431ba9bb5b6e356ab46`
- Branch: `codex/oauth-images-mime-compat`
- Implementation commit: `4adfe246b4087b15c41a21b3d2ec7afb60c5f3be`
- Implementation tree: `9b2edca81b0c8e738777384ccef8ce8a09d9e8de`

## TDD evidence

### RED

Command:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIResponsesImageUploadToDataURL|TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME' -count=1
```

Result: expected failure, exit 1. The service test package failed to compile because `openAIResponsesImageUploadToDataURL` was undefined. This demonstrated that the focused test required the new OAuth Responses-only contract before production code was added.

### GREEN

The same focused command passed after the minimal implementation:

```text
ok  github.com/Wei-Shaw/sub2api/internal/service  1.072s
```

After correct repository-root formatting, the combined focused and regression slice also passed:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIResponsesImageUploadToDataURL|TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME|TestOpenAIGatewayServiceForwardImages_(OAuthEditsMultipartUsesResponsesAPI|APIKeyEditUsesConfiguredV1BaseURL)|TestOpenAIGatewayServiceParseOpenAIImagesRequest_MultipartEdit' -count=1
```

```text
ok  github.com/Wei-Shaw/sub2api/internal/service  1.884s
```

## Changed files and behavior

- `upstream/sub2api/backend/internal/service/openai_images_responses.go`
  - Adds the unexported OAuth Responses-only `openAIResponsesImageUploadToDataURL` helper.
  - Normalizes media types for comparison with `mime.ParseMediaType`.
  - Sniffs bytes only when MIME is empty or normalized `application/octet-stream`.
  - Accepts only normalized `image/*` MIME values.
  - Preserves an explicitly supplied image MIME string in the emitted Data URL.
  - Routes only Responses input uploads and mask uploads through the strict helper.
- `upstream/sub2api/backend/internal/service/openai_images_responses_mime_test.go`
  - Covers empty, plain octet-stream, parameterized/case-varied octet-stream, rejected text fallbacks, and preserved explicit image MIME.
  - Covers rejection through both request-builder upload call sites and verifies no body is returned.
- `docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-implementation.md`
  - Records implementation and validation evidence.
- `.superpowers/sdd/2026-08-16-oauth-images-edit-mime-compat/task-1-report.md`
  - Provides the task-controller handoff copy of this report.

| Input MIME | Bytes | OAuth Responses result |
| --- | --- | --- |
| empty | PNG | `data:image/png;base64,...` |
| `application/octet-stream` | PNG | `data:image/png;base64,...` |
| case/parameter-varied octet-stream | PNG | `data:image/png;base64,...` |
| empty or octet-stream | plain text | error; no request body |
| explicit `image/*` | arbitrary non-empty bytes | original trimmed MIME preserved |

## Preserved contracts and scope

- The shared `openAIImageUploadToDataURL` implementation was not edited.
- Grok callers and `grok_media.go` were not edited.
- API-key and multipart routing behavior was not edited; their focused regressions passed.
- Error mapping, client/upstream response contracts, configuration, dependencies, migrations, frontend, operations scripts, GitHub Actions, production state, account grouping, and remote state were untouched.

## Validation evidence

- Focused helper/request-builder tests: PASS.
- OAuth/API-key/multipart regression slice: PASS.
- Combined focused/regression command: PASS.
- Compile-only service package check, `go test ./internal/service -run '^$'`: PASS (`[no tests to run]`).
- Repository-root `gofmt`: completed for the two Go files.
- `git diff --check`: clean before the implementation commit.
- Forbidden-path diff from baseline: empty before the implementation commit.
- Final scope, forbidden-path, email-content, exact-diff, and clean-worktree checks are rerun after this report is committed.

## Release properties

- Production deployment and production OAuth/API-key `/images/edits` verification were not performed in this task worktree.
- Migration change: none.
- Configuration change: none.
- Dependency change: none.
- Expected `downtime_required=false`; this is a service-code-only blue-green candidate with no schema or configuration transition.
- Rollback: revert the implementation commit and the evidence commit before release, or revert their eventual merge commit from the reviewed root release flow.
- Residual risk: `http.DetectContentType` inspects only the initial bytes and supports a bounded signature set; valid image formats it identifies as a non-image MIME will now be rejected on the OAuth Responses fallback path. Explicit `image/*` MIME values remain trusted and preserved by design.
