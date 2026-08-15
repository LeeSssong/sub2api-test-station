# T08 Task 3 Implementation Handoff

Status: `FINAL_REVIEW_PENDING`

## 1. Scope and approved specification path

Approved spec: `docs/superpowers/specs/2026-08-15-t08-do-not-recommend-light-hint-design.md`

Scope kept to the compact `not_recommended` hint only:
- compact label stays visible;
- reason is on-demand only, via desktop hover/click and mobile tap;
- reason is limited to one or two wrapped lines;
- no permanent explanation area;
- no recommendation algorithm, group migration, card layout, backend, or production-state change.

## 2. Baseline SHA, implementation commit SHAs, and changed files

Baseline SHA: `751402105`

Refresh merge: `2248cfa1d`

Implementation commits:
- `62e585ca3` `feat: support hover-click help tooltips`
- `e5c2b4055` `fix: reset hover-click pin state on trigger changes`
- `d2fe007bb` `feat: add on-demand not-recommended reason`
- `1e9b9591e` `fix: harden not-recommended hint interactions`
- `82eddab22` `fix: ignore hidden tooltip escape events`
- `6bcf2180e` `fix: reset identical recommendation replacements`
- `9ba1c8a8a` `fix: keep hover-click tooltip in viewport`

Changed frontend files in the implementation slice:
- `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue`
- `upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

Complete branch delta against `main@751402105` is limited to:
- `.superpowers/sdd/2026-08-15-t08-do-not-recommend-light-hint/progress.md`
- `docs/superpowers/specs/2026-08-15-t08-do-not-recommend-light-hint-design.md`
- `docs/superpowers/plans/2026-08-15-t08-do-not-recommend-light-hint.md`
- `docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md`
- `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue`
- `upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

## 3. TDD tests and exact command results

Focused combined frontend checks already passed:

```bash
cd upstream/sub2api/frontend
pnpm vitest run \
  src/components/common/__tests__/HelpTooltip.spec.ts \
  src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Result: `64 passed`

Underlying suite evidence:
- `HelpTooltip.spec.ts`: `18/18` passed
- `AccountMonitorCard.spec.ts`: `46/46` passed

Follow-up TDD evidence for the scoped review finding:
- RED: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts -t "does not steal focus on Escape when hover-click details are hidden"` failed because hidden Escape moved focus from the external next control to the tooltip trigger.
- GREEN: the same focused test passed after `82eddab22`.
- GREEN: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts` passed `15/15`.
- GREEN: the combined two-file run passed `60/60`.

Follow-up TDD evidence for the identical cloned-payload replacement finding:
- RED: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts -t "identical recommendation object replaces"` failed because the tooltip stayed visible after a cloned `not_recommended` recommendation object replaced the previous payload with the same `reason_codes[0]` and `observed_at`.
- GREEN: the same focused regression passed after `6bcf2180e`.
- GREEN: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts` passed `46/46`.

Follow-up TDD evidence for the viewport placement and resetKey close-button focus findings:
- RED: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts -t "extremely narrow|top-edge|restores focus on reset key"` failed because the tooltip had no `max-width`, top-edge triggers still produced an offscreen top value, and resetKey closure left focus on the hidden Teleported close button.
- RED: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts -t "refreshed recommendation payload changes|identical recommendation object replaces"` failed before the resetKey focus fix once close-button focus restoration was asserted in the card replacements.
- GREEN: the same HelpTooltip targeted run passed after `9ba1c8a8a`.
- GREEN: the same AccountMonitorCard replacement run passed after `9ba1c8a8a`.
- GREEN: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts` passed `64/64`.

