# Account Monitor Card Data Prototype V3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Revise the existing standalone account-monitor prototype so one group shows its seven approved native fields and two rank-sorted account cards demonstrate procurement cost, upstream multiplier cost, inline global-priority editing, and five-second native-concurrency behavior.

**Architecture:** Keep the prototype isolated in `docs/prototypes/account-monitor-card-v2/` and make no production API calls. Replace the single hard-coded card with a small in-page data model and deterministic render functions so both cards, all three time windows, inline editors, filtering, and simulated concurrency share one source of truth.

**Tech Stack:** Semantic HTML, CSS, vanilla JavaScript, Lucide browser icons, browser-based responsive QA.

## Global Constraints

- Do not modify production Vue, Go, database migrations, APIs, or deployment files in this plan.
- Preserve the approved visual language; this is a data and layout revision, not a visual redesign.
- The group summary must show only `status`, `platform`, `rate_multiplier`, `rpm_limit`, `account_count`, `active_account_count`, and `rate_limited_account_count`.
- Group multiplier appears only in the group summary and never in an account card.
- Desktop shows two cards per row; mobile shows one card per row.
- Ranked accounts render in ascending `groupRank`; unranked accounts render after ranked accounts in ascending `accountId`.
- Procurement cost is CNY, uses the native account expiration time, and numerically maps 1:1 to site USD quota without currency conversion.
- A non-null procurement cost selects procurement mode; a null procurement cost selects native account-multiplier mode.
- Real requests drive 24-hour, 7-day, and 30-day metrics, scores, and ranks; probe failures remain separate from real-request failures.
- Current concurrency displays as `current / configured maximum`, updates every five seconds while visible, and must not resize the card.
- Implementation starts by keeping the existing 2026-08-04 project-progress entry in `进行中` and revising its scope to match this approved prototype.

---

### Task 1: Revise the standalone two-card prototype

**Files:**
- Modify: `docs/project/project-progress.md`
- Modify: `docs/prototypes/account-monitor-card-v2/index.html`
- Modify: `docs/prototypes/account-monitor-card-v2/design-qa.md`
- Create: `docs/prototypes/account-monitor-card-v2/prototype-v3-desktop.png`
- Create: `docs/prototypes/account-monitor-card-v2/prototype-v3-mobile-top.png`

**Interfaces:**
- Consumes: `docs/superpowers/specs/2026-08-04-account-monitor-card-data-completion-design.md` and the current V2 prototype.
- Produces: `state.group`, `state.accounts`, `state.range`, `renderGroupSummary()`, `renderCards()`, `setRange(range)`, `beginPriorityEdit(accountId)`, `savePriority(accountId)`, `cancelPriorityEdit(accountId)`, `beginCostEdit(accountId)`, `saveCost(accountId)`, `clearCost(accountId)`, `applyFilters()`, and `startConcurrencyTicker()` inside the standalone page.

- [x] **Step 1: Register the revised prototype scope in the project ledger before changing the prototype.**

Update the existing 2026-08-04 account-monitor prototype entry instead of adding a duplicate entry. Keep its state as `进行中` and make these decisions explicit:

```text
本轮先修订最小样本 HTML 原型，不改生产页面；新增一次性采购成本、卡片内全局优先级编辑、每组双列排名卡片、七项原生分组汇总和 5 秒实时并发演示。采购成本非空时使用采购模式，为空时使用原生倍率；分组倍率只出现在分组汇总。
```

Run:

```bash
rg -n "2026-08-04.*账号监控卡片" docs/project/project-progress.md
```

Expected: one current 2026-08-04 entry describes the revised prototype and remains `进行中`.

- [x] **Step 2: Add a failing static acceptance check for the new semantic surfaces.**

Before editing `index.html`, run:

```bash
test "$(rg -c 'class="group-summary"' docs/prototypes/account-monitor-card-v2/index.html)" -eq 1 && \
rg -q 'accountId: 113' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q 'accountId: 207' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q '当前并发' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q '采购成本' docs/prototypes/account-monitor-card-v2/index.html
```

Expected: FAIL because V2 has no group summary, only one card, and no procurement-cost or current-concurrency surfaces.

- [x] **Step 3: Replace the single-card sample with the exact minimum data model.**

Use this state shape and values as the page's single source of truth:

```js
const state = {
  scope: "group",
  range: "24h",
  group: {
    name: "GPT-PLUS-内测",
    status: "active",
    platform: "openai",
    rateMultiplier: 1.2,
    rpmLimit: 120,
    accountCount: 2,
    activeAccountCount: 2,
    rateLimitedAccountCount: 0
  },
  accounts: [
    {
      accountId: 113,
      name: "93_dowel.paddler@icloud.com",
      status: "正常",
      priority: 1,
      concurrency: 3,
      maxConcurrency: 10,
      procurementCostCny: 120,
      expiresAt: "2026-09-01",
      multiplier: null,
      windows: {
        "24h": { score: 91, groupRank: 1, requests: 72, failures: 1, successRate: "98.6%", ttft: "1018 ms", latency: "1962 ms", effectiveMultiplier: "0.48×" },
        "7d": { score: 89, groupRank: 1, requests: 426, failures: 8, successRate: "98.1%", ttft: "1094 ms", latency: "2280 ms", effectiveMultiplier: "0.51×" },
        "30d": { score: 86, groupRank: 1, requests: 1324, failures: 31, successRate: "97.7%", ttft: "1188 ms", latency: "2614 ms", effectiveMultiplier: "0.56×" }
      }
    },
    {
      accountId: 207,
      name: "upstream-codex-02",
      status: "正常",
      priority: 2,
      concurrency: 1,
      maxConcurrency: 8,
      procurementCostCny: null,
      expiresAt: null,
      multiplier: 0.62,
      multiplierPolicy: "upstream_managed",
      windows: {
        "24h": { score: 87, groupRank: 2, requests: 51, failures: 2, successRate: "96.1%", ttft: "1120 ms", latency: "2180 ms", effectiveMultiplier: "0.62×" },
        "7d": { score: 85, groupRank: 2, requests: 338, failures: 11, successRate: "96.7%", ttft: "1175 ms", latency: "2390 ms", effectiveMultiplier: "0.62×" },
        "30d": { score: 83, groupRank: 2, requests: 1098, failures: 39, successRate: "96.4%", ttft: "1240 ms", latency: "2790 ms", effectiveMultiplier: "0.62×" }
      }
    }
  ]
};
```

Render the seven group values in a `.group-summary` between the toolbar and `.account-grid`. Do not render subscription type, exclusivity, request count, success rate, TTFT, latency, model routing, peak pricing, or any other group field.

Show `.group-summary` only when `state.scope === "group"`. The all-site tab hides this single-group summary because there is no all-site equivalent for a group multiplier or group RPM limit.

Render cards into `.account-grid` after sorting with:

```js
function compareAccounts(left, right) {
  const leftRank = left.windows[state.range].groupRank;
  const rightRank = right.windows[state.range].groupRank;
  if (leftRank == null && rightRank == null) return left.accountId - right.accountId;
  if (leftRank == null) return 1;
  if (rightRank == null) return -1;
  return leftRank - rightRank || left.accountId - right.accountId;
}
```

- [x] **Step 4: Implement the stable responsive layout and both cost modes.**

Use a two-column account grid at desktop widths and one column below 900px:

```css
.account-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

@media (max-width: 900px) {
  .account-grid { grid-template-columns: 1fr; }
}
```

Keep score, rank, and global priority in the existing prominent score strip. Add a fifth metric named `当前并发`, with tabular numerals and a fixed minimum height, and display `current / max`.

Render account cost with one branch:

```js
function renderCost(account) {
  if (account.procurementCostCny != null) {
    const window = account.windows[state.range];
    return {
      value: `¥${account.procurementCostCny.toFixed(2)}`,
      detail: `有效至 ${account.expiresAt} · 当前窗口等效倍率 ${window.effectiveMultiplier}`
    };
  }
  return {
    value: `${account.multiplier.toFixed(2)}×`,
    detail: account.multiplierPolicy === "upstream_managed" ? "上游托管倍率" : "手动倍率"
  };
}
```

The group multiplier must not appear in either card. Preserve separate real-request and probe-failure disclosure text for both cards.

- [x] **Step 5: Implement inline global-priority and procurement-cost interactions.**

For priority:

- Pencil click calls `beginPriorityEdit(accountId)` and replaces only that score cell's value with a number input, save icon, and cancel icon.
- `savePriority(accountId)` accepts integers `>= 1`, updates the in-page account only on valid input, closes the editor, and re-renders.
- Invalid input keeps the editor open and displays `请输入大于或等于 1 的整数`.
- `cancelPriorityEdit(accountId)` restores the last saved value.

