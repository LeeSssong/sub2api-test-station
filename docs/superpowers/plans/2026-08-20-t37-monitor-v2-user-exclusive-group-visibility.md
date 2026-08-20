# T37 Monitor V2 Current-User Exclusive Group Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/api/v1/monitor-v2` keep every active public group while projecting an active exclusive group only when the authenticated user can access it through Sub's native group authorization.

**Architecture:** The handler passes only `AuthSubject.UserID`; role is removed from Monitor V2 visibility. `MonitorV2Service` combines `GroupRepository.ListActive()` with a narrow `APIKeyService.GetAvailableGroups(userID)` dependency, filters before T34's `ProjectMonitorV2Groups`, and preserves the existing v7 response and native probe projection.

**Tech Stack:** Go, Gin, Testify, Google Wire, Sub2API service/repository interfaces

**Spec:** `docs/superpowers/specs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-design.md`

## Global Constraints

- Baseline is exactly `main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`.
- Keep every active non-exclusive group visible to every authenticated user.
- Keep an active exclusive group only when its ID is in `APIKeyService.GetAvailableGroups(ctx, userID)`.
- Filter group IDs before `ProjectMonitorV2Groups`; unauthorized exclusive IDs must never reach the native projection reader.
- Admin role does not change visibility; only `AuthSubject.UserID` is an input.
- Preserve Monitor V2 contract version `7`, `Cache-Control: no-store`, stable group order, and 24/28/30 buckets for `24h/7d/30d`.
- Preserve T34's `account_monitor_results`-only projection; do not change probing, scheduling, scoring, billing, grouping, or frontend behavior.
- No migration, schema, configuration, dependency, frontend, GitHub Actions, or production-data changes.
- Do not modify `docs/project/project-progress.md` or `docs/project/native-sub-task-package-queue.md`.
- Candidate ends at `READY_FOR_ROOT_REVIEW`; no main merge, push, deployment, or production access.

## File Structure

- Modify `upstream/sub2api/backend/internal/service/monitor_v2.go`: visibility filter, native authorization dependency, user-bound snapshot, pre-projection fail-closed behavior.
- Modify `upstream/sub2api/backend/internal/service/monitor_v2_test.go`: public/exclusive intersection, errors, order, and native-reader boundary.
- Modify `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`: extract authenticated user ID and remove role scope.
- Modify `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`: user-ID forwarding, role independence, missing-subject rejection.
- Modify `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`: route stub signature and authenticated subject fixture.
- Modify `upstream/sub2api/backend/internal/service/wire.go` and regenerate `upstream/sub2api/backend/cmd/server/wire_gen.go`: inject the existing `*APIKeyService`.
- Create `docs/superpowers/reports/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-verification.md`.
- Create `docs/handoffs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-handoff.md`.

---

### Task 1: Lock the public/exclusive visibility rule

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Test: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`

**Interfaces:**
- Consumes: `Group.ID`, `Group.Status`, `Group.IsExclusive`, `StatusActive`.
- Produces: `monitorV2VisibleGroups(allGroups []Group, availableGroups []Group) ([]Group, []int64)`.

- [ ] **Step 1: Write the failing test**

```go
func TestMonitorV2VisibleGroupsKeepsPublicAndAuthorizedExclusiveInRepositoryOrder(t *testing.T) {
	allGroups := []Group{
		{ID: 1, Name: "Public subscription", Status: StatusActive},
		{ID: 2, Name: "Allowed exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 3, Name: "Denied exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 4, Name: "Inactive exclusive", Status: StatusDisabled, IsExclusive: true},
		{ID: 5, Name: "Public absent from available", Status: StatusActive},
	}
	availableGroups := []Group{{ID: 2}, {ID: 2}, {ID: 4}}

	visible, ids := monitorV2VisibleGroups(allGroups, availableGroups)

	require.Equal(t, []int64{1, 2, 5}, ids)
	require.Equal(t, []string{"Public subscription", "Allowed exclusive", "Public absent from available"}, []string{
		visible[0].Name, visible[1].Name, visible[2].Name,
	})
}
```

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2VisibleGroupsKeepsPublicAndAuthorizedExclusiveInRepositoryOrder$' -count=1 -v
```

