# T10 Account Quality Monitor Final Review

Date: 2026-08-16
Candidate: `codex/t10-account-quality-monitor@0294cd97fb18d20ebe704406b68f23d2e991dc32`
Baseline: `main@f6ec86733d26219830646fd88859aa39613797a3`
Candidate tree: `7d7ec2f5ac2a67155dda140d2197b0838a17e4bf`

## Result

`APPROVED_WITH_REVIEW_PROCESS_EXCEPTION`

The original independent review correctly rejected the earlier implementation.
Its P0/P1 findings were fixed in `7680c270a`. Fresh reviewer dispatch then
failed at the task-coordination layer, and an independent local Codex review
process could not authenticate because the locally configured API credential
was invalid. Neither failure is presented as review PASS.

At the user's direction to keep the release path fast, the root release
controller performed the final full-diff review. That review found an
additional real defect: the UID 10002 preflight passed Ruby source to
`/bin/sh`. Commit `0294cd97f` changes the entrypoint to
`/usr/local/bin/ruby`, adds a static contract assertion, and completes the
failure-unit installation instructions. No P0-P2 finding remains after this
fix and fresh functional validation.

## Functional Verification

- Failure signal tests: 2 tests, 75 assertions, PASS.
- Wrapper/systemd tests: 4 tests, 111 assertions, PASS.
- Collector/publisher tests: 13 tests, 53 assertions, PASS.
- systemd static contract: PASS.
- relay-ops native/outbound contract: PASS.
- Shell and Ruby syntax: PASS.
- `git diff --check`: PASS.
- Relay-ops Docker image build: PASS.
- Real Docker UID/GID 10002 evidence write/fsync/rename/readback/cleanup: PASS.
- Business-data mutation diff scan: no business write path added.
- Production tree remains `0700 root:root`; evidence directory is separately
  restricted to `0700 10002:10002`.

## Waivers And Remaining Online Checks

- A6 actual on-call delivery is user-waived and unverified; no receipt is
  claimed.
- A10 time-based observation is user-waived and unverified; no time waiting is
  required.
- Immediate host systemd execution, evidence publication, service result, and
  health checks remain required after deployment.
- `downtime_required=false` is expected because there is no migration or
  application traffic interruption, but the root preflight remains
  authoritative.

## Scope

No database migration, application API, UI, scheduling, billing, account
admission, account routing, GitHub Actions, new receiver, or parallel control
plane is added.

## Post-merge production finding and disposition

The first host execution found one additional production topology defect: the
wrapper's fixed `http://sub2api:8080` target does not exist in the blue-green
network. The failure was contained at collector exit code `44`; no evidence or
business data was partially published. Fix `d075da534` reads the protected
active-upstream value, rejects anything outside the exact blue/green allowlist,
and was merged as `main@b1b92cf30`.

The latest root tree passed the focused functional matrix and a real production
run. The service returned `Result=success`, `ExecMainStatus=0`; the timer is
enabled and waiting; both evidence files are valid `0600`; the production root
remains `0700 root:root`; and no new `203/EXEC` occurred. This closes the
functional finding without changing the recorded independent-review process
exception or claiming the waived A6/A10 checks as PASS.
