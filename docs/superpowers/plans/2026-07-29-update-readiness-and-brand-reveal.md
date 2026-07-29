# Update Readiness and Brand Reveal Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make qualified-candidate readiness explicit, keep unavailable upgrades disabled, restore theme-aware homepage particles, load the qualified Sub2API `0.1.168` candidate, and roll out only updater/Caddy surfaces.

**Architecture:** The host updater remains the authority for official-target and local-image qualification. It exposes an authenticated, side-effect-free readiness endpoint and a typed candidate-not-ready error that the update mutation maps to HTTP 409. The injected update UI polls this endpoint, while `BrandReveal` receives the resolved homepage theme and rebuilds its Canvas palette when the theme changes.

**Tech Stack:** Go 1.24+, `net/http`, Docker CLI, Node.js test runner, JSDOM, React 19, TypeScript, Vitest, Canvas 2D, GitHub Actions, Docker Compose, systemd

## Global Constraints

- A missing candidate is `UPDATE_CANDIDATE_NOT_READY`, never a generic updater outage.
- Readiness checks must not create an operation, write updater state, or start an executor.
- The update POST must re-resolve the candidate after any earlier readiness check.
- The dialog polls readiness every 30 seconds and keeps submit disabled until the exact target is ready.
- Dark particles use `#dce4fa`; light particles use `#354e78`.
- Candidate qualification must use `.github/workflows/sub2api-release-preparation.yml`; do not manufacture qualification labels manually.
- Loading `0.1.168` must not switch the running Sub2API container.
- Only updater and Caddy surfaces may be rolled out.
- PostgreSQL and Redis container IDs, start times, and restart counts must be identical before and after deployment.
- Do not rebuild relay-ops, PostgreSQL, or Redis.

---

### Task 1: Typed Candidate Readiness in the Resolver

**Files:**
- Modify: `sub2api-updater/internal/updater/resolver.go`
- Test: `sub2api-updater/internal/updater/resolver_test.go`

**Interfaces:**
- Consumes: `CommandRunner.Run(context.Context, []string, string, ...string)`
- Produces: exported sentinel `ErrCandidateNotReady`
- Produces: unchanged `UpdateResolver.Resolve(context.Context, string) (string, error)`

- [ ] **Step 1: Write the failing missing-image classification test**

Add:

