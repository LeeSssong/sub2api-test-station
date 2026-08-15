# T10: Read-only Account Quality Monitor Executable Chain

Status: `SPEC_APPROVED_FOR_PLAN`
Baseline: `8f79f1330fa007761b2a82af9a845529fbc5b31d`
Scope owner: T10 (specification only)
`downtime_required`: `false`, subject to release-controller preflight

## 1. Problem Evidence

The existing read-only account quality monitor timer is waiting, while its
service repeatedly fails before the wrapper starts with `203/EXEC
Permission denied`. The unit runs as `ubuntu` and executes
`/opt/sub2api/production/ops/account-quality/run-account-quality-monitor.sh`.
The production tree is `0700 root:root`, so the nominal service identity cannot
traverse it. The weekly service-quality report records 457 launch failures and
requires a 24-hour natural window with zero `203/EXEC` failures.

The oneshot also depends on the Docker daemon and writes two evidence JSON
artifacts. The current failure leaves the executable chain, container write
contract, and failure alert delivery unproven as one system.

## 2. Goals and Non-goals

### Goals

1. Make the systemd service executable without weakening the `0700 root:root`
   production tree or relocating secrets/evidence.
2. Prove the complete path: systemd -> root host orchestration -> restricted
   collector container -> atomic evidence publication.
3. Emit one stable, redacted, deduplicable failure signal for each failed run,
   including a failure where `ExecStart` never runs (`203/EXEC`).
4. Reuse the existing production alert receiver and verify one controlled
   failure reaches the existing on-call endpoint.
5. Provide deterministic stage/reason classification, safe rollback, and a
   24-hour timer acceptance window.

### Non-goals

No changes to scheduling algorithms, account admission, balance/profit logic,
card dual-domain semantics, user pages, external control planes, official
update conflict handling, account routing, or business data writes. No new
relay-ops/Feishu API, database table, receiver, or parallel alert plane.

## 3. Impact and Boundaries

The systemd host-side oneshot is run as `root` because it must traverse the
existing `0700` production tree and control Docker. This is an orchestration
identity, not a collector privilege grant. The collector remains
`10002:10002`, read-only rootfs, `cap-drop=ALL`, `no-new-privileges`, noexec
temporary filesystem, 64 PID limit, 128 MiB memory limit, and 0.25 CPU limit.

Secrets and evidence remain at their existing production paths. No permission
change, ACL, symlink, or parent-directory migration may be used to make the
chain work. The host process may only read the existing Admin-Key input and
publish the two existing evidence files.

## 4. Alternatives and Decision

| Option | Description | Trade-off | Decision |
| --- | --- | --- | --- |
| A | Root host orchestration; restricted UID 10002 collector; existing mounts and receiver | Adds root to the orchestration unit, but preserves the production tree and closes all four executable-chain segments | **Recommended and approved** |
| B | Keep `User=ubuntu` and add traversal permissions/ACLs | Creates permission drift through the production tree and weakens the intended boundary | Rejected |
| C | Relocate runtime, secrets, or evidence below an ubuntu-readable path | Requires migration, changes evidence ownership/paths, and increases rollback and secret exposure risk | Rejected |

## 5. Control Flow

1. systemd timer starts the oneshot service.
2. systemd invokes a root-owned, mode-0755 wrapper/helper from the existing
   production path.
3. The wrapper performs fixed path/mode preflight and reads the existing
   Admin-Key without logging its value.
4. A focused container preflight, executed with effective UID 10002 against the
   real evidence bind mount, creates a temporary file, writes and fsyncs it,
   renames it within the same directory, reads it back, and removes it.
5. The wrapper starts the collector with the existing resource and security
   options. The collector performs only its existing read-only account checks.
6. Each output is written to a same-directory temporary file, fsynced, renamed
   atomically, and left mode `0600`. A successful service must read back both
   JSON files before exit.
7. On any failure, the wrapper exits with a fixed allowlisted code and emits one
   journal signal. `ExecStopPost`/`OnFailure` covers service failures for which
   `ExecStart` never executed, including `203/EXEC`.
8. The existing alert receiver consumes the stable signal. Delivery is a
   separate acceptance assertion; it is never fabricated by the wrapper.

