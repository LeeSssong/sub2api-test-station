# D04 Controlled Launch Delta Implementation Plan

1. Add failing tests for Admin `GET` authentication and read-only scheduler zero-write behavior.
2. Add failing configuration tests for explicit, qualified cost policy and write-mode fail-closed startup.
3. Replace user-visible “内测” copy with “首发计划” and add a repository-level contract assertion.
4. Wire a controlled D04 alert sender without introducing a second unmanaged bot; keep read-only startup tolerant and write startup strict.
5. Run focused tests, full race tests, vet, image build, Compose/Caddy contracts, and `git diff --check`.
6. Deploy only `internal-test-service` in `read_only`, then prove one scheduler interval and zero production writes.
7. Review the independent zero-cost nonfunctional baseline.
8. Pause topology changes until XM PLUS/PRO credentials are installed and qualified; then submit and verify the exact routing proposal before applying it.
9. Complete isolated low-budget D04 write acceptance and update `current-state.md`, `llm-handoff.md`, and verification reports.
