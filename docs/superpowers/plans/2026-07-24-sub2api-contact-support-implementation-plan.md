# Sub2API Contact Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top-header “联系客服” entry to Sub2API v0.1.164 that opens the supplied QQ group QR code, shows group `1080152144`, and copies the number in one click.

**Architecture:** Import an auditable source snapshot of upstream commit `cd8bb98c44303b2c8f04c0da340447c992f0cb7d` under `upstream/sub2api`, customize its Vue frontend, and build the normal upstream all-in-one image with the frontend embedded in the Go binary. Keep PostgreSQL, Redis, and all existing volumes unchanged.

**Tech Stack:** Vue 3, TypeScript, Tailwind CSS, Vue Test Utils, Vitest, Go embedded assets, Docker Compose.

## Global Constraints

- Upstream stays fixed at `Wei-Shaw/sub2api` v0.1.164 commit `cd8bb98c44303b2c8f04c0da340447c992f0cb7d` (annotated tag object `38a46fd33795c8946a1e88d0f72597c79ca02a76`).
- QQ group number is exactly `1080152144`.
- QR source is `/Users/gongtengxinwen/Pictures/中转站群二维码.jpg` and must be shipped locally, not fetched at runtime.
- Existing PostgreSQL, Redis, `sub2api_data`, and other named volumes remain unchanged.
- Existing unrelated worktree changes must not be overwritten, reverted, or included in feature commits.
- The current `homepage` application is out of scope.

---

### Task 1: Import The Auditable Upstream Source Snapshot

**Files:**
- Create: `upstream/sub2api/**`
- Create: `upstream/sub2api/XINGQIAO_UPSTREAM.md`

**Interfaces:**
- Consumes: upstream Git commit `cd8bb98c44303b2c8f04c0da340447c992f0cb7d`.
- Produces: a buildable Sub2API source tree at `upstream/sub2api` with its normal root `Dockerfile`.

- [ ] **Step 1: Verify the requested tag resolves to the pinned commit**

Run:

```bash
git ls-remote https://github.com/Wei-Shaw/sub2api.git refs/tags/v0.1.164
```

Expected: the peeled `refs/tags/v0.1.164^{}` output begins with `cd8bb98c44303b2c8f04c0da340447c992f0cb7d`.

- [ ] **Step 2: Export the exact source without nested Git metadata**

```bash
git -C /tmp/sub2api-upstream.OxPr7f archive cd8bb98c44303b2c8f04c0da340447c992f0cb7d | tar -x -C upstream/sub2api
```

- [ ] **Step 3: Record provenance**

Create `XINGQIAO_UPSTREAM.md` with the repository URL, tag, full commit, import date, and a note that Xingqiao customizations are maintained in this snapshot.

- [ ] **Step 4: Verify baseline source identity and build inputs**

Run:

```bash
test -f upstream/sub2api/Dockerfile
test -f upstream/sub2api/frontend/pnpm-lock.yaml
test -f upstream/sub2api/backend/go.sum
rg -n 'v0.1.164|cd8bb98|38a46fd' upstream/sub2api/XINGQIAO_UPSTREAM.md
```

Expected: all commands exit 0.

### Task 2: Build The Dialog With Test-First Behavior

**Files:**
- Create: `upstream/sub2api/frontend/src/components/layout/ContactSupportDialog.vue`
- Create: `upstream/sub2api/frontend/src/components/layout/__tests__/ContactSupportDialog.spec.ts`
- Create: `upstream/sub2api/frontend/public/support/qq-group-1080152144.png`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/common.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/common.ts`

**Interfaces:**
- Consumes: `BaseDialog`, `Icon`, `useClipboard`, and `show: boolean`.
- Produces: `ContactSupportDialog` with a single `close` event.

- [ ] **Step 1: Copy the supplied QR resource into the frontend public tree**

```bash
cp '/Users/gongtengxinwen/Pictures/中转站群二维码.jpg' upstream/sub2api/frontend/public/support/qq-group-1080152144.png
```

- [ ] **Step 2: Write failing component tests**

The test must mount the dialog with `attachTo: document.body` and assert:

```ts
expect(document.body.textContent).toContain('1080152144')
expect(document.body.querySelector('img')?.getAttribute('src'))
  .toBe('/support/qq-group-1080152144.png')
await wrapper.get('[data-testid="copy-qq-group"]').trigger('click')
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('1080152144')
expect(document.body.textContent).toContain('common.copied')
```

Add separate cases for clipboard failure, close button/overlay emission through `BaseDialog`, and `Escape` emission.

- [ ] **Step 3: Run the focused test and verify RED**

Run:

```bash
cd upstream/sub2api/frontend && pnpm test:run src/components/layout/__tests__/ContactSupportDialog.spec.ts
```

Expected: FAIL because `ContactSupportDialog.vue` does not exist.

- [ ] **Step 4: Add localized copy**

Add this structure to both common locale objects:

```ts
contactSupportDialog: {
  title: '联系客服',
  groupNumber: 'QQ群号',
  copyGroupNumber: '复制群号',
  scanQrCode: '使用 QQ 扫描二维码加入群聊',
  qrAlt: 'QQ群 1080152144 二维码'
}
```

Use equivalent English values in `en/common.ts`.

- [ ] **Step 5: Implement the minimal dialog**

Use `BaseDialog` with `width="narrow"`, `close-on-click-outside`, and a square `overflow-hidden` QR viewport. Define:

```ts
const QQ_GROUP_NUMBER = '1080152144'
const copyFailed = ref(false)
const { copied, copyToClipboard } = useClipboard()

