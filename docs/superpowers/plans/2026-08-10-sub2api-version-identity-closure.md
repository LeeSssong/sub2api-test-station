# Sub2API v0.1.173 Version Identity Closure Plan

## Goal

Ensure every qualified upstream candidate carries one consistent release version across metadata, candidate source, built binary, image labels, updater status, and the administrator UI.

## Context

- Production promotion `20260810T194408Z-production-3545218` succeeded from `main@0b8377a971e95edff8a9a332dbfbedcc40932128`.
- The promoted source contains `upstream/sub2api/backend/cmd/server/VERSION=0.1.172` although release metadata and image upstream labels target `0.1.173`.
- `upstream/sub2api/Dockerfile` resolves the binary version from that file when no `VERSION` build argument is supplied.
- Production therefore truthfully displays `v0.1.172`; this is a release qualification defect, not a browser cache issue.

## Constraints

- Preserve the two protected worktrees unchanged.
- Do not use GitHub Actions.
- Add a regression that fails on the current tree before changing production code.
- Fail closed when metadata version and candidate source VERSION disagree.
- Do not weaken candidate tree, provenance, migration, image, or deployment checks.
- All production deployment must come from a reviewed, pushed `main` with fresh final-tree evidence.

## Task 1: Candidate version identity contract

1. Add a real merge/release regression proving a candidate targeting `0.1.173` is rejected if its candidate source VERSION remains `0.1.172`.
2. Run the focused test and capture the expected RED.
3. Implement the smallest merge qualification fix so the official target VERSION is present in the candidate and mismatches fail closed before publication.
4. Run focused and full merge/release tests, Bash syntax checks, and `git diff --check`.
5. Commit and write an implementation report.
6. Obtain independent task review; address any findings.

## Task 2: Integrate and republish

1. Fast-forward the reviewed task into `main`, rerun final-tree gates, write fresh 0600 evidence, and push `main`.
2. Build and deploy the preloaded Linux/AMD64 candidate from the verified `main` through the reviewed host blue-green chain.
3. Verify production release state, binary version `0.1.173`, health/ready/home/auth boundary, updater state, administrator version UI, New API/Sub upstream-cost behavior, and account monitor third-and-later card click tooltips.
4. Update the project ledger with exact release evidence and remaining GHCR credential blocker.
5. Run final whole-branch review before cleanup.

## Done when

- `main` and `origin/main` point to the reviewed fix.
- Production runs that exact source/tree and reports version `0.1.173`.
- The release record is `succeeded/promoted` and public/administrative acceptance passes.
- The project ledger is pushed and the two protected worktrees remain intact.
