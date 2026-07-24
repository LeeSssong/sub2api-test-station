# Official Sub2API Image Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the patched production Sub2API container with the immutable official `0.1.164` image while preserving current storage and moving `联系客服` to the supported official sidebar without leaking login tokens.

**Architecture:** The Sub2API container becomes disposable and is selected by a version-controlled release environment plus a Compose image-only overlay. Xingqiao keeps support UI in the independently built homepage/Caddy image; Sub2API stores only a persistent `md:support` menu item and a Markdown iframe pointing to same-origin `/support`. Backup, storage-identity checks, health checks, authenticated smoke checks, release records, and rollback are owned by deployment scripts, with a hard production confirmation gate after an isolated rehearsal.

**Tech Stack:** Docker Compose v2, official `weishaw/sub2api` OCI image, Bash, Ruby JSON/YAML helpers, PostgreSQL `pg_dump`, Caddy 2.10, React 19/Vite/Vitest, Playwright browser acceptance.

## Global Constraints

- Do not submit changes to the upstream Sub2API repository.
- Do not maintain a long-lived Sub2API fork or patched production image.
- Use `weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659`.
- Preserve the current custom rollback baseline `xingqiao-sub2api:v0.1.164-contact-v1`, image ID `sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220`.
- Preserve the production Compose project `sub2api-deploy` and its current bind mounts: `data -> /app/data`, `postgres_data -> /var/lib/postgresql/data`, and `redis_data -> /data`.
- Never run `docker compose down -v`, delete a production volume, or start production through `infra/compose.yaml` until its resolved storage mappings have been proven identical to production.
- Do not overlay the official executable or embedded frontend and do not inject HTML or JavaScript into Sub2API responses.
- Keep QQ group `1080152144` and the exact QR image SHA-256 `35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16`.
- The official custom menu uses `url: "md:support"`; `support.md` embeds only `/support`, never a direct URL-mode menu item.
- Caddy rejects `POST /api/v1/admin/system/update` and `POST /api/v1/admin/system/rollback` before proxying to Sub2API.
- No production container recreation is authorized until Task 8 rehearsal evidence passes and the user explicitly confirms Task 9.
- Preserve all unrelated dirty-worktree changes and secrets.

---

## File Map

- `ops/capture-sub2api-runtime-baseline.sh`: emits a redacted, deterministic runtime/storage/image inventory.
- `tests/operations/capture_sub2api_runtime_baseline_test.sh`: proves the inventory rejects wrong project, mounts, or image identity.
- `docs/migrations/2026-07-24-sub2api-runtime-baseline.md`: captured production facts and rollback identity.
- `docs/migrations/2026-07-24-sub2api-custom-delta-inventory.md`: every custom delta classified as Externalize, Configuration, or Retire.
- `homepage/src/pages/SupportPage.tsx`: independently owned support page.
- `homepage/src/pages/SupportPage.test.tsx`: support content and copy behavior contract.
- `homepage/public/support/qq-group-1080152144.png`: byte-for-byte preserved production QR asset.
- `homepage/src/main.tsx`, `homepage/src/styles.css`: pathname selection and responsive light/dark support styling.
- `config/sub2api/support.md`: persistent Markdown bridge containing only the same-origin iframe.
- `ops/configure-sub2api-support.sh`: atomic page install plus idempotent partial settings update.
- `tests/operations/configure_sub2api_support_test.sh`: verifies merge behavior, file safety, and absence of token-bearing URL mode.
- `infra/Caddyfile`: `/support` static route, same-origin frame policy, and update/rollback guard.
- `tests/infra/validate-official-sub2api-release.sh`: repository boundary check for official image, no build/overlay, support route, guard, and explicit bind paths.
- `config/releases/sub2api.env`: non-secret pinned production release reference.
- `infra/compose.sub2api-release.yaml`: image-only overlay used against the existing production Compose file.
- `infra/compose.yaml`, `infra/.env.example`, `tests/infra/validate-baseline.sh`: future integrated deployment points at the same official digest and explicit storage paths.
- `ops/backup-sub2api-release.sh`: PostgreSQL plus `/app/data` release backup with checksum metadata.
- `ops/smoke-sub2api-release.sh`: health, version, settings/menu, record-count, and bounded gateway checks.
- `ops/deploy-sub2api-release.sh`: preflight, backup, pull, recreate-only-sub2api, smoke, record, and rollback orchestration.
- `tests/operations/backup_sub2api_release_test.sh`, `tests/operations/deploy_sub2api_release_test.sh`, `tests/operations/smoke_sub2api_release_test.sh`: failure-injection contracts.
- `docs/runbooks/sub2api-official-image-release.md`: routine update, rehearsal, production confirmation, and recovery procedure.
- `evidence/sub2api-rehearsal/`: non-secret isolated rehearsal records.

### Task 1: Freeze Runtime Identity And Classify Every Custom Delta

**Files:**
- Create: `ops/capture-sub2api-runtime-baseline.sh`
- Create: `tests/operations/capture_sub2api_runtime_baseline_test.sh`
- Create: `docs/migrations/2026-07-24-sub2api-runtime-baseline.md`
- Create: `docs/migrations/2026-07-24-sub2api-custom-delta-inventory.md`

**Interfaces:**
- Consumes: a running Compose service name and expected project/storage paths.
- Produces: JSON with `project`, `config_files`, `image`, `image_id`, `mounts`, `network_names`, and `captured_at`; exits nonzero on any mismatch.

