# D04 v3 Active Upstream Read-only Acceptance

- [x] Confirm D04 is `read_only` and registration is false.
- [x] Confirm relay-ops is `read_only + dry_run`.
- [x] Capture only non-secret pre-state: relay-ops image/health/restarts, route canonical hash, account scheduling projection, business table counts, candidate/probe counts, Feishu delivery count.
- [x] Run only `GET /api/v1/admin/accounts` through the existing Admin-key file.
- [x] Verify pagination fields and record the redacted schema/hash, not credentials or full account payloads.
- [x] Generate balance and natural account-attributed quality evidence keyed by discovered account ID.
- [x] Generate the v3 snapshot; verify every `active && schedulable` account is present exactly once.
- [x] Run the v3 offline evaluator and save only its secret-free JSON result.
- [x] Leave D04 closed on `NO-GO`; do not fabricate traffic, probe, balance, candidate, or Feishu events.
- [x] Mount the report-only result read-only for `/ops`; reserve the D04 launch overlay for a fresh same-snapshot `GO`.
- [x] Verify authenticated `/ops` shows the same account-set hash and per-account decisions.
- [x] Recheck all pre-state values; route, scheduling, balances, Keys, candidates, probes, Feishu rows, and D04 mode must be unchanged.
