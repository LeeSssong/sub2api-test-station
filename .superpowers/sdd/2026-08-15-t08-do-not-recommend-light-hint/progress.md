# SDD ledger — plan: docs/superpowers/plans/2026-08-15-t08-do-not-recommend-light-hint.md

Setup complete. Branch `codex/t08-do-not-recommend-light-hint` is isolated from `main`; no global ledger, queue, production evidence, or workflow files are in scope.
Task 1: review found P2 — clickPinned guards are not scoped to hover-click across runtime trigger changes; fix round 1 required.
Task 1: fix round 1/5 (P2 addressed by implementer commit e5c2b4055; scoped re-review pending).
Task 1: complete (commits 62e585ca3..e5c2b4055, review clean after fix round 1).
Task 2: complete (commits e5c2b4055..d2fe007bb, review clean).
Task 3: handoff report written at docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md; final whole-branch review requested.
Task 3: follow-up accessibility fix applied; whole-branch review clean and ready for root review.
Refresh: root main `751402105` merged into candidate at `2248cfa1d`; no conflicts recorded.
Root fresh whole-branch review on `751402105..2248cfa1d`: CHANGES REQUIRED with four P2 findings (mobile viewport clamp, Teleport focus lifecycle, refreshed not_recommended reset, stale report/ledger baseline/status).
Scoped fix package: commit `1e9b9591e` addressed viewport clamp, Teleport focus lifecycle, and refreshed payload reset; implementer-reported checks were two targeted specs `59 passed`, `pnpm typecheck` exit 0, and `git diff --check` exit 0.
Scoped review of `2248cfa1d..1e9b9591e`: original three code P2 findings addressed; new Important found for hidden tooltip Escape focus hijack.
Scoped fix follow-up: RED hidden-Escape test failed as expected, then commit `82eddab22` added `!show.value` guard and corrected the visible-Escape focus-restoration test; `HelpTooltip.spec.ts` `15/15` and combined targeted specs `60/60` passed.
Scoped re-review of `1e9b9591e..82eddab22`: root-control fresh read-only reviewer APPROVED; P0/P1/P2/P3 = 0 and open findings = 0. Scope was limited to `HelpTooltip.vue` and `HelpTooltip.spec.ts`; reviewer confirmed `!show.value` guard, real reopen test, hidden-state Escape regression, HelpTooltip `15/15`, diff-check, and no worktree writes.
Root fresh whole-branch review on `751402105..334951bef`: CHANGES REQUIRED with P2 = 2 and P3 = 1 (identical cloned not_recommended payload replacement did not reset state; local mock/browser screenshot requirement over-classified as blocking; branch delta list omitted report and SDD progress).
Scoped fix: RED identical-cloned-payload regression failed, then commit `6bcf2180e` added card-local recommendation object identity revision to the reset key; focused regression and refreshed-payload tests passed, and `AccountMonitorCard.spec.ts` passed `46/46`.
Documentation fix: implementation report now lists the full branch delta including itself and `.superpowers/sdd/2026-08-15-t08-do-not-recommend-light-hint/progress.md`, and records root acceptance override that real desktop/mobile/narrow-screen/browser action validation is a root post-merge/deploy logged-in administrator-session hard gate, not a local mock blocker here.
Root fresh whole-branch re-review on `751402105..935be31a3`: CHANGES REQUIRED with P2 = 2 (HelpTooltip viewport placement/width constraints incomplete; resetKey close while focus is on Teleported close button did not restore focus).
Scoped fix: RED HelpTooltip tests failed for extreme narrow width, top-edge placement, and resetKey close-button focus; RED AccountMonitorCard replacement tests failed once close-button focus restoration was asserted. Commit `9ba1c8a8a` added common HelpTooltip max-width, measured above/below placement with 12px viewport padding, top/left clamping, and resetKey focus restoration only when activeElement is inside the Teleported tooltip. Targeted HelpTooltip and AccountMonitorCard runs passed; combined two-spec run passed `64/64`.
