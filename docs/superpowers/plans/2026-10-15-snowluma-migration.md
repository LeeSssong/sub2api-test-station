# SnowLuma migration implementation plan

Goal: Replace NapCat and AstrBot on 43.133.75.82 with official SnowLuma and standalone Python business service.

Approved design: current conversation, 2026-10-15. User accepts downtime and no isolated live validation.

- [x] Verify target, clean baseline and isolated branch; 157 existing tests, 6 skipped.
- [ ] Verify official pinned Linux runtime, native QQ startup prerequisites, license and recoverable deployment.
- [ ] Add failing tests for OneBot event routing, authorization, duplicate handling, API reply correlation and disconnect behavior.
- [ ] Extract business helpers without AstrBot imports; preserve report binding and outbox keys.
- [ ] Implement standalone model conversation/tool loop, existing persona and knowledge import, persistence and minimal supervisor configuration.
- [ ] Run directly relevant tests and review migration and rollback behavior.
- [ ] Back up current state; switch only robot services; verify actual login and message delivery, or restore old chain.

Constraints: Never print credentials. Do not modify unrelated test-station services. Preserve original data and rollback images. Runtime state is configuration input, never a code source. No claimed delivery without real evidence.
