# Candidate Upstream Admin Intake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让管理员在生产 `/ops` 中一次性安全录入候选中转站及独立低额度 Key，并进入既有候选采集流程。

**Architecture:** `candidates.FileSecretStore` 只负责在受控目录安装和删除 `0600` Key 文件；`candidates.Service.CreateWithKey` 编排秘密安装、既有候选校验/持久化和失败回滚。HTTP 层只处理管理员同源 JSON、主动清理所持 Key 缓冲区和稳定错误码；管理台通过现有登录态调用 API。Compose 为托管候选 Key 增加单独的读写挂载，其他秘密和根文件系统继续只读。

**Tech Stack:** Go 1.24, `net/http`, `encoding/json`, HTML templates, vanilla JavaScript, Docker Compose, existing PostgreSQL repository.

## Global Constraints

- 生产保持 `RELAY_OPS_MODE=read_only` 和 `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`。
- 不修改 Sub2API 分组、路由、倍率、价格、用户余额、生产 Key 或数据库表内容。
- 候选 Key 不进入数据库、日志、响应、飞书、Agent 输入、HTML、命令行或文档。
- 容器根文件系统及既有秘密挂载保持只读；仅 `/var/lib/relay-ops/candidate-keys` 可写。
- 候选付费 probe 保持关闭；创建候选不等于批准同步/SSE 请求。
- 所有生产验收都使用非秘密无效值验证失败路径，不创建测试候选或残留秘密。

---

### Task 1: Candidate secret file store

**Files:**
- Create: `relay-ops-service/internal/candidates/secrets.go`
- Create: `relay-ops-service/internal/candidates/secrets_test.go`

**Interfaces:**
- Produces: `SecretStore`, `FileSecretStore`, `ErrSecretConflict`, `Install(name string, key []byte) (string, error)`, `Remove(path string) error`.
- Consumes: an existing absolute directory with mode `0700` owned by runtime UID `10002` in production.

- [ ] **Step 1: Write failing secret-store tests**

Cover exact behavior:

```go
func TestFileSecretStoreInstalls0600FileWithoutNameTraversal(t *testing.T) {
    dir := t.TempDir()
    if err := os.Chmod(dir, 0o700); err != nil { t.Fatal(err) }
    store := FileSecretStore{Directory: dir}
    key := []byte("sk-candidate-secret-1234")
    path, err := store.Install("../../Candidate A", key)
    if err != nil { t.Fatal(err) }
    if filepath.Dir(path) != dir { t.Fatalf("path escaped: %s", path) }
    info, _ := os.Stat(path)
    if info.Mode().Perm() != 0o600 { t.Fatalf("mode=%o", info.Mode().Perm()) }
    raw, _ := os.ReadFile(path)
    if string(raw) != "sk-candidate-secret-1234" { t.Fatal("wrong secret") }
}

```

Add `TestFileSecretStoreRejectsInvalidDirectoryKeyAndOverwrite` with four named cases: directory mode `0755`, directory symlink, three-byte Key, and a second install for the same normalized name. Add `TestFileSecretStoreRemoveRejectsOutsidePath` by creating one file beside the managed directory, calling `Remove` with that path, asserting an error, and confirming the outside file still exists.

- [ ] **Step 2: Run RED**

Run:

```bash
cd relay-ops-service
go test ./internal/candidates -run 'TestFileSecretStore' -count=1
```

Expected: compile failure because `FileSecretStore` does not exist.

- [ ] **Step 3: Implement the minimal file store**

`secrets.go` must:

```go
var ErrSecretConflict = errors.New("candidate secret already exists")

type SecretStore interface {
    Install(string, []byte) (string, error)
    Remove(string) error
}

type FileSecretStore struct{ Directory string }
```

Implementation requirements:

- `filepath.Abs` and `filepath.Clean` the configured directory; require an existing non-symlink directory with `0700` permissions.
- Trim Key whitespace and accept 4 through `maxProbeKeyBytes` bytes.
- Derive filename as `hex(sha256(strings.ToLower(strings.TrimSpace(name)))) + ".key"`.
- Use `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)`, write all bytes, `Sync`, and `Close`.
- Remove a partial file on write/sync/close failure.
- Map `fs.ErrExist` to `ErrSecretConflict`.
- `Remove` may delete only a direct child of the configured directory and treats `fs.ErrNotExist` as success.

- [ ] **Step 4: Run GREEN**

```bash
cd relay-ops-service
go test ./internal/candidates -run 'TestFileSecretStore' -count=1
```

Expected: PASS.

---

### Task 2: Candidate intake orchestration and rollback

**Files:**
- Modify: `relay-ops-service/internal/candidates/service.go`
- Modify: `relay-ops-service/internal/candidates/service_test.go`

**Interfaces:**
- Consumes: `SecretStore` from Task 1 and existing `Service.Create` file-reference path.
- Produces: `CandidateIntakeInput` and `Service.CreateWithKey(context.Context, domain.AdminActor, CandidateIntakeInput) (Candidate, error)`.

- [ ] **Step 1: Write failing service tests**

Add tests using a real temporary `FileSecretStore` and fake repository:

