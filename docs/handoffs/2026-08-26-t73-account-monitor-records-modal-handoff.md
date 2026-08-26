# T73 Account Detection Records Modal Handoff

## Candidate

- Task: T73 account detection records modal and dual-evidence search.
- Original baseline: `main@6d30ac9ef5526aa6860969baf6806ddd604bf8bc`.
- Refreshed baseline: `main@dd158600949dd65d196fd3511495791f3e24b327`.
- Refreshed candidate before this handoff update: `76a3a956a`.
- Branch: `codex/t73-account-monitor-modal`.
- State: `READY_FOR_ROOT_REVIEW`; the user has authorized production deployment through the root release controller.

## Delivered

- Replaced the detection-history side drawer with the approved centered dark modal.
- Promoted Juice result, behavior fingerprint, and overall conclusion to the primary record columns; tier and sample count remain secondary.
- Added Juice result, fingerprint result, and overall-conclusion filters. Juice and fingerprint filters are applied by the existing paginated backend query before cursor pagination.
- Expanded records now present Juice and fingerprint evidence in separate panels. Historical rows remain explicitly marked as historical and do not fabricate evidence or zero sample counts.
- On the 390px record layout, long fingerprint candidates wrap rather than truncating.

## Validation

- `pnpm vitest run src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts`: 60 passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed after the `dd1586009` refresh; existing Browserslist and chunking warnings only.
- `go test ./internal/handler/admin ./internal/service -run 'AccountModelDetection|DetectionHistory' -count=1`: passed.
- `go build ./cmd/server`: passed.
- `gofmt -d` for changed Go files and `git diff --check`: clean.
- Browser check used the actual component with representative current and historical API records. Desktop and 390px modal views rendered without console errors; at 390px, `scrollWidth=clientWidth=390`, including expanded evidence.

## Release Boundary

- No migration, configuration, dependency, or production-data change.
- Expected `downtime_required=false`; final release preflight remains the root controller's responsibility.
- T71 is frozen and T72/T74 are deployed, so the release lane is available for T73.
- Rollback is a revert of the candidate commit after integration; no data rollback is required.

## Production Verification Follow-up

- The first production promotion of `main@bdc12f20c` succeeded with `downtime_required=false` and activated the blue slot.
- The authenticated production modal confirmed the centered layout, three filters, evidence-first columns, and historical-record semantics, but exposed the detector's raw Juice success value `verified` as an untranslated i18n key because the UI had assumed `pass`.
- The follow-up fix keeps `verified` as the server-side filter value and maps it to the existing user-facing “通过 / Passed” presentation for labels, colors, and fallback detail copy. Legacy `pass` display compatibility remains intact.
- TDD evidence: the new production-shaped regression failed against the released implementation, then passed after the minimal mapping fix. The two directly related component suites now pass 61/61; typecheck and production build also pass.
