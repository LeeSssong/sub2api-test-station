# GPT Group Configuration Baseline Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a secret-free, evidence-backed baseline for GPT-Pro, exclusive GPT-Pro, GPT-Plus, and GPT-特惠 account membership and scheduling priority from existing production history only.

**Architecture:** A read-only evidence task captures current production configuration and 7/30-day aggregate real-request/probe facts without request content or credentials. A separate analysis task applies the frozen per-metric source-selection rule and current group scoring formulas, then produces group placement, scheduling bands, exceptions, and a scoring-system audit. Independent reviews verify every recommendation against the captured evidence.

**Tech Stack:** OpenSSH using the registered `sub2api-prod` alias, PostgreSQL read-only aggregate SQL, Sub2API admin monitor API, Ruby or `jq` for secret-free transformations, Markdown reports, Git.

## Global Constraints

- Use existing production history only; do not generate any upstream or gateway request.
- Exclude image-only and Claude accounts; do not recommend the temporary self-test group.
- Use 7 days for the current baseline and 30 days only as drift context.
- Per metric, use real data only when its sample count is at least 20 and not lower than the corresponding probe sample count; otherwise use probe data. Record missing evidence rather than inferring it.
- Preserve current production group weights, threshold values, cost modes, and effective multipliers when recalculating scores.
- Exclusive GPT-Pro must mirror public GPT-Pro's upstream accounts and effective priorities.
- All SSH, SQL, and API actions are read-only. Do not change accounts, groups, priorities, scheduling, routes, services, containers, files, or database rows.
- Never print or persist `.env`, Admin/API Keys, cookies, passwords, TOTP data, request bodies, prompts, model output, or authorization headers.
- Preserve the root `main` dirty worktree and the protected “新建运营界面” and “优化账号卡片” threads.

---

### Task 1: Capture Secret-Free Production Evidence

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md`
- Create in the plan SDD workspace: `production-evidence.json`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Produces: a JSON object with `captured_at`, `scope`, `groups`, `accounts`, `real_7d`, `real_30d`, `probe_7d`, `probe_30d`, `latest_probe`, and `scheduler` collections, plus a Markdown evidence inventory.
- Every account row is keyed by numeric account ID. No credential, request ID, user ID, prompt, response, or raw log line may be present.

- [ ] **Step 1: Verify the SSH and production read-only boundary**

Run:

```bash
ssh -o BatchMode=yes -o ConnectTimeout=15 sub2api-prod \
  'set -eu; test "$(uname -s)" = Linux; sudo -n true; sudo docker ps --format "{{.Names}} {{.Status}}" | sed -n "1,20p"'
```

Expected: successful Linux login, passwordless `sudo -n`, and running production containers. Do not read environment files.

- [ ] **Step 2: Inspect the production schema before writing aggregate queries**

Run read-only `information_schema.columns` queries through the existing PostgreSQL container for only these tables: `accounts`, `groups`, `account_groups`, `usage_logs`, `ops_error_logs`, `account_monitor_results`, `account_monitor_group_score_weights`, and `settings`. Use the container's existing `POSTGRES_USER` and `POSTGRES_DB` environment variables inside the container; never print them.

Expected: column names/types only. If a table name differs, discover the matching table through `information_schema.tables`; do not guess or mutate schema.

- [ ] **Step 3: Capture configuration and aggregate evidence**

Query only non-sensitive fields required by the design:

```text
groups: id, name, status, platform, rate multiplier, exclusive/public state,
        score weights and TTFT/latency target/limit values
accounts: id, name, platform, type, status, schedulable, global priority,
          concurrency, rate multiplier, procurement/effective-cost inputs,
          group memberships and association priority
real windows: per account and group request/success/error counts,
              TTFT sample count/P50/P95, duration sample count/P50/P95,
              last observation, dominant models; windows 7d and 30d
probe windows: per account request/success/error counts, TTFT sample
               count/P50/P95, latency sample count/P50/P95, last observation;
               windows 7d and 30d
latest probe: status, stable error code, HTTP status, model and checked_at
scheduler: current OpenAI scheduler settings relevant to priority, load,
           queue, error rate and TTFT
```

Use `SELECT`/CTEs only. Match real errors using the repository's current `usage_logs` plus non-token-counting `ops_error_logs` logic. Aggregate raw rows on the server. Do not export request IDs or raw logs.

- [ ] **Step 4: Capture the current admin monitor projections without exposing the Admin Key**

On the production host, read the protected Admin Key only into a short-lived shell variable, call the local/current Sub2API admin monitor API for `7d` and `30d`, pipe immediately through `jq` to retain only the fields listed in Step 3, then unset the variable. The key/header must never be printed, traced, copied, or stored.

Expected: API projections corroborate database account/group counts, group weights, cost modes/effective multipliers, and current monitor statuses.

- [ ] **Step 5: Build and validate the secret-free evidence artifact**

Create `production-evidence.json` in this plan's SDD workspace and the Markdown evidence inventory using `apply_patch` for tracked documentation. Validate:

```bash
jq -e '
  has("captured_at") and has("groups") and has("accounts") and
  has("real_7d") and has("real_30d") and
  has("probe_7d") and has("probe_30d") and has("latest_probe") and
  has("scheduler")
' "$SDD_WORKSPACE/production-evidence.json"
rg -n -i 'authorization:|x-api-key|api[_ -]?key|cookie|password|totp|bearer |prompt|response_body|request_body' \
  "$SDD_WORKSPACE/production-evidence.json" \
  docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md
