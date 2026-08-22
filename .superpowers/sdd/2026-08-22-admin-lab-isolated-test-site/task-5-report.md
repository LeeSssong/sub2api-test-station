# Task 5 report — isolated admin lab mocks and fail-closed notifications

- Scope: deterministic `lab-mock-upstream` / `lab-mock-payment` fixtures, strict egress guard, and a lab-only notification outbox adapter. No production notification fact source or external transport was added.
- Files: `tools/admin-lab/mock_server.py`, `tools/admin-lab/mock_server_test.py`, `tests/admin_lab/mock_egress_test.sh`, `infra/compose.admin-lab.yaml`, `upstream/sub2api/backend/internal/lab/{egress_guard.go,egress_guard_test.go,guard.go,guard_test.go,notification_outbox.go,notification_outbox_test.go}`.
- Behavior: upstream normal/502/stream-interrupt fixtures use fixed IDs, usage, costs/traces; payment create/confirm/failure/refund fixtures are deterministic; webhook targets are restricted to the lab API; notifications are redacted and stored only in an in-memory `LAB_ONLY` outbox. `NOTIFICATION_TRANSPORT=lab-outbox` is required by the lab compose/API command and guard.
- Tests: `go test ./internal/lab -count=1`; `python3 tools/admin-lab/mock_server_test.py`; `bash tests/admin_lab/mock_egress_test.sh`; `git diff --check` — all passed.
- Risks: the mock outbox is intentionally process-local and not a production persistence mechanism; Task 6 may add versioned seed/reset integration. The existing application notification wiring remains unchanged outside lab configuration.
