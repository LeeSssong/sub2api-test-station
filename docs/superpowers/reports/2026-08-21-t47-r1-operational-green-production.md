# T47-R1 Operational Timeline Green Production Report

## Release identity

- Root source: `main@11d87183216502640c5df9aa2fc586882f49310b`
- Tested tree: `a8d6451adfd94b616366e83a51635911d3b752c2`
- Candidate: `codex/t47-r1-operational-green@8d38819d0`
- Evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-21-main-11d871832-t47-r1-operational-green.json`
- Host record: `/var/lib/sub2api/release-records/20260821T032911Z-production-1936940.json`
- Result: `succeeded`, `promoted`, `rolled_back=false`
- `downtime_required=false`; active slot `blue`

## Correction

Removed the frontend-only latency `>= 2000ms` degraded override. Monitor V2 timeline buckets now follow the native status contract: every `operational` bucket is emerald green with `UP`, unavailable probe results remain red, and missing probe data remains gray. No API, backend, data source, migration, configuration, or production-data change was made.

## Verification

- Focused Vitest: 3 files, 20 tests passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed; 1058 modules transformed.
- `git diff --check`: passed; no migration or GitHub Actions change.
- Production `/healthz`, `/readyz`, `/health`, and `/custom/performance-monitor`: HTTP 200.
- API and worker use image ID `sha256:dbd0e6508fb76f5451d5f075156b238550adb7c203e24c5e9bb945212ed493f3`, are healthy, and have restart count 0.
- PostgreSQL, Redis, and Caddy identities were preserved.
- Logged-in production browser inspection found 84 timeline buckets: 82 `operational` buckets, all computed as `rgb(52, 211, 153)` with zero amber classes; 2 no-data buckets remained gray.

## Release transport note

The standard preloaded controller safely stopped twice before host execution because the 47,549,440-byte archive exceeded its 600-second SCP stage limit on the current network path. Both attempts left the previous production source active and created no partial/final release record. The identical archive was then transferred with resumable rsync, verified byte-for-byte by SHA-256 `23316628a7a73a161483700f8fab7f2497d2a2dfa049546f569ec7a2f0ca709e`, installed root-owned at mode 0600 in the standard staging path, and passed to the exact attested host executor. All original image, source/tree, migration, downtime, health, atomic promotion, and rollback gates remained in force.

## Rollback

Use the retained previous green slot and host rollback procedure. No migration or data rollback is required.

The candidate was archived as `/Users/gongtengxinwen/Documents/sub2api-archives/t47-r1-operational-green-8d38819d0.bundle` (mode 0600, verified complete history, SHA-256 `3cecf5314e8ed3bf0eaee02e92912e421998450daff6242026d70472c7a5fba1`). After confirming production and remote `main`, the candidate worktree/local branch and detached release worktree were removed. Local, remote temporary, and remote staging archive copies were deleted; the root untracked `tools/gpt56_api_detector-git/` directory and historical worktrees were not changed.
