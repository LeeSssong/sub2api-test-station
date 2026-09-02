# T114 多窗口服务质量评分与慢首输出观测交接

## Candidate

- Task: T114
- Base main: `b15ccd267d6745166af4f36e75f31fbd2987ab13`
- Candidate branch: `codex/t114-multi-window-quality-scheduling`
- Candidate implementation commit: `c39009039`
- Final branch tip: recorded by the root task from `git rev-parse HEAD`
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t114-multi-window-quality-scheduling`

## Delivered

- OpenAI ordinary HTTP text quality aggregation uses one seven-day scan with mutually exclusive `w1`, `w24`, and `w7` rows.
- Composite quality ranking uses success, P50/P90 TTFT, output-rate, and native live-load components while preserving native eligibility, profit partition, slots, waits, retries, billing, and account state.
- Process-local 60-second slow first-output observation records `openai.first_output_slow`, overlays later W1 ranking, and never cancels, closes, retries, switches, or changes billing for the active attempt.
- Existing Ops scheduler experience exposes an additive slow-output count.

## Verification

- Repository quality-window tests: pass.
- Service quality, scheduler, visible-output, slow-tracker, and observability focused tests: pass.
- Admin handler focused tests: pass.
- `go build ./cmd/server`: pass.
- Frontend AccountMonitor/MonitorV4 focused tests: 58/58 pass.
- `pnpm typecheck`: pass.
- `pnpm build`: pass.
- Full frontend run was not used as a gate because it contains seven unrelated existing failures in HomeView/refresh/MonitorForm/ChannelStatus/OpsErrorDetail paths.

## Release Boundary

- No migration, configuration, account-pool, group, billing, production-data, or GitHub Actions changes.
- No push, acceptance-station deployment, main-site deployment, or production traffic was performed.
- `downtime_required=unverified until root preflight`.
- Rollback: do not integrate this candidate; after root integration use the previous verified blue-green slot/image according to the root release chain.
- Shadow comparison and acceptance-station functional validation remain root-controlled follow-up work before formal enablement.
