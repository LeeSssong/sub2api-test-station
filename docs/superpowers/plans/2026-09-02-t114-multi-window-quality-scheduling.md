# T114 Multi-Window Quality Scheduling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace T96 seven-day lexicographic OpenAI text ordering with a confidence-smoothed 1h/24h/7d quality score and add a 60-second observation-only slow-first-output signal.

**Architecture:** Keep `usage_logs` and `ops_error_logs` as the only persisted facts. One cached seven-day scan emits mutually exclusive window metrics; pure service functions blend them with candidate-pool neutral baselines, then the existing selector adds native load and acquires the existing slot in score order. A bounded process-local tracker overlays W1 TTFT evidence and emits additive Ops events without owning cancellation, replay, billing, or account state.

**Tech Stack:** Go 1.27, PostgreSQL conditional aggregation, Redis-backed native concurrency, Gin, Testify, sqlmock.

**Spec:** `docs/superpowers/specs/2026-09-02-t114-multi-window-quality-scheduling-design.md`

## Global Constraints

- Only explicitly opted-in ordinary OpenAI-compatible HTTP text requests change; images, Responses WebSocket, alpha search, forced protocol bindings, and non-OpenAI paths stay unchanged.
- Preserve native eligibility, profit partition, slot/wait behavior, retry budget, safe replay, billing, usage completeness, account state, and cooldown semantics.
- Windows are `W1=[now-1h,now)`, `W24=[now-24h,now-1h)`, `W7=[now-7d,now-24h)` with weights `50/30/20` and confidence targets `20/100/300`.
- Score weights are success `40`, P50 TTFT `24`, P90 TTFT `16`, output rate `10`, live load `10`; no TTFT value is an eligibility veto.
- A slow signal records only `TTFT >= 60000ms` in memory for later requests. It never cancels, closes, replays, switches, mutates billing, or creates a second attempt.
- No migration, backfill, new fact table, production write, GitHub Actions workflow, push, or deployment.

---

### Task 1: Multi-Window Repository Projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_quality.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_quality.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_log_quality_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`

**Produces:** `OpenAIAccountQuality.Windows map[OpenAIQualityWindow]OpenAIQualityWindowMetrics` with attempt/success counts, P50/P90 TTFT, and robust output rate.

- [ ] Write literal sqlmock RED tests for three exclusive windows, boundary timestamps, physical-attempt deduplication, P50/P90, and `output_tokens / (duration_ms-first_token_ms)` eligibility.
- [ ] Run `go test ./internal/repository -run 'TestUsageLogRepositoryListOpenAIAccountQuality|TestOpenAIAccountQualityQuery' -count=1` and confirm the old one-row trimmed-mean contract fails.
- [ ] Change the single query to classify each attempt with one `CASE` window and aggregate by account/window; keep all existing failure exclusions.
- [ ] Add a provider test proving returned window maps are deep clones, then implement cloning.
- [ ] Add a refresh-throttle test proving expired snapshots inside the five-minute production cooldown serve stale data without another SQL scan, then allow one scan after cooldown.
- [ ] Run focused repository/provider tests and commit `feat: aggregate OpenAI quality by scheduling windows`.

### Task 2: Pure Scoring and Slow Evidence State

**Files:**
- Create: `upstream/sub2api/backend/internal/service/openai_quality_score.go`
- Create: `upstream/sub2api/backend/internal/service/openai_quality_score_test.go`
- Create: `upstream/sub2api/backend/internal/service/openai_first_output_slow.go`
- Create: `upstream/sub2api/backend/internal/service/openai_first_output_slow_test.go`

**Produces:** `OpenAIQualityBreakdown` and process-local `OpenAIFirstOutputSlowTracker` keyed by group/account/attempt.