Expected: compile failure `undefined: monitorV2VisibleGroups`.

- [ ] **Step 3: Add the minimal filter**

```go
func monitorV2VisibleGroups(allGroups, availableGroups []Group) ([]Group, []int64) {
	availableIDs := make(map[int64]struct{}, len(availableGroups))
	for _, group := range availableGroups {
		availableIDs[group.ID] = struct{}{}
	}
	visibleGroups := make([]Group, 0, len(allGroups))
	groupIDs := make([]int64, 0, len(allGroups))
	for _, group := range allGroups {
		if group.Status != StatusActive {
			continue
		}
		if group.IsExclusive {
			if _, ok := availableIDs[group.ID]; !ok {
				continue
			}
		}
		visibleGroups = append(visibleGroups, group)
		groupIDs = append(groupIDs, group.ID)
	}
	return visibleGroups, groupIDs
}
```

- [ ] **Step 4: Run GREEN**

Run the Step 2 command. Expected: PASS with IDs `1,2,5` in repository order.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/internal/service/monitor_v2_test.go
git commit -m "test: lock monitor group visibility rule"
```

---

### Task 2: Enforce current-user native authorization in MonitorV2Service

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Test: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`

**Interfaces:**
- Consumes: Task 1 filter, `GroupRepository.ListActive`, `MonitorV2NativeProbeReader.ProjectMonitorV2Groups`.
- Produces: `MonitorV2AvailableGroupReader.GetAvailableGroups(context.Context, int64) ([]Group, error)` and `Snapshot(context.Context, int64, MonitorV2Window, time.Time)`.

- [ ] **Step 1: Add the authorization stub and failing tests**

```go
type monitorV2AvailableGroupReaderStub struct {
	groups  []Group
	err     error
	userIDs []int64
}

func (s *monitorV2AvailableGroupReaderStub) GetAvailableGroups(_ context.Context, userID int64) ([]Group, error) {
	s.userIDs = append(s.userIDs, userID)
	return append([]Group(nil), s.groups...), s.err
}
```

Replace the old public/admin visibility test with:

```go
func TestMonitorV2SnapshotUsesCurrentUserAvailableGroupsBeforeNativeProjection(t *testing.T) {
	available := &monitorV2AvailableGroupReaderStub{groups: []Group{{ID: 3}}}
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 1, Name: "Public", Status: StatusActive},
			{ID: 2, Name: "Denied exclusive", Status: StatusActive, IsExclusive: true},
			{ID: 3, Name: "Allowed exclusive", Status: StatusActive, IsExclusive: true},
		}}, available, native, nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.NoError(t, err)
	require.Equal(t, []int64{42}, available.userIDs)
	require.Equal(t, []int64{1, 3}, native.groupIDs)
	require.Equal(t, []int64{1, 3}, []int64{snapshot.Groups[0].ID, snapshot.Groups[1].ID})
}
```

Add fail-closed coverage:

```go
func TestMonitorV2SnapshotStopsBeforeNativeProjectionWhenAuthorizationFails(t *testing.T) {
	available := &monitorV2AvailableGroupReaderStub{err: errors.New("authorization unavailable")}
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 1, Name: "Public", Status: StatusActive}}},
		available, native, nil,
	)

	_, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.ErrorContains(t, err, "authorization unavailable")
	require.Nil(t, native.groupIDs)
}
```

Update every other constructor with an available-reader stub and pass a positive user ID to `Snapshot`.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2' -count=1 -v
```

Expected: compile failure because constructor and `Snapshot` lack the new arguments.

- [ ] **Step 3: Implement the service boundary**

```go
type MonitorV2AvailableGroupReader interface {
	GetAvailableGroups(context.Context, int64) ([]Group, error)
}

type MonitorV2Service struct {
	groupRepo GroupRepository
	available MonitorV2AvailableGroupReader
	native    MonitorV2NativeProbeReader
	settings  MonitorV2SettingsReader
}

