# Official Sub2API v0.1.175 Production Release

## Result

- Status: completed and production-verified.
- Official source: tag `v0.1.175`, commit `93c32fa1a2450351561abc46156d2e28cb5f74ca`.
- Production source commit: `350e050575377d8e31ed153624bb19da3591517f`.
- Production source tree: `47e98861f921bebb6d62e41e8a44c142d4d7fe4f`.
- Production acceptance and this report are pushed to `origin/main`.
- Final review: `APPROVE` in commit `011bf2b0c48f775e624c82bff3bceddf8e12c69a`.

## Qualification

The merged production tree passed the focused handler/service Go tests and vet, UsageView/UsageTable Vitest (39/39), frontend typecheck and production build, version/provenance checks, and `git diff --check`.

The mode-0600 evidence file is:

`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-12-main-350e050575377d8e31ed153624bb19da3591517f-v1.json`

There are no migration path changes in the official merge. The migration-set hash remains `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`.

## Deployment

- Release record: `/var/lib/sub2api/release-records/20260812T212526Z-production-1519752.json`.
- Result/state: `succeeded/promoted`.
- `rolled_back=false`.
- `downtime_required=false`.
- Active slot: green.
- Image: `ghcr.io/leesssong/xingqiao-sub2api:release-350e050575377d8e31ed153624bb19da3591517f-2f73be08b1d62bbdb40c25cd3f049bdcdd3501eeaea8fe71982ba1eb06adc566`.
- Green API and worker are healthy. The previous blue slot remains healthy for rollback.
- PostgreSQL, Redis, and Caddy were not replaced.

The first Task 3 controller call had no release profile loaded and stopped before SSH, image build, or host mutation. A later authorized release execution loaded the established operator-controlled profile and completed the deployment above. No credentials are recorded in this report.

## Online Acceptance

- `https://api.xingqiaolab.top/healthz`: HTTP 200, `alive`.
- `https://api.xingqiaolab.top/readyz`: HTTP 200, `ready`.
- `https://api.xingqiaolab.top/health`: HTTP 200, `ok`.
- Authenticated administrator version state: `0.1.175`.
- Authenticated Usage stats/list: HTTP 200 with data.
- Unauthenticated protected administrator endpoint: expected HTTP 401.
- Read-only final recheck after documentation push reconfirmed all three public health endpoints and the healthy green API/worker image identity.

## Handoff

T03-R1 was not modified by this release. Its required new production code baseline is `350e050575377d8e31ed153624bb19da3591517f`; it should merge or rebase this baseline before continuing. The rollback target is the still-healthy blue slot running the previous `be9e124d65c…` image.