- [ ] Write RED table tests with hand-derived literals for every success/TTFT curve node, midpoint interpolation, clamping, nil/non-finite inputs, and no NaN.
- [ ] Write RED tests for confidence carry W1 -> W24 -> W7 -> neutral, metric-specific counts, and a `1/1` account not outranking a supported near-perfect account.
- [ ] Write RED tests for candidate P20/P80 output-rate normalization and native load scoring: slot remaining `70%`, inverse wait `30%`, missing evidence neutral `50`.
- [ ] Implement private interpolation, confidence transfer, neutral-baseline, output-rate, live-load, and final-score functions.
- [ ] Write RED tracker tests using an injected clock/timer: one 60-second lower bound/event, early semantic output no event, real TTFT replacement, failure removal, unknown/disconnect retention, one-hour expiry.
- [ ] Implement the tracker without cancel/body/selector/billing/account-state references.
- [ ] Run `go test ./internal/service -run 'TestOpenAI(Quality|FirstOutputSlow)' -count=1` and commit `feat: score multi-window OpenAI quality`.

### Task 3: Composite Ranking and Native Load

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_service.go`
- Test: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_upstream_cost_test.go`

**Produces:** ordering by `quality_score DESC -> success_score DESC -> P50 TTFT ASC -> account_id ASC` and selected-account score details on `OpenAIAccountScheduleDecision`.

- [ ] Write RED ranking tests for `100%+18s` versus `99.65%+4.7s`, low-success fast protection, all accounts above 60 seconds still selectable, neutral missing metrics, and stable ID tie-break.
- [ ] Write RED selector tests for one native batch-load read, score-ordered slot attempts, busy fallback, unchanged all-busy wait plan, and unchanged post-slot profit recheck.
- [ ] Batch native loads, overlay slow W1 evidence on a cloned snapshot, score every candidate, keep effective U only in the existing profit partition, and sort by the approved comparator.
- [ ] Populate score/components/windows/snapshot/slow-count/score-gap/non-selection observability fields without changing request behavior.
- [ ] Run focused selector tests and commit `feat: rank OpenAI accounts by composite quality`.

### Task 4: Observation-Only Attempt Wiring

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`
- Modify: existing semantic-output call sites in `openai_gateway_response_handling.go`, `openai_gateway_passthrough.go`, `openai_gateway_chat_completions.go`, `openai_gateway_chat_completions_raw.go`, `openai_gateway_messages.go`, `gateway_forward_as_chat_completions.go`, and `openai_gateway_cc_pipeline.go` only where composition is needed.
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_embeddings.go`
- Test: `upstream/sub2api/backend/internal/service/openai_visible_ttft_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_admission_first_output_wiring_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_first_output_timeout_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go`

- [ ] Add RED semantic cases for reasoning summary, custom tool input, transcript, refusal, empty reasoning, heartbeat, preamble, and `[DONE]`.
- [ ] Add a blocked-upstream RED integration test with a short injected threshold proving upstream calls stay `1`, account/switch metrics stay unchanged, context/body remain open, and the original response later completes.
- [ ] Compose the existing semantic callback with `sync.Once`; do not change flush or replay-safety transitions.
- [ ] Start the observer immediately before forwarding an already-selected unified-quality attempt. Finalize success with real TTFT/latency; remove on known failure; retain only unknown/disconnect lower bounds. Skip images, WebSocket, non-OpenAI, and non-opted-in requests.
- [ ] Run focused service/handler tests and commit `feat: observe slow OpenAI first output without replay`.

### Task 5: Additive Ops Evidence and Candidate Verification

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience.go`
- Test: `upstream/sub2api/backend/internal/service/openai_unified_quality_observability_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/ops_scheduler_experience_handler_test.go`
- Create: `docs/handoffs/2026-09-02-t114-multi-window-quality-scheduling-handoff.md`

- [ ] Write RED projection tests for score/components, per-window samples/confidence/weights, snapshot time/staleness, slow count/replacement, rank, gap, and no sensitive payloads.
- [ ] Extend the existing bounded 4096-event ledger and scheduler-experience response additively; clone maps/slices and deduplicate slow replacement by attempt ID.
- [ ] Run focused repository, service, handler, admin-handler tests and `go build ./cmd/server`.
- [ ] Run `gofmt`, `git diff --check`, and assert no diff under `upstream/sub2api/backend/migrations` or `.github/workflows`.
- [ ] Write the handoff with base/candidate SHA, files, exact results, known unrelated failures, no migration/config changes, `downtime_required=unverified until root preflight`, rollback, shadow/acceptance follow-up, and commit `docs: hand off T114 quality scheduling candidate`.
