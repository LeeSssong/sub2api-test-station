# Task 8 Qualification Gate Update

## Status

`IN_PROGRESS`: local implementation and verification only. No push, merge,
deployment, production access, traffic switch, or GitHub Actions workflow.

## Changes

- Qualification no longer writes an unconditional `ready` state.
- A root-owned absolute `QUALIFICATION_COMMAND` must return a complete JSON
  evidence object bound to the requested tag.
- The script validates release identity, SHA-256 checksum, adapter/contract
  versions, non-empty tests and data diff, `expand_only` migration class, and
  `passed=true`; invalid evidence writes `blocked` and exits non-zero.
- `ready_until` is persisted for one hour and promotion rejects expired state.
- Promotion and rollback require an executable absolute `SUB2API_HOST_EXECUTOR`
  and delegate the actual traffic operation to it.
- Added `tests/operations/qualification_scripts_test.sh` covering valid evidence,
  executor delegation, and missing-evidence fail-closed behavior.

## Verification

```text
bash tests/operations/qualification_scripts_test.sh       PASS
cd sub2api-updater && go test ./...                       PASS
cd sub2api-updater && go vet ./...                        PASS
git diff --check                                          PASS
```

Remaining acceptance work is to connect the reviewed host executor to the
real inactive-slot discovery, isolated database migration/contract tests, and
blue-green promotion rehearsal, then push, deploy, and verify online.