```go
func TestResolverClassifiesMissingQualifiedImageAsCandidateNotReady(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{
		stderr: "Error response from daemon: No such image: xingqiao-sub2api:upstream-1.2.3",
		err:    errors.New("exit status 1"),
	}}}

	_, err := NewResolver(github.Client(), github.URL, docker).Resolve(context.Background(), "1.2.3")
	if !errors.Is(err, ErrCandidateNotReady) {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run the resolver test and verify RED**

Run:

```bash
go -C sub2api-updater test ./internal/updater -run TestResolverClassifiesMissingQualifiedImageAsCandidateNotReady -count=1
```

Expected: FAIL because `ErrCandidateNotReady` does not exist.

- [ ] **Step 3: Implement the typed error**

Declare:

```go
var ErrCandidateNotReady = errors.New("qualified update candidate is not ready")
```

When `docker image inspect` fails, wrap that sentinel while retaining the
target version and command failure:

```go
return "", fmt.Errorf("%w for %s: %v", ErrCandidateNotReady, target, commandFailure(stderr, err))
```

Do not classify malformed/mismatched qualification labels as readiness; those
remain hard errors.

- [ ] **Step 4: Run resolver tests and verify GREEN**

```bash
go -C sub2api-updater test ./internal/updater -run Resolver -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sub2api-updater/internal/updater/resolver.go sub2api-updater/internal/updater/resolver_test.go
git commit -m "fix: classify unavailable update candidates"
```

### Task 2: Side-Effect-Free Readiness API

**Files:**
- Modify: `sub2api-updater/internal/updater/service.go`
- Modify: `sub2api-updater/internal/updater/http.go`
- Test: `sub2api-updater/internal/updater/service_test.go`
- Test: `sub2api-updater/internal/updater/http_test.go`

**Interfaces:**
- Consumes: `Resolver.Resolve(context.Context, string) (string, error)`
- Produces: `Readiness` with `TargetVersion`, `Ready`, and `Reason`
- Produces: `Service.Readiness(context.Context, string) (Readiness, error)`
- Produces: `GET /api/v1/admin/system/host-update/readiness?target_version=<version>`
- Produces: API code `UPDATE_CANDIDATE_NOT_READY`

- [ ] **Step 1: Write failing service tests**

Add one test where the resolver returns an image and assert:

```go
readiness, err := service.Readiness(context.Background(), "1.2.3")
if err != nil || !readiness.Ready || readiness.TargetVersion != "1.2.3" {
	t.Fatalf("readiness=%#v err=%v", readiness, err)
}
if _, err := service.Status(); !errors.Is(err, ErrNoOperation) {
	t.Fatalf("readiness created operation: %v", err)
}
```

Add a second test where the resolver returns `ErrCandidateNotReady`; assert
`Ready == false`, `Reason == "candidate_not_ready"`, no operation, and no state
file.

- [ ] **Step 2: Run service readiness tests and verify RED**

```bash
go -C sub2api-updater test ./internal/updater -run TestServiceReadiness -count=1
```

Expected: FAIL because `Readiness` and `Service.Readiness` do not exist.

- [ ] **Step 3: Implement the service method**

Use:

```go
type Readiness struct {
	TargetVersion string `json:"target_version"`
	Ready         bool   `json:"ready"`
	Reason        string `json:"reason,omitempty"`
}
```

`Service.Readiness` calls only `s.resolver.Resolve`. Convert
`ErrCandidateNotReady` into a normal `Readiness{Ready:false}` result and return
all other errors.

- [ ] **Step 4: Run service readiness tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Write failing HTTP behavior tests**

Add tests proving:

- active admin + target `1.2.3` gets HTTP 200 and `ready=true`;
- missing target gets HTTP 400 `UPDATE_CONFIRMATION_REQUIRED`;
- missing candidate gets HTTP 200 with `ready=false` and
  `reason=candidate_not_ready`;
- missing bearer and non-admin identity are rejected;
- readiness produces no executor call and no operation;
- POST with resolver error `ErrCandidateNotReady` gets HTTP 409 and
  `UPDATE_CANDIDATE_NOT_READY`.

- [ ] **Step 6: Run HTTP tests and verify RED**

```bash
go -C sub2api-updater test ./internal/updater -run 'TestHTTPReadiness|TestHTTPUpdateReturnsCandidateNotReady' -count=1
```

Expected: FAIL because the route and API code do not exist.

- [ ] **Step 7: Implement the HTTP route and error mapping**

Register:

```go
mux.HandleFunc("GET /api/v1/admin/system/host-update/readiness", h.readiness)
```

Authorize with `h.authorize(w, r, false)`, require exactly one non-empty
`target_version`, call `h.service.Readiness`, and return the official envelope.
In `writeServiceError`, map `ErrCandidateNotReady` to:

```go
writeError(w, http.StatusConflict, codeCandidateNotReady)
```

- [ ] **Step 8: Run updater tests and verify GREEN**

```bash
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add sub2api-updater/internal/updater/service.go sub2api-updater/internal/updater/service_test.go sub2api-updater/internal/updater/http.go sub2api-updater/internal/updater/http_test.go
git commit -m "feat: expose qualified update readiness"
```

### Task 3: Route Readiness Through Caddy

**Files:**
- Modify: `infra/Caddyfile`
- Modify: `tests/infra/validate-sub2api-update-routing.sh`

**Interfaces:**
- Consumes: updater Unix socket `/run/sub2api-updater/updater.sock`
- Produces: same-origin proxy for the readiness endpoint

- [ ] **Step 1: Add a failing routing contract**

Require:

```bash
require_fixed 'path /api/v1/admin/system/host-update/readiness' infra/Caddyfile
require_fixed 'reverse_proxy @sub2api_host_update_readiness unix//run/sub2api-updater/updater.sock' infra/Caddyfile
```

- [ ] **Step 2: Run the contract and verify RED**

```bash
bash tests/infra/validate-sub2api-update-routing.sh
```

Expected: FAIL because the matcher is absent.

- [ ] **Step 3: Add the Caddy matcher**

Place it before the generic Sub2API proxy:

```caddyfile
@sub2api_host_update_readiness {
	method GET
	path /api/v1/admin/system/host-update/readiness
}
reverse_proxy @sub2api_host_update_readiness unix//run/sub2api-updater/updater.sock
```

- [ ] **Step 4: Run the routing contract and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add infra/Caddyfile tests/infra/validate-sub2api-update-routing.sh
git commit -m "feat: route update candidate readiness"
```

