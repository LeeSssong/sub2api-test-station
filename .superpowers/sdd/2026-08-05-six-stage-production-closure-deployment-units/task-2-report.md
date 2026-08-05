CONTEXT_ACK=2026-08-05-six-stage-production-closure
TASK_BRIEF=/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-monitor-completion/.superpowers/sdd/2026-08-05-six-stage-production-closure-deployment-units/task-2-brief.md
DEPLOYMENT_GATE=yes-coordinator-only

# Task 2 implementation report

Status: deployment-prequalified; no push and no production deployment performed.

Base and tested canonical commit before this report commit: `7f7093be83a56116a123ef978b7dfcee199f3657`. No scoped source file failed validation and no runtime code was modified. The report commit is intentionally followed by evidence generation so the evidence can bind the exact committed source tree accepted for release.

## Scoped validation

```text
cd relay-ops-service && go test ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store -count=1
# PASS: accounting, reconciliation, app, config, http, store

cd relay-ops-service && go vet ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store
# PASS: exit 0, no diagnostics

bash tests/operations/release_relay_ops_test.sh
# PASS: relay-ops release controller

bash tests/operations/deploy_relay_ops_host_test.sh
# PASS: relay-ops host executor
```

## Disabled contract and protected surface

- Compose defaults `RELAY_OPS_ACCOUNTING_ENABLED` to `false`; production secret must remain explicitly `false`.
- Disabled accounting creates no accounting service and does not mount `/relay-ops/accounting`; the scoped app test proves `404`.
- Reconciliation endpoints remain admin-auth protected; scoped HTTP coverage proves an unauthenticated request returns `401`.
- The immutable release controller validates clean-tree, source-commit/tree and migrations hash binding before any Docker or SSH transport.

## Evidence handoff

`RELAY_OPS_EVIDENCE_FILE=/private/var/tmp/relay-ops-task2-evidence.4BcvMC/relay-ops-test-evidence.json`

The evidence is generated at mode `0600` after this report commit and is bound to its exact commit/tree/migrations hash. Only the coordinator may use it with `ops/release-relay-ops.sh --mode production --evidence "$RELAY_OPS_EVIDENCE_FILE"` and then perform the required production checks. This implementation agent did not invoke that script, push, deploy, migrate production, or alter `docs/project/project-progress.md`.

Concern: Task 2 remains in progress until the coordinator has pushed, deployed the immutable image with accounting disabled, completed the limited production verification, and recorded `awaiting_user_acceptance`; local evidence is not production completion.

## 2026-08-05 approved build-proxy deployment-blocker fix

The amended Task 2 brief authorizes only the build-time Go module proxy interface. No runtime environment, accounting setting, migration, production evidence, production host, or `docs/project/project-progress.md` was changed.

### TDD record

The test names the break it catches: a release controller that either omits the approved build proxy from `buildx` or accepts an unapproved proxy before transport. It executes the real `ops/release-relay-ops.sh` controller through its existing fake Docker/SCP/SSH boundary and observes the controller's external command behavior.

RED command:

```text
bash tests/operations/release_relay_ops_test.sh
# exit 1
# FAIL: default build GOPROXY was not passed to buildx
```

The RED occurred because the pre-fix controller neither supplied a Docker build argument for its default `https://proxy.golang.org,direct` value nor validated `RELAY_OPS_BUILD_GOPROXY` before transport.

Minimal implementation:

- `infra/Dockerfile.relay-ops` declares `ARG GOPROXY=https://proxy.golang.org,direct` and applies it only to `go mod download`.
- `ops/release-relay-ops.sh` accepts `RELAY_OPS_BUILD_GOPROXY`, defaults it to the same value, permits exactly `https://proxy.golang.org,direct` and `https://goproxy.cn,direct`, and passes it to both registry and preloaded `buildx build` paths as `--build-arg GOPROXY=...`.
- The Dockerfile does not set `GOSUMDB=off` or `GONOSUMDB=*`; Go checksum verification remains enabled.

GREEN command:

```text
bash tests/operations/release_relay_ops_test.sh
# PASS: relay-ops release controller
```

The new fake-Docker cases prove the default value reaches buildx, the approved `https://goproxy.cn,direct` override reaches buildx, and `https://mirror.example.invalid,direct` is rejected before Docker, SCP, or SSH is invoked.

### Required scoped regression verification

```text
cd relay-ops-service && go test ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store -count=1
# PASS: six packages

cd relay-ops-service && go vet ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store
# PASS: exit 0, no diagnostics

bash tests/operations/release_relay_ops_test.sh
# PASS: relay-ops release controller

bash tests/operations/deploy_relay_ops_host_test.sh
# PASS: relay-ops host executor; exit 0
```

Deployment gate remains `yes-coordinator-only`. The coordinator may set `RELAY_OPS_BUILD_GOPROXY=https://goproxy.cn,direct` for this one image build only; it is not a production runtime secret and must not alter the explicit production setting `RELAY_OPS_ACCOUNTING_ENABLED=false`. No push, deployment, or production evidence generation occurred in this fix round.

The previous evidence file binds the pre-fix `de739afaf` tree and is no longer valid for this release. A coordinator-owned evidence writer run is required after this fix commit is pushed and before invoking the production controller.
