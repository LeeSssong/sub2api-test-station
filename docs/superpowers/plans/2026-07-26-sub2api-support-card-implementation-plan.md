# Sub2API 客服联系卡 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将管理端“联系客服”Markdown 页面改为紧凑二维码联系卡，并提供二维码放大和 QQ 群号一键复制。

**Architecture:** `support.md` 只负责声明受信任的支持卡标记和内容；`CustomPageView.vue` 继续负责清洗 HTML，并通过 Markdown 容器事件委托处理显式的复制和图片预览动作。通用交互解析放入独立工具，便于在不加载整页布局的情况下测试。

**Tech Stack:** Vue 3、TypeScript、Marked、DOMPurify、Vitest、Vue Test Utils、Tailwind CSS

## Global Constraints

- 二维码默认视觉尺寸约 220px，其他 Markdown 图片不受影响。
- 点击或键盘激活二维码可查看大图。
- QQ 群号固定为 `1080152144`，复制成功和失败都必须有反馈。
- 只响应 DOMPurify 清洗后显式存在的 `data-copy-text` 和 `data-image-preview` 元素。
- 宽屏两列，窄屏单列；不得出现横向滚动。
- 保持现有深浅色视觉和 `useClipboard()` 通知机制。

---

### Task 1: 锁定发布配置契约

**Files:**
- Modify: `config/sub2api/support.md`
- Modify: `tests/operations/configure_sub2api_support_test.sh`

**Interfaces:**
- Produces: `.support-contact-card`、`data-copy-text="1080152144"`、`data-image-preview` 标记

- [ ] **Step 1: 写入失败断言**

在运维测试中加入：

```bash
rg -Fq 'class="support-contact-card"' "$fixture/data/pages/support.md"
rg -Fq 'data-copy-text="1080152144"' "$fixture/data/pages/support.md"
rg -Fq 'data-image-preview' "$fixture/data/pages/support.md"
```

- [ ] **Step 2: 运行测试并确认失败**

Run:

```bash
bash tests/operations/configure_sub2api_support_test.sh
```

Expected: FAIL，旧 `support.md` 不包含支持卡和交互标记。

- [ ] **Step 3: 写入支持卡 Markdown**

将客服页改为语义化支持卡，保留相对图片路径：

```markdown
# 联系客服

<div class="support-contact-card">
  <button type="button" class="support-qr-trigger" data-image-preview aria-label="放大查看QQ群二维码">
    <img src="qq-group-1080152144.png" alt="QQ群 1080152144 二维码">
  </button>
  <div class="support-contact-details">
    <p>使用 QQ 扫描二维码加入群聊，或复制群号手动搜索。</p>
    <span class="support-group-label">QQ群号</span>
    <strong class="support-group-number">1080152144</strong>
    <button type="button" class="support-copy-button" data-copy-text="1080152144">
      <span data-copy-label>复制群号</span>
    </button>
  </div>
</div>
```

- [ ] **Step 4: 更新图片 URL 重写逻辑的测试范围**

测试脚本继续确认发布后的 Markdown 和二维码文件同时存在、哈希不变。

- [ ] **Step 5: 提交配置契约**

```bash
git add config/sub2api/support.md tests/operations/configure_sub2api_support_test.sh
git commit -m "feat: define compact support contact card"
```

---

### Task 2: 添加安全的 Markdown 交互解析

**Files:**
- Create: `upstream/sub2api/frontend/src/utils/markdownSupportActions.ts`
- Create: `upstream/sub2api/frontend/src/utils/__tests__/markdownSupportActions.spec.ts`

**Interfaces:**
- Produces: `resolveMarkdownSupportAction(target: EventTarget | null): MarkdownSupportAction | null`
- Produces: `setMarkdownCopyState(button, state, labels): void`

- [ ] **Step 1: 写入失败测试**

覆盖以下行为：

```ts
expect(resolveMarkdownSupportAction(copyLabel)).toMatchObject({
  kind: 'copy',
  text: '1080152144',
})
expect(resolveMarkdownSupportAction(qrImage)).toMatchObject({
  kind: 'preview',
  src: '/api/pages/support/images/qq.png',
})
expect(resolveMarkdownSupportAction(unmarkedButton)).toBeNull()
```

并断言复制状态只修改当前按钮的 `data-copy-state` 和 `[data-copy-label]` 文本。