func NewMonitorV2Service(groupRepo GroupRepository, available MonitorV2AvailableGroupReader, native MonitorV2NativeProbeReader, settings MonitorV2SettingsReader) *MonitorV2Service {
	return &MonitorV2Service{groupRepo: groupRepo, available: available, native: native, settings: settings}
}
```

Change `Snapshot` to `Snapshot(ctx context.Context, userID int64, window MonitorV2Window, now time.Time)`. Delete `MonitorV2Scope` and its constants. After `ListActive` and before the native call, add:

```go
if userID <= 0 {
	return nil, fmt.Errorf("monitor v2 authenticated user unavailable")
}
if s.available == nil {
	return nil, fmt.Errorf("monitor v2 available group reader unavailable")
}
availableGroups, err := s.available.GetAvailableGroups(ctx, userID)
if err != nil {
	return nil, fmt.Errorf("load available groups for monitor v2: %w", err)
}
visibleGroups, groupIDs := monitorV2VisibleGroups(allGroups, availableGroups)
```

Remove the role-scope loop. Leave window bounds, native projection, empty projections, v7 fields, settings, and 24/28/30 buckets unchanged.

- [ ] **Step 4: Run GREEN**

Run the Step 2 command. Expected: PASS; unauthorized exclusive ID `2` never reaches `native.groupIDs`; authorization failure leaves it nil.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/internal/service/monitor_v2_test.go
git commit -m "fix: scope monitor groups to current user"
```

---

### Task 3: Pass AuthSubject.UserID through handler, routes, and Wire

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Test: `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
- Test: `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Regenerate: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes: Task 2 `Snapshot(context.Context, int64, MonitorV2Window, time.Time)`.
- Produces: role-independent authenticated HTTP projection; existing `*APIKeyService` satisfies the new reader.

- [ ] **Step 1: Write handler RED tests**

Change the snapshotter stub to capture `userID int64` and use the Task 2 signature. Replace the admin-scope test with a table over `service.RoleUser` and `service.RoleAdmin`; set `middleware.AuthSubject{UserID: 42}` and assert both forward `42`. Add a missing-subject test asserting HTTP 401 and zero snapshotter calls. Set an AuthSubject in the existing successful response-contract test and in `TestMonitorV2RouteUsesAuthenticatedUserBoundary`.

Use this assertion core:

```go
for _, role := range []string{service.RoleUser, service.RoleAdmin} {
	context.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	context.Set(string(middleware.ContextKeyUserRole), role)
	handler.Snapshot(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.userID)
}
```

Missing-subject core:

```go
handler.Snapshot(context)
require.Equal(t, http.StatusUnauthorized, recorder.Code)
require.Zero(t, stub.calls)
```

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/handler ./internal/server/routes -run 'TestMonitorV2' -count=1 -v
```

Expected: FAIL until handler forwards the authenticated user ID.

- [ ] **Step 3: Implement the handler boundary**

Change the interface to:

```go
type monitorV2Snapshotter interface {
	Snapshot(context.Context, int64, service.MonitorV2Window, time.Time) (*service.MonitorV2Snapshot, error)
}
```

Replace role scope selection with:

```go
subject, ok := middleware.GetAuthSubjectFromContext(c)
if !ok || subject.UserID <= 0 {
	response.Unauthorized(c, "User not authenticated")
	return
}
snapshot, err := h.service.Snapshot(c.Request.Context(), subject.UserID, window, time.Now().UTC())
```

Preserve `Cache-Control`, window parsing, service availability, error mapping, and response conversion.

- [ ] **Step 4: Wire APIKeyService and regenerate**

```go
func ProvideMonitorV2Service(groupRepo GroupRepository, apiKeyService *APIKeyService, native *AccountMonitorService, settingService *SettingService) *MonitorV2Service {
	return NewMonitorV2Service(groupRepo, apiKeyService, native, settingService)
}
```

Run:

```bash
cd upstream/sub2api/backend
go generate ./cmd/server
```

Require the generated construction line to be:

```go
monitorV2Service := service.ProvideMonitorV2Service(groupRepository, apiKeyService, accountMonitorService, settingService)
```

Reject unrelated generated deltas.

- [ ] **Step 5: Run GREEN and compile checks**

```bash
cd upstream/sub2api/backend
go test ./internal/handler ./internal/service ./internal/server/routes -run 'TestMonitorV2' -count=1 -v
go test ./cmd/server -run '^$' -count=1
go build ./cmd/server
```

Expected: PASS; both roles forward the same user ID and Wire has no cycle.

- [ ] **Step 6: Format and commit**

```bash
cd upstream/sub2api/backend
gofmt -w internal/handler/monitor_v2_handler.go internal/handler/monitor_v2_handler_test.go internal/server/routes/monitor_v2_routes_test.go internal/service/monitor_v2.go internal/service/monitor_v2_test.go internal/service/wire.go
cd ../../..
git diff --check
git add upstream/sub2api/backend/internal/handler/monitor_v2_handler.go \
  upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go \
  upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/backend/cmd/server/wire_gen.go
