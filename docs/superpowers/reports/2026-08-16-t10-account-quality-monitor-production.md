# T10 Account Quality Monitor Production Acceptance

Date: 2026-08-16
Status: `DONE`

## Release identity

- Root source: `main@b1b92cf30a791d0573c212e865d3a52c43564d95`
- Source tree: `748f9d505c54d65ffc22810800057fed6457fc89`
- Release evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-16-main-b1b92cf30-t10-account-quality-monitor-v2.json`
- Application release: `main@081dc0a035335ac900b0022bd73321503c124641`
- Application record: `/var/lib/sub2api/release-records/20260815T165259Z-production-206174.json`
- Active slot/upstream: `green` / `sub2api-green:8080`
- Host rollback backup: `/var/lib/sub2api/t10-backups/20260816-b1b92cf30`

The application release completed through the reviewed blue-green chain with
`result=succeeded`, `state=promoted`, `rolled_back=false`, and
`downtime_required=false`. The later `b1b92cf30` change is host-only: it fixes
monitor targeting without changing the application image, migration set,
database, API, UI, routing, billing, or scheduler.

## Production defect and fix

The first real service execution reached the collector but exited `44` because
the wrapper used `http://sub2api:8080`; production exposes only the blue and
green service aliases. Fix `d075da534` makes the wrapper read the protected
`SUB2API_ACTIVE_UPSTREAM` from `release.env` and accepts exactly the two known
blue-green values. The fixed wrapper and related reviewed artifacts were
installed from pushed root `main@b1b92cf30`; six local/host SHA-256 values
matched exactly. The existing `/etc/sub2api/account-quality-monitor.env` was
preserved and not overwritten.

## Immediate functional acceptance

- systemd unit verification: PASS.
- Service: `Result=success`, `ExecMainStatus=0`, one-shot inactive after success.
- Timer: enabled, `active/waiting`.
- New `203/EXEC` after deployment: absent.
- Snapshot: `ACCOUNT-QUALITY-20260815T171407Z`.
- Eligible accounts: 50.
- `account-quality-result.json`: valid JSON, mode/owner `0600 10002:10002`.
- `account-quality-history.json`: valid JSON, mode/owner `0600 10002:10002`.
- Evidence directory: `0700 10002:10002`.
- Production root: `0700 root:root`.
- Public `/healthz`, `/readyz`, and `/health`: HTTP 200.

## Verification boundary

A6 actual alert delivery and A10 time-based observation were explicitly waived
by the user. They remain unverified residual risks and are not claimed PASS.
No hourly or 24-hour observation was performed. T10 completion is based on the
focused merged-tree tests, deployed artifact identity, one real successful
production execution, valid evidence publication, timer state, and immediate
health checks.

## Archive and cleanup

The completed candidate was archived as a verified, mode-`0600`, complete-history
bundle at
`/Users/gongtengxinwen/Documents/sub2api-archives/t10-account-quality-monitor-d075da534.bundle`
with SHA-256
`f02cc7907bd749c8f289716c2cb4b65a8d2554a4441d8b64d2823d45e478b471`.
After bundle verification, the candidate worktree/local branch and the two T10
detached release worktrees were removed. The production rollback backup,
release evidence, systemd journal, and final JSON evidence remain available.