### Task 4: Disable Unready Updates in the Host UI

**Files:**
- Modify: `infra/sub2api-update-ui/update-ui.js`
- Modify: `infra/sub2api-update-ui/index.html`
- Test: `tests/infra/sub2api-update-ui.test.mjs`
- Test: `homepage/src/test/update-ui.contract.test.ts`

**Interfaces:**
- Consumes: readiness envelope from Task 2
- Produces: readiness UI state and 30-second poll lifecycle

- [ ] **Step 1: Extend the test fetch fixture**

Make the default fixture respond to:

```text
/api/v1/admin/system/host-update/readiness?target_version=1.2.3
```

with:

```js
response({ code: 0, data: { target_version: '1.2.3', ready: true } })
```

- [ ] **Step 2: Write failing candidate-not-ready tests**

Add tests that return:

```js
{ code: 0, data: { target_version: '1.2.3', ready: false, reason: 'candidate_not_ready' } }
```

and assert:

- message contains `候选版本正在准备，暂不可升级`;
- submit remains disabled even after confirmation;
- no POST request occurs after clicking submit;
- readiness polling is active;
- after the next poll returns `ready=true`, confirmation enables submit;
- closing the dialog removes the readiness poll timer.

Add a POST-envelope test proving
`UPDATE_CANDIDATE_NOT_READY` is rendered with the preparation copy, not
`更新服务不可用`.

- [ ] **Step 3: Run UI tests and verify RED**

```bash
node --test tests/infra/sub2api-update-ui.test.mjs
pnpm --dir homepage test:run -- src/test/update-ui.contract.test.ts
```

Expected: FAIL because readiness state and polling are absent.

- [ ] **Step 4: Implement readiness state**

Add:

```js
var READINESS_PATH = '/api/v1/admin/system/host-update/readiness'
var READINESS_POLL_INTERVAL_MS = 30000
```

Track `candidateReady`, `readinessTimer`, and the exact target. Fetch readiness
after update information is known. Update `updateSubmitState` so readiness is a
required condition. Keep scheduled/running-operation rendering authoritative.
Stop the readiness timer in `closeDialog`.

- [ ] **Step 5: Add explicit error copy**

Map `UPDATE_CANDIDATE_NOT_READY` to
`候选版本正在准备，暂不可升级` and distinguish readiness network failures as
actual updater unavailability.

- [ ] **Step 6: Bust the immutable UI asset version**

Update the query string in `infra/sub2api-update-ui/index.html` and the matching
contract expectation so browsers cannot retain the pre-readiness script.

- [ ] **Step 7: Run UI tests and verify GREEN**

Run the commands from Step 3 plus:

```bash
bash tests/infra/validate-sub2api-update-routing.sh
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add infra/sub2api-update-ui/update-ui.js infra/sub2api-update-ui/index.html tests/infra/sub2api-update-ui.test.mjs homepage/src/test/update-ui.contract.test.ts tests/infra/validate-sub2api-update-routing.sh
git commit -m "fix: gate updates on qualified candidate readiness"
```

### Task 5: Theme-Aware Brand Canvas

**Files:**
- Modify: `homepage/src/App.tsx`
- Modify: `homepage/src/sections/BrandReveal.tsx`
- Modify: `homepage/src/sections/BrandReveal.test.tsx`
- Test: `homepage/src/App.test.tsx`

**Interfaces:**
- Consumes: `Theme` already resolved by `App`
- Produces: `BrandReveal({ theme }: { theme: Theme })`

- [ ] **Step 1: Read the test-quality rules**

Read `superpowers:test-driven-development/writing-good-tests.md` from the
installed skill directory before editing the Canvas tests.

- [ ] **Step 2: Write failing theme propagation and Canvas tests**

