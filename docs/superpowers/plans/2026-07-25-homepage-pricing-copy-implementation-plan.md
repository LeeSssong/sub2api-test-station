# Homepage Pricing Copy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change the homepage pricing statement to the exact approved text `官方价格的0.1——0。3倍`.

**Architecture:** Keep the existing React markup and accessibility text synchronized. Change only the source component and its focused test; preserve all unrelated homepage work already present in the worktree.

**Tech Stack:** React, TypeScript, Vite, Vitest, Testing Library.

## Global Constraints

- Modify only `/Users/gongtengxinwen/Documents/sub2api搭建/homepage/src/sections/ValueSections.tsx` and its focused test.
- Preserve the full-width Chinese stop in `0。3`.
- Do not touch the existing unrelated changes in `homepage/public/docs/index.html` or `homepage/src/docs/BeginnerGuide.test.ts`.
- Do not modify any local `sub` runtime or deployment data.

---

### Task 1: Update the approved pricing copy

**Files:**
- Modify: `homepage/src/sections/ValueSections.tsx:53`
- Test: `homepage/src/sections/ValueSections.test.tsx:10`

**Interfaces:**
- Consumes: Existing `ValueSections` rendered text and screen-reader text.
- Produces: Both visible and accessible text equal to `官方价格的0.1——0。3倍`.

- [ ] **Step 1: Update the focused expectation**

Change the test expectation to:

```ts
expect(screen.getByText('星桥价格 官方价格的0.1——0。3倍')).toBeInTheDocument()
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd homepage
pnpm exec vitest run src/sections/ValueSections.test.tsx
```

Expected: FAIL because the component still renders the old `折` wording.

- [ ] **Step 3: Change both rendered strings**

Update the visible `<dd>` and the adjacent `sr-only` text in
`homepage/src/sections/ValueSections.tsx` to the exact approved copy. Do not
change surrounding layout, semantics, or styling.

- [ ] **Step 4: Run focused verification**

Run:

```bash
cd homepage
pnpm exec vitest run src/sections/ValueSections.test.tsx
pnpm run typecheck
```

Expected: PASS with no TypeScript errors.

- [ ] **Step 5: Commit only this task**

```bash
git add homepage/src/sections/ValueSections.tsx homepage/src/sections/ValueSections.test.tsx
git commit -m "fix: correct homepage pricing multiplier copy"
```

