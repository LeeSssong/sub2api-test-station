# Final review fix wave report

## Outcome

Implemented every finding in `final-fix-brief.md` without changing updater HTTP
routes or unrelated UI behavior.

## Changes

- Bound each readiness response to its dialog instance, target, and generation.
  New dialogs reset readiness before rendering; stale responses cannot change a
  replacement dialog.
- On `UPDATE_TARGET_CHANGED` from readiness or POST, the dialog fails closed,
  reloads official update information, displays the refreshed target, and polls
  only that refreshed target.
- Changed candidate preparation copy to the explicit `warning` tone and added a
  non-danger warning style.
- Cleared `BrandReveal`'s animation-frame ref during effect cleanup so pointer
  animation resumes after a theme rebuild.
- Completed resolver qualification: the latest release tag is resolved through
  GitHub's `git/ref/tags/<tag>` endpoint and annotated tags are dereferenced via
  `git/tags/<sha>`. The resulting immutable official commit, a valid source
  commit, Linux, AMD64, version, digest, and qualification label are required.
  This deliberately does not use release `target_commitish`, which is `main`
  for the production `v0.1.168` release.

## TDD evidence

- The supplied stale-dialog regression was already verified RED before this
  wave; it is now green.
- New RED tests were observed for incomplete/mismatched resolver qualification,
  readiness and POST target changes, warning tone, and pointer animation after
  a theme rebuild. Each was made green with the minimal associated change.
- Resolver tests include both direct and annotated tag references, and use the
  production-representative `target_commitish: "main"` response shape.

## Verification

All commands completed successfully:

```text
go test ./...                                      # sub2api-updater: all packages pass
node --test --test-name-pattern='ignores readiness responses from a closed dialog' tests/infra/sub2api-update-ui.test.mjs
node --test tests/infra/sub2api-update-ui.test.mjs # 17/17 pass
pnpm --dir homepage test:run -- src/test/update-ui.contract.test.ts # 77/77 pass
pnpm --dir homepage test:run                       # 77/77 pass
bash tests/infra/validate-sub2api-update-routing.sh # pass
git diff --check                                    # clean
```

## Concerns

None. No deployment or main-worktree changes were made.
