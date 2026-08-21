# T48 Model Detection Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 将模型目录、主动响应声明模型和可选指纹候选分开为有界证据，对映射/替换风险给出可读结论，并在不匹配时明确展示上游返回的模型或指纹候选。

**Architecture:** 继续使用 T15 的 sidecar 合同、异步 run 和 `juice_summary` bounded JSON 存储。`native-1` 在 `/v1/models` 之外发送一次低 Token `/v1/responses` 主动探针，只保留目录摘要、顶层 `model` 和稳定 verdict；前端把已有指纹字段与新证据包一起投影为管理员可读证据。不新增数据库字段或平行检测系统。

**Tech Stack:** Go `net/http`/`httptest`, Vue 3 `<script setup>`, TypeScript, Vue I18n, Vitest + Vue Test Utils.

**Spec:** `docs/superpowers/specs/2026-08-21-t48-model-detection-evidence-design.md`

## Global Constraints

- 目录候选不得标记为单次上游响应模型。
- `active_response.returned_model` 只能来自主动响应顶层 `model`。
- `native-1` 没有行为指纹基线时必须显示“未检测”，不得用响应模型伪造指纹候选。
- 不持久化 API Key、Base URL、Authorization、完整 prompt/output/response 或无界模型目录。
- `returned_models` 最多 10 个，每个模型 ID 最多 128 字节。
- 不修改账号探测、评分、调度、计费、盈利、分组建议或生产数据。
- 无数据库迁移、无配置 schema 变更、无 GitHub Actions。
- T47-R2 生产收口前，T48 仅准备候选，不合并、推送或发布。

---

### Task 1: Sidecar 证据模型与综合判定

**Files:**
- Create: `upstream/sub2api/backend/cmd/model-detector/evidence.go`
- Create: `upstream/sub2api/backend/cmd/model-detector/evidence_test.go`
- Modify: `upstream/sub2api/backend/cmd/model-detector/main.go`

**Interfaces:**
- Consumes: `requestedModel string`, catalog observation, active-response observation, optional fingerprint observation.
- Produces: `detectionEvidence` with `EvidenceVersion`, `RequestedModel`, `Catalog`, `ActiveResponse`, `Fingerprint`, `Verdict`; `classifyEvidence(detectionEvidence) string`; `evidenceSummary(detectionEvidence) map[string]any`.

- [x] **Step 1: Write failing verdict matrix tests**

Add table tests covering:

```go
tests := []struct {
    name string
    evidence detectionEvidence
    want string
}{
    {"verified", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "match"}, ActiveResponse: activeResponseEvidence{Status: "match", ReturnedModel: "gpt-5.6-sol"}, Fingerprint: fingerprintEvidence{Status: "unavailable"}}, "verified"},
    {"mapping", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "missing"}, ActiveResponse: activeResponseEvidence{Status: "match", ReturnedModel: "gpt-5.6-sol"}}, "suspected_mapping"},
    {"replacement by response", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "match"}, ActiveResponse: activeResponseEvidence{Status: "mismatch", ReturnedModel: "gpt-5.4"}}, "suspected_replacement"},
    {"replacement by fingerprint", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "match"}, ActiveResponse: activeResponseEvidence{Status: "match", ReturnedModel: "gpt-5.6-sol"}, Fingerprint: fingerprintEvidence{Status: "mismatch", Candidate: "gpt-5.4", Similarity: 0.98}}, "suspected_replacement"},
    {"high risk conflicting evidence", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "missing"}, ActiveResponse: activeResponseEvidence{Status: "mismatch", ReturnedModel: "gpt-5.4"}, Fingerprint: fingerprintEvidence{Status: "mismatch", Candidate: "gpt-5.6-terra", Similarity: 0.97}}, "high_risk_inconsistent"},
    {"insufficient", detectionEvidence{RequestedModel: "gpt-5.6-sol", Catalog: catalogEvidence{Status: "unavailable"}, ActiveResponse: activeResponseEvidence{Status: "unavailable"}, Fingerprint: fingerprintEvidence{Status: "unavailable"}}, "insufficient"},
}
```

