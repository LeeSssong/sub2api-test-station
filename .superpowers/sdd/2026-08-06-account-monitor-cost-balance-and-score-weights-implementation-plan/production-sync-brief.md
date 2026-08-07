# Production sync task brief

CONTEXT_ACK=2026-08-06-account-monitor-cost-balance

## Objective

Synchronize the complete qualified production release chain ending at `9aab62c203ce9546d77ecf558558bb1a360a634e` into branch `codex/account-monitor-cost-balance-implementation`, preserving the reviewed account-monitor feature commits.

## Fixed constraints

- Work only in the existing isolated worktree.
- Merge the complete production commit chain; do not copy only migrations 192/193.
- Preserve production behavior unless a genuine textual/semantic overlap with the account-monitor feature requires resolution.
- Preserve feature rules: API-key accounts use multiplier; non-key OpenAI accounts use procurement cost / estimated quota; New API TTL is six hours; single-card force refresh bypasses TTL; balance is display-only; score-weight entry and inline reload errors remain.
- Do not push or deploy.
- Do not modify production state.
- Keep project ledgers and SDD artifacts out of the implementation commit.

## Required investigation and implementation

1. Record pre-merge HEAD and merge-base.
2. Merge `9aab62c203ce9546d77ecf558558bb1a360a634e` with a merge commit.
3. Resolve conflicts by comparing both sides and tests; never choose an entire side mechanically for overlapping account-monitor/upstream-billing files.
4. Verify migrations 192, 193, 197 all exist and compute the candidate migration hash with the repository's canonical algorithm.
5. Run focused backend/frontend feature tests plus tests for any production files touched by conflict resolution.
6. Run typecheck/build and `git diff --check`.
7. Commit conflict resolutions as part of the merge and write a report to `production-sync-report.md`.

## Report contract

Report merge commit, conflicts and their resolutions, migration list/hash, exact test results, and concerns. Return only status, commit, one-line verification summary, and concerns.
