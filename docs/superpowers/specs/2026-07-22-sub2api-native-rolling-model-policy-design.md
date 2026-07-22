# Sub2API Native Rolling Model Policy Design

**Date:** 2026-07-22 (Asia/Shanghai)
**Status:** Approved

## Goal

Keep the public model catalog small and current without hard-coding individual GPT model IDs into application logic. Reuse Sub2API `v0.1.161` as the source of live upstream model discovery and model-aware account scheduling, then add a lightweight, evidence-gated rolling-release policy that never publishes a newly discovered model automatically.

The initial controlled-test catalog covers the approved official GPT-5.5 and GPT-5.6 text models. Future GPT minor releases enter a candidate state first. `/ops` displays `可升级` only after compatibility, coverage, pricing, balance, and fresh quality evidence pass; Codex then reviews the evidence and performs a controlled Sub2API-native replacement.

## Product Decisions

- Sub2API remains the authority for upstream accounts, credentials, group membership, scheduling, model mappings, and model pricing.
- The current upstream account set remains all undeleted Sub2API accounts where `status == "active" && schedulable == true`.
- Live upstream model discovery uses Sub2API's native `POST /api/v1/admin/accounts/:id/models/sync-upstream` endpoint. Base URLs and Keys are not copied into relay-ops or entered again by the operator.
- Discovery is read-only. A model returned by an upstream is only `discovered`; it is not qualified, priced, mapped, or public.
- The public policy retains at most two approved GPT minor families. A newer minor family becomes a candidate; the oldest published family is not removed until the candidate passes and a controlled replacement succeeds.
- Public groups use the same approved customer-facing catalog. Multiple active accounts in one group may collectively cover that catalog, and each account is eligible only for the models in its explicit Sub2API account mapping.
- `/ops` remains a hidden, read-only administrator projection. It gains no model editor, Base URL/Key input, mutation button, probe button, or release button.
- No provider name, hostname, account name, fixed account ID, or individual long-lived model ID controls discovery, account membership, or gate behavior.
- D04 remains closed until the separate lightweight launch gate passes. This model policy does not open registration or authorize route, multiplier, balance, Key, or group-membership changes.

## Reuse Audit

### Direct reuse

- Sub2API native active/schedulable accounts and account-to-group relationships.
- Sub2API native upstream model sync, including account headers, proxy, TLS profile, Base URL normalization, and credential handling.
- Sub2API account `model_mapping` as a model whitelist. In `v0.1.161`, scheduling calls `Account.IsModelSupported` and filters OpenAI-compatible accounts that do not map the requested model.
- Sub2API channel `restrict_models`, channel model mapping, and channel model pricing for the customer-facing catalog.
- Existing upstream benchmark protocol adapters, bounded sync/SSE request execution, content-free evidence ledger, and cleanup discipline.
- Existing D04 lightweight balance and fresh quality thresholds.
- Existing `/ops` hidden-admin authentication and read-only projection.

### Adapted reuse

- The model-directory phase accepts a policy-selected candidate set instead of treating every discovered text model as a required test.
- `catalog_quick` tests only the candidate models for the current decision, not the upstream's entire historical or niche catalog.
- The readiness evidence adds published families, discovered candidate families, per-account qualified model subsets, per-group coverage, pricing completeness, and upgrade state.
- `/ops` renders rolling-model evidence alongside the existing controlled-test readiness status.

### New only where absent

- A provider-neutral rolling GPT family policy.
- A secret-free upgrade proposal with optimistic concurrency hashes.
- A controlled promoter that updates only Sub2API-native model mappings and pricing, then re-reads or restores the previous snapshot.

## Model Lifecycle

Each model has one of the following states:

1. `discovered`: returned by a current account's native upstream model sync.
2. `candidate`: belongs to the proposed next GPT minor family and is not excluded as a special-purpose or versioned alias.
3. `qualified`: passed the bounded compatibility checks on a specific account.
4. `covered`: at least one qualified current account in each required public group can serve it.
5. `ready`: covered, priced, and accompanied by fresh account-level balance and quality evidence.
6. `published`: present in the verified Sub2API account mappings and restricted public channel catalog.
7. `retired`: removed only after a newer family is successfully published and the post-write re-read passes.

