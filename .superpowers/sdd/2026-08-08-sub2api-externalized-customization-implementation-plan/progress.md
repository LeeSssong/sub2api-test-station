# SDD ledger — plan: docs/superpowers/plans/2026-08-08-sub2api-externalized-customization-implementation-plan.md

Task 1: complete — baseline inventory contract passes; historical implementation report retained.
Task 2: partial — versioned contract and outbox primitives pass focused tests, but no production code calls Outbox.Append; runtime contract is not yet proven.
Task 3: in progress — prior commit created only in-memory projections and schema; no production consumer/persistent rebuild path is wired.
Task 4: pending — route types exist but application dependencies use no control-plane handler.
Task 5: pending — client/components exist but no production admin page imports them.
Task 6: pending — contracts exist but collectors and official writer are not wired.
Task 7: pending — event constructors exist but no request/health path calls the outbox.
Task 8: pending — state types and placeholder scripts exist but updater service/http/store do not execute them.
Task 9: pending — flags/report types exist but no comparator or page cutover is wired.
Task 10: pending — production has not activated external-primary reads or retired the deep fork.

2026-08-10 audit evidence: official v0.1.173 merge failed closed with conflicts across request, billing, scheduler, Ent, admin API and frontend paths; no candidate bundle/report was emitted and production was unchanged.
2026-08-10 code review: Task 1 complete; Tasks 2-9 are skeleton/partial and Task 10 is unimplemented. Qualification currently writes ready without discovery/checksum/migration/contracts/data comparison; promote/rollback are no-op messages. Core outbox, relay consumer/control-plane, frontend flags, comparator and updater state types have no real runtime callers.
2026-08-10 production review: relay-ops remains read_only, outbox has zero rows, relay_ops schema and /api/v1/xingqiao routes are absent, qualification scheduling is failed/disabled; production is not external_primary.
Task 3 fix round 1: reviewer verdict SPEC FAIL / QUALITY FAIL. Required fixes: make event journal completion and every projection mutation one fenced PostgreSQL transaction or otherwise prove crash-safe event_id idempotency; prevent partial handler commits; add claim token/fencing and serialize or CAS multi-instance projection updates; do not report completeness=complete while processing/dead gaps exist; make migration 013 upgrade the old placeholder schema in place and test that exact transition.
Task 3 fix round 2: scoped re-review still FAIL. Fencing and transaction atomicity passed, but persisted projection rows still write completeness=complete while Journal has processing/dead gaps; serialized snapshots still omit per-field event positions; legacy account rows add position columns without backfilling from observed_at/source_watermark, so delayed old events can overwrite preserved rows.
Task 3: fix round 2/5 (3 addressed, 1 open — multi-source global projection completeness can be overwritten by a clean source; commits 6ad8d75..3cc6122).
Task 3 fix round 3: in progress — split per-source watermark completeness from global projection completeness and add a real PostgreSQL two-source regression.
Task 3: fix round 3/5 (1 addressed, 0 open; commits 3cc6122..4bcc28e).
Task 3: complete (commits c07b0a9..4bcc28e, review clean).
Task 7: initial review FAIL — production `CreateBestEffort` bypassed outbox, actual_response_model was persisted after event creation, health history query errors were ignored, and transaction/protocol evidence was incomplete.
Task 7: fix round 1/5 (3 P1 addressed, 2 P2 open — health identity semantics and real transaction coverage; commits 71ad5d5..a6db568).
Task 7: fix round 2/5 (2 P2 addressed, 0 open; commits a6db568..e52bdf1). Fresh Testcontainers PostgreSQL 18.1/Redis 8.4 integration tests prove same-transaction commit and injected-append rollback.
Task 7: complete (commits 71ad5d5..e52bdf1, review clean).
Task 4: initial review FAIL — P0: default read_only startup requires an absent core DB secret; Caddy does not route /api/v1/xingqiao; processing claims cannot be lease-reclaimed; auth loses browser IP/User-Agent and can revoke valid sessions. P1: claim failure lacks ownership/fencing evidence; repeated Idempotency-Key re-dispatches writes and omits command name; freshness is frozen/invalid for empty rows.
Task 4: fix round 1/5 (5 addressed, 3 open — browser session IP still selected from the relay peer; empty metadata remained invalid; dispatch plus completion failure could be masked; commits 4ca2c59..1375d0d).
Task 4: fix round 2/5 (3 addressed, 1 open — private/loopback CIDR trust allowed another Compose container to forge forwarded identity; commits 1375d0d..e59cad2).
Task 4: fix round 3/5 (1 addressed, 1 open — exact Caddy identity was safe but its startup-only DNS result rejected a recreated Caddy until relay restart; commits e59cad2..5c070d2).
Task 4: fix round 4/5 (1 addressed, 0 open — fixed-host Docker DNS refresh accepts the new Caddy IP, rejects the old IP and same-network peers, and fails closed without stale trust; commit 569db06).
Task 4: complete (commits c847d1f..569db06, independent SPEC/QUALITY review clean; production status remains in progress until push, deployment, and online verification).
Task 6: initial review FAIL — Critical: balance collection and account-update commands have no runtime wiring; command idempotency is not bound to CommandID/full payload and the official account update handler does not enforce the idempotency key. Important: failed-command replay returns success; actor authorization is caller-supplied rather than derived from admin context; injected HTTP clients bypass timeout guarantees; collector retries lack a stable observation identity.
Task 6: minor (deferred): future-dated balance facts are treated as fresh; final whole-branch review must triage before merge.
Task 6 fix round 1: in progress — wire bounded scheduled collection and authenticated command routing, add end-to-end core idempotency, durable failure replay, trusted actor derivation, timeout enforcement, and stable retry identity.
Task 6: fix round 1/5 (6 addressed, 0 open — runtime collection/command wiring, payload-bound end-to-end idempotency, failed replay semantics, trusted admin actor, injected-client timeout, and stable observation identity; commits 77c9ed5..3dcd86b).
Task 6: complete (commits 370aa1b..3dcd86b, scoped independent re-review clean; PostgreSQL durable-identity test remains environment-skipped and future-dated fact Minor remains deferred to final review; production status remains in progress until push, deployment, and online verification).
Task 5: initial review FAIL — Important: external_primary guards validate only shallow response fields and can replace legacy output with an incomplete contract; tests do not prove compatible/incompatible external-primary behavior or real 401/403 session isolation across all three read surfaces.
Task 5: minor (deferred): controlPlaneApi session-recovery test checks request options through a mock rather than the real interceptor; final whole-branch review must triage after the Task 5 integration-level interceptor regression is added.
Task 5 fix round 1: in progress — keep legacy output unless a complete mapped contract is proven, add external-primary compatible/incompatible page assertions, and prove 401/403 remains local through the real API-client interceptor on monitor, profitability, and usage reads.
Task 5: fix round 1/5 (2 addressed, 0 open — incomplete external-primary contract selection and missing visible fallback/session-isolation regressions; commits 2fec49f..ceac2df).
Task 5: deferred minor resolved incidentally — the real Axios response-interceptor regression now proves a control-plane 401 preserves tokens and route state while skipping primary session recovery.
Task 5: complete (commits 2827ecf..ceac2df, scoped independent re-review clean; external_primary intentionally remains legacy-rendering and locally degraded until Task 9 supplies exact mapping/comparison evidence; production status remains in progress until push, deployment, and online verification).