async function copyGroupNumber() {
  copyFailed.value = !(await copyToClipboard(QQ_GROUP_NUMBER))
}
```

The copy button text is `common.copied`, `common.copyFailed`, or `contactSupportDialog.copyGroupNumber` according to state and includes `data-testid="copy-qq-group"`.

- [ ] **Step 6: Run the focused test and verify GREEN**

Run the same focused Vitest command. Expected: all dialog tests pass with 0 failures.

### Task 3: Add The Header Entry With Test-First Integration

**Files:**
- Create: `upstream/sub2api/frontend/src/components/layout/__tests__/AppHeaderContactSupport.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppHeader.vue`

**Interfaces:**
- Consumes: `ContactSupportDialog` with `show` and `close`.
- Produces: a logged-in header button labeled by `common.contactSupport`.

- [ ] **Step 1: Write a failing header integration test**

Mock the existing stores/router and mount `AppHeader`. Assert:

```ts
const trigger = wrapper.get('[data-testid="contact-support-trigger"]')
expect(trigger.attributes('aria-label')).toBe('common.contactSupport')
await trigger.trigger('click')
expect(document.body.textContent).toContain('1080152144')
```

Then click the dialog close control and assert the QQ number is removed from `document.body`.

- [ ] **Step 2: Run the header test and verify RED**

Expected: FAIL because the support trigger does not exist.

- [ ] **Step 3: Add the entry and state to `AppHeader.vue`**

Place the trigger between `AnnouncementBell` and the docs link. Use the existing `Icon name="chat"`, keep the icon visible on narrow screens, and show text from `md` upward. Add:

```ts
const contactSupportOpen = ref(false)
```

Render:

```vue
<ContactSupportDialog
  :show="contactSupportOpen"
  @close="contactSupportOpen = false"
/>
```

- [ ] **Step 4: Run dialog and header tests and verify GREEN**

Expected: both focused test files pass with 0 failures.

### Task 4: Switch Compose To The Custom Source Build

**Files:**
- Modify: `infra/compose.yaml`
- Create: `tests/infra/validate-custom-sub2api-build.sh`

**Interfaces:**
- Consumes: `upstream/sub2api/Dockerfile` and source tree.
- Produces: Compose service image `xingqiao-sub2api:v0.1.164-contact-v1`.

- [ ] **Step 1: Write a failing deployment contract check**

The script must assert that the `sub2api` service has build context `../upstream/sub2api`, uses its root Dockerfile, has the expected custom image tag, and retains the existing `sub2api_data:/app/data` mount.

- [ ] **Step 2: Run the contract check and verify RED**

Expected: FAIL because Compose still references `weishaw/sub2api@sha256:...`.

- [ ] **Step 3: Modify only the Sub2API image source**

Use:

```yaml
sub2api:
  build:
    context: ../upstream/sub2api
    dockerfile: Dockerfile
    args:
      VERSION: v0.1.164-xingqiao-contact-v1
      COMMIT: cd8bb98c44303b2c8f04c0da340447c992f0cb7d
  image: xingqiao-sub2api:v0.1.164-contact-v1
```

Do not alter service environment, dependencies, healthcheck, or volumes.

- [ ] **Step 4: Run the deployment contract and Compose validation**

Run:

```bash
bash tests/infra/validate-custom-sub2api-build.sh
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
```

Expected: both commands exit 0.

### Task 5: Full Verification And Visual Acceptance

**Files:**
- Modify only if verification finds a requirement failure.

**Interfaces:**
- Consumes: completed customized source and Compose build.
- Produces: fresh evidence for tests, types, production frontend build, image build, and responsive rendering.

- [ ] **Step 1: Run focused and full frontend verification**

```bash
cd upstream/sub2api/frontend
pnpm test:run src/components/layout/__tests__/ContactSupportDialog.spec.ts src/components/layout/__tests__/AppHeaderContactSupport.spec.ts
pnpm typecheck
pnpm build
```

Expected: 0 test failures, 0 type errors, and a successful Vite build.

- [ ] **Step 2: Build the custom image**

```bash
docker compose --env-file infra/.env.example -f infra/compose.yaml build sub2api
```

Expected: image `xingqiao-sub2api:v0.1.164-contact-v1` builds successfully with the frontend embedded.

- [ ] **Step 3: Run the local service without touching production**

Use an isolated Compose project and temporary environment values, then wait for the Sub2API healthcheck before opening the page.

- [ ] **Step 4: Capture desktop and mobile screenshots**

Verify at 1440x900 and 390x844 that the header entry is visible, the dialog is within the viewport, the QR code is sharp and unobstructed, text does not overlap, and keyboard close works.

- [ ] **Step 5: Review the final diff and data-safety boundary**

Run:

```bash
git diff --check
git status --short
git diff -- infra/compose.yaml upstream/sub2api/frontend/src upstream/sub2api/XINGQIAO_UPSTREAM.md tests/infra/validate-custom-sub2api-build.sh
```

Expected: only the intended source snapshot, support feature, QR asset, deployment contract, Compose change, and plan/spec files are in scope; no secrets or volume deletions appear.
