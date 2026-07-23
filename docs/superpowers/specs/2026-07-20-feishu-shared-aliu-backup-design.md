# Feishu Shared Aliu Backup Design

**Status:** Approved for implementation and production dry-run configuration on 2026-07-20.

## Goal

Allow the existing deterministic Feishu routing controller to use Aliu account `2` as the shared backup for both `GPT-Pro` and `GPT-Plus` without allowing concurrent commands to overwrite the shared account's Sub2API `group_ids`.

## Approved Production Route

| Public group | Primary account | Backup account |
| --- | ---: | ---: |
| GPT-Pro (`2`) | Neko (`7`) | Aliu (`2`) |
| GPT-Plus (`6`) | Wawazz (`8`) | Aliu (`2`) |

Neko account `9` remains present, unbound and unschedulable. It is not deleted or reused by this change.

## Configuration Rules

- The routing file still contains exactly `GPT-Pro` and `GPT-Plus`.
- Public group IDs remain unique and positive.
- Primary account IDs remain unique and positive.
- A primary account cannot appear in any backup role.
- Backup account IDs remain positive and may be reused by both groups.
- Each route's primary and backup must differ.
- The existing strict JSON, file mode, model and platform validation remains unchanged.

## Concurrency Control

Every switch command acquires transaction-scoped PostgreSQL advisory locks for three resources:

1. the public group ID;
2. the primary account ID;
3. the backup account ID.

Resource keys include their namespace (`group` or `account`), are deduplicated and sorted before acquisition. This gives all workers the same lock order and prevents deadlock. Commands for unrelated groups with unrelated accounts may still execute concurrently, while both groups serialize whenever they share Aliu.

The lock transaction surrounds the complete Sub2API read, preflight, write and verification sequence. No Sub2API database tables are written directly.

## Routing Behavior

The controller continues to call only Sub2API `v0.1.161` native Admin API methods. It never sends `confirm_mixed_channel_risk`.

Binding a shared backup adds only the requested public group to the current `group_ids`. Restoring a primary removes only that public group from Aliu. The other group's binding is preserved. The existing target-first, verify, source-remove, final-verify sequence remains unchanged.

## Deployment Boundary

- Keep `RELAY_OPS_MODE=read_only`.
- Keep `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`.
- Rebuild and recreate only `relay-ops`.
- Do not recreate Sub2API, PostgreSQL, Redis or Caddy.
- Update `/opt/sub2api/secrets/feishu-routing.json` to the approved shared Aliu route.
- Do not perform a real failover or recovery in this change.
- Do not send invitations or publish broader bot configuration changes.

## Acceptance

- Configuration accepts a backup reused only as a backup.
- Configuration rejects duplicate primaries and any primary/backup role overlap.
- Store tests prove shared-account commands serialize while unrelated routes remain concurrent.
- Controller tests prove adding/removing one group preserves the shared backup's other group binding.
- `go test ./... -race -count=1`, `go vet ./...` and the relay-ops contract script pass.
- Production remains healthy with unchanged base container IDs, `read_only/dry_run` modes and both groups still reported as `primary`.
- Dry-run switch predictions for both groups target Aliu without changing Sub2API route state.