- [x] **Step 2: Run tests and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./cmd/model-detector -run 'TestClassifyEvidence|TestEvidenceSummary' -count=1
```

Expected: FAIL because the evidence types/classifier do not exist.

- [x] **Step 3: Implement bounded evidence types and classifier**

Implement constants for statuses/verdicts, a deterministic classifier, model ID truncation/deduplication/sorting, `returned_models` cap 10, and summary conversion. Do not include request/response bodies or credentials.

- [x] **Step 4: Run focused tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [x] **Step 5: Commit Task 1**

```bash
git add upstream/sub2api/backend/cmd/model-detector/evidence.go upstream/sub2api/backend/cmd/model-detector/evidence_test.go upstream/sub2api/backend/cmd/model-detector/main.go
git commit -m "feat: classify model detection evidence"
```

### Task 2: 主动 Responses 探针与上游返回模型提取

**Files:**
- Modify: `upstream/sub2api/backend/cmd/model-detector/main.go`
- Modify: `upstream/sub2api/backend/cmd/model-detector/main_test.go`

**Interfaces:**
- Consumes: `fetchModels(ctx, baseURL, apiKey)` and new `fetchResponseModel(ctx, baseURL, apiKey, requestModel)`.
- Produces: active request `POST <base>/v1/responses` with `{model,input,max_output_tokens,stream}` and returns only the bounded top-level response `model` plus stable observation status.

- [x] **Step 1: Write failing active-probe tests**

Add tests that assert:

```go
if r.URL.Path == "/v1/responses" {
    var body map[string]any
    _ = json.NewDecoder(r.Body).Decode(&body)
    if body["model"] != "gpt-5.6-sol" || body["input"] != "Reply with exactly OK." || body["max_output_tokens"] != float64(8) || body["stream"] != false {
        t.Fatalf("probe body = %#v", body)
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_1", "model": "gpt-5.4", "output": []any{map[string]any{"content": "must-not-persist"}}})
}
```

Cover returned model mismatch, missing top-level model, non-2xx/invalid JSON, and URL normalization for root and `/v1` bases. Assert the sidecar response contains `juice_summary.evidence_version`, `active_response.returned_model`, and no output text.

- [x] **Step 2: Run tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./cmd/model-detector -run 'TestDetector.*Evidence|TestResponsesEndpoint' -count=1
```

Expected: FAIL because the active probe and evidence summary are absent.

- [x] **Step 3: Implement the one-shot active probe**

Generalize endpoint construction without accepting userinfo or non-HTTP(S) schemes. Send one non-stream request with `max_output_tokens=8`; parse through the existing 2 MiB limit; never log or return output. Gather catalog and active evidence independently, classify them, set legacy-compatible `Status`, `JuiceStatus`, `JuiceSummary`, `DetectorVer`, and stable `ErrorCode`.

Use these mappings:

```text
verified -> status=normal, error_code=""
suspected_mapping -> status=abnormal, error_code=model_not_advertised
suspected_replacement -> status=abnormal, error_code=response_or_fingerprint_mismatch
high_risk_inconsistent -> status=abnormal, error_code=evidence_inconsistent
insufficient -> status=insufficient, error_code=evidence_insufficient
```

- [x] **Step 4: Run all model-detector tests and verify GREEN**

```bash
cd upstream/sub2api/backend
go test ./cmd/model-detector -count=1
```

Expected: PASS.

- [x] **Step 5: Commit Task 2**

```bash
git add upstream/sub2api/backend/cmd/model-detector/main.go upstream/sub2api/backend/cmd/model-detector/main_test.go
git commit -m "feat: capture upstream response model evidence"
```

### Task 3: Sub sidecar 客户端证据保留与脱敏合同

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go`

**Interfaces:**
- Consumes: existing `AccountModelDetectionResponse.JuiceSummary` and `boundedSummary`.
- Produces: regression evidence that `evidence_v1` survives bounded sanitization while `api_key`, `base_url`, `authorization`, `prompt`, `output`, `request`, and `response` keys remain removed recursively.

- [x] **Step 1: Write a failing/contract-strengthening sanitization test**

Extend the fake `/v1/detect` payload with the complete bounded evidence envelope and nested sensitive keys. Assert:

```go
active := response.JuiceSummary["active_response"].(map[string]any)
if active["returned_model"] != "gpt-5.4" { t.Fatalf(...) }
catalog := response.JuiceSummary["catalog"].(map[string]any)
if len(catalog["returned_models"].([]any)) != 2 { t.Fatalf(...) }
if _, exists := active["output"]; exists { t.Fatalf(...) }
```

- [x] **Step 2: Run the test**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestHTTPAccountModelDetectionSidecar(CatalogAndDetect|SanitizesSummaryFields)' -count=1
```

Expected: PASS if the existing sanitizer fully supports the envelope; if it fails for a real bounded-summary gap, continue to Step 3. A passing contract test is acceptable here because this task explicitly verifies reuse of an existing security primitive rather than adding production behavior.

- [x] **Step 3: Apply only the minimal sanitizer correction if required**

Modify `account_model_detection_sidecar.go` only if the new real payload exposes a sanitizer defect. Do not relax the 8 KiB total limit, 32-key map limit, depth limit, sensitive-key list, or string limit.

- [x] **Step 4: Re-run focused service tests**

Run the command from Step 2. Expected: PASS.

- [x] **Step 5: Commit Task 3**

```bash
git add upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go
git commit -m "test: preserve bounded model detection evidence"
```

### Task 4: 管理员弹窗证据投影与中英文文案

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts`

**Interfaces:**
- Consumes: `recent.juice_summary` evidence envelope plus legacy `claimed_model`, `fingerprint_candidate`, `fingerprint_similarity`, `detector_version`, `error_code`, and timestamps.
- Produces: computed safe evidence projection and user-facing rows for verdict, requested model, upstream returned model, catalog evidence, fingerprint evidence, and technical details.

- [x] **Step 1: Write failing Vue tests**

Add separate tests for:

1. `suspected_mapping`: contains `疑似模型映射`, requested Sol, returned Sol, and bounded directory summary.
2. `suspected_replacement`: contains `请求模型：gpt-5.6-sol` and `上游响应模型：gpt-5.4`.
3. missing response model: contains `上游未返回 model 字段`; directory candidate must not appear inside the returned-model row.
4. fingerprint mismatch: contains `指纹候选：gpt-5.4` and `98%`.
5. high-risk conflict: shows both response `gpt-5.4` and fingerprint `gpt-5.6-terra`.
6. legacy result: dialog opens without `evidence_version`, retains existing candidate/error/time information, and does not fabricate an upstream returned model.

- [x] **Step 2: Run focused Vitest and verify RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: FAIL because the evidence rows and localized verdicts do not exist.

- [x] **Step 3: Implement safe evidence parsing and display**

Add local TypeScript guards that accept only expected string/number/array/map shapes. Do not cast the entire summary blindly. Replace raw `Juice` rows with explicit evidence rows. Keep raw stable error code in a subdued technical-details row and retain the cautious disclaimer.

Add locale keys for:

```text
verdict.verified
verdict.suspected_mapping
verdict.suspected_replacement
verdict.high_risk_inconsistent
verdict.insufficient
requestedModel
upstreamResponseModel
upstreamModelMissing
activeResponseUnavailable
catalogEvidence
catalogMatch
catalogMissing
catalogUnavailable
catalogReturned
fingerprintEvidence
fingerprintMatch
fingerprintMismatch
fingerprintUnavailable
technicalDetails
```

- [x] **Step 4: Run focused Vitest and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [x] **Step 5: Commit Task 4**

```bash
git add upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionDialog.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts
git commit -m "feat: explain model detection mismatch evidence"
```

### Task 5: 候选收口、验证与交接

**Files:**
- Create: `docs/handoffs/2026-08-21-t48-model-detection-evidence-handoff.md`
- Modify: `docs/superpowers/specs/2026-08-21-t48-model-detection-evidence-design.md` only for final approval/test evidence corrections if needed.
- Modify: `docs/superpowers/plans/2026-08-21-t48-model-detection-evidence.md` checkbox statuses.

**Interfaces:**
- Consumes: Tasks 1-4 implementation.
- Produces: clean candidate commit and handoff at `READY_FOR_ROOT_REVIEW`; no root merge, push, deployment, or production mutation.

- [x] **Step 1: Run backend focused verification**

```bash
cd upstream/sub2api/backend
gofmt -w cmd/model-detector/*.go internal/service/account_model_detection_sidecar*.go
go test ./cmd/model-detector -count=1
go test ./internal/service -run 'TestHTTPAccountModelDetectionSidecar' -count=1
go test ./internal/repository -run 'TestAccountModelDetection' -count=1
```

- [x] **Step 2: Run frontend focused verification**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm typecheck
pnpm build
```

- [x] **Step 3: Run scope/security/diff checks**

```bash
git diff --check
git diff --name-only 525b35d3aa2ec567cef5570a16bdfe1dd6803411..HEAD
git diff 525b35d3aa2ec567cef5570a16bdfe1dd6803411..HEAD -- .github/workflows upstream/sub2api/backend/migrations
rg -n 'api_key|authorization|base_url|prompt|output|request|response' upstream/sub2api/backend/cmd/model-detector upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go
```

Review each match and confirm it is request handling, redaction, or a negative test rather than persisted/logged secret material.

- [x] **Step 4: Write handoff**

Record baseline SHA, candidate SHA, changed files, RED/GREEN commands, final tests, no migration/config/schema changes, expected `downtime_required=false`, rollback, known limitation that `native-1` does not fabricate behavior-fingerprint candidates, and the T47-R2 release-lane dependency.

- [x] **Step 5: Commit final docs and report READY**

```bash
git add docs/superpowers/specs/2026-08-21-t48-model-detection-evidence-design.md docs/superpowers/plans/2026-08-21-t48-model-detection-evidence.md docs/handoffs/2026-08-21-t48-model-detection-evidence-handoff.md
git commit -m "docs: hand off T48 model detection evidence"
git status --short --branch
```

Expected: clean worktree on `codex/t48-model-detection-evidence`; status may advance only to `READY_FOR_ROOT_REVIEW` after direct tests pass. Do not merge to root `main`, push, deploy, or modify production.

