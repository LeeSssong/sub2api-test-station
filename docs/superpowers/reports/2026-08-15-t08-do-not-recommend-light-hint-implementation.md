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

Changed files in the implementation slice:
- `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue`
- `upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

Branch-local planning/spec artifacts also exist from the approved prep phase:
- `docs/superpowers/specs/2026-08-15-t08-do-not-recommend-light-hint-design.md`
- `docs/superpowers/plans/2026-08-15-t08-do-not-recommend-light-hint.md`

## 3. TDD tests and exact command results

Focused combined frontend checks already passed:

```bash
cd upstream/sub2api/frontend
pnpm vitest run \
  src/components/common/__tests__/HelpTooltip.spec.ts \
  src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Result: `60 passed`

Underlying suite evidence:
- `HelpTooltip.spec.ts`: `15/15` passed
- `AccountMonitorCard.spec.ts`: `45/45` passed

Follow-up TDD evidence for the scoped review finding:
- RED: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts -t "does not steal focus on Escape when hover-click details are hidden"` failed because hidden Escape moved focus from the external next control to the tooltip trigger.
- GREEN: the same focused test passed after `82eddab22`.
- GREEN: `pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts` passed `15/15`.
- GREEN: the combined two-file run passed `60/60`.

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

## 10. Remaining risks and independent review result

Remaining risks are limited to the final root integration surface:
- responsive overflow or tooltip placement on the live page;
- any unintended regression in the existing formal-migration tooltip branch;
- real production auth/session state after deploy.

Independent whole-branch review: pending after final validation and scoped re-review.
