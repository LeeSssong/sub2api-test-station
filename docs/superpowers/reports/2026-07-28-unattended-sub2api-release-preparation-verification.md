# Verification: Unattended Sub2API Release Preparation

**Date:** 2026-07-28
**Status:** local implementation verified; production activation pending

## Scope

- Discover the latest stable official Sub2API Release every six hours.
- Reproduce the official/customized three-way merge without credentials.
- Qualify and publish one private immutable Linux AMD64 image to GHCR.
- Pull and verify the candidate on production through a forced SSH command.
- Prove that candidate staging does not change the running container or
  production Compose.
- Advance qualified source with compare-and-swap fast-forward semantics.
- Send a fact-only Feishu success or stable-code failure card.

## Commands / Steps

Implementation followed test-first checkpoints for:

- Release metadata normalization and malicious Release body handling.
- Deterministic three-way merge and conflict rollback.
- Production candidate loader and atomic state.
- Forced SSH packaging and installer boundaries.
- Feishu renderer and strict event CLI.
- Immutable GHCR publishing, source compare-and-swap, and workflow contracts.

Fresh complete local verification:

```bash
for test_file in tests/operations/*_test.rb; do ruby "$test_file"; done
for test_file in tests/operations/*_test.sh; do bash "$test_file"; done
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
go -C relay-ops-service test ./... -count=1
go -C relay-ops-service vet ./...
bash upstream/sub2api/deploy/test-caddyfile-cache.sh
git diff --check
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 \
  .github/workflows/sub2api-release-preparation.yml
```

GitHub Actions and production evidence will be appended after activation.

## Results

Implemented commits:

- `0350d8496`: normalized official Release metadata.
- `9cc29b5ad`: reproducible customized source merge.
- `6c983b34c`: immutable production candidate loader.
- `b7f70e7c2`: forced SSH boundary and packaging.
- `e0e8d55d1`: fact-only release preparation cards.
- `249720c6a`: pinned GitHub Actions workflow, private GHCR publisher, and
  compare-and-swap source advancement.

Local workflow-specific verification:

- Release card and CLI target tests passed.
- Publisher tests: 4 cases, 25 assertions.
- Source advancement tests: 3 cases, 16 assertions.
- Workflow tests: 7 cases, 125 assertions.
- `bash -n` passed for both trusted release scripts.
- Actionlint `v1.7.7` returned exit code `0`.

Complete local regression:

- all 10 Ruby operation files passed: 56 tests, 465 assertions, zero failures
  and zero errors;
- every Shell operation contract passed;
- the packaging test skipped only local `systemd-analyze`, which is unavailable
  on macOS and remains a required production-host check;
- all `sub2api-updater` packages passed `go test ./... -count=1` and
  `go vet ./...`;
- all `relay-ops-service` packages passed `go test ./... -count=1` and
  `go vet ./...`;
- the Caddy cache/reverse-proxy contract passed;
- YAML parsing and `git diff --check` passed.

## Evidence

The production mutation boundary is enforced twice:

1. The forced SSH wrapper accepts only
   `prepare <digest-ref> <version> <official-commit> <source-commit>`.
2. The candidate loader snapshots container ID, image ID, start time, status,
   health, restart count, and Compose SHA-256 before and after pull,
   qualification, isolated version execution, and local tagging. Any difference
   fails the run.

The loader contains no Compose, update API, database client, container
lifecycle, or prune path. GHCR credentials enter through stdin, live only in a
temporary Docker config, and are absent from state and notification artifacts.

## Not Verified

- GitHub Environment secrets and package permissions are not yet configured.
- The candidate loader and forced SSH key are not yet installed on production.
- No real `workflow_dispatch` has run from `main`.
- No real private GHCR digest, production candidate image ID, Actions run URL,
  or Feishu delivery evidence exists yet.

## Follow-ups

1. Run the complete repository verification and review the final diff.
2. Push the feature branch and fast-forward `main` only if remote `main`
   remains at the recorded base.
3. Install the production candidate loader and dedicated forced key.
4. Configure the GitHub Environment without an approval gate.
5. Dispatch the workflow and append exact no-mutation evidence to this report.
