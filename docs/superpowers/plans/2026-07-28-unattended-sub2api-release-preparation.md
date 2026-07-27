# Sub2API Official Release Preparation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect the latest stable official Sub2API release every six hours,
qualify a customized Linux AMD64 image outside production, stage and verify the
immutable image on production, advance the qualified source, and notify Feishu
without switching the running container.

**Architecture:** GitHub Actions separates untrusted upstream build execution
from jobs that hold repository, package, production, or Feishu credentials.
Small Ruby tools normalize release metadata and reproduce the three-way source
merge. A Go candidate loader, reachable only through a forced SSH command,
performs bounded image-store operations and proves the production runtime did
not change. Existing relay-ops notification primitives render and deliver the
result.

**Tech Stack:** GitHub Actions, Ruby/Minitest, Git, Go, Docker Buildx, private
GHCR, OpenSSH forced commands, Feishu webhook

## Source

- Design:
  `docs/superpowers/specs/2026-07-28-unattended-sub2api-release-preparation-design.md`
- Existing host updater:
  `sub2api-updater/internal/updater/resolver.go`
- Existing notification renderer:
  `relay-ops-service/internal/notify/feishu.go`
- Existing release runbook:
  `docs/runbooks/sub2api-official-image-release.md`

## Global Constraints

- Schedule is `17 */6 * * *`; `workflow_dispatch` is the only manual trigger.
- Only the latest non-draft, non-prerelease official Release is eligible.
- Build and test jobs receive no production, Feishu, repository-write, or
  package-write credential.
- Candidate image is single-platform `linux/amd64`.
- GHCR version tags are immutable; mismatched existing content is an error.
- Production receives an exact digest and never builds source.
- The production command must not call Docker Compose, the update API, a
  database client, container restart, or any Docker prune operation.
- The running container identity, image, start time, health, restart count, and
  production Compose SHA-256 must be identical before and after staging.
- Source advances only with a compare-and-swap fast-forward; no force push.
- Feishu cards contain facts and no fixed “下一步” or click-update instruction.
- No secret value is written to Git, artifacts, command arguments, logs, or
  notification text.

---

### Task 1: Normalize Official Release Metadata

**Files:**
- Create: `ops/sub2api-release-metadata.rb`
- Create: `tests/operations/sub2api_release_metadata_test.rb`

**Interfaces:**
- Consumes:
  `discover --release FILE --provenance FILE --base-sha SHA --output FILE`
- Produces: normalized JSON with `has_update`, `base_sha`, `base_version`,
  `base_commit`, `version`, `tag`, `official_commit`, `name`, `body`,
  `published_at`, and `html_url`.
- Consumes:
  `advance-provenance --metadata FILE --provenance FILE --imported YYYY-MM-DD`
- Produces: an atomically rewritten `XINGQIAO_UPSTREAM.md`.

- [ ] **Step 1: Write failing metadata tests**

Add literal fixtures for:

```ruby
{
  "tag_name" => "v0.1.167",
  "name" => "v0.1.167",
  "body" => "Official fixes",
  "published_at" => "2026-07-28T01:02:03Z",
  "html_url" => "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.167",
  "draft" => false,
  "prerelease" => false,
  "target_commitish" => "0123456789abcdef0123456789abcdef01234567"
}
```

Tests must prove:

- stable versions newer than provenance produce `has_update=true`;
- the same version and commit produce `has_update=false`;
- older, draft, prerelease, malformed version, malformed commit, non-GitHub URL,
  invalid timestamp, and oversized JSON fail closed;
- multiline Release body remains JSON data and never becomes shell output;
- provenance rewrite changes only Release tag, source commit, annotated tag
  when supplied, and imported date;
- the output file is mode `0600` and is replaced atomically.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
ruby tests/operations/sub2api_release_metadata_test.rb
```

Expected: failure because `ops/sub2api-release-metadata.rb` does not exist.

- [ ] **Step 3: Implement the metadata CLI**

Implement a pure `Sub2APIReleaseMetadata` module and a thin CLI. Use
`JSON.parse`, `Time.iso8601`, `Gem::Version`, bounded file reads, exact
`github.com/Wei-Shaw/sub2api` Release URLs, and `Tempfile` +
`File.rename`. Do not perform HTTP requests or execute child processes.

- [ ] **Step 4: Verify GREEN and mutation cases**

Run:

```bash
ruby tests/operations/sub2api_release_metadata_test.rb
```

Expected: all metadata tests pass. Manually mutate the version comparison in
the test worktree, confirm the newer-version test fails, then restore it.

- [ ] **Step 5: Commit**

```bash
git add ops/sub2api-release-metadata.rb \
  tests/operations/sub2api_release_metadata_test.rb
