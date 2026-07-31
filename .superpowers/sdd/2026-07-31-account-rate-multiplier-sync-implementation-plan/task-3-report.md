# Task 3 report — lifecycle and periodic probe integration

STATUS: READY FOR INDEPENDENT REVIEW

## Scope completed locally

- An eligible active `openai` / `apikey` account now schedules one asynchronous,
  best-effort native billing probe after successful create or edit.
- Enabling native billing probing schedules the same forced lifecycle probe.
  The lifecycle service boundary checks eligibility and active status both
  before dispatch and on the account reload immediately preceding HTTP, so an
  inactive account makes no upstream HTTP request.
- Lifecycle probes use the existing `ProbeAccount` persistence route and mark
  its synchronization context as `lifecycle`; scheduled probes retain the
  existing route and default `scheduled` trigger.
- Failed probes preserve the prior billing snapshot data and current account
  multiplier. Usage recording snapshots the synchronized account multiplier
  (`0.07`) rather than its group multiplier (`1.4`).
- Ordinary OpenAI forwarding code was not modified and does not invoke a
  billing probe.

The project ledger remains **进行中**. No production data or configuration
was changed; this task has not been pushed, deployed, or verified online.

## Changed files

- `upstream/sub2api/backend/internal/service/upstream_billing_probe.go`
- `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`
- `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/account_upstream_billing_probe.go`
- `upstream/sub2api/backend/internal/handler/admin/account_upstream_billing_probe_lifecycle_test.go`
- `upstream/sub2api/backend/internal/handler/admin/admin_service_stub_test.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go`
- `docs/project/project-progress.md`
- `.superpowers/sdd/2026-07-31-account-rate-multiplier-sync-implementation-plan/progress.md`

## TDD evidence

RED (lifecycle service API before implementation):

```text
go test ./internal/service -run TestUpstreamBillingProbeLifecycleProbeUsesLifecycleSyncTrigger -count=1
...
svc.ProbeLifecycleAccount undefined
FAIL
```

GREEN (minimal lifecycle API):

```text
go test ./internal/service -run TestUpstreamBillingProbeLifecycleProbeUsesLifecycleSyncTrigger -count=1
ok github.com/Wei-Shaw/sub2api/internal/service
```

RED (handler lifecycle test before the handler depended on a narrow probe
interface):

```text
cannot use lifecycle spy as *service.UpstreamBillingProbeService
```

GREEN (handler integration after the narrow interface and explicit lifecycle
context were added):

```text
go test ./internal/handler/admin -run 'TestAccountHandler(Create|Update|Enable)ForcesLifecycleBillingProbe' -count=1
ok github.com/Wei-Shaw/sub2api/internal/handler/admin
```

RED (inactive lifecycle protection):

```text
go test ./internal/service -run TestUpstreamBillingProbeLifecycleProbeSkipsInactiveAccount -count=1
Expected nil, but got: &service.UpstreamBillingProbeSnapshot{Status:"ok", ...}
FAIL
```

GREEN (active-status service guard):

```text
go test ./internal/service -run 'TestUpstreamBillingProbeLifecycleProbe(UsesLifecycleSyncTrigger|SkipsInactiveAccount)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 2.058s
```

Scheduled-trigger propagation, failed-probe preservation, and the usage-log
account-multiplier selection are regression tests of behavior already present
when written; they passed on their first targeted run and are not represented
as artificial RED cycles.

## Verification

GREEN (full affected-package regression, exit status 0):

```text
go test ./internal/service ./internal/handler/admin ./internal/repository ./internal/server/routes ./cmd/server -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 103.090s
ok github.com/Wei-Shaw/sub2api/internal/handler/admin 2.938s
ok github.com/Wei-Shaw/sub2api/internal/repository 1.969s
ok github.com/Wei-Shaw/sub2api/internal/server/routes 3.055s
ok github.com/Wei-Shaw/sub2api/cmd/server 3.422s
```

`git diff --check` passed immediately before commit.

## Self-review

- The lifecycle handler invokes only `ProbeLifecycleAccount`; it never makes a
  request directly and the account write response is not coupled to probe
  availability.
- The service reuses the existing native billing probe, snapshot writer, and
  Task 2 synchronization transaction instead of implementing a parallel write
  path.
- All new lifecycle paths use `context.Background()` intentionally because the
  work is asynchronous after the HTTP response; the synchronization trigger is
  explicitly applied before probing.
- A source search confirms that lifecycle scheduling is limited to the admin
  create, update, and probe-enable paths; no forwarding package was changed.

## Remaining concerns

- Lifecycle probes are best effort. Their failures are logged and do not roll
  back a successful account create, edit, or enable action.
- A concurrent probe for the same account may be coalesced by the existing
  singleflight protection, preserving the existing no-duplicate-upstream-call
  behavior.
- Push, deployment, and live verification are deliberately out of scope and
  still required before this project item can be marked complete.

## Fix round 1 — lifecycle active-status TOCTOU

Independent review found that the original lifecycle precheck ran on one
account read, while `ProbeAccount` issued HTTP from a second read without an
active-status requirement. A disable between those reads could therefore still
contact the upstream.

`ProbeLifecycleAccount` now uses an explicit active-required probe mode for
its second load. Scheduled mode retains its existing active/enabled checks;
manual `ProbeAccount` remains active-status agnostic, including when the
per-account probe switch is disabled.

RED (the first read returns active and changes the second read to disabled):

```text
go test ./internal/service -run TestUpstreamBillingProbeLifecycleProbeRechecksActiveStatusBeforeHTTPRequest -count=1
Expected nil, but got: &service.UpstreamBillingProbeSnapshot{Status:"ok", ...}
FAIL
```

GREEN (second read blocks the request):

```text
go test ./internal/service -run 'TestUpstreamBillingProbe(LifecycleProbe(RechecksActiveStatusBeforeHTTPRequest|UsesLifecycleSyncTrigger|SkipsInactiveAccount)|RunnerIsBoundedAndManualProbeIgnoresSwitches)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 2.025s
```

The race regression asserts two repository reads and zero upstream HTTP calls.

GREEN (full affected-package regression, exit status 0):

```text
go test ./internal/service ./internal/handler/admin ./internal/repository ./internal/server/routes ./cmd/server -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 101.134s
ok github.com/Wei-Shaw/sub2api/internal/handler/admin 2.523s
ok github.com/Wei-Shaw/sub2api/internal/repository 3.749s
ok github.com/Wei-Shaw/sub2api/internal/server/routes 3.198s
ok github.com/Wei-Shaw/sub2api/cmd/server 3.223s
```
