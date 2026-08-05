# Task 1 report: version-pinned `0.1.171` release conflict resolver

## Implementation

- Extended the `0.1.169 -> 0.1.171` manifest with clean-index preimages for the five files whose clean merge results must receive the same semantic correction as the official release. The recorded postimage patch also includes the additional non-conflicting hunks in the upstream billing probe update test.
- Added exact patch-scope validation. The resolver now rejects a pinned patch that touches a path outside the semantic conflict set plus clean-preimage set.
- Added an index/worktree snapshot before the first write. Any failure after mutation restores the original unmerged index and worktree, preserving fail-closed behavior.
- Applied the compact zero-context semantic patch with `git apply --unidiff-zero`; conflict stages, clean preimages, manifest identities, patch blob, generated paths, conflict markers, unmerged entries, and unstaged changes remain gated.
- Regenerated `backend/go.sum` and `backend/cmd/server/wire_gen.go` through the Go toolchain in the resolver. The resolver still rejects unknown release records and identity/conflict/preimage mismatches before writing.

## Verification

Commands and observed output:

```text
ruby tests/operations/merge_sub2api_release_test.rb
11 runs, 79 assertions, 0 failures, 0 errors, 0 skips

bash -n ops/merge-sub2api-release.sh
bash -n ops/resolve-sub2api-release-conflicts.sh
git diff --check
all task1 focused checks passed
```

Focused semantic backend tests on a clean resolved `0.1.171` candidate:

```text
GOFLAGS=-mod=mod go -C <candidate>/backend test ./internal/repository -run 'Test(UpdateUpstreamBillingProbeSnapshot|LockAndMergeAccountProbeExtra)' -count=1
ok  github.com/Wei-Shaw/sub2api/internal/repository  1.946s

GOFLAGS=-mod=mod go -C <candidate>/backend test ./internal/service -run 'Test(UpstreamBillingProbe|ProvideContentModerationService|ContentModeration(CallRoutesThroughProxy|Proxy|UpdateConfigProxy))' -count=1
ok  github.com/Wei-Shaw/sub2api/internal/service  2.076s
```

Real clean reproduction against the official repository and exact identities:

```text
pre_resolve_conflicts=11
sub2api_release_resolution status=succeeded base_version=0.1.169 target_version=0.1.171
post_unmerged=0 post_unstaged=0
wire_hash=4d7679ddc311aaf1d2b6727a6c595d4e4a4f1b45
sum_hash=cf051f2073250b46589c7a633502db66c948ea0c
```

The resolver was also exercised against the same clean reproduction without `--unidiff-zero`; it failed with `reason=postimage_mismatch` and restored all 11 unmerged entries. A generation failure fixture similarly restored the byte-identical index and worktree (`test_v0171_resolution_restores_original_merge_state_when_generation_fails`).

## Self-review

- Fail-closed checks occur before mutation for target/base identities, exact conflict set, all conflict stages, all clean preimages, patch blob, and patch scope.
- Generated files are not selected as opaque semantic postimages; they are recreated and staged only after `go mod tidy` and `go generate ./cmd/server` succeed.
- The recorded semantic corrections retain Xingqiao monitoring, procurement cost fields, and managed/manual multiplier behavior while keeping official rate-sync, captcha/settings/profit, and Codex changes present in the resolved tree.
- No controller-owned document rename or project-ledger date file was modified by this task.

## Risks / follow-up

- The compact supplemental patch intentionally uses zero-context hunks and therefore requires `--unidiff-zero`; safety comes from the exact manifest stage/preimage hashes, patch blob, and patch-scope gate.
- Broader frontend and release qualification remain Task 2+ work; this task only establishes deterministic, fail-closed conflict resolution and generated-file reproducibility.