Update the App test to assert the rendered brand section records the supplied
theme. In the component test, install Canvas 2D fakes that capture `fillStyle`
and `fillRect`, render dark then light, and assert:

```ts
expect(darkDrawColors).toContain('#dce4fa')
expect(lightDrawColors).toContain('#354e78')
```

Keep the reduced-motion fallback test unchanged except for the required
`theme` prop.

- [ ] **Step 3: Run the focused tests and verify RED**

```bash
pnpm --dir homepage test:run -- src/sections/BrandReveal.test.tsx src/App.test.tsx
```

Expected: FAIL because `BrandReveal` does not accept or use `theme`.

- [ ] **Step 4: Implement theme propagation**

Pass `theme` from `App`:

```tsx
<BrandReveal theme={theme} />
```

Define:

```ts
const particleColor: Record<Theme, string> = {
  dark: '#dce4fa',
  light: '#354e78',
}
```

Use `particleColor[theme]` for source text and cells, include `theme` in the
Canvas effect dependency, and expose `data-theme={theme}` for browser
acceptance.

- [ ] **Step 5: Run focused and full homepage verification**

```bash
pnpm --dir homepage test:run -- src/sections/BrandReveal.test.tsx src/App.test.tsx
pnpm --dir homepage typecheck
pnpm --dir homepage test:run
pnpm --dir homepage build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add homepage/src/App.tsx homepage/src/App.test.tsx homepage/src/sections/BrandReveal.tsx homepage/src/sections/BrandReveal.test.tsx
git commit -m "fix: theme homepage brand particles"
```

### Task 6: Integrated Verification and Review

**Files:**
- Verify all files changed in Tasks 1-5

**Interfaces:**
- Consumes all code behavior from Tasks 1-5
- Produces a reviewed branch ready for `main`

- [ ] **Step 1: Format**

```bash
gofmt -w sub2api-updater/internal/updater/resolver.go sub2api-updater/internal/updater/resolver_test.go sub2api-updater/internal/updater/service.go sub2api-updater/internal/updater/service_test.go sub2api-updater/internal/updater/http.go sub2api-updater/internal/updater/http_test.go
```

- [ ] **Step 2: Run complete relevant suites**