State progression is monotonic within one evidence snapshot. A later discovery or test failure creates a new blocked snapshot; it does not silently rewrite old evidence.

## Family Selection

The policy derives GPT families from normalized model IDs rather than maintaining a permanent list of exact model names.

- A family is identified by its GPT major and minor version, such as `5.6` or `5.7`.
- The published window contains the latest two approved minor families.
- The current bootstrap decision is the already approved GPT-5.5 and GPT-5.6 official text catalog: `gpt-5.5`, `gpt-5.6`, `gpt-5.6-luna`, `gpt-5.6-sol`, and `gpt-5.6-terra`.
- Future model IDs are derived from native sync results. IDs with dated-version suffixes or markers for compact, Codex, image, audio, realtime, preview, or other clearly special-purpose variants are excluded by policy.
- An unfamiliar suffix is displayed as `待确认`; it does not become a candidate or public model until Codex classifies it as an official customer-facing text model.
- Discovery of a newer family never removes an older family. Retirement occurs only as part of a successful controlled promotion.

The bootstrap catalog is an approved initial state, not a hard-coded rule for future versions.

## Discovery And Qualification Flow

1. Read the current active/schedulable accounts and group membership from Sub2API.
2. Call the native model-sync endpoint for each current account without changing its stored mapping.
3. Canonicalize and hash the account set and discovery results. Reject future timestamps, duplicates, malformed IDs, and stale snapshots.
4. Apply the rolling family policy and produce the candidate set. Unknown specializations remain review-only.
5. Run three bounded synchronous and three bounded SSE requests for each candidate model on each account that discovered it. Requests use minimal output, store no model output, and keep credentials only in process memory.
6. Qualify a model/account pair only when all six requests succeed and every SSE stream reaches its expected terminal event.
7. Build per-group coverage from current account group membership and qualified account model subsets.
8. Require complete coverage of every candidate model in each customer-facing public group.
9. Require complete public pricing and the existing lightweight account gates before reporting `ready`.
10. Write a secret-free proposal and expose it read-only through `/ops`.

An account does not need to support the complete public catalog. Its explicit model mapping contains only its qualified subset, allowing Sub2API's native model-aware scheduler to choose a compatible account. A group cannot publish a model unless its current accounts collectively cover it.

## Readiness Gates

`/ops` reports `可升级` only when all of the following are true for the same canonical account set and proposal:

- Discovery evidence is no more than 20 minutes old.
- Every candidate model has complete sync and SSE coverage in every public group.
- Every participating current account has known balance of at least USD 5.00 with evidence no more than 20 minutes old.
- Account-attributed quality evidence is no more than 20 minutes old and contains at least 20 samples in the 15-minute window.
- Success rate is at least 95%, error rate is at most 5%, TTFT P95 is at most 5 seconds, and total latency P95 is at most 45 seconds.
- Every candidate public model has complete Sub2API pricing; unknown price or billing semantics fail closed.
- The current Sub2API model configuration hash matches the proposal's base hash.
- Relay-ops remains `read_only`, Feishu commands remain `dry_run`, and D04 remains closed during discovery and proposal generation.

A compatibility probe proves that a candidate model can be called. It does not replace the account-level fresh quality gate.

## `/ops` Projection

The hidden-admin page adds one unframed read-only section named `模型版本` containing:

- Current published GPT families and exact public models.
- Last native-sync time and canonical account-set hash.
- Candidate family and candidate models.
- Per-group coverage and per-account qualified subsets.
- Pricing, balance, and quality evidence state.
- One status: `未发现更新`, `待确认`, `待测试`, `测试未通过`, `可升级`, or `已发布`.
- Plain-language blockers with stable reason codes in technical details.

The section contains no form control or command button. Missing, invalid, inactive, and non-admin sessions continue to receive HTTP 404 for operational data.

## Controlled Promotion

Promotion is a separate, explicit operation. It cannot run from the `/ops` browser page or a scheduler.