For procurement cost:

- A cost pencil or `录入采购成本` action calls `beginCostEdit(accountId)`.
- `saveCost(accountId)` accepts a finite number `>= 0`, sets `procurementCostCny`, assigns the displayed prototype effective date, and re-renders procurement mode.
- `clearCost(accountId)` asks for confirmation, then sets `procurementCostCny` to `null` and re-renders multiplier mode.
- The upstream-managed multiplier itself stays read-only.

Use Lucide `pencil`, `check`, `x`, and `trash-2` icons with accessible labels and tooltips. Do not add persistent explanatory copy outside the existing compact helper lines.

- [x] **Step 6: Make every visible prototype control functional.**

- `setRange(range)` updates both cards' score, rank, request metrics, cost detail, and disclosure values from the same window object.
- Group and all-site tabs update `state.scope`, selected state, score/rank labels, and group-summary visibility without losing edits.
- `applyFilters()` filters the two cards by the existing search input and status select.
- Each card's call disclosure opens independently using account-specific `aria-controls`.
- Refresh buttons update the visible checked-at time without changing request counts.

Implement five-second concurrency simulation with stable values:

```js
function tickConcurrency() {
  state.accounts.forEach((account, index) => {
    const delta = index === 0 ? 1 : -1;
    account.concurrency = Math.max(0, Math.min(account.maxConcurrency, account.concurrency + delta));
  });
  renderConcurrencyOnly();
}

function startConcurrencyTicker() {
  window.setInterval(() => {
    if (document.visibilityState === "visible") tickConcurrency();
  }, 5000);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") tickConcurrency();
  });
}
```

`renderConcurrencyOnly()` must update only `[data-concurrency-account-id]` text so the card DOM, open disclosures, and edit state do not reset every five seconds.

- [x] **Step 7: Run static and browser acceptance checks.**

Run the static check again:

```bash
test "$(rg -c 'class="group-summary"' docs/prototypes/account-monitor-card-v2/index.html)" -eq 1 && \
rg -q 'accountId: 113' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q 'accountId: 207' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q '当前并发' docs/prototypes/account-monitor-card-v2/index.html && \
rg -q '采购成本' docs/prototypes/account-monitor-card-v2/index.html
```

Expected: PASS.

Serve the prototype from the repository root:

```bash
python3 -m http.server 4178
```

Reuse the existing server if port 4178 is already serving this workspace; do not start a second process on the same port.

At a 1440x1000 desktop viewport, verify:

- The one group summary shows exactly seven native fields.
- Cards `#113` and `#207` share one row and appear in rank 1 then rank 2 order.
- `#113` shows `¥120.00`; `#207` shows `0.62×`.
- Both cards show current concurrency as `current / max`.
- Priority save/cancel, procurement save/clear, time-window switching, search, status filter, independent disclosure, and refresh controls all work.
- Waiting at least five seconds changes only concurrency text and causes no layout shift.
- Browser console has no errors or warnings.

At a 390x844 mobile viewport, verify:

- Cards are single-column with no horizontal overflow.
- All score, rank, priority, cost, and concurrency text remains inside its container.
- Inline priority and cost editors remain operable.

Save screenshots as `prototype-v3-desktop.png` and `prototype-v3-mobile-top.png`.

- [x] **Step 8: Replace the QA report with V3 evidence and commit the prototype revision.**

Update `design-qa.md` so it names the V3 screenshots, both account modes, seven-field group summary, two-column/one-column behavior, inline editing checks, five-second concurrency check, and console/overflow results. Remove statements that V3 contradicts, including the old claim that group multiplier and cost UI are absent.

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only the project ledger, V3 prototype files, V3 screenshots, and QA report are part of this task's commit. Do not stage the historical untracked V2 plan or unrelated release evidence.

Commit:

```bash
git add docs/project/project-progress.md \
  docs/prototypes/account-monitor-card-v2/index.html \
  docs/prototypes/account-monitor-card-v2/design-qa.md \
  docs/prototypes/account-monitor-card-v2/prototype-v3-desktop.png \
  docs/prototypes/account-monitor-card-v2/prototype-v3-mobile-top.png
git commit -m "docs: revise account monitor data prototype"
```