## 6. Interfaces and Field Contracts

### 6.1 systemd contract

- `Type=oneshot`, existing timer relationship and schedule preserved.
- Host orchestration identity: `User=root` (and matching group settings).
- `ExecStart` points to the fixed wrapper path.
- `ExecStopPost` and/or `OnFailure` invokes the fixed failure helper even when
  `ExecStart` cannot be executed.
- Runtime drop-ins used for the controlled `203/EXEC` test are temporary,
  recorded, and removed with the original timer state restored.

### 6.2 Evidence contract

- Existing two JSON artifacts only; no new table or file contract.
- Final files are mode `0600`, valid JSON, and atomically replaced in place.
- A failed run preserves the last valid evidence; partial temporary files are
  cleaned up.
- UID 10002 real bind-mount verification must demonstrate write, fsync,
  same-directory rename, readback, and cleanup.

### 6.3 Failure signal schema

Schema version: `t10.failure.v1`.

Exactly one signal is emitted per failed service invocation, with stable fields:

```text
schema_version=t10.failure.v1
unit=sub2api-account-quality-monitor.service
service_result=<allowlisted systemd result>
exit_code=<allowlisted code or unknown>
exit_status=<numeric status or unknown>
failure_phase=<allowlisted phase>
reason_code=<allowlisted reason>
dedupe_key=<hash of stable non-secret fields>
```

The signal contains no raw stderr, real paths, commands, account identifiers,
models, responses, Admin-Key, or other credentials. Unknown values map to
`unknown`. The mapping is versioned, deterministic, and unique:

| Failure phase | Reason code |
| --- | --- |
| systemd | `systemd_exec_203` |
| preflight | `path_or_mode_preflight` |
| evidence | `uid10002_evidence_write` |
| credentials | `admin_key_read` |
| runtime | `docker_start_or_runtime` |
| collector | `collector_nonzero` |
| resource | `timeout_or_resource` |
| publish | `evidence_publish` |
| signal delivery | `failure_signal_delivery` (acceptance/release classification only) |
| any unmapped value | `unknown` |

`failure_signal_delivery` must not appear in a journal event claiming successful
journal emission. If the helper cannot write the journal, or the existing
receiver cannot confirm delivery, acceptance fails and the release is blocked;
the classification is recorded only in release/acceptance evidence or by an
independent receiver confirmation chain.

The fixed wrapper exit code or a helper allowlist performs the mapping. Tests
must prove one input maps to one code, unknown inputs are redacted to
`unknown`, and one failed run produces one unified signal.

## 7. Failure and Security Semantics

The chain is fail-closed: missing paths, unsafe modes, unreadable Admin-Key,
failed UID 10002 write, Docker/runtime errors, collector non-zero exit,
timeouts/resource limits, or failed atomic publication cause a non-success
exit and preserve the last valid evidence. No business mutation is permitted.

The container must retain UID 10002, read-only rootfs, all capabilities dropped,
no-new-privileges, noexec tmpfs, and the approved resource limits. The root
host process must not copy secrets into new locations or expose secret values in
journal, stdout, stderr, or alert payloads.

## 8. Compatibility and Migration

The existing timer name, schedule, evidence filenames, JSON shape, receiver,
and collector API remain compatible. The only required unit identity change is
the host orchestration user and failure-hook wiring. No data migration,
database migration, secret migration, ACL migration, or path migration is
allowed. Configuration changes must be reversible drop-ins or the reviewed unit
change, with a recorded pre-change snapshot.

## 9. Acceptance Matrix

