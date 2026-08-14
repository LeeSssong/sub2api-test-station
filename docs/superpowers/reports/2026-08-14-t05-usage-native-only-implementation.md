# T05 用量页仅使用原生 Sub 数据实施报告

## Baseline

- Baseline SHA: 4c5f0d1587004cfb4d7386d0c947f157678d8803
- Branch: codex/t05-usage-native-only

## RED

- Command: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts`
- Result: RED as expected. `UsageView.spec.ts` failed 2 tests because native-only assertions observed `/api/v1/xingqiao/externalization/pages/accounting?timezone=Asia%2FShanghai` during initial load and refresh; 16 tests passed.

## GREEN

- Not run yet; Task 2 must replace this line with focused and neighboring GREEN evidence before commit.

## Scope Review

- Not reviewed yet; Task 2 must replace this line with the final scope review before commit.