- [ ] **Step 1: Write the failing inventory contract test**

Create a fake `docker` executable that returns fixture JSON for `docker inspect`, then assert success for the exact three production bind mounts and failure when `/app/data` points anywhere else. The success fixture must include the extra PostgreSQL anonymous `/var/lib/postgresql` volume but require the authoritative `/var/lib/postgresql/data` bind.

```bash
PATH="$fixture/bin:$PATH" \
EXPECTED_PROJECT=sub2api-deploy \
EXPECTED_SUB2API_DATA="$fixture/deploy/data" \
EXPECTED_POSTGRES_DATA="$fixture/deploy/postgres_data" \
EXPECTED_REDIS_DATA="$fixture/deploy/redis_data" \
ops/capture-sub2api-runtime-baseline.sh >"$fixture/baseline.json"

jq -e '.project == "sub2api-deploy" and .mounts.sub2api.destination == "/app/data"' \
  "$fixture/baseline.json"

if EXPECTED_SUB2API_DATA="$fixture/wrong" PATH="$fixture/bin:$PATH" \
  ops/capture-sub2api-runtime-baseline.sh >/dev/null 2>&1; then
  fail 'wrong application-data bind was accepted'
fi
```

- [ ] **Step 2: Run the test and verify RED**

Run: `bash tests/operations/capture_sub2api_runtime_baseline_test.sh`

Expected: FAIL because `ops/capture-sub2api-runtime-baseline.sh` does not exist.

- [ ] **Step 3: Implement the baseline collector**

The script must use `docker inspect` only, compare canonical absolute sources, redact environment values, and emit JSON through `jq -n`. Its fixed identity checks are:

```bash
expected_project=${EXPECTED_PROJECT:-sub2api-deploy}
expected_image_id=${EXPECTED_IMAGE_ID:-sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220}
expected_sub2api_data=${EXPECTED_SUB2API_DATA:?required}
expected_postgres_data=${EXPECTED_POSTGRES_DATA:?required}
expected_redis_data=${EXPECTED_REDIS_DATA:?required}

inspect_mount() {
  docker inspect "$1" | jq -er --arg destination "$2" '
    .[0].Mounts[] | select(.Type == "bind" and .Destination == $destination) |
    {type: .Type, source: .Source, destination: .Destination, rw: .RW}'
}
```

Reject a project label other than `sub2api-deploy`, a source mismatch after `cd "$source" && pwd -P`, a read-only data mount, or a current Sub2API image ID mismatch. Never print `.Config.Env`.

- [ ] **Step 4: Run the collector tests and capture current evidence**

```bash
bash tests/operations/capture_sub2api_runtime_baseline_test.sh
EXPECTED_SUB2API_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/data' \
EXPECTED_POSTGRES_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/postgres_data' \
EXPECTED_REDIS_DATA='/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/redis_data' \
ops/capture-sub2api-runtime-baseline.sh > /tmp/sub2api-runtime-baseline.json
```

Expected: test PASS; captured JSON names only the approved project, image, mounts, and networks and contains no secret values.

- [ ] **Step 5: Write the migration records**

The runtime Markdown records the capture command, JSON SHA-256, custom image/tag/ID, Compose file/working directory, three bind mounts, and rollback tag. The delta inventory must contain these rows with no `Blocked` item:

| Delta | Disposition | Replacement/removal proof |
|---|---|---|
| `AppHeader.vue` 联系客服 trigger | Retire | official sidebar menu acceptance |
| `ContactSupportDialog.vue` | Externalize | `/support` page browser acceptance |
| QQ QR embedded frontend asset | Externalize | preserved homepage asset hash |
| support frontend tests | Retire | homepage and end-to-end support tests |
| custom image build args/tag | Retire | official image release contract |
| `custom_menu_items` | Configuration | idempotent admin settings script |
| `support.md` | Configuration | persistent `/app/data/pages/support.md` |
| in-process update/rollback | Configuration | Caddy guard plus release runbook |
| D04 behavior | Externalize | unchanged internal-test-service contract |
| relay operations/reporting | Externalize | unchanged relay-ops contract |

- [ ] **Step 6: Verify and commit**

```bash
rg -n 'Blocked|token=|POSTGRES_PASSWORD|JWT_SECRET' docs/migrations/2026-07-24-sub2api-*.md
git diff --check
git add ops/capture-sub2api-runtime-baseline.sh tests/operations/capture_sub2api_runtime_baseline_test.sh docs/migrations/2026-07-24-sub2api-runtime-baseline.md docs/migrations/2026-07-24-sub2api-custom-delta-inventory.md
git commit -m "ops: freeze Sub2API migration baseline"
```

Expected: only the inventory header contains the word `Blocked` when explaining that there are zero blocked items; no secret patterns; commit succeeds.

### Task 2: Externalize The Support Page With The Exact QR Asset

**Files:**
- Create: `homepage/src/pages/SupportPage.tsx`
- Create: `homepage/src/pages/SupportPage.test.tsx`
- Create: `homepage/public/support/qq-group-1080152144.png`
- Modify: `homepage/src/main.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- Consumes: `SiteConfig.support.qqGroup` and the existing `CopyControl` fallback.
- Produces: same-origin `/support`, containing the QQ QR, number, and copy action without authentication or query parameters.

- [ ] **Step 1: Write the failing support page tests**

```tsx
it('shows the preserved QQ group and QR code', () => {
  render(<SupportPage qqGroup="1080152144" />)
  expect(screen.getByRole('heading', { name: '联系客服' })).toBeInTheDocument()
  expect(screen.getByText('1080152144')).toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'QQ群 1080152144 二维码' }))
    .toHaveAttribute('src', '/support/qq-group-1080152144.png')
})

