# Lightweight drawing judgment implementation plan

Goal: after saving each new intelligence drawing, judge one screenshot once and show a ribbon on the existing artwork. No deployment, gallery, animation grading, history backfill, regeneration or account intervention.
Architecture: retain generation status; persist a pending/final visual judgment in existing result configuration JSON. The current background run saves the artwork before rendering/judging, finishes with a conditional result update. GET stays read-only. Reuse quality judge account selection, one multimodal request, short JSON verdict/reason. Render with isolated Chromium, no model scripts/network, bounded time and image size.
Spec: approved lightweight design in this chat (2026-10-13).
Execution: inline, test-first. Root main and other worktrees are not modified.

- [x] Backend: tests for drawing config snapshot, visual judgment state and stale pending projection, single-request verdict/errors and generation-result preservation. Add snapshot renderer and PNG validation; integrate pending save then final update using existing JSON storage. No schema migration. At most one judge request after account selection; failures become unknown, not a retry.
- [x] Transport/runtime: test one image reaches native Responses, Chat Completions and Anthropic requests without changing candy payloads; unsupported platforms fail explicitly. Package Chromium/fonts in application Dockerfile. Single 1024x768 render reduced to 768x576, one frame; no stored base64 in DB.
- [x] UI: tests for latest failed artwork, ribbon pending/pass/fail/unknown/unjudged, refresh updates same record without extra detail requests, and failure preview in dialog. Add overlay and reason in existing detail. Add optional drawing judge group/model within existing rule form, inherit candy judge when unspecified.
- [x] Verify: focused Go unit/repository tests, real local Chromium screenshot test, Vue component tests/typecheck, desktop/mobile browser visual check. Record actual checks and limitations. No upstream paid test without configured local fixture; no push/deploy.

Outcome: implementation and direct local checks complete. Real paid model evaluation and native Linux container screenshot remain unverified; see docs/implementation/2026-10-13-intelligence-drawing-judge.md. No merge, push, or deployment.