Add these exact test functions: `TestCreateWithKeyInstallsSecretAndStoresOnlyMetadata`, `TestCreateWithKeyRemovesSecretWhenRepositoryFails`, and `TestCreateWithKeyRejectsMissingStoreAndClearsOwnedBuffer`.

Assertions:

- success leaves exactly one `0600` file and repository input contains only `file:` reference, fingerprint and last four;
- repository error leaves the directory empty;
- error strings and serialized repository input never contain the Key;
- caller-owned Key slice is zeroed before return.

- [ ] **Step 2: Run RED**

```bash
cd relay-ops-service
go test ./internal/candidates -run 'TestCreateWithKey' -count=1
```

Expected: compile failure because `CreateWithKey` and `CandidateIntakeInput` do not exist.

- [ ] **Step 3: Implement intake orchestration**

Add:

```go
type CandidateIntakeInput struct {
    Name, BaseURL, PricingURL, UsageURL, PerformanceURL string
    ProbeKey []byte
}

type Service struct {
    Repository Repository
    Resolver Resolver
    SecretStore SecretStore
}
```

`CreateWithKey` installs the Key, defers zeroing the supplied slice, then calls existing `Create` with the installed file path. Any `Create` failure invokes `SecretStore.Remove`; a cleanup failure returns a stable error without including the path or original error text. Preserve `errors.Is(err, ErrConflict)` and `errors.Is(err, ErrSecretConflict)` so HTTP can return 409.

- [ ] **Step 4: Run GREEN and existing candidate tests**

```bash
cd relay-ops-service
go test ./internal/candidates -count=1
```

Expected: PASS.

---

### Task 3: Configuration and app wiring

**Files:**
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`

**Interfaces:**
- Produces: `Config.CandidateSecretDir` loaded from `RELAY_OPS_CANDIDATE_SECRET_DIR`, default `/var/lib/relay-ops/candidate-keys`.
- Consumes: `candidates.FileSecretStore` and existing candidate repository.

- [ ] **Step 1: Write failing config and wiring tests**

Assert the default and override path, and that application candidate service receives a non-nil secret store. Keep path validation limited to an absolute cleaned path; directory existence is checked by `FileSecretStore` at use time so startup remains deterministic in tests.

- [ ] **Step 2: Run RED**

```bash
cd relay-ops-service
go test ./internal/config ./internal/app -run 'CandidateSecret' -count=1
```

Expected: failure because `CandidateSecretDir` is absent.

- [ ] **Step 3: Add configuration and wiring**

Load `RELAY_OPS_CANDIDATE_SECRET_DIR`; reject relative paths. Construct:

```go
candidateService := candidates.Service{
    Repository: database,
    SecretStore: candidates.FileSecretStore{Directory: cfg.CandidateSecretDir},
}
```

Do not change scheduler probe mode or budgets.

- [ ] **Step 4: Run GREEN**

```bash
cd relay-ops-service
go test ./internal/config ./internal/app -count=1
```

Expected: PASS.

---

### Task 4: Candidate create API with one-time Key

**Files:**
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`

**Interfaces:**
- Changes `CandidateService` to expose `CreateWithKey` while preserving `List` and `Disable`.
- Consumes JSON field `probe_key`; never accepts `probe_key_file` from the browser.

- [ ] **Step 1: Write failing HTTP contract tests**

Add/replace tests for:

```json
{
  "name":"candidate",
  "base_url":"https://candidate.example/v1",
  "pricing_url":"https://candidate.example/pricing",
  "usage_url":"https://candidate.example/usage",
  "performance_url":"https://candidate.example/performance",
  "probe_key":"temporary-secret-1234"
}
```

Verify:

- missing Origin returns `403 ORIGIN_REJECTED`;
- valid request calls `CreateWithKey` and returns 201 without Key/path;
- `probe_key_file`, unknown fields, missing Key and an over-8192-byte Key are rejected;
- service conflict returns `409 CANDIDATE_CONFLICT`;
- fake service captures Key only long enough to assert input and zeroes it before handler returns.

- [ ] **Step 2: Run RED**

```bash
cd relay-ops-service
go test ./internal/http -run 'Candidate' -count=1
```

Expected: failure because handler still accepts `probe_key_file` and interface lacks `CreateWithKey`.

- [ ] **Step 3: Implement the HTTP change**

Use a dedicated request type with `ProbeKey []byte` populated from JSON, `http.MaxBytesReader` at 32 KiB, `DisallowUnknownFields`, and a deferred `clear(input.ProbeKey)`. Call `CreateWithKey` and map:

- `ErrConflict` or `ErrSecretConflict` -> 409 `CANDIDATE_CONFLICT`;
- all validation/install errors -> 400 `CANDIDATE_REJECTED`;
- never include `err.Error()` in the response.

- [ ] **Step 4: Run GREEN**

```bash
cd relay-ops-service
go test ./internal/http -run 'Candidate' -count=1
```

Expected: PASS.

---

### Task 5: `/ops` candidate form and disable control