| ID | Acceptance | Evidence required | Gate |
| --- | --- | --- | --- |
| A1 | Static unit/helper contract | Unit diff, modes/owners, timer relationship, hook inspection | Required |
| A2 | UID 10002 evidence mount | Actual container UID write/fsync/rename/readback/cleanup transcript | Required |
| A3 | Successful service | Real systemd run; both `0600` JSON files valid, atomically published and read back | Required |
| A4 | Controlled `203/EXEC` | Reversible runtime drop-in to harmless mode-0644 target; `203/EXEC` observed; timer state preserved/restored | Required |
| A5 | Failure signal | Exactly one redacted journal signal with schema, phase, reason, dedupe key | Required |
| A6 | Existing receiver delivery | Same controlled failure reaches existing on-call endpoint once | User-waived for this release; retain as unverified risk |
| A7 | Wrapper-stage failures | Inject preflight, Admin-Key, Docker, collector, timeout, and publish failures; verify unique mappings and no raw data | Required |
| A8 | Security invariants | Container UID/options/limits and unchanged production-tree mode | Required |
| A9 | No business writes | Before/after read-only database and account-operation evidence | Required |
| A10 | Natural timer window | 24 hours with zero `203/EXEC` failures and normal evidence cadence | Required |

If the existing receiver is absent or actual delivery cannot be verified, record
A6 as unverified under the user's explicit waiver. Do not fabricate receipt
evidence or create a new receiver to satisfy the matrix. A6 does not block this
release, but remains a residual operational risk.

## 10. Test Strategy

Tests cover static contract parsing, allowlist uniqueness, unknown-value
redaction, one-signal deduplication, real UID 10002 mount behavior, successful
systemd execution, each wrapper failure stage, and security invariants. The
`203/EXEC` test uses only a runtime drop-in, performs no account probe or
business write, preserves/restores timer state, and fails closed if restoration
is incomplete. The success test verifies both JSON files and modes after the
real service run. The natural-window check samples journal and timer state for
24 hours.

## 11. Release, Rollback, and Open Items

Release is allowed only after A1-A5 and A7-A9 pass, A6 is explicitly recorded
as user-waived/unverified, and the release controller confirms
`downtime_required=false`.
Install/reload operations must be reversible and retain pre-change unit and
timer snapshots. Rollback restores the prior unit/drop-in, disables the known
broken timer only under the release controller's explicit command, and
preserves the last valid evidence and all failure transcripts. Any failed merge,
deployment, or online verification keeps the candidate worktree and evidence.

Open items for root review:

1. Confirm host-level preflight access and whether unit reload requires a
   maintenance window; do not assume downtime is needed.

## 12.1 Root Review and Specification Closure (2026-08-15)

- Candidate refresh is complete: the candidate is detached at
  `main@8f79f1330fa007761b2a82af9a845529fbc5b31d`, with no source changes
  relative to `origin/main`; this specification is the only untracked task
  artifact restored after refresh.
- The existing receiver is identified as the healthy `relay-ops` service,
  using the native Sub2API `/api/v1/admin/ops/alert-events` projection and its
  existing Feishu outbound delivery path. T10 must reuse this chain and must
  not add a receiver, API, table, or parallel control plane.
- Root review accepts Option A (root host orchestration plus a restricted
  `10002:10002` collector) and confirms the scope, security invariants,
  failure schema, rollback contract, and acceptance matrix are closed enough
  to draft the implementation plan.
- A6 is explicitly waived by the user on 2026-08-15. The controlled
  `203/EXEC` delivery drill is not required to unblock implementation or this
  release, but the absence of delivery evidence remains an unverified residual
  risk. Healthy receiver presence is not treated as delivery evidence, and no
  receipt may be fabricated.
- No implementation, deployment, production mutation, or global queue/progress
  edit is authorized by this specification update. The next permitted action
  is drafting the task-specific writing-plans document.

## 12. Specification Self-review

- Scope excludes all business, UI, routing, billing, and external-control work.
- Root orchestration is separated from the UID 10002 collector boundary.
- Actual bind-mount write and actual systemd success are distinct acceptance
  items.
- `ExecStopPost`/`OnFailure` explicitly covers an unexecuted `ExecStart` and
  controlled `203/EXEC`.
- Failure mapping is fixed, versioned, unique, redacted, and includes the
  required diagnostic stages; delivery failure is not falsely self-reported.
- Existing receiver reuse and its blocking semantics are explicit.
- No migration, destructive action, production operation, or implementation
  plan is included.
- No placeholders or unresolved contradictions remain beyond the three root
  review open items above.

The worktree is intentionally stopped before implementation. The specification
is closed for implementation, with A6 recorded as a user-waived, unverified
residual risk rather than a release blocker.
