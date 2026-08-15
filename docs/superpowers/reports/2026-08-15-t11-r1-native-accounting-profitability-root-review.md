# T11-R1 Native Accounting Profitability Root Review

Actual review date: 2026-08-15 (Saturday, Asia/Shanghai). Files named `2026-08-16` were pre-agreed future-dated handoff artifacts and do not represent the current date.

## Integration identity

- Main baseline and fetched `origin/main`: `6289c22a31a9c6a53836e2086f2f356c13be1c1b`.
- Authorized candidate: `codex/t11-r1-native-accounting-profitability@86d5c4cd41245e1adf98cb1dc52200044de38036`.
- Implementation commit recorded by the candidate: `c5df650cd4c1ab2f8dcd6982f31ffd842bf303a9`.
- Root authorization: `AUTHORIZE_MERGE_TO_MAIN`.
- Local merge commit: `b1cb220b1d8280aaace9a617152dc48e75020786`.
- Candidate and merged runtime tree: `747f3f` prefix matched in merged-main verification.

## Worktree and protection audit

- T07 and T08 detached release worktrees were clean and behind main; left untouched.
- `official-v0175-fast-merge` was one historical commit ahead and 220 commits behind main; left frozen and untouched.
- `upstream-resilience-hardening` was one historical commit ahead, 237 commits behind main, and had six protected dirty entries; left frozen and untouched.
- The T11-R1 candidate was ten commits ahead, zero behind, and clean before merge.
- Root protected modifications and untracked files matched the source handoff allowlist. No unexpected dirty path appeared.

## Candidate review gates

- Task 1 native pair aggregation review: clean.
- Task 2 service/API review required one test-gate fix round; all three findings were independently re-reviewed as addressed.
- Task 3 frontend review required two fix rounds; all Important findings were independently re-reviewed as addressed.
- Fresh whole-branch review found no Critical or Important issues. The retained obsolete locale keys were classified as non-blocking compatibility cleanup.
- Real PostgreSQL integration was subsequently run on the existing Colima Docker daemon with an actual PostgreSQL testcontainer. Verbose and fresh confirmation runs both passed, and the qualification containers were cleaned automatically.

## Merged-main verification

The visible merged-main verification task validated `main@b1cb220b1d8280aaace9a617152dc48e75020786`:

- Git identity, candidate ancestry, merge range, diff checks, protected-scope and forbidden-source guards: pass.
- Backend focused repository/service/admin/server commands: pass.
- Neighboring repository/service/admin/server package regression: pass.
- Real PostgreSQL native-contract integration through Colima/testcontainers: pass.
- Frontend API/view tests: 18/18 pass.
- Frontend typecheck and production build: pass.
- Release controller, host executor, and evidence-writer shell syntax: pass.
- No migration, configuration, dependency, lockfile, or GitHub Actions change.
- Static release classification: `downtime_required=false`.

Canonical release evidence is intentionally deferred until a clean release worktree exists at the final pushed main commit.

## Post-deployment VERIFYING gates

The final reviewer reclassified the remaining evidence as post-deployment gates rather than merge blockers:

1. Authenticated administrator profitability route at exactly 390x844. Capture a screenshot and the document/table client/scroll widths. Page-level overflow must remain absent; the table wrapper may scroll horizontally.
2. The 31-day administrator profitability request. Record HTTP status, response latency, visible values, and—if latency or host evidence warrants it—a read-only query-plan observation. This gate must not mutate production data.

The task must not be marked `DONE`, and T09/S1-R2 must not resume, until push, blue-green deployment, authenticated online verification, and both gates above are closed.

## Release and rollback classification

- `downtime_required=false`: no migrations, schema, configuration, dependency, workflow, or data changes.
- Release path: reviewed local/host blue-green scripts only; no GitHub Actions.
- Rollback: restore the previous active application/worker image and Caddy upstream. No database rollback or data cleanup is required.