Required static checks already passed:

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
cd ../..
git diff --check
git diff --name-only 751402105..HEAD
```

Result before documentation update: all commands exited `0`; scope audit stayed on the approved frontend slice plus T08 spec/plan/report/SDD ledger paths.

Refresh audit note: the root-supplied diff evidence reported `git diff --exit-code 751402105..2248cfa1d -- docs/project/native-sub-task-package-queue.md docs/project/project-progress.md` as clean, and `git diff --name-status 751402105..2248cfa1d` stayed on the T08 spec/plan/report/SDD ledger plus the four frontend files.

## 4. Desktop/mobile screenshot and DOM evidence paths

Local browser capture was intentionally not expanded further in this turn.

Evidence path: deferred to the root total's logged-in online verification after merge/deploy.

Root acceptance override: the root controller explicitly rejected treating local mock/browser screenshots as a blocking candidate-gate requirement for this round. Real desktop, mobile, narrow-screen, and account-action interaction acceptance remains a root hard gate after merge/deploy using an administrator logged-in online session. This task window did not build local mocks, inspect login, modify auth data, or touch production.

## 5. Interface, migration, dependency, configuration, and GitHub Actions status

- Interface surface: unchanged outside the `HelpTooltip` trigger contract and the `not_recommended` hint render path.
- Migration: none.
- Dependency: none.
- Configuration: none.
- GitHub Actions: untouched.

## 6. `downtime_required=false` precondition

`downtime_required=false`

Reason: this slice is frontend-only, has no schema/data migration, and does not change release orchestration or production state.

## 7. Rollback

Rollback is a frontend tree/image restore only.

No data rollback is required or allowed for this task.

## 8. Unverified items: production deployment and online verification

Still unverified here:
- production deploy from root `main`;
- logged-in desktop/mobile online verification on the deployed environment;
- post-deploy health and identity checks.

## 9. Root review findings and fix-loop status

Root fresh whole-branch review over `751402105..2248cfa1d` returned four P2 findings:
- mobile/narrow viewport outer tooltip placement lacked viewport clamp;
- Teleported close button focus lifecycle did not close when tabbing onward and did not prove focus restoration for close/Escape;
- refreshed `not_recommended` payloads with stable `account_id` could retain open/pinned hover-click state;
- implementation report and SDD ledger still carried the old baseline and stale review status.

Scoped fix package:
- `1e9b9591e` addressed the first three code P2s with viewport clamping, Teleport focus lifecycle handling, focus restoration tests, and `resetKey` reset behavior.
- Fresh read-only scoped review of `2248cfa1d..1e9b9591e` found all three original P2s addressed, then raised one new Important finding: hidden `Escape` events could restore focus to an unrelated hidden tooltip trigger.
- `82eddab22` added RED/GREEN coverage for hidden Escape focus hijacking and guarded `onDocumentKeydown` with `!show.value`.
- Root-control fresh read-only scoped re-review of `1e9b9591e..82eddab22` returned APPROVE: P0/P1/P2/P3 = 0, open findings = 0. Review scope was limited to `HelpTooltip.vue` and `HelpTooltip.spec.ts`; it confirmed the `!show.value` guard closes the hidden-state Escape focus hijack, the real reopen test remains valid, HelpTooltip `15/15` passed, `git diff --check` passed, and the reviewer did not write the worktree.

Root fresh whole-branch review over `751402105..334951bef` returned CHANGES REQUIRED with P2 = 2 and P3 = 1:
- P2: identical cloned `not_recommended` payload replacement did not reset open/pinned tooltip state when visible fields stayed the same.
- P2: local mock/browser screenshot requirement was over-classified as blocking for the candidate; root acceptance override keeps real browser/device validation as a post-merge/deploy administrator-session gate.
- P3: implementation report branch delta omitted the report itself and `.superpowers/sdd` progress.

Second scoped fix package:
- `6bcf2180e` added a RED/GREEN regression for identical cloned recommendation replacement and combined a card-local recommendation object identity revision with the existing reason/observed reset key.
- This report now includes the full branch delta, including itself and `.superpowers/sdd/2026-08-15-t08-do-not-recommend-light-hint/progress.md`.
- The browser validation classification is documented as a root acceptance override, with logged-in online desktop/mobile/narrow-screen acceptance deferred to root after merge/deploy.

Root fresh whole-branch re-review over `751402105..935be31a3` returned CHANGES REQUIRED with P2 = 2:
- P2: HelpTooltip viewport constraint was incomplete because it only clamped horizontal center, top-edge triggers could render above the viewport, and tooltip widths greater than `viewport - 24px` could fall back to an unclamped center.
- P2: resetKey refresh while focus was on the Teleported close button closed the tooltip without restoring focus, leaving focus on a hidden `v-show` control.

Third scoped fix package:
- `9ba1c8a8a` added common HelpTooltip viewport max-width, measured width/height placement, above/below placement with 12px viewport padding, and top/left clamping without adding a card-specific floating layer.
- `9ba1c8a8a` also changed resetKey handling to restore focus only when `document.activeElement` is inside the Teleported tooltip; outside focus is not stolen.
- AccountMonitorCard changed-fields and identical-cloned replacement regressions now verify that when the close button is focused during refresh, focus returns to `[data-test="recommendation-reason-button"]`.
- Root control remains responsible for fresh scoped/whole-branch re-review; this task window does not dispatch reviewers.

## 10. Remaining risks and independent review result

Remaining risks are limited to the final root integration surface:
- responsive overflow or tooltip placement on the live page;
- any unintended regression in the existing formal-migration tooltip branch;
- real production auth/session state after deploy.

Independent whole-branch review: pending; this controller must report `BLOCKED_ON_WHOLE_BRANCH_REVIEW` after final local verification because root control owns the fresh scoped/whole-branch re-review.