1. Re-read modes, current accounts, groups, account mappings, channel restrictions, channel mappings, and pricing.
2. Require exact matches for the proposal ID, account-set hash, base configuration hash, candidate set, and `ready` result.
3. Save a server-local, permission-restricted snapshot of the affected Sub2API account and channel configuration. No encrypted offsite backup is required.
4. Through Sub2API's native Admin API, update only the affected account model mappings and the restricted public channel catalog/pricing.
5. Re-read all affected objects and compare them with the proposal.
6. If any write or verification is partial, restore the pre-promotion snapshot through the native Admin API and report failure.
7. After successful verification, mark the new family published and the displaced oldest family retired in evidence.

Promotion does not change account status, schedulable state, account groups, route roles, multipliers, balances, credentials, D04 mode, registration, probe mode, or Feishu command mode.

## Failure Handling

- Native sync failure: keep the published catalog unchanged, mark discovery unavailable, and do not test or promote.
- Empty or malformed model directory: fail closed and keep the published catalog.
- Unknown suffix: show `待确认`; do not expose or test it as a required public model.
- Partial compatibility: keep successful account/model evidence, but block any group lacking complete candidate coverage.
- Stale or mismatched account set: invalidate the proposal and regenerate from current Sub2API state.
- Missing price, balance, or quality evidence: report the exact blocker and keep `可升级` false.
- Promotion precondition mismatch: perform zero writes and require a fresh proposal.
- Partial promotion: restore the prior native configuration, re-read it, and leave the old published family active.
- Post-promotion quality regression: generate a rollback recommendation. No background component silently changes production routing or model mappings.

## User Assistance Boundary

The operator does not re-enter Base URLs, Keys, or model IDs and does not manually maintain a second model catalog.

The operator only needs to:

- Keep intended Sub2API accounts `active + schedulable`.
- Maintain at least USD 5.00 provider balance where automated financial evidence is available.
- Provide balance evidence only when an upstream exposes no trustworthy machine-readable balance.
- Explicitly approve opening D04 registration; model readiness alone never opens controlled testing.

When `/ops` reports `可升级`, Codex reviews and executes the controlled promotion in an active task. There is no unattended background production mutation.

## Validation

Automated validation covers:

- Family parsing, rolling two-family selection, exclusion markers, and unknown-suffix review state.
- Native sync response normalization, deduplication, freshness, and zero-write behavior.
- Candidate-only benchmark scope and proof that unrelated discovered models are never requested.
- Three sync and three SSE attempts per candidate/account pair, including terminal-event enforcement.
- Per-account subsets, per-group union coverage, missing-group coverage, and account-set hash mismatch.
- Balance, quality, pricing, and mode gates using the existing threshold boundaries.
- `/ops` states, Chinese summaries, administrator 404 behavior, auto-refresh, and absence of write controls.
- Promotion preflight, optimistic concurrency mismatch, exact native Admin API write scope, post-write re-read, partial failure, and rollback.
- Secret and model-output scans, full tests, race tests, static checks, deployment contracts, and diff checks.

Production acceptance proceeds in two phases:

1. Read-only discovery, candidate qualification, and `/ops` evidence verification with proof that Sub2API mappings, pricing, routes, balances, Keys, modes, registration, and container identities did not change.
2. A separately evidenced controlled promotion only after `可升级`, followed by native object re-read, public model-list verification, minimal gateway sync/SSE checks, mode verification, and zero unrelated writes.

## Acceptance Criteria

- The initial required public catalog is exactly the approved GPT-5.5/GPT-5.6 official text set; unrelated models no longer affect readiness.
- A future GPT minor family discovered through Sub2API appears as a candidate without becoming public.
- `/ops` shows `可升级` only when every candidate model is covered in every public group and all compatibility, pricing, balance, quality, freshness, and mode gates pass.
- Each account is scheduled only for its qualified mapped subset, using Sub2API's native model-aware filtering.
- Discovery, testing, or proposal generation performs no production configuration write.
- Promotion is impossible from `/ops`, impossible with stale or mismatched evidence, and limited to native model mappings and pricing.
- Failed or partial promotion restores the previous model configuration and never changes route, multiplier, balance, Key, group membership, account scheduling, D04 registration, probe mode, or Feishu mode.
- Current production remains `relay-ops=read_only`, `Feishu=dry_run`, and `D04=read_only/registration=false` until separate launch approval.
