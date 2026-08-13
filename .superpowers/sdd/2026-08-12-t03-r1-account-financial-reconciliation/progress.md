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

Task 4: in progress
- Initial implementation commit: `5a8d830b2de8063e2b99876e13e044b0e1930cdb` (supersedes pre-review draft `f876fe725`).
- Independent review: Spec ❌, Quality ❌. Critical: wrong activation key; review mutations admit confirmed/OAuth/pre-enable rows; mutation audit helper is disconnected. Important: Beijing today starts at 08:00; OAuth completed-day revenue/overrides wrong; exception interface loses filters/reasons; override old/new values wrong; filtered review is non-atomic with partial unreported commits.
- Task 4: minor (deferred): usage-detail review lookup suppresses database errors.
- Task 4: minor (deferred): snapshot supporting-table reads are unbounded and may hold a long repeatable-read transaction.
- PostgreSQL transaction/concurrency behavior remains an explicit verification requirement for the fix/re-review; SQLite evidence alone is insufficient.
- Fix round 1 takeover commit: `962e468c7858bb753fb1a47ebdbeae45891211fa`. Closed all 3 Critical + 5 Important findings with canonical activation, eligibility rechecks, mutation audit wiring and truthful old/new, Beijing 00:00, OAuth day aggregation, exception filters/reasons, override result semantics, and atomic filtered review. Original GREEN, focused regressions, compile, vet and diff checks pass. Fresh PostgreSQL integration was attempted with `SUB2API_TEST_POSTGRES_TMPFS=1 SUB2API_TEST_POSTGRES_IMAGE=postgres:15-alpine`; blocked before migrations/tests by exact environment error `panic: rootless Docker not found` in testcontainers Docker host discovery. Awaiting independent fix re-review; not merged, pushed, deployed or online-verified.