**Files:**
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/static/app.css`
- Modify: `relay-ops-service/internal/http/server_test.go`

**Interfaces:**
- `CandidateView` gains `Enabled bool`.
- Form IDs: `candidate-source-form`, `candidate-source-status`, `candidate-probe-key`.
- Disable buttons use `data-candidate-disable` and `data-candidate-name`.

- [ ] **Step 1: Write failing rendered-UI and script tests**

Require six fields, `type="password"`, `autocomplete="new-password"`, low-budget warning, candidate submit endpoint, success reset/reload, Key clearing in `finally`, and disable endpoint with `window.confirm`. Prohibit `innerHTML`, `console.log`, `probe_key_file`, and any literal secret.

- [ ] **Step 2: Run RED**

```bash
cd relay-ops-service
go test ./internal/http -run 'CandidateAdmin|OpsProvides' -count=1
```

Expected: failure because form and controls are absent.

- [ ] **Step 3: Implement template, JavaScript and restrained styles**

Insert the candidate form immediately before the candidate list. Submit same-origin JSON with the existing bearer token. In `finally`, set the password input value to empty and overwrite `payload.probe_key` with an empty string. On success call `candidateForm.reset()` and reload after 350 ms.

Disable buttons POST `{}` to `/relay-ops/api/candidates/{id}/disable` only after confirmation; disabled rows have no command button. Add a compact command column without changing the table layout on hover. Bump `ops-admin.js` and CSS asset version strings.

- [ ] **Step 4: Run GREEN and JavaScript syntax check**

```bash
cd relay-ops-service
go test ./internal/http -count=1
node --check internal/http/static/ops-admin.js
```

Expected: PASS and no syntax output.

---

### Task 6: Dedicated writable secret mount and deployment contract

**Files:**
- Modify: `infra/compose.yaml`
- Modify: `infra/.env.example`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Modify: `docs/runbooks/relay-ops-monitoring.md`

**Interfaces:**
- Host variable: `RELAY_OPS_CANDIDATE_MANAGED_KEYS_HOST_DIR`.
- Container variable: `RELAY_OPS_CANDIDATE_SECRET_DIR=/var/lib/relay-ops/candidate-keys`.

- [ ] **Step 1: Add failing contract assertions**

Require both mounts:

```text
${RELAY_OPS_CANDIDATE_KEYS_HOST_DIR:-./secrets/candidate-keys}:/run/secrets/candidates:ro
${RELAY_OPS_CANDIDATE_MANAGED_KEYS_HOST_DIR:-./secrets/candidate-managed-keys}:/var/lib/relay-ops/candidate-keys:rw
```

Also require the container environment path and retain `read_only: true`.

- [ ] **Step 2: Run RED**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: FAIL for missing managed-key mount.

- [ ] **Step 3: Update Compose, env example and runbook**

Document that the managed directory is created once with `0700`, owner `10002:10002`; Key values are thereafter entered only through `/ops`. Keep the legacy read-only candidate directory for existing file-reference records.

- [ ] **Step 4: Run GREEN**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
docker compose -f infra/compose.yaml config >/dev/null
```

Expected: PASS.

---

### Task 7: Full verification and production deployment

**Files:**
- Create: `docs/superpowers/reports/2026-07-21-candidate-upstream-admin-intake-production-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Production image tag: `sub2api-relay-ops:candidate-admin-intake-20260721-v1`.
- Production managed directory: `/opt/sub2api/production/secrets/candidate-managed-keys`.

- [ ] **Step 1: Run fresh local verification**

```bash
cd relay-ops-service
go test ./... -race -count=1
go vet ./...
node --check internal/http/static/ops.js
node --check internal/http/static/ops-admin.js
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
git diff --check
```

Expected: all exit 0.

- [ ] **Step 2: Capture production pre-state**

Record relay-ops mode, container IDs/restarts, Feishu routing-file SHA-256 and the same normalized Sub2API group/account/model snapshot used by the prior proactive-alert verification. Store only redacted hashes under a new restrictive evidence directory.

- [ ] **Step 3: Build and deploy only relay-ops**

Build the AMD64 image, load it on `sub2api-prod`, create the managed directory as `10002:10002` mode `0700`, add only the new env/mount to production Compose, then run:

```bash
cd /opt/sub2api/production
sudo docker compose up -d --no-deps relay-ops
```

Do not recreate Sub2API, PostgreSQL, Redis or Caddy.

- [ ] **Step 4: Verify production behavior without creating a candidate**

Check HTTP 200 for `/healthz`, `/readyz`, `/pricing`, `/ops`, `/monitor`; container `healthy`, restart count 0, modes unchanged. In authenticated browser `/ops`, verify the six-field form, password semantics and disable column. Submit a deliberately invalid non-secret Key/URL combination and confirm `CANDIDATE_REJECTED`, empty managed directory and no new candidate row.

- [ ] **Step 5: Verify zero route/config mutation**

Recompute Feishu routing-file hash and normalized Sub2API canonical snapshot; both must equal pre-state. Confirm the managed directory is the only read-write mount and all existing secrets remain read-only.

- [ ] **Step 6: Document and update mainline**

Write the verification report, update current-state and handoff to state that the admin can enter the first real candidate, and leave the goal active only if another explicit objective item remains unverified.