it('copies the group number through the shared resilient control', async () => {
  const user = userEvent.setup()
  render(<SupportPage qqGroup="1080152144" />)
  await user.click(screen.getByRole('button', { name: '复制 QQ 群号' }))
  expect(navigator.clipboard.writeText).toHaveBeenCalledWith('1080152144')
})
```

Add a `main.tsx` test around an exported `selectRuntimePage(pathname)` and assert `/support` selects support while `/` selects home.

- [ ] **Step 2: Run the tests and verify RED**

Run: `cd homepage && npm run test:run -- src/pages/SupportPage.test.tsx src/main.test.tsx`

Expected: FAIL because the page and selector do not exist.

- [ ] **Step 3: Preserve the exact QR bytes**

Copy the already captured read-only file, then verify before staging:

```bash
mkdir -p homepage/public/support
cp /var/folders/26/3qc7y_lx2s11df_9sh7dqg_40000gn/T/tmp.oJnQHFHdKl/qq-group-1080152144.png \
  homepage/public/support/qq-group-1080152144.png
test "$(shasum -a 256 homepage/public/support/qq-group-1080152144.png | awk '{print $1}')" = \
  35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16
```

Expected: the file is 621455 bytes and hash comparison exits 0. During execution use `apply_patch` for code edits; this binary preservation copy is the one allowed mechanical asset operation.

- [ ] **Step 4: Implement the page and pathname selection**

`SupportPage.tsx` renders one quiet full-height surface, not a marketing page:

```tsx
export function SupportPage({ qqGroup }: { qqGroup: string }) {
  return (
    <main className="support-page">
      <section className="support-panel" aria-labelledby="support-title">
        <div className="support-copy">
          <p className="support-kicker">XINGQIAO SUPPORT</p>
          <h1 id="support-title">联系客服</h1>
          <p>扫描二维码加入 QQ 群，或复制群号手动搜索。</p>
          <div className="support-number-row">
            <span>QQ群号</span><strong>{qqGroup}</strong>
            <CopyControl value={qqGroup} label="复制 QQ 群号" />
          </div>
        </div>
        <figure className="support-qr">
          <img src="/support/qq-group-1080152144.png" alt={`QQ群 ${qqGroup} 二维码`} />
        </figure>
      </section>
    </main>
  )
}
```

Export `selectRuntimePage(pathname: string): 'support' | 'home'` from `main.tsx`; render `SupportPage` for `/support` and `/support/`, otherwise retain the existing homepage path. Do not initialize Lenis on support.

- [ ] **Step 5: Add responsive light/dark styling**

Use stable dimensions and no nested cards:

```css
.support-page { min-height: 100svh; display: grid; place-items: center; padding: 32px; color-scheme: light dark; background: #f6f7f9; color: #16191f; }
.support-panel { width: min(100%, 880px); display: grid; grid-template-columns: minmax(0, 1fr) minmax(260px, 360px); gap: 48px; align-items: center; }
.support-number-row { display: grid; grid-template-columns: 1fr auto; gap: 8px 20px; align-items: center; margin-top: 28px; padding-top: 20px; border-top: 1px solid #d9dde4; }
.support-number-row > span { grid-column: 1 / -1; color: #656b76; font-size: .8rem; }
.support-number-row strong { font: 700 clamp(1.35rem, 4vw, 2rem)/1.1 ui-monospace, monospace; letter-spacing: 0; }
.support-qr { margin: 0; aspect-ratio: 1; overflow: hidden; background: #fff; border: 1px solid #d9dde4; }
.support-qr img { width: 100%; height: 100%; object-fit: contain; }
@media (prefers-color-scheme: dark) { .support-page { background: #111318; color: #f3f5f8; } .support-number-row, .support-qr { border-color: #343944; } }
@media (max-width: 680px) { .support-page { place-items: start center; padding: 28px 20px; } .support-panel { grid-template-columns: 1fr; gap: 28px; } .support-qr { width: min(100%, 360px); justify-self: center; } }
```

- [ ] **Step 6: Verify and commit**

```bash
cd homepage
npm run test:run -- src/pages/SupportPage.test.tsx src/main.test.tsx src/domain/clipboard.test.ts
npm run typecheck
npm run build
cd ..
git add homepage/src/pages homepage/src/main.tsx homepage/src/styles.css homepage/public/support/qq-group-1080152144.png
git commit -m "feat: externalize Xingqiao support page"
```

Expected: all tests, typecheck, and build pass; asset hash is unchanged.

### Task 3: Configure The Official Sidebar Without Token Leakage

**Files:**
- Create: `config/sub2api/support.md`
- Create: `ops/configure-sub2api-support.sh`
- Create: `tests/operations/configure_sub2api_support_test.sh`

**Interfaces:**
- Consumes: `ADMIN_API_URL`, protected `ADMIN_API_KEY_FILE`, and writable `SUB2API_DATA_DIR`.
- Produces: `/app/data/pages/support.md` and one idempotent menu item `xingqiao-support` using `md:support`.

- [ ] **Step 1: Write failing file/API merge tests**

The test uses a temporary data directory and fake `curl` that returns an existing storefront item. It captures the PUT body and asserts both items remain:

```bash
run_configure
cmp "$ROOT/config/sub2api/support.md" "$fixture/data/pages/support.md"
jq -e '.custom_menu_items | length == 2' "$fixture/put.json"
jq -e '.custom_menu_items | any(.id == "xingqiao-support" and .url == "md:support" and .visibility == "user")' "$fixture/put.json"
jq -e '.custom_menu_items | any(.id == "xingqiao-storefront")' "$fixture/put.json"
! rg -n 'token|user_id|src_url|https?://' "$fixture/data/pages/support.md"
run_configure
jq -e '[.custom_menu_items[] | select(.id == "xingqiao-support")] | length == 1' "$fixture/put.json"
```

- [ ] **Step 2: Run the test and verify RED**

Run: `bash tests/operations/configure_sub2api_support_test.sh`

Expected: FAIL because the script and Markdown source do not exist.

- [ ] **Step 3: Add the same-origin Markdown bridge**

The complete `config/sub2api/support.md` is:

```html
<iframe src="/support" title="联系客服" loading="eager" style="display:block;width:100%;height:min(760px,calc(100vh - 180px));border:0;background:transparent"></iframe>
```

- [ ] **Step 4: Implement atomic install and partial settings update**

The script validates required inputs, installs through a temporary file in the same directory, preserves all other menu entries, and sends only the changed field:

```bash
mkdir -p "$SUB2API_DATA_DIR/pages"
temporary_page="$SUB2API_DATA_DIR/pages/.support.md.$$"
trap 'rm -f -- "$temporary_page"' EXIT
install -m 0644 "$repo_root/config/sub2api/support.md" "$temporary_page"
mv "$temporary_page" "$SUB2API_DATA_DIR/pages/support.md"

settings=$(curl --fail-with-body --silent --show-error \
  -H "X-API-Key: $admin_key" "$ADMIN_API_URL/api/v1/admin/settings")
payload=$(jq '{custom_menu_items: (((.data // .).custom_menu_items // []) |
  map(select(.id != "xingqiao-support"))) + [{
    id:"xingqiao-support", label:"联系客服",
    icon_svg:"<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\"><path d=\"M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z\"/></svg>",
    url:"md:support", page_slug:"support", visibility:"user", sort_order:80
  }] )}' <<<"$settings")
curl --fail-with-body --silent --show-error -X PUT \
  -H 'Content-Type: application/json' -H "X-API-Key: $admin_key" \
  --data-binary "$payload" "$ADMIN_API_URL/api/v1/admin/settings" |
  jq -e '(.data // .).custom_menu_items | any(.id == "xingqiao-support" and .url == "md:support")' >/dev/null
```

Reject a symlinked `pages` directory or existing symlink `support.md`; never print the API key or whole settings response.

- [ ] **Step 5: Verify and commit**

```bash
bash tests/operations/configure_sub2api_support_test.sh
git add config/sub2api/support.md ops/configure-sub2api-support.sh tests/operations/configure_sub2api_support_test.sh
git commit -m "ops: configure official support menu"
```

Expected: PASS; repeated runs produce exactly one support item and leave existing menu items untouched.

### Task 4: Add The Caddy Support Route And Binary-Mutation Guard

**Files:**
- Modify: `infra/Caddyfile`
- Modify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: `/srv/home` output containing the support route and asset.
- Produces: same-origin `/support`; stable JSON 409 for two POST mutation endpoints; official page CSP permits only same-origin support embedding.

- [ ] **Step 1: Add failing Caddy contract assertions**

```bash
require_fixed 'path /support /support/' infra/Caddyfile
require_fixed 'rewrite * /index.html' infra/Caddyfile
require_fixed 'path /support/*' infra/Caddyfile
require_fixed 'method POST' infra/Caddyfile
require_fixed 'path /api/v1/admin/system/update /api/v1/admin/system/rollback' infra/Caddyfile
require_fixed 'DOCKER_DEPLOYMENT_UPDATE_REQUIRED' infra/Caddyfile
require_fixed "frame-src 'self'" infra/Caddyfile
```

Also assert the guard block appears before `reverse_proxy sub2api:8080` by comparing line numbers.

- [ ] **Step 2: Run baseline and verify RED**

Run: `bash tests/infra/validate-baseline.sh`

Expected: FAIL on the missing support route or guard.

- [ ] **Step 3: Add route, headers, and guard before the fallback proxy**

```caddyfile
	@support_index path /support /support/
	handle @support_index {
		root * /srv/home
		rewrite * /index.html
		header Cache-Control "no-store, max-age=0"
		header Content-Security-Policy "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'"
		header X-Frame-Options SAMEORIGIN
		file_server
	}

	@support_assets path /support/*
	handle @support_assets {
		root * /srv/home
		header Cache-Control "public, max-age=31536000, immutable"
		file_server
	}

	@sub2api_binary_mutation {
		method POST
		path /api/v1/admin/system/update /api/v1/admin/system/rollback
	}
	respond @sub2api_binary_mutation `{"code":"DOCKER_DEPLOYMENT_UPDATE_REQUIRED","message":"Docker 部署仅支持受控 Compose 更新，请使用运维发布流程"}` 409
```

Change the proxy CSP replacement so the existing `frame-src` begins with `'self'` and retains the approved storefront origins. Do not remove the backend nonce or replace the whole CSP.

- [ ] **Step 4: Validate Caddy and commit**

```bash
bash tests/infra/validate-baseline.sh
docker run --rm -e SITE_ADDRESS=api.example.com \
  -v "$PWD/infra/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d \
  caddy validate --config /etc/caddy/Caddyfile
git add infra/Caddyfile tests/infra/validate-baseline.sh
git commit -m "feat: guard Docker updates at Caddy boundary"
```

Expected: both validations pass.

### Task 5: Declare The Immutable Official Release And Explicit Storage

**Files:**
- Create: `config/releases/sub2api.env`
- Create: `infra/compose.sub2api-release.yaml`
- Create: `tests/infra/validate-official-sub2api-release.sh`
- Modify: `infra/compose.yaml`
- Modify: `infra/.env.example`
- Modify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: non-secret `SUB2API_IMAGE` and explicit storage directories.
- Produces: resolved official image with no Sub2API build section and bind mounts that cannot silently become empty named volumes.

- [ ] **Step 1: Write the failing official-release contract**

Parse Compose as JSON and assert:

```ruby
expected = "weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659"
service = compose.fetch("services").fetch("sub2api")
abort "wrong image" unless service.fetch("image") == expected
abort "custom build remains" if service.key?("build")
%w[/app/data].each do |target|
  mount = service.fetch("volumes").find { |v| v["target"] == target }
  abort "#{target} is not a bind" unless mount && mount["type"] == "bind"
end
```

Add fixed-string bans for `xingqiao-sub2api`, `../upstream/sub2api`, `:latest`, executable/frontend bind mounts, and missing release digest.

- [ ] **Step 2: Run the contract and verify RED**

Run: `bash tests/infra/validate-official-sub2api-release.sh`

Expected: FAIL because the custom source build is still declared.

- [ ] **Step 3: Add the release env and image-only overlay**

`config/releases/sub2api.env` contains exactly:

```dotenv
SUB2API_IMAGE=weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659
```

`infra/compose.sub2api-release.yaml` contains exactly:

```yaml
services:
  sub2api:
    image: ${SUB2API_IMAGE:?SUB2API_IMAGE is required}
```

- [ ] **Step 4: Switch integrated Compose and make storage explicit**

Replace the custom `build` and image with:

```yaml
image: ${SUB2API_IMAGE:?SUB2API_IMAGE is required}
volumes:
  - ${SUB2API_DATA_DIR:?SUB2API_DATA_DIR is required}:/app/data
```

Replace PostgreSQL and Redis data mounts in the same file with `${POSTGRES_DATA_DIR:?required}` and `${REDIS_DATA_DIR:?required}` bind sources. Add deterministic non-production examples:

```dotenv
SUB2API_IMAGE=weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659
SUB2API_DATA_DIR=./data
POSTGRES_DATA_DIR=./postgres_data
REDIS_DATA_DIR=./redis_data
```

Remove only the now-unused `sub2api_data`, `postgres_data`, and `redis_data` top-level volume declarations. Keep Caddy and Xingqiao service volumes unchanged.

- [ ] **Step 5: Verify digest and contracts**

```bash
docker buildx imagetools inspect weishaw/sub2api:0.1.164 --format '{{json .Manifest}}' |
  jq -e '.digest == "sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659"'
bash tests/infra/validate-official-sub2api-release.sh
bash tests/infra/validate-baseline.sh
```

Expected: digest comparison and both contracts pass.

- [ ] **Step 6: Commit**

```bash
git add config/releases/sub2api.env infra/compose.sub2api-release.yaml infra/compose.yaml infra/.env.example tests/infra/validate-official-sub2api-release.sh tests/infra/validate-baseline.sh
git commit -m "ops: pin official Sub2API image"
```

### Task 6: Build Validated Backup And Smoke-Check Primitives

**Files:**
- Create: `ops/backup-sub2api-release.sh`
- Create: `ops/smoke-sub2api-release.sh`
- Create: `tests/operations/backup_sub2api_release_test.sh`
- Create: `tests/operations/smoke_sub2api_release_test.sh`

**Interfaces:**
- Backup produces `<timestamp>/sub2api.dump`, `app-data.tar.gz`, `record-counts.json`, `SHA256SUMS`, and `metadata.json`, promoted atomically.
- Smoke consumes expected version, admin API key file, bounded gateway API key file, expected record counts, and base URL; returns nonzero on any mismatch.

- [ ] **Step 1: Write backup failure-injection tests**

Cover successful permissions/checksums, empty `pg_dump`, failed app-data archive, existing lock, and retention of the three newest verified sets. A failed run must leave no timestamp directory and must not delete operator notes.

- [ ] **Step 2: Run backup tests and verify RED**

Run: `bash tests/operations/backup_sub2api_release_test.sh`

Expected: FAIL because the backup script does not exist.

- [ ] **Step 3: Implement the release backup**

Use the locking/partial-directory pattern already proven in `ops/backup-d04-account-data.sh`. The PostgreSQL commands are:

```bash
compose exec -T postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' >"$partial/sub2api.dump"
compose exec -T postgres sh -c 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At' <<'SQL' >"$partial/record-counts.json"
select json_build_object(
  'users', (select count(*) from users),
  'accounts', (select count(*) from accounts),
  'groups', (select count(*) from groups),
  'api_keys', (select count(*) from api_keys),
  'settings', (select count(*) from settings),
  'usage_logs', (select count(*) from usage_logs)
)::text;
SQL
tar -C "$SUB2API_DATA_DIR" -czf "$partial/app-data.tar.gz" .
```

Validate `pg_restore --list` through the pinned PostgreSQL image, reject symlinked backup/data roots, write checksums and byte sizes, then atomically rename the partial set.

- [ ] **Step 4: Write smoke-check tests**

Fake `curl` responses and assert failures for green health with wrong version, missing support menu, a lower post-release record count, update guard not returning 409, or gateway `/v1/models` failing. Assert all output is redacted.

- [ ] **Step 5: Run smoke tests and verify RED**

Run: `bash tests/operations/smoke_sub2api_release_test.sh`

Expected: FAIL because the smoke script does not exist.

- [ ] **Step 6: Implement read-only smoke checks**

The fixed requests are:

```bash
curl -fsS "$BASE_URL/health" | jq -e '.status == "ok"'
curl -fsS -H "X-API-Key: $admin_key" "$BASE_URL/api/v1/admin/system/version" |
  jq -e --arg version "$EXPECTED_VERSION" '(.data // .).version == $version'
curl -fsS "$BASE_URL/api/v1/settings/public" |
  jq -e '(.data // .).custom_menu_items | any(.id == "xingqiao-support" and .url == "md:support")'
curl -sS -o "$guard_body" -w '%{http_code}' -X POST \
  -H "X-API-Key: $admin_key" "$BASE_URL/api/v1/admin/system/update"
curl -fsS -H "Authorization: Bearer $gateway_key" "$BASE_URL/v1/models" |
  jq -e '.data | type == "array"'
```

Require 409 plus `DOCKER_DEPLOYMENT_UPDATE_REQUIRED` for both guarded endpoints. Compare the six SQL counts with the backup baseline and require post-release counts to be greater than or equal. Do not make a paid inference request in the automated smoke script; streaming/non-streaming inference remains an explicit rehearsal acceptance using a capped test key.

- [ ] **Step 7: Verify and commit**

```bash
bash tests/operations/backup_sub2api_release_test.sh
bash tests/operations/smoke_sub2api_release_test.sh
git add ops/backup-sub2api-release.sh ops/smoke-sub2api-release.sh tests/operations/backup_sub2api_release_test.sh tests/operations/smoke_sub2api_release_test.sh
git commit -m "ops: add Sub2API release safety checks"
```

### Task 7: Orchestrate Recreate-Only Deployment And Rollback

**Files:**
- Create: `ops/deploy-sub2api-release.sh`
- Create: `tests/operations/deploy_sub2api_release_test.sh`
- Create: `docs/runbooks/sub2api-official-image-release.md`

**Interfaces:**
- Consumes: `--mode rehearsal|production`, base Compose file, image overlay, release env, secret env, data paths, API key files, and backup root.
- Produces: one immutable JSON release record with `previous`, `requested`, `backup`, `checks`, and `state`; recreates only `sub2api`; automatically restores the previous image only when `ROLLBACK_COMPATIBLE=true`.

- [ ] **Step 1: Write a fake-Docker state-machine test**

Assert this exact success order:

```text
inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api -> wait -> smoke -> promoted
```

Assert smoke failure produces:

```text
... -> smoke failed -> compose up sub2api(previous image) -> rollback smoke -> rolled_back
```

Reject production mode unless `PRODUCTION_CONFIRMATION` equals the requested digest and `REHEARSAL_RECORD` is a readable promoted record for the same digest.

- [ ] **Step 2: Run the orchestrator test and verify RED**

Run: `bash tests/operations/deploy_sub2api_release_test.sh`

Expected: FAIL because the deploy script does not exist.

- [ ] **Step 3: Implement preflight and storage identity checks**

Build the Compose command as an array, preserving the production root:

```bash
compose=(docker compose --project-name sub2api-deploy --project-directory "$DEPLOY_ROOT"
  --env-file "$SECRET_ENV" --env-file "$RELEASE_ENV"
  -f "$BASE_COMPOSE" -f "$IMAGE_OVERLAY")
```

Resolve `${compose[@]} config --format json`, require all three service mounts to match the canonical expected paths, require the running container's Compose project label to equal `sub2api-deploy`, require PostgreSQL and Redis healthy, require at least 2 GiB free at backup and data roots, and reject a dirty partial release lock.

- [ ] **Step 4: Implement pull, recreate, health, and rollback**

The only service mutation commands allowed are:

```bash
SUB2API_IMAGE="$requested_image" "${compose[@]}" pull sub2api
SUB2API_IMAGE="$requested_image" "${compose[@]}" up -d --no-deps --force-recreate sub2api
SUB2API_IMAGE="$previous_image" "${compose[@]}" up -d --no-deps --force-recreate sub2api
```

Never call `down`, never recreate dependencies, and never use `--renew-anon-volumes`. Poll `docker inspect` health for up to 180 seconds. Verify the resulting image ID belongs to the requested digest before smoke checks.

- [ ] **Step 5: Write release records atomically**

The record schema is:

```json
{
  "schema_version": 1,
  "mode": "rehearsal",
  "previous": {"image": "xingqiao-sub2api:v0.1.164-contact-v1", "image_id": "sha256:939e6f..."},
  "requested": {"image": "weishaw/sub2api:0.1.164@sha256:a94c25...", "version": "0.1.164"},
  "backup": {"path": "/absolute/path", "sha256_verified": true},
  "checks": {"storage_identity": true, "health": true, "version": true, "records": true, "support": true, "guard": true, "gateway": true},
  "state": "promoted"
}
```

Use full values at runtime; the ellipses above are explanatory only and must not appear in generated records. Mode is `0600`; state is one of `preflight_failed`, `promoted`, `rolled_back`, or `rollback_failed`.

- [ ] **Step 6: Write the runbook**

Document prerequisites, exact rehearsal and production commands, meaning of every gate, restoring the previous custom image, manual database restore boundary, observation window, and routine future release process. Explicitly state that the admin UI updater is unsupported in Docker production.

- [ ] **Step 7: Verify and commit**

```bash
bash tests/operations/deploy_sub2api_release_test.sh
shellcheck ops/capture-sub2api-runtime-baseline.sh ops/configure-sub2api-support.sh ops/backup-sub2api-release.sh ops/smoke-sub2api-release.sh ops/deploy-sub2api-release.sh
git add ops/deploy-sub2api-release.sh tests/operations/deploy_sub2api_release_test.sh docs/runbooks/sub2api-official-image-release.md
git commit -m "ops: add controlled Sub2API image release"
```

Expected: state-machine and shell checks pass.

### Task 8: Rehearse The Official Image In Complete Isolation

**Files:**
- Create: `infra/compose.sub2api-rehearsal.yaml`
- Create: `evidence/sub2api-rehearsal/2026-07-24-result.md`
- Create: `evidence/sub2api-rehearsal/2026-07-24-release-record.json`

**Interfaces:**
- Consumes: a fresh validated backup and temporary bind directories/network/ports.
- Produces: a promoted rehearsal record for the exact production digest plus rollback evidence; never connects to production PostgreSQL, Redis, or bind paths.

- [ ] **Step 1: Add and validate an isolated rehearsal Compose file**

Use project `sub2api-official-rehearsal`, loopback-only port `127.0.0.1:18080:8080`, and temporary absolute directories created by `mktemp -d`. The file must require `REHEARSAL_*_DIR` variables and must not contain any production absolute path.

- [ ] **Step 2: Restore the latest validated backup into rehearsal**

```bash
docker compose -p sub2api-official-rehearsal -f infra/compose.sub2api-rehearsal.yaml up -d postgres redis
docker compose -p sub2api-official-rehearsal -f infra/compose.sub2api-rehearsal.yaml exec -T postgres \
  sh -c 'exec pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges' \
  <"$backup/sub2api.dump"
tar -C "$REHEARSAL_SUB2API_DATA_DIR" -xzf "$backup/app-data.tar.gz"
```

Expected: restore exits 0; no command references the production data directories.

- [ ] **Step 3: Install support configuration and launch official image**

Run `configure-sub2api-support.sh` against `http://127.0.0.1:18080` after the service becomes healthy, then run the deployment orchestrator in `rehearsal` mode with the exact official digest.

- [ ] **Step 4: Run business and browser acceptance**

Verify administrator login, ordinary user login, accounts/groups/prices/balances/usage/API keys counts, account scheduler visibility, `/v1/models`, one capped non-streaming request, one capped streaming request, D04 read-only contract, and relay-ops read-only contract. With Playwright at 1440x900 and 390x844 verify:

```text
sidebar -> 联系客服 -> md:support -> iframe src exactly /support
no iframe URL/query contains token, user_id, src_url, or Authorization
QR visible and decodable; number 1080152144 visible; copy success and fallback work
light and dark render without overlap or horizontal scrolling
```

- [ ] **Step 5: Rehearse rollback**

Recreate only rehearsal `sub2api` with a rollback tag pointing to image ID `sha256:939e6f...`, verify storage/counts and health, then promote the official image again and repeat smoke checks. Record both image IDs and all pass/fail results.

- [ ] **Step 6: Tear down only the rehearsal project**

```bash
docker compose -p sub2api-official-rehearsal -f infra/compose.sub2api-rehearsal.yaml down -v
```

This is permitted only because the exact project is `sub2api-official-rehearsal` and its paths were created for rehearsal. Confirm `sub2api`, `sub2api-postgres`, and `sub2api-redis` production containers remain running and unchanged.

- [ ] **Step 7: Commit non-secret rehearsal evidence**

```bash
git add infra/compose.sub2api-rehearsal.yaml evidence/sub2api-rehearsal/2026-07-24-result.md evidence/sub2api-rehearsal/2026-07-24-release-record.json
git commit -m "test: rehearse official Sub2API image"
```

Expected: record state is `promoted`, requested digest is exact, all checks true, and no secret/token appears.

### Task 9: Production Confirmation Gate And Cutover

**Files:**
- Modify outside repository only through the controlled command: production container state and production `/app/data/pages/support.md`.
- Create: `evidence/sub2api-production/<timestamp>-release-record.json`
- Modify: `docs/migrations/2026-07-24-sub2api-runtime-baseline.md` with post-cutover identity.

**Interfaces:**
- Consumes: promoted Task 8 rehearsal record, explicit user confirmation of the exact digest, protected admin/gateway keys, and a fresh backup.
- Produces: official production container on the same project/network/storage with support and all smoke checks passing, or verified rollback to the custom image.

- [ ] **Step 1: STOP and request explicit production approval**

Present the rehearsal result, exact official digest, fresh-backup destination, current custom rollback image ID, expected interruption window, and the exact command. Do not proceed on a generic “continue”; require confirmation that names `sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659`.

- [ ] **Step 2: Create a durable rollback tag and fresh backup**

```bash
docker tag sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220 \
  xingqiao-sub2api:rollback-20260724-contact-v1
ops/backup-sub2api-release.sh
```

Expected: rollback tag resolves to the exact custom image ID; backup checksums and `pg_restore --list` pass.

- [ ] **Step 3: Deploy homepage/Caddy support boundary first**

Deploy the verified Caddy/homepage image through its existing deployment workflow, then prove public `/support` returns 200, QR hash/content is correct, and both mutation endpoints return guarded 409. Keep the current custom Sub2API container running during this step.

- [ ] **Step 4: Install the menu/page configuration while still on the custom image**

Run `configure-sub2api-support.sh` against production and verify public settings contains exactly one `xingqiao-support` `md:support` item and `/app/data/pages/support.md` matches the repository source. The old top-bar button may coexist for this short transition.

- [ ] **Step 5: Run the production orchestrator**

```bash
PRODUCTION_CONFIRMATION=sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659 \
ops/deploy-sub2api-release.sh --mode production \
  --deploy-root '/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy' \
  --base-compose '/Users/gongtengxinwen/Documents/Codex/2026-07-11/readme-cn-md-https-github-com/sub2api-deploy/docker-compose.yml' \
  --image-overlay "$PWD/infra/compose.sub2api-release.yaml" \
  --release-env "$PWD/config/releases/sub2api.env" \
  --rehearsal-record "$PWD/evidence/sub2api-rehearsal/2026-07-24-release-record.json"
```

Expected: only `sub2api` is recreated; PostgreSQL/Redis container IDs and all bind sources remain unchanged; release state is `promoted`.

- [ ] **Step 6: Observe and either promote or roll back**

For at least 30 minutes inspect error rate, scheduler/account health, login, gateway traffic, usage writes, D04, relay-ops, and support. Any failed required check triggers recreate-only rollback to `xingqiao-sub2api:rollback-20260724-contact-v1`; database restore is never automatic.

- [ ] **Step 7: Record post-cutover identity**

Capture official RepoDigest, image ID, application version, unchanged storage sources and counts, Caddy guard responses, and support browser acceptance. Do not delete the rollback image or fresh backup during the observation window.

### Task 10: Final Boundary And Routine-Update Verification

**Files:**
- Modify: `docs/runbooks/sub2api-official-image-release.md`
- Modify: `docs/migrations/2026-07-24-sub2api-custom-delta-inventory.md`

**Interfaces:**
- Consumes: promoted production release record.
- Produces: stable future one-command release procedure and repository evidence that custom core patches cannot silently return.

- [ ] **Step 1: Run all repository contracts**

```bash
bash tests/infra/validate-official-sub2api-release.sh
bash tests/infra/validate-baseline.sh
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/operations/capture_sub2api_runtime_baseline_test.sh
bash tests/operations/configure_sub2api_support_test.sh
bash tests/operations/backup_sub2api_release_test.sh
bash tests/operations/smoke_sub2api_release_test.sh
bash tests/operations/deploy_sub2api_release_test.sh
cd homepage && npm run test:run && npm run typecheck && npm run build
```

Expected: every command exits 0.

- [ ] **Step 2: Prove the custom core is no longer in the release path**

```bash
! rg -n 'xingqiao-sub2api|build:.*sub2api|../upstream/sub2api|:latest' \
  infra/compose.yaml infra/compose.sub2api-release.yaml config/releases/sub2api.env
! rg -n 'token=|user_id=|src_url=' config/sub2api/support.md
test "$(shasum -a 256 homepage/public/support/qq-group-1080152144.png | awk '{print $1}')" = \
  35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16
```

Expected: all negative searches and hash check pass.

- [ ] **Step 3: Update inventory and routine release instructions**

Mark each inventory row verified with its evidence path. Document that a future update changes only `config/releases/sub2api.env`, reruns rehearsal, requests production digest confirmation, and invokes the same deploy command. Explicitly forbid the in-app update button and floating tags.

- [ ] **Step 4: Review user-owned changes and commit only migration scope**

```bash
git diff --check
git status --short
git diff -- infra config/sub2api config/releases homepage/src/pages homepage/public/support ops/backup-sub2api-release.sh ops/configure-sub2api-support.sh ops/deploy-sub2api-release.sh ops/smoke-sub2api-release.sh tests docs/runbooks/sub2api-official-image-release.md docs/migrations evidence/sub2api-rehearsal
```

Expected: no unrelated dirty-worktree file is staged or reverted.

- [ ] **Step 5: Final commit**

```bash
git add docs/runbooks/sub2api-official-image-release.md docs/migrations/2026-07-24-sub2api-custom-delta-inventory.md
git commit -m "docs: finalize official Sub2API release boundary"
```

## Self-Review

- Spec coverage: Tasks 1-10 cover inventory, support externalization, token-safe menu configuration, Caddy mutation guard, immutable official image, explicit storage, backup, authenticated smoke checks, rollback, rehearsal, production gate, and routine updates.
- Placeholder scan: the plan contains no `TBD`, deferred implementation step, or unspecified error handling. JSON ellipses in Task 7 are explicitly prohibited from runtime artifacts.
- Interface consistency: `SUB2API_IMAGE`, `SUB2API_DATA_DIR`, `POSTGRES_DATA_DIR`, `REDIS_DATA_DIR`, `xingqiao-support`, `md:support`, the official digest, the custom rollback image ID, and release-record states are consistent across all tasks.
- Safety: only the isolated rehearsal project permits `down -v`; production allows recreate-only `sub2api`, preserves the existing Compose project and bind sources, and requires an exact-digest user confirmation after rehearsal.