```

Expected: `jq` exits 0; the secret scan has no credential/content findings. Account names and stable error codes are allowed, but report any ambiguous match for manual review.

- [ ] **Step 6: Record evidence-capture status and commit**

Update the task ledger entry to say the read-only production snapshot is captured, include the UTC capture time and artifact SHA-256, and keep overall status `进行中`.

```bash
git add docs/project/project-progress.md \
  docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md
git commit -m "docs: capture GPT group baseline evidence"
```

---

### Task 2: Calculate Placement, Priority Baseline, And Scoring Audit

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md`
- Read: the Task 1 SDD `production-evidence.json`
- Read: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Read: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`

**Interfaces:**
- Consumes: the exact Task 1 evidence JSON and production group configurations.
- Produces: one row per in-scope account with current groups, evidence source/sample count per metric, selected metrics, score breakdown under each relevant group, hard-block state, recommended group, scheduling band, recommended effective priority, confidence, and rationale.

- [ ] **Step 1: Apply the frozen scope and evidence-selection rules**

Classify every production account as `in_scope`, `excluded_image`, or `excluded_claude`. For each in-scope account select success, TTFT, and total-latency evidence independently using the Global Constraints rule. Accounts with neither source for a required metric remain visible as `manual_follow_up`.

- [ ] **Step 2: Recalculate current group scores**

Reproduce the production formula exactly:

```text
cost = cost_weight * clamp((group_multiplier - effective_multiplier) / group_multiplier, 0, 1)
success = success_weight * clamp(success_rate, 0, 1)
ttft = ttft_weight * linear_score(ttft_p50, ttft_target_ms, ttft_limit_ms)
latency = latency_weight * linear_score(latency_p95, latency_target_ms, latency_limit_ms)
total = round(cost + success + ttft + latency)
```

`linear_score` is 1 at or below the target, 0 at or above the limit, and linearly interpolated between them. Preserve `nil`/unknown cost as zero cost points rather than inventing a multiplier.

- [ ] **Step 3: Establish group placement and scheduling bands**

Apply hard faults before scores. Evaluate the highest service tier each account can support using the user-approved product meanings:

```text
GPT-Pro: strongest reliability and latency evidence; price is not a ranking objective beyond cost eligibility.
GPT-Plus: strong reliability, with moderately weaker latency accepted.
GPT-特惠: cost-sensitive; instability and latency variation are accepted, but fatal auth/model/billing faults are not.
```

Use the observed cohort distribution, current group target/limit configuration, 7d/30d drift, and common-provider failure domains. Explain every placement; do not invent an SLA from historical samples. Within each group assign `primary`, `secondary`, or `fallback`, and recommend concrete priority values preserving the scheduler's lower-number-is-higher convention. Exclusive GPT-Pro must copy the public Pro roster and priorities exactly.

- [ ] **Step 4: Audit the monitor and scheduler against the baseline objective**

Evaluate at minimum:

```text
real-request versus probe evidence selection
metric-specific sample sufficiency and source visibility
same-model/mixed-model comparability
TTFT P50 and total-latency P95 tail visibility
cost-score circularity across groups
fatal/stale/capped score gates
shared-provider failure-domain visibility
global accounts.priority versus account_groups.priority
whether the page can flag current-group and priority deviations
how a new ungrouped account can be previewed against target groups
```

State `keep`, `adjust`, or `add` for each scoring-system element and distinguish required changes from optional improvements.

- [ ] **Step 5: Write and validate the baseline report**

The report must contain:

```text
executive conclusion
scope and evidence rules
account-by-account recommendation table
four group baseline rosters and scheduling bands
public/exclusive Pro parity result
manual follow-up list
current-versus-recommended changes
scoring-system audit and concrete next-state rules
limitations and capture timestamp/hash
```

Cross-check that every in-scope account appears exactly once in the recommendation or manual-follow-up sections and every excluded account appears in the exclusion count.

- [ ] **Step 6: Commit the analysis**

```bash
git add docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md
git commit -m "docs: establish GPT group configuration baseline"
```

---

### Task 3: Verify Evidence Traceability And Close The Research Ledger

**Files:**
- Modify: `docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md`
- Modify: `docs/project/project-progress.md`
- Modify: `docs/project/current-state.md`

**Interfaces:**
- Consumes: Task 1 evidence summary, Task 2 baseline report, task-review findings, and the SDD ledger.
- Produces: a reviewed research artifact with no production mutation claim and a project-ledger/current-state entry that remains truthful about deployment status.

- [ ] **Step 1: Verify report totals and traceability**

Use a small read-only checker or `jq` queries against `production-evidence.json` to prove account scope totals, recommendation uniqueness, source/sample rules, group roster totals, and Pro/exclusive-Pro parity. Record the commands and exact counts in the report.

- [ ] **Step 2: Re-run credential/content scans and inspect the diff**

```bash
rg -n -i 'authorization:|x-api-key|api[_ -]?key|cookie|password|totp|bearer |prompt|response_body|request_body' \
  docs/superpowers/reports/2026-08-09-gpt-group-baseline-production-evidence.md \
  docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md
git diff --check
git status --short
```

Expected: no credential or request-content disclosure, no whitespace errors, and only planned documentation changes.

- [ ] **Step 3: Update project state without overstating completion**

Update `project-progress.md` with final evidence paths, review status, and the fact that no production configuration was changed. Classify this as an operations/research baseline. Update `current-state.md` with a concise pointer to the new baseline and any required future monitor/scheduler changes. Do not mark any proposed code/config change as deployed or effective.

- [ ] **Step 4: Commit the verified closure**

```bash
git add docs/project/project-progress.md docs/project/current-state.md \
  docs/superpowers/reports/2026-08-09-gpt-group-configuration-baseline.md
git commit -m "docs: verify GPT group baseline analysis"
```