git commit -m "fix: bind monitor visibility to authenticated user"
```

---

### Task 4: Direct verification and task-owned evidence

**Files:**
- Create: `docs/superpowers/reports/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-verification.md`
- Create: `docs/handoffs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-handoff.md`

**Interfaces:**
- Consumes: completed user-bound handler/service/wire implementation.
- Produces: clean `READY_FOR_ROOT_REVIEW` candidate with reproducible evidence.

- [ ] **Step 1: Run final focused validation**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestMonitorV2' -count=1 -v
go test ./internal/handler -run '^TestMonitorV2' -count=1 -v
go test ./internal/server/routes -run '^TestMonitorV2' -count=1 -v
go test ./cmd/server -run '^$' -count=1
go build ./cmd/server
```

Expected: all PASS.

- [ ] **Step 2: Run scope guards**

```bash
cd ../../..
git diff --check b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD
git diff --name-only b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD
git diff --name-only b5ad0cdd624e3590bd0d19000c0f78cde200ef68...HEAD -- upstream/sub2api/backend/migrations upstream/sub2api/frontend .github/workflows
rg -n 'MonitorV2Scope|GetUserRoleFromContext' upstream/sub2api/backend/internal/handler/monitor_v2_handler.go upstream/sub2api/backend/internal/service/monitor_v2.go
```

Expected: diff check exits 0; migration/frontend/workflow query and stale role-scope search print nothing.

- [ ] **Step 3: Write verification report and handoff**

The report records exact RED failure reasons, GREEN outputs, changed-file scope, and confirms:

```text
public active groups remain visible when absent from GetAvailableGroups;
exclusive active groups use the native current-user intersection;
unauthorized exclusive IDs do not reach ProjectMonitorV2Groups;
authorization errors stop before native projection;
v7 and 24/28/30 buckets remain unchanged.
```

The handoff records Task `T37`, status `READY_FOR_ROOT_REVIEW`, baseline SHA, final committed tip/tree, changed files, tests, no migration/config/production writes, candidate expectation `downtime_required=false`, rollback by reverting T37 commits or retaining the previous blue-green slot/image, and that production/logged-in browser verification remains for root.

- [ ] **Step 4: Commit evidence and verify the final tree**

```bash
git add docs/superpowers/reports/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-verification.md \
  docs/handoffs/2026-08-20-t37-monitor-v2-user-exclusive-group-visibility-handoff.md
git commit -m "docs: hand off t37 monitor visibility fix"
git status --short --branch
git log --oneline --decorate b5ad0cdd624e3590bd0d19000c0f78cde200ef68..HEAD
git rev-parse HEAD
git rev-parse HEAD^{tree}
```

Expected: worktree clean and status reported only as `READY_FOR_ROOT_REVIEW`.

## Plan Self-Review

- Spec coverage: Tasks 1–3 cover current-user identity, public preservation, exclusive native intersection, pre-projection filtering, fail-closed errors, role removal, v7 preservation, and Wire integration; Task 4 covers validation, rollback, and handoff.
- Placeholder scan: no implementation placeholder or deferred behavior remains.
- Type consistency: all tasks use `MonitorV2AvailableGroupReader.GetAvailableGroups(context.Context, int64) ([]Group, error)` and `Snapshot(context.Context, int64, MonitorV2Window, time.Time)`.
- Scope check: one backend projection-boundary change; no frontend or independent subsystem is bundled.
