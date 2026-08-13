# SDD ledger — plan: docs/superpowers/plans/2026-08-12-t03-r1-account-financial-reconciliation.md

Task 1: complete
- Commits: `26da1cd72`, `48770ff65`, `c0e83454a`
- Result: frozen direct `usage_logs` persistence implementation reverted; legacy-field guard added and reviewed.

Task 2: complete
- Commits: `bdf278ce6`, `125bdf814`
- Result: expand-only evidence/review/daily-value/setting tables generated; explicit unique indexes fixed and scoped re-review approved.

Main synchronization: complete
- Root main/origin main: `19492c57da24270eb2b3e9b5d9727c2865aebb9e`
- Candidate merge commit: `ddeb78f1b99171482122b05a324519ee7d96ec3f`
- Conflict resolution authorized by user: retained T03-R1 records and adopted final official v0.1.175 production-complete ledger entry.

Task 3: in progress
- Pre-sync uncommitted implementation was preserved in stash and restored without conflict.
- Restored scope: registrar/repository services, Sub/New parser reuse, response-after usage hooks, tests, Wire providers, and project ledger.
- Initial implementation commit: `b8c5a7f8c0342a7176a5ad232170480c3af6c77b`.
- Review round 1: Spec ❌, Quality ❌. Critical: replaced official `CreateBestEffort`; Important: missing persisted `enabled_at` gate; Minor: unstable reason codes.
- Review warnings adjudication: missing handler stream/nonstream tests is a real brief gap and enters fix round 1; batch-image Vertex and OpenAI Live are structurally outside Sub/New but the exclusion must be locked/documented; raw-request maximum is bounded parser behavior, not a delayed retry, and must be covered where practical.
- Fix round 1 commit: `f41f4682c575231f72a82ec3c98fa44a7a12b661`.
- Scoped re-review round 1: original Critical/Important/Minor closed; bounded requests and Vertex/Live exclusions closed. Two open Important findings remain: tests do not traverse real handler stream/nonstream success branches, and `AccountTypeAPIKey` is not positive Sub/New ledger identity because official-provider API-key accounts share it.
- Fix round 2 implementation is present in the working tree: positive native ledger identity gate plus real `ChatCompletions`/`Responses` success-path tests. Focused service/repository/handler/server/tagged-fallback matrices pass; awaiting commit and scoped re-review round 2.
- Task 3: fix round 2/5 (2 addressed, 0 open; commit `70a6d89703bcbf664db3737aef40e8f67d9b9619`).
- Task 3: complete (commits `ddeb78f1b99171482122b05a324519ee7d96ec3f..70a6d89703bcbf664db3737aef40e8f67d9b9619`, scoped re-review round 2 clean; residual non-blocking gap: no fresh concurrent PostgreSQL `CreateOnce` integration race test).

Task 4: complete
- Initial implementation commit: `5a8d830b2de8063e2b99876e13e044b0e1930cdb` (supersedes pre-review draft `f876fe725`).
- Independent review: Spec ❌, Quality ❌. Critical: wrong activation key; review mutations admit confirmed/OAuth/pre-enable rows; mutation audit helper is disconnected. Important: Beijing today starts at 08:00; OAuth completed-day revenue/overrides wrong; exception interface loses filters/reasons; override old/new values wrong; filtered review is non-atomic with partial unreported commits.
- Task 4: minor (deferred): usage-detail review lookup suppresses database errors.
- Task 4: minor (deferred): snapshot supporting-table reads are unbounded and may hold a long repeatable-read transaction.
- PostgreSQL transaction/concurrency behavior remains an explicit verification requirement for the fix/re-review; SQLite evidence alone is insufficient.
- Fix round 2 implementation addresses the five remaining review findings: confirmed-after-existing-review eligibility ordering, nil audit recorder fail-closed, filtered idempotent skip per-row audit result, selected partial-success per-row audit, and override `MutationKind` persisted in audit `Extra`. RED regressions failed before implementation; focused/full Task 4 matrices, compile, vet and diff checks pass. Report: `task-4-fix2-report.md`. Awaiting independent scoped re-review. PostgreSQL integration remains blocked by exact `panic: rootless Docker not found`; no migration/container/production state changed.
- Fix round 3 commit: `9601ab34e6a6a67cc0a1a247c87c68c3f6f0cbfa` fixed per-row audit emission when `ReviewSelected` hits a later validation error; targeted regression, vet and diff checks passed.
- Scoped re-review round 3: `task-4-rereview-r3.md` — Spec Compliance ✅, Code Quality ✅, open code findings 0. PostgreSQL remains an environment-only warning (`rootless Docker not found`).
- Task 4 complete; no merge, push, deployment or production verification.

Task 5: in progress
- Initial implementation: `1461c706f`; request-correlation fix round 1: `524e55be1384556f345e75f772c796dc5c950cee`.
- Fix round 2 is limited to shared RequestLogger normalization for handler fallbacks and real handler-to-service-to-audit tests for all five financial mutations.
- Implementation and focused verification complete locally. Commit is blocked because the session cannot create the shared Git worktree `index.lock` outside the writable root (`Operation not permitted`); awaiting a writable Git metadata session, then independent scoped re-review. No schema, migration, Task 6, main, push, deployment, or production changes.
- Fix round 2 commit: `89597a5c455e808825613964f45ff101d9381610`; fix round 3 route-aware test commit: `24e00a90486e915feefd0d0eb71acf51fe56351b`.
- Scoped re-review round 3: `task-5-rereview-r3.md` — Spec Compliance APPROVE, Code Quality APPROVE, open findings 0; five mutation handlers traverse Gin params through service and durable audit correlation.
- Task 5: complete (commits `1461c706f..24e00a904`, review clean; no merge, push, deployment or production verification).

Task 6: complete
- Implementation: `2aff29c6ef333bb17a14399e75848c1da68ee9ae`; fix round 1: `94063b3393eca9857625af1b68420bc7eb29b7f8`; fix round 2: `3d0d44630` with report `129cf29fb`.
- Scope: administrator financial home, six summary cards, unified generated_at, 60s refresh, today revenue/cost/OAuth edits, readonly non-today/profit/margin, exception navigation, and removal of external/control-plane display.
- Scoped re-review round 1 found two Important findings (missing cost/OAuth editors and UTC business date); fix round 2 restored account financial columns while retaining both fixes.
- Scoped re-review round 2: `task-6-rereview-r2.md` — Spec Compliance APPROVE, Code Quality APPROVE, open findings 0; Vitest/typecheck/build/diff-check passed.
- Task 6 complete; no merge, push, deployment or production verification.
