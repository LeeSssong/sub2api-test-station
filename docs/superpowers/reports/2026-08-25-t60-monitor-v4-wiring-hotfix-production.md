# T60 Monitor V4 Wiring Hotfix Production Verification

## Outcome

The fourth monitoring mode was configured in production, but `/api/v1/monitor-v4` returned HTTP 500 with `monitor v4 unavailable`. The response came from the handler's nil-service guard: the generated Wire assembly had not created or passed `MonitorV4Handler` into the top-level `Handlers` struct. The monitor SQL and configuration data were not the cause.

The fix creates `MonitorV4Service` and `MonitorV4Handler` in the generated server wiring, adds the handler parameter to `ProvideHandlers`, and stores it on `Handlers.MonitorV4`. A regression test covers this wiring contract.

## Verification

- Source: `main@d3ed66d087bf1b46d3a366ad544bf0e2e4e1c802`, tree `067680a3256ecc1b92e9b0ddef8a3aa74dd23d7e`.
- Focused backend tests and `go build ./cmd/server`: passed.
- Production release: `/var/lib/sub2api/release-records/20260825T024354Z-production-2390618.json`; `succeeded/promoted`, `downtime_required=false`, active slot `green`.
- Public health checks: `/healthz`, `/readyz`, `/health` all returned HTTP 200.
- Authenticated performance page: `/custom/performance-monitor` rendered GPT-Pro, GPT-Plus, and GPT-特惠分组 cards, with availability, TTFT P95, total latency P95, and unified real-request sample counts.