git commit -m "feat: normalize Sub2API release metadata"
```

### Task 2: Reproduce The Customized Three-Way Merge

**Files:**
- Create: `ops/merge-sub2api-release.sh`
- Create: `tests/operations/merge_sub2api_release_test.rb`

**Interfaces:**
- Consumes:
  `--root PATH --metadata FILE --official-repository URL --bundle FILE
  --report FILE`
- Produces: a candidate source commit whose parent is `metadata.base_sha`, a
  Git bundle containing `refs/heads/candidate-artifact`, and a `0600` report
  containing the allowed changed paths and candidate commit.

- [ ] **Step 1: Write failing fixture-repository tests**

Build temporary official and root Git repositories. Tests must prove:

- official base + Xingqiao overlay + newer official commit merge without
  conflict and preserve both changes;
- a real same-line conflict exits non-zero without changing root HEAD;
- deleted official files are deleted from the resulting snapshot;
- `.git` is never copied into `upstream/sub2api`;
- only `upstream/sub2api/**`,
  `docs/project/current-state.md`, and the release verification report may
  change;
- candidate commit parent equals the supplied base SHA;
- fixed Release publication time makes candidate commit deterministic;
- output bundle imports as `candidate-artifact`;
- failure removes temporary repositories and does not leave credentials or
  arbitrary official paths in the report.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
ruby tests/operations/merge_sub2api_release_test.rb
```

Expected: failure because `ops/merge-sub2api-release.sh` does not exist.

- [ ] **Step 3: Implement the merge script**

Use `mktemp -d`, `git clone --no-checkout`, `rsync --delete`, fixed Git author
and dates, `git merge --no-ff --no-edit`, and the Task 1 CLI for provenance.
Install a trap before creating temporary state. Validate every SHA and path
before use. Never evaluate metadata as shell.

- [ ] **Step 4: Verify GREEN**

Run:

```bash
ruby tests/operations/merge_sub2api_release_test.rb
bash -n ops/merge-sub2api-release.sh
```

Expected: all merge behaviors pass.

- [ ] **Step 5: Commit**

```bash
git add ops/merge-sub2api-release.sh \
  tests/operations/merge_sub2api_release_test.rb
git commit -m "feat: automate qualified upstream source merge"
```

### Task 3: Build The Fail-Closed Production Candidate Loader

**Files:**
- Create: `sub2api-updater/internal/candidate/loader.go`
- Create: `sub2api-updater/internal/candidate/state.go`
- Create: `sub2api-updater/internal/candidate/loader_test.go`
- Create: `sub2api-updater/internal/candidate/state_test.go`
- Create: `sub2api-updater/cmd/sub2api-candidate-loader/main.go`
- Create: `sub2api-updater/cmd/sub2api-candidate-loader/main_test.go`

**Interfaces:**

```go
type Request struct {
    Reference      string
    Version        string
    OfficialCommit string
    SourceCommit   string
    RegistryToken  []byte
}

type RuntimeSnapshot struct {
    ContainerID   string `json:"container_id"`
    ImageID       string `json:"image_id"`
    StartedAt     string `json:"started_at"`
    Status        string `json:"status"`
    Health        string `json:"health"`
    RestartCount  int    `json:"restart_count"`
    ComposeSHA256 string `json:"compose_sha256"`
}

type Result struct {
    Version        string          `json:"version"`
    Reference      string          `json:"reference"`
    ImageID        string          `json:"image_id"`
    OfficialCommit string          `json:"official_commit"`
    SourceCommit   string          `json:"source_commit"`
    Runtime        RuntimeSnapshot `json:"runtime"`
}

func (l *Loader) Prepare(context.Context, Request) (Result, error)
```

- [ ] **Step 1: Write failing loader tests**

Use a deterministic fake command runner and disk checker. Tests must prove:

- only `ghcr.io/leesssong/xingqiao-sub2api@sha256:<64 hex>` is accepted;
- version and both commits are strictly validated;
- token is read from stdin, bounded, and absent from args, state, stdout, and
  errors;
- temporary `DOCKER_CONFIG` is `0700` and removed on success and every failure;
- disk below 5 GiB free or at/above 85% fails before pull;
- pull uses `--platform linux/amd64` and the exact digest;
- inspect requires `linux/amd64`, exact image ID, and all four labels;
- version check uses `--network none --read-only --cap-drop ALL
  --security-opt no-new-privileges`;
- binary output must contain the exact version and official commit;
- tag is exactly `xingqiao-sub2api:upstream-<version>`;
- before/after runtime snapshots and Compose hashes must be identical;
- no command contains `compose`, update API paths, database clients, restart,
  stop, kill, or prune;
- state is atomically written at mode `0600`;
- a matching existing state and tag is an idempotent no-op.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go -C sub2api-updater test ./internal/candidate \
  ./cmd/sub2api-candidate-loader -count=1
```

Expected: packages or symbols do not exist.

- [ ] **Step 3: Implement the loader and CLI**

Follow the existing updater `CommandRunner` style but keep the candidate
package independent of the mutation executor. The CLI accepts exactly:

```text
prepare <digest-ref> <version> <official-commit> <source-commit>
```

It reads the non-secret registry user and fixed registry prefix from the
root-owned `/etc/sub2api/sub2api-candidate-loader.env` file and reads the token
only from stdin. Tests inject a temporary config path. Production defaults are:

```text
container: sub2api-sub2api-1
compose: /opt/sub2api/production/compose.yaml
state: /var/lib/sub2api-candidate-loader/state.json
registry: ghcr.io/leesssong/xingqiao-sub2api
```

- [ ] **Step 4: Verify GREEN and full updater regression**

Run:

```bash
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
```

Expected: all tests and vet pass.

- [ ] **Step 5: Commit**

```bash
git add sub2api-updater/internal/candidate \
  sub2api-updater/cmd/sub2api-candidate-loader
git commit -m "feat: add immutable production candidate loader"
```

### Task 4: Package The Forced SSH Boundary

**Files:**
- Create: `ops/sub2api-candidate-ssh.sh`
- Create: `ops/install-sub2api-candidate-loader.sh`
- Create: `infra/sub2api-candidate-loader.env.example`
- Create: `tests/operations/install_sub2api_candidate_loader_test.sh`
- Create: `tests/operations/sub2api_candidate_ssh_test.rb`

**Interfaces:**
- `sub2api-candidate-ssh.sh` reads `SSH_ORIGINAL_COMMAND`, accepts one exact
  `prepare` command, and executes the root-owned loader through sudo.
- Installer consumes a prebuilt Linux AMD64 loader and a dedicated Ed25519
  public key file.

- [ ] **Step 1: Write failing packaging and forced-command tests**

Tests must prove:

- empty command, shell metacharacters, extra args, PTY shell requests, wrong
  registry, malformed digest/version/commits, and newline injection are
  rejected;
- a valid command forwards exactly four public arguments and token stdin;
- stdout is one bounded JSON result and stderr does not contain stdin;
- authorized key uses `restrict,command="/usr/local/libexec/sub2api-candidate-ssh"`;
- installer creates root-owned `0755` binaries, `0700` state directory, `0600`
  environment, and a minimal sudoers rule;
- installer accepts a prebuilt binary and cross-compiles only when Go exists;
- installer refuses non-Linux, non-root, non-default Docker context, wrong
  production directory, insecure public-key mode, and non-Ed25519 keys before
  mutation;
- templates contain no real host, key, token, webhook, or password;
- no installer or wrapper contains a Compose/update/database/prune operation.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
bash tests/operations/install_sub2api_candidate_loader_test.sh
ruby tests/operations/sub2api_candidate_ssh_test.rb
```

Expected: missing packaging files.

- [ ] **Step 3: Implement packaging**

The forced wrapper validates `SSH_ORIGINAL_COMMAND` without `eval`, sets a
minimal `PATH`, clears inherited environment, and uses:

```text
sudo /usr/local/libexec/sub2api-candidate-loader prepare ...
```

The sudoers rule permits only that binary. The dedicated key remains forced
even though it belongs to the existing `ubuntu` SSH account.

- [ ] **Step 4: Verify GREEN**

Run:

```bash
bash tests/operations/install_sub2api_candidate_loader_test.sh
ruby tests/operations/sub2api_candidate_ssh_test.rb
bash -n ops/sub2api-candidate-ssh.sh
bash -n ops/install-sub2api-candidate-loader.sh
```

- [ ] **Step 5: Commit**

```bash
git add ops/sub2api-candidate-ssh.sh \
  ops/install-sub2api-candidate-loader.sh \
  infra/sub2api-candidate-loader.env.example \
  tests/operations/install_sub2api_candidate_loader_test.sh \
  tests/operations/sub2api_candidate_ssh_test.rb
git commit -m "feat: restrict candidate staging over SSH"
```

### Task 5: Render And Send Release Preparation Cards

**Files:**
- Create: `relay-ops-service/internal/notify/release_preparation.go`
- Create: `relay-ops-service/internal/notify/release_preparation_test.go`
- Create: `relay-ops-service/cmd/release-prep-notify/main.go`
- Create: `relay-ops-service/cmd/release-prep-notify/main_test.go`

**Interfaces:**

```go
type ReleasePreparationView struct {
    Status           string   `json:"status"`
    Stage            string   `json:"stage"`
    Version          string   `json:"version"`
    ReleaseName      string   `json:"release_name"`
    ReleaseBody      string   `json:"release_body"`
    PublishedAt      string   `json:"published_at"`
    ReleaseURL       string   `json:"release_url"`
    OfficialCommit   string   `json:"official_commit"`
    SourceCommit     string   `json:"source_commit"`
    ImageDigest      string   `json:"image_digest"`
    ProductionImage  string   `json:"production_image_id"`
    RunningVersion   string   `json:"running_version"`
    RunningImage     string   `json:"running_image_id"`
    RunningHealth    string   `json:"running_health"`
    RunningStartedAt string   `json:"running_started_at"`
    ComposeSHA256    string   `json:"compose_sha256"`
    Checks           []string `json:"checks"`
    ErrorCode        string   `json:"error_code"`
    WorkflowURL      string   `json:"workflow_url"`
}

func RenderReleasePreparation(ReleasePreparationView) FeishuMessage
```

- [ ] **Step 1: Write failing renderer and CLI tests**

Tests must prove:

- success title is `Sub2API <version> 候选镜像已静默准备`;
- failure title is `Sub2API <version> 候选准备失败`;
- success contains official facts, gate summary, immutable digest, production
  image, running container evidence, unchanged Compose, and the no-mutation
  boundary;
- no card contains `下一步`, `请点击`, or a click-update instruction;
- Release body is safely truncated without breaking UTF-8 or 30 KiB card size;
- secret-like content in body and errors is redacted;
- failure accepts only stable error codes and does not render raw stderr;
- unsafe links are removed and official/workflow HTTPS links remain;
- CLI rejects unknown JSON fields, oversized input, invalid status, insecure
  webhook file mode, and secret values in command arguments;
- CLI passes the rendered card to the existing webhook Client.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go -C relay-ops-service test ./internal/notify \
  ./cmd/release-prep-notify -count=1
```

Expected: renderer and command do not exist.

- [ ] **Step 3: Implement renderer and CLI**

Use existing `newCardMessage`, `safeValue`, link filtering, webhook Client,
and 30 KiB enforcement. The CLI syntax is:

```text
release-prep-notify --event /absolute/event.json \
  --webhook-file /absolute/0600/webhook
```

- [ ] **Step 4: Verify GREEN and relay-ops regression**

Run:

```bash
go -C relay-ops-service test ./... -count=1
go -C relay-ops-service vet ./...
```

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/notify/release_preparation.go \
  relay-ops-service/internal/notify/release_preparation_test.go \
  relay-ops-service/cmd/release-prep-notify
git commit -m "feat: report Sub2API candidate preparation"
```

### Task 6: Add The Pinned GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/sub2api-release-preparation.yml`
- Create: `tests/operations/sub2api_release_workflow_test.rb`
- Create: `ops/publish-sub2api-candidate.sh`
- Create: `ops/advance-sub2api-source.sh`
- Create: `tests/operations/publish_sub2api_candidate_test.rb`
- Create: `tests/operations/advance_sub2api_source_test.rb`

**Interfaces:**
- Publish consumes a Docker archive, candidate bundle, validated metadata, and
  private GHCR target; it returns immutable digest and audit branch.
- Advance consumes candidate bundle and base SHA; it fast-forwards `main` only
  when remote `main` still equals base.

- [ ] **Step 1: Write failing publish, advance, and workflow tests**

Behavior tests must prove:

- existing matching GHCR content is reused without push;
- existing mismatched version content is never overwritten;
- new content is pushed once and returns an exact digest;
- candidate scripts or image entrypoints never execute in the credentialed
  publish job;
- audit branch is `automation/sub2api-upstream-<version>`;
- source advance succeeds only as a fast-forward from exact base;
- concurrent remote main change fails without force;
- schedule parses as minute 17 every 6 hours;
- concurrency has `cancel-in-progress: false`;
- `prepare` permissions are read-only and reference no production/Feishu
  secrets;
- `publish`, `stage-production`, `advance-source`, and `notify` each receive
  only their required permissions;
- production token travels through stdin, not args or artifacts;
- all action references are 40-character SHAs;
- notification checks out `${{ github.sha }}`, not candidate source;
- production staging precedes source advance and notification;
- no workflow step calls Compose, update API, database clients, restart, or
  prune.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
ruby tests/operations/publish_sub2api_candidate_test.rb
ruby tests/operations/advance_sub2api_source_test.rb
ruby tests/operations/sub2api_release_workflow_test.rb
```

Expected: missing scripts and workflow.

- [ ] **Step 3: Implement trusted publish and advance scripts**

Use Docker inspect/pull/push with exact tags and labels. Use `git ls-remote`,
bundle import, ancestry validation, and ordinary fast-forward push. Do not use
`eval`, mutable image references for production, or `--force`.

- [ ] **Step 4: Implement workflow**

Pin:

```text
actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02
actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093
actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5
actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020
actions/cache@5a3ec84eff668545956fd18022155c47e93e2684
docker/setup-buildx-action@e468171a9de216ec08956ac3ada2f0791b6bd435
```

Jobs are:

```text
discover -> prepare -> publish -> stage-production -> advance-source -> notify
```

`prepare` builds with `--provenance=false --sbom=false`,
`FRONTEND_NODE_MAX_OLD_SPACE_SIZE=3072`, and exports a Docker archive rather
than pushing directly. `stage-production` has `packages:read`, writes its
ephemeral SSH key and known-host data to `0600` files, and pipes the masked
token to the forced command. `notify` uses a cache key formed from normalized
version and stage to suppress repeated failure cards.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
ruby tests/operations/publish_sub2api_candidate_test.rb
ruby tests/operations/advance_sub2api_source_test.rb
ruby tests/operations/sub2api_release_workflow_test.rb
bash -n ops/publish-sub2api-candidate.sh
bash -n ops/advance-sub2api-source.sh
git diff --check
```

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/sub2api-release-preparation.yml \
  ops/publish-sub2api-candidate.sh \
  ops/advance-sub2api-source.sh \
  tests/operations/publish_sub2api_candidate_test.rb \
  tests/operations/advance_sub2api_source_test.rb \
  tests/operations/sub2api_release_workflow_test.rb
git commit -m "feat: automate Sub2API candidate preparation"
```

### Task 7: Document Operations And Run Full Local Verification

**Files:**
- Modify: `docs/runbooks/sub2api-official-image-release.md`
- Modify: `docs/project/current-state.md`
- Create:
  `docs/superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md`

- [ ] **Step 1: Update durable operations documentation**

Document schedule, workflow permissions, GHCR naming, forced SSH key rotation,
ephemeral package credentials, production state path, rerun behavior, failure
codes, and the exact proof that staging is not deployment.

- [ ] **Step 2: Run local verification**

Run:

```bash
for test_file in tests/operations/*_test.rb; do ruby "$test_file"; done
for test_file in tests/operations/*_test.sh; do bash "$test_file"; done
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
go -C relay-ops-service test ./... -count=1
go -C relay-ops-service vet ./...
bash upstream/sub2api/deploy/test-caddyfile-cache.sh
git diff --check
```

Expected: all tests pass.

- [ ] **Step 3: Review the complete diff**

Confirm:

- no secret, real SSH host, private key, webhook, token, or credential file is
  tracked;
- no candidate path can mutate production runtime;
- no untrusted job has a write or external secret;
- workflow and docs agree on completion semantics;
- all new functions have tests that were observed failing first.

- [ ] **Step 4: Commit documentation**

```bash
git add docs/runbooks/sub2api-official-image-release.md \
  docs/project/current-state.md \
  docs/superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md
git commit -m "docs: record unattended release preparation"
```

### Task 8: Push, Install, Activate, And Verify The Real Workflow

**Files:**
- Modify:
  `docs/superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md`

- [ ] **Step 1: Capture production and repository before-state**

Read-only checks:

```bash
ssh -o BatchMode=yes sub2api-prod \
  'cd /opt/sub2api/production &&
   sha256sum compose.yaml &&
   docker inspect sub2api-sub2api-1 \
     --format "{{.Id}} {{.Image}} {{.State.StartedAt}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}} {{.RestartCount}}"'
git ls-remote origin refs/heads/main
```

Record values without printing secrets.

- [ ] **Step 2: Build and install the candidate loader**

Build:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go -C sub2api-updater build \
  -o ../.staging/sub2api-candidate-loader \
  ./cmd/sub2api-candidate-loader
```

Generate a dedicated Ed25519 key in a `0700` temporary directory, transfer only
the public key and prebuilt loader, then run the installer from
`/opt/sub2api/production`. Verify installed ownership, modes, forced key,
sudoers syntax, and candidate loader state directory. Remove transfer files.

- [ ] **Step 3: Push the feature branch and merge it into remote main**

Push `codex/sub2api-release-prep-automation`, run the complete verification
again against the branch, then fast-forward local and remote `main` only when
the remote base matches the recorded SHA. Never force push.

- [ ] **Step 4: Configure GitHub Environment**

Configure `production-candidate` with:

```text
SUB2API_PREP_SSH_KEY
SUB2API_PREP_SSH_HOST
SUB2API_PREP_SSH_PORT
SUB2API_PREP_SSH_USER
SUB2API_PREP_KNOWN_HOSTS
SUB2API_RELEASE_FEISHU_WEBHOOK
```

Use the dedicated private key, pinned existing host key, and existing Feishu
webhook value without printing them. Confirm Actions workflow permissions allow
repository contents and package writes, the GHCR package is private and linked
to the repository, and the environment has no approval gate.

- [ ] **Step 5: Run `workflow_dispatch` and observe to completion**

The first real run must prepare the current official latest stable release. Do
not manually invoke any workflow job step outside Actions. Wait for every job
to finish and capture run URL, source commit, image digest, production image
ID, and notification delivery result.

- [ ] **Step 6: Verify production no-mutation and candidate readiness**

Read the validated workflow event artifact into a shell variable and
additionally verify:

```bash
validated_version=$(
  jq -er '.version | select(test("^[0-9]+\\.[0-9]+\\.[0-9]+$"))' \
    "$VALIDATED_WORKFLOW_EVENT"
)
ssh -o BatchMode=yes sub2api-prod \
  "docker image inspect xingqiao-sub2api:upstream-$validated_version \
     --format '{{.Id}} {{.Os}}/{{.Architecture}} {{json .Config.Labels}}' &&
   ! find /run /tmp -maxdepth 2 -type d -name 'sub2api-candidate-docker.*' \
     -print -quit | grep -q ."
```

The version is taken from the validated workflow output, not typed from an
untrusted source. Confirm:

- running container fields and Compose SHA-256 exactly match before-state;
- updater remains active and has no new operation;
- public `/health` remains OK;
- temporary Docker credential directory is absent;
- Feishu success card contains no fixed next step.

- [ ] **Step 7: Finalize and commit the verification report**

Record exact run URL, commits, digest, image ID, before/after runtime evidence,
tests, no-mutation proof, and any residual GitHub scheduling latency risk.

```bash
git add docs/superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md
git commit -m "docs: verify unattended Sub2API release preparation"
git push origin main
```

## Acceptance

- [ ] Scheduled workflow is active at minute 17 every 6 hours.
- [ ] Current official latest stable version completed one real end-to-end run.
- [ ] Private GHCR contains the immutable qualified image.
- [ ] Production has the exact candidate image and local updater-compatible tag.
- [ ] Qualified source is on `main`.
- [ ] Feishu delivered the fact-only completion card.
- [ ] Production runtime, Compose, database, and updater operation state did not
  change.
- [ ] No long-lived GHCR token remains on production.
- [ ] Local Mac can be offline for all future scheduled runs.