- [ ] **Step 2: 运行测试并确认失败**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/utils/__tests__/markdownSupportActions.spec.ts
```

Expected: FAIL，工具文件尚不存在。

- [ ] **Step 3: 实现动作解析**

使用 `Element.closest()` 查找显式标记：

```ts
export type MarkdownSupportAction =
  | { kind: 'copy'; text: string; button: HTMLButtonElement }
  | { kind: 'preview'; src: string; alt: string; trigger: HTMLButtonElement }
```

空复制值、缺失图片和非元素目标返回 `null`。

- [ ] **Step 4: 实现复制按钮状态**

状态仅允许 `idle | copied | failed`，同步更新：

```ts
button.dataset.copyState = state
label.textContent = labels[state]
```

- [ ] **Step 5: 运行单元测试并提交**

```bash
pnpm vitest run src/utils/__tests__/markdownSupportActions.spec.ts
git add upstream/sub2api/frontend/src/utils/markdownSupportActions.ts upstream/sub2api/frontend/src/utils/__tests__/markdownSupportActions.spec.ts
git commit -m "test: add markdown support action contract"
```

---

### Task 3: 接入复制与二维码预览

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/CustomPageView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/misc.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/misc.ts`

**Interfaces:**
- Consumes: `resolveMarkdownSupportAction`、`setMarkdownCopyState`、`useClipboard`
- Produces: Markdown 支持卡复制反馈和图片预览层

- [ ] **Step 1: 在 Markdown 容器接入委托点击**

```vue
@click="onMarkdownClick"
```

复制动作调用：

```ts
const success = await copyToClipboard(action.text)
setMarkdownCopyState(action.button, success ? 'copied' : 'failed', copyLabels.value)
```

两秒后恢复 `idle`；组件卸载时清除计时器。

- [ ] **Step 2: 添加图片预览状态**

预览动作保存 `src`、`alt` 和触发按钮。使用现有 `BaseDialog` 展示完整图片，关闭后通过 `nextTick()` 把焦点返回触发按钮。

- [ ] **Step 3: 添加中英文文案**

在 `customPage` 下加入：

```ts
copyGroupNumber: '复制群号'
copiedGroupNumber: '已复制'
copyGroupNumberFailed: '复制失败'
qrPreviewTitle: 'QQ群二维码'
```

英文使用 `Copy group number`、`Copied`、`Copy failed`、`QQ group QR code`。

- [ ] **Step 4: 运行现有前端测试**

```bash
pnpm vitest run src/utils/__tests__/markdownSupportActions.spec.ts
```

Expected: PASS。

- [ ] **Step 5: 提交交互改动**

```bash
git add upstream/sub2api/frontend/src/views/user/CustomPageView.vue upstream/sub2api/frontend/src/i18n/locales/zh/misc.ts upstream/sub2api/frontend/src/i18n/locales/en/misc.ts
git commit -m "feat: add support page copy and QR preview"
```

---

### Task 4: 添加限定样式与完整验证

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/CustomPageView.vue`

**Interfaces:**
- Consumes: `.support-contact-card` 及其子元素
- Produces: 桌面两列、移动单列、220px 二维码布局

- [ ] **Step 1: 添加支持卡限定样式**

所有选择器以 `.markdown-page-content .support-contact-card` 开始。关键约束：

```css
grid-template-columns: minmax(180px, 220px) minmax(0, 1fr);
max-width: 720px;
```

二维码使用 `aspect-ratio: 1`、`object-fit: contain`，复制按钮沿用主色按钮视觉。

- [ ] **Step 2: 添加移动断点**

在 `640px` 以下改为单列，二维码保持 `min(220px, 100%)`，群号与按钮不溢出。

- [ ] **Step 3: 运行自动化验证**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/utils/__tests__/markdownSupportActions.spec.ts
pnpm type-check
pnpm build
```

并运行不依赖 Docker 的运维测试阶段；如完整脚本进入 PostgreSQL 集成阶段，记录前置轻量断言结果后再使用可用 Docker 环境验证。

- [ ] **Step 4: 运行浏览器视觉验证**

在桌面与移动视口检查：

- 二维码默认宽度不超过 220px。
- 群号和复制按钮清晰、可点击。
- 复制成功/失败反馈可见。
- 点击二维码打开预览，Escape 可关闭。
- 页面无横向溢出，控制台无错误。

- [ ] **Step 5: 提交样式与验证结果**

```bash
git add upstream/sub2api/frontend/src/views/user/CustomPageView.vue
git commit -m "style: compact support contact card"
```