```bash
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
node --test tests/infra/sub2api-update-ui.test.mjs
bash tests/infra/validate-sub2api-update-routing.sh
bash tests/operations/update_sub2api_host_test.sh
pnpm --dir homepage typecheck
pnpm --dir homepage test:run
pnpm --dir homepage build
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 3: Browser visual regression**

Serve the production homepage build locally. Capture the bottom brand reveal at
desktop size in dark and light themes. For each capture verify:

- `data-canvas-active="true"`;
- `data-theme` matches the page theme;
- Canvas has non-zero dimensions;
- computed section background differs visibly from the sampled particle color;
- console contains no errors or warnings.

- [ ] **Step 4: Review the branch**

Review the full base-to-HEAD diff for:

- readiness checks that mutate state;
- a POST path that could bypass revalidation;
- timer leaks;
- auth/origin regression;
- Docker qualification weakening;
- Canvas cleanup or resize regression;
- any Compose command that includes PostgreSQL or Redis.

- [ ] **Step 5: Re-run affected tests after review fixes**

Repeat Step 2 after any change. Expected: every command exits 0.

### Task 7: Integrate and Prepare the `0.1.168` Candidate

**Files:**
- Merge the implementation branch into `main`
- Execute: `.github/workflows/sub2api-release-preparation.yml`

**Interfaces:**
- Consumes reviewed implementation commits
- Produces pushed `main`, immutable candidate artifacts, production-loaded qualified image, and source-advance commit

- [ ] **Step 1: Verify integration preconditions**

```bash
git status --short
git fetch origin main
git rev-list --left-right --count origin/main...main
```

Expected: implementation worktree clean; remote has no unknown commits that
would be overwritten.

- [ ] **Step 2: Merge with a normal merge commit**

Merge the implementation branch into `main`, resolve without dropping either
the readiness or Canvas changes, and rerun Task 6 Step 2 on `main`.

- [ ] **Step 3: Push without force**

```bash
git push origin main
```

Expected: ordinary fast-forward push succeeds.

- [ ] **Step 4: Dispatch the trusted release workflow**

Run `Sub2API release preparation` on the pushed `main`. Record the workflow run
ID. Do not use a locally relabelled image.

- [ ] **Step 5: Inspect every qualification job**

Require success for:

- `discover`;
- `prepare`;
- `publish`;
- `stage-production`;
- `advance-source`;
- all later evidence/report jobs defined by the workflow.

Download and inspect the artifacts. Confirm version `0.1.168`, official commit,
source commit, immutable digest, and audit branch agree across metadata,
preparation, publish, and production-stage evidence.

- [ ] **Step 6: Reconcile source advancement**

Fetch `origin/main`, verify the workflow source-advance commit contains the
implementation commits, then fast-forward local `main`. Do not force-push.

- [ ] **Step 7: Verify production candidate without switching service**

On production:

```bash
docker image inspect xingqiao-sub2api:upstream-0.1.168
docker compose ps -q sub2api
```

Require Linux AMD64, `qualified=true`, version `0.1.168`, matching official and
source commits, and an unchanged running Sub2API container ID.

### Task 8: Roll Out Only Updater and Caddy

**Files:**
- Deploy updater binary/unit/executor/UI files from final `main`
- Build and deploy Caddy/homepage image from final `main`

**Interfaces:**
- Consumes production-loaded candidate and final source revision
- Produces production readiness and visible brand particles without database recreation

- [ ] **Step 1: Capture immutable production baseline**

Record to a timestamped release evidence directory:

```text
postgres container ID / StartedAt / RestartCount
redis container ID / StartedAt / RestartCount
sub2api container ID / image / revision
relay-ops container ID / image / revision
updater binary, unit, executor, and UI SHA-256
Caddy container ID / image / revision
production Compose SHA-256
```

- [ ] **Step 2: Build and validate deployable artifacts off-path**

Build the Linux AMD64 updater with the installer flags, build the Caddy/homepage
image with the final source revision, and validate:

```bash
go -C sub2api-updater test ./... -count=1
systemd-analyze verify infra/systemd/sub2api-updater.service
docker compose config
```

No running container changes occur in this step.

- [ ] **Step 3: Back up updater/Caddy artifacts**

Copy the existing updater binary, unit, environment file, executor, update UI,
Compose file, and current Caddy image reference into the release evidence
directory. Do not back up or alter database volumes because no database
mutation is planned.

- [ ] **Step 4: Install and restart updater**

Atomically install the validated updater binary, unit/executor contract, and
update UI assets. Reload systemd and restart only
`sub2api-updater.service`. Verify the Unix socket, active status, and
unauthenticated 401 behavior.

- [ ] **Step 5: Recreate only Caddy**

Update only the Caddy image reference and run:

```bash
docker compose up -d --no-deps caddy
```

Do not name any other service.

- [ ] **Step 6: Verify readiness and UI without upgrading**

With an authenticated admin session:

- readiness for `0.1.168` returns `ready=true`;
- the dialog enables submit after confirmation;
- no POST `/system/update` is sent;
- no updater operation is created.

- [ ] **Step 7: Verify production light/dark Canvas**

Capture production homepage bottom-section screenshots in both themes and
verify the Canvas is active, particles have useful contrast, and the console is
clean.

- [ ] **Step 8: Enforce the database invariants**

Re-read PostgreSQL and Redis container ID, `StartedAt`, and `RestartCount`.
Compare byte-for-byte with Step 1. Any difference fails the deployment and must
be reported; do not attempt database recreation as remediation.

- [ ] **Step 9: Final service acceptance**

Verify:

- updater active, socket present;
- Caddy running on the final revision;
- public homepage, `/health`, admin UI, updater status, and readiness succeed;
- Sub2API and relay-ops container IDs remain unchanged;
- PostgreSQL and Redis invariants remain unchanged;
- `docker compose ps` has no unhealthy application service;
- no actual Sub2API update operation was scheduled or executed.

- [ ] **Step 10: Retain rollback evidence and clean staging**

Keep the timestamped updater/Caddy rollback bundle and candidate qualification
evidence. Remove only verified temporary build/staging artifacts. Preserve the
previous Caddy and Sub2API images until post-deployment acceptance is complete.
