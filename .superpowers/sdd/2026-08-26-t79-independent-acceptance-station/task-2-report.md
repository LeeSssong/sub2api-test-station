# T79 Task 2 implementation report

## Scope

Implemented the local, acceptance-only release controller and its delivery
contract. No production release script, root `main`, global queue, progress
ledger, production configuration, or deployment target was changed.

## TDD evidence

1. Added `tests/acceptance_station/release_delivery_contract_test.sh` first.
2. Ran `bash tests/acceptance_station/release_delivery_contract_test.sh` before
   the controller existed. It failed as expected with:
   `FAIL: ops/release-sub2api-acceptance.sh is missing`.
3. Added `ops/release-sub2api-acceptance.sh` and made both files executable.

## Delivered behavior

- Refuses a dirty Git worktree before any SSH or SCP operation.
- Requires an absolute, regular, non-symlink `ACCEPTANCE_ENV_FILE` at mode
  `0600`, and never prints its contents.
- Requires `ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES`, refuses
  production identities and refuses mock/lab provider declarations.
- Resolves the exact source commit and tree, builds one Linux/amd64 image from
  `upstream/sub2api` using `docker buildx build --platform linux/amd64 --load`,
  saves a tar archive and produces a SHA-256 sidecar.
- Packages the independent Compose/Caddy/env and source identity into a private
  bundle, creates a remote temporary staging directory through SSH, transfers
  the image and bundle through SCP, and invokes only the dedicated acceptance
  host executor through `sudo -n bash -s`.

## Verification

- `bash -n ops/release-sub2api-acceptance.sh tests/acceptance_station/release_delivery_contract_test.sh` — PASS
- `git diff --check` — PASS
- `bash tests/acceptance_station/release_delivery_contract_test.sh` — expected
  transitional failure: `ops/deploy-sub2api-acceptance-host.sh` is intentionally
  absent until T79 Task 3. The controller-specific static contracts are present.

## Concern / handoff

The full release-delivery contract cannot pass until Task 3 supplies the
dedicated host executor. No acceptance-host deployment was attempted: operator
provided independent SSH/DNS and real-provider credentials are still required.
