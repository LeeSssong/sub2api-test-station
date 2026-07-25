# 星桥首页截图整改 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按四张标注截图上提首屏标题、强化请求链路与观测指标的视觉关联、修正价格倍率，并让接入示例复用星桥现有 `baseUrl`。

**Architecture:** 保持现有 React 组件边界和请求旅程状态机不变。内容契约通过现有 `SiteConfig.apiOrigin` 从 `App` 传入 `IntegrationSection`，视觉调整集中在现有 CSS 类和少量可测试的数据属性中。

**Tech Stack:** React 19、TypeScript 7、Vite 8、Vitest、Testing Library、Motion、CSS

## Global Constraints

- 仅修改 `homepage` 及本计划和设计文档。
- OpenAI 与 Anthropic 共用 `config.apiOrigin`，不硬编码生产域名。
- 接口路径继续分别显示 `/v1/chat/completions` 与 `/v1/messages`。
- 价格文案必须精确为 `官方价格的 0.1—0.3 倍`。
- 不改变请求旅程的播放周期、阶段切换或模拟指标数值。
- 桌面端强化对角线构图；`980px` 以下保持自然单列，不强制位移。
- 减少动态效果模式必须保留完整静态链路和指标。

---

### Task 1: 修正价格与接入示例契约

**Files:**
- Create: `homepage/src/sections/IntegrationSection.test.tsx`
- Modify: `homepage/src/sections/IntegrationSection.tsx`
- Modify: `homepage/src/sections/ValueSections.test.tsx`
- Modify: `homepage/src/sections/ValueSections.tsx`
- Modify: `homepage/src/App.tsx`
- Modify: `homepage/src/App.test.tsx`

**Interfaces:**
- Consumes: `SiteConfig` from `homepage/src/domain/siteConfig.ts`
- Produces: `IntegrationSection({ config }: { config: SiteConfig }): JSX.Element`

- [ ] **Step 1: 写入失败测试**

在 `ValueSections.test.tsx` 中将倍率断言改为：

```tsx
expect(screen.getAllByText('官方价格的 0.1—0.3 倍')).toHaveLength(2)
expect(screen.queryByText(/0。3|——/)).not.toBeInTheDocument()
```

创建 `IntegrationSection.test.tsx`：

```tsx
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { DEFAULT_SITE_CONFIG } from '../domain/siteConfig'
import { IntegrationSection } from './IntegrationSection'

describe('IntegrationSection', () => {
  it('reuses the configured Xingqiao base URL without brace icons', () => {
    render(<IntegrationSection config={{
      ...DEFAULT_SITE_CONFIG,
      apiOrigin: 'https://api.xingqiao.test',
    }} />)

    expect(screen.getByText('OPENAI_BASE_URL=https://api.xingqiao.test')).toBeVisible()
    expect(screen.getByText('ANTHROPIC_BASE_URL=https://api.xingqiao.test')).toBeVisible()
    expect(screen.queryByText('https://你的星桥域名')).not.toBeInTheDocument()

    const labels = screen.getAllByTestId('integration-label')
    for (const label of labels) {
      expect(label.querySelectorAll('svg')).toHaveLength(1)
    }
  })
})

afterEach(cleanup)
```

在 `App.test.tsx` 的完整首页契约测试中加入：

```tsx
expect(screen.getByText('OPENAI_BASE_URL=https://api.example.com')).toBeVisible()
expect(screen.getByText('ANTHROPIC_BASE_URL=https://api.example.com')).toBeVisible()
```

- [ ] **Step 2: 运行测试并确认失败**

Run:

```bash
cd homepage
npm test -- --run src/sections/ValueSections.test.tsx src/sections/IntegrationSection.test.tsx src/App.test.tsx
```

Expected: FAIL；旧倍率文案仍存在，`IntegrationSection` 尚不接收 `config`，并仍显示占位域名。

- [ ] **Step 3: 实现最小内容改动**

将 `IntegrationSection.tsx` 调整为：

```tsx
import { ArrowRight, Terminal } from 'lucide-react'
import { Reveal } from '../components/Reveal'
import type { SiteConfig } from '../domain/siteConfig'

interface IntegrationSectionProps {
  config: SiteConfig
}

const examples = [
  { label: 'OpenAI', path: '/v1/chat/completions', sdk: 'OPENAI_BASE_URL' },
  { label: 'Anthropic', path: '/v1/messages', sdk: 'ANTHROPIC_BASE_URL' },
]

export function IntegrationSection({ config }: IntegrationSectionProps) {
  return (
    <section className="integration-band grid-surface" id="docs" aria-labelledby="integration-title" tabIndex={-1}>
      <div className="section-inner integration-inner">
        <Reveal as="header" className="section-intro">
          <p className="eyebrow"><span />兼容接入</p>
          <h2 id="integration-title">只改基础 URL，保留熟悉的 SDK</h2>
          <p>OpenAI 与 Anthropic 使用各自原生接口路径，共用星桥网关和密钥管理。</p>
        </Reveal>
        <Reveal className="integration-grid">
          {examples.map((item) => (
            <article key={item.label} className="integration-item">
              <div className="integration-label" data-testid="integration-label">
                <span>{item.label}</span><ArrowRight aria-hidden="true" />
              </div>
              <div className="terminal-block">
                <div className="terminal-bar"><Terminal aria-hidden="true" /><span>{item.path}</span><b className="terminal-cursor" aria-hidden="true">|</b></div>
                <code>{item.sdk}={config.apiOrigin}</code>
              </div>
            </article>
          ))}
        </Reveal>
      </div>
    </section>
  )
}
```

在 `App.tsx` 中传入配置：

```tsx
<IntegrationSection config={config} />
```

在 `ValueSections.tsx` 中同步可见文本与 `sr-only` 文本：

```tsx
<div>
  <dt>星桥价格</dt>
  <dd>官方价格的 0.1—0.3 倍</dd>
  <span className="sr-only">官方价格的 0.1—0.3 倍</span>
</div>
```

- [ ] **Step 4: 运行测试并确认通过**

Run:

```bash
cd homepage
npm test -- --run src/sections/ValueSections.test.tsx src/sections/IntegrationSection.test.tsx src/App.test.tsx
```

Expected: 相关测试全部 PASS。

- [ ] **Step 5: 提交内容契约改动**

```bash
git add homepage/src/App.tsx homepage/src/App.test.tsx homepage/src/sections/IntegrationSection.tsx homepage/src/sections/IntegrationSection.test.tsx homepage/src/sections/ValueSections.tsx homepage/src/sections/ValueSections.test.tsx
git commit -m "fix: reuse homepage base URL and pricing copy"
```

---

### Task 2: 上提首屏标题形成桌面对角线

**Files:**
- Modify: `homepage/src/sections/HeroSection.test.tsx`
- Modify: `homepage/src/sections/HeroSection.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- Consumes: 现有 `HeroSection({ session })` 与 `data-layout="diagonal"`
- Produces: `data-composition="raised-diagonal"` 的桌面布局契约

- [ ] **Step 1: 写入失败测试**

在 `HeroSection.test.tsx` 的首屏契约测试中加入：

```tsx
const heroGrid = screen.getByLabelText('星桥首页首屏').querySelector('.hero-grid')
expect(heroGrid).toHaveAttribute('data-layout', 'diagonal')
expect(heroGrid).toHaveAttribute('data-composition', 'raised-diagonal')
```

- [ ] **Step 2: 运行测试并确认失败**

Run:

```bash
cd homepage
npm test -- --run src/sections/HeroSection.test.tsx
```

Expected: FAIL；`.hero-grid` 尚无 `data-composition`。

- [ ] **Step 3: 添加布局契约与桌面样式**

在 `HeroSection.tsx` 中使用：

```tsx
<div className="hero-grid" data-layout="diagonal" data-composition="raised-diagonal">
```

在 `styles.css` 桌面规则中让标题块通过自身占位上提，不使用会与 Motion 动画冲突的 `transform`：

```css
.hero-title {
  display: grid;
  justify-items: start;
  row-gap: .12em;
  padding-bottom: clamp(4.5rem, 8vh, 6.5rem);
}
```

在 `@media (max-width: 980px)` 中取消桌面对角线位移：

```css
.hero-title {
  margin-bottom: 0;
  padding-bottom: 0;
}
```

- [ ] **Step 4: 运行组件测试**

Run:

```bash
cd homepage
npm test -- --run src/sections/HeroSection.test.tsx
```

Expected: PASS。

- [ ] **Step 5: 提交首屏布局改动**

```bash
git add homepage/src/sections/HeroSection.tsx homepage/src/sections/HeroSection.test.tsx homepage/src/styles.css
git commit -m "style: raise homepage hero composition"
```

---

### Task 3: 强化链路与延迟、Token 的视觉关联

**Files:**
- Modify: `homepage/src/sections/RequestJourney.test.tsx`
- Modify: `homepage/src/sections/RequestJourney.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- Consumes: `Phase = 'send' | 'route' | 'observe'` 与现有 `data-journey-phase`
- Produces: `data-telemetry-source="route"`、`data-telemetry-target="latency-token"` 的视觉关联契约

- [ ] **Step 1: 写入失败测试**

在 `RequestJourney.test.tsx` 的减少动态效果测试中加入：

```tsx
expect(screen.getByLabelText('请求观测指标')).toHaveAttribute(
  'data-telemetry-target',
  'latency-token',
)
const routeTrack = screen.getByLabelText('应用经星桥连接模型通道')
  .querySelector('.map-track--route')
expect(routeTrack).toHaveAttribute('data-telemetry-source', 'route')
```

- [ ] **Step 2: 运行测试并确认失败**

Run:

```bash
cd homepage
npm test -- --run src/sections/RequestJourney.test.tsx
```

Expected: FAIL；指标和路由轨道尚无对应数据属性。

- [ ] **Step 3: 添加视觉关联契约**

在 `RequestJourney.tsx` 中修改指标容器：

```tsx
<div
  className="journey-metrics"
  aria-label="请求观测指标"
  data-telemetry-target="latency-token"
>
```

修改模型侧轨道：

```tsx
<div
  className="map-track map-track--route"
  data-flow-direction="forward"
  data-telemetry-source="route"
  style={trackStyle(routedProgress)}
  aria-hidden="true"
><i /></div>
```

- [ ] **Step 4: 提高轨道、光标和同步指标高亮**

在 `styles.css` 中加入或覆盖以下规则：

```css
.journey-metrics {
  position: relative;
  padding-top: .9rem;
  border-top: 1px solid rgba(86,129,255,.24);
  transition: border-color .35s ease, filter .35s ease;
}
.map-track {
  height: 2px;
  background: rgba(86,129,255,.5);
  box-shadow: 0 0 12px rgba(86,129,255,.16);
}
.map-track::after {
  background: linear-gradient(90deg, var(--blue), var(--cyan));
  box-shadow: 0 0 16px rgba(76,201,232,.62);
}
.map-track i {
  top: -5px;
  width: 12px;
  height: 12px;
  border: 2px solid rgba(237,241,247,.92);
  box-shadow: 0 0 8px rgba(237,241,247,.85), 0 0 22px var(--blue);
}
.map-track--route i {
  box-shadow: 0 0 8px rgba(237,241,247,.9), 0 0 24px var(--cyan);
}
.request-journey[data-journey-phase="observe"] .journey-metrics {
  border-color: rgba(76,201,232,.82);
  filter: drop-shadow(0 0 14px rgba(76,201,232,.2));
}
.request-journey[data-journey-phase="observe"] .journey-metrics strong {
  text-shadow: 0 0 18px rgba(76,201,232,.42);
}
```

移动端保留 `2px` 轨道，但确保 `.request-map` 的 `24px` 最小轨道宽度和现有网格不变。

- [ ] **Step 5: 运行组件测试**

Run:

```bash
cd homepage
npm test -- --run src/sections/RequestJourney.test.tsx
```

Expected: PASS。

- [ ] **Step 6: 提交链路视觉改动**

```bash
git add homepage/src/sections/RequestJourney.tsx homepage/src/sections/RequestJourney.test.tsx homepage/src/styles.css
git commit -m "style: strengthen homepage telemetry flow"
```

---

### Task 4: 全量验证与响应式视觉检查

**Files:**
- Verify: `homepage/src/**`
- Verify: `homepage/src/styles.css`

**Interfaces:**
- Consumes: Tasks 1–3 的组件和样式
- Produces: 通过的单元测试、类型检查、构建和桌面/移动浏览器证据

- [ ] **Step 1: 运行自动化验证**

Run:

```bash
cd homepage
npm run test:run
npm run typecheck
npm run build
```

Expected: 三条命令均退出码 `0`。

- [ ] **Step 2: 启动本地预览**

Run:

```bash
cd homepage
npm run dev -- --host 127.0.0.1
```

Expected: Vite 输出可访问的本地 URL。

- [ ] **Step 3: 验证桌面视口**

使用 `1440x900` 浏览器视口检查：

- 首屏标题明显高于右侧介绍和按钮，形成左上到右下的对角线
- 首屏标题、导航、按钮、向下探索入口无重叠
- 请求旅程轨道与光标比原状态更亮
- `observe` 阶段中轨道、光标与“延迟 / Token”出现同步高亮
- 价格文案显示为 `官方价格的 0.1—0.3 倍`
- 两行基础地址显示配置中的星桥 `apiOrigin`
- OpenAI、Anthropic 标签前没有花括号图标

- [ ] **Step 4: 验证移动视口与无障碍动态偏好**

使用 `390x844` 浏览器视口以及 `prefers-reduced-motion: reduce` 检查：

- 首屏回到自然单列，无桌面上提造成的空洞或遮挡
- 页面 `scrollWidth === innerWidth`
- 接入示例长 URL 可换行且不溢出
- 静态请求旅程显示完整轨道、指标和三个步骤
- 页面控制台无错误

- [ ] **Step 5: 检查工作区差异**

Run:

```bash
git status --short
git diff --check
git diff HEAD~3 -- homepage
```

Expected: 仅出现本计划范围内文件；`git diff --check` 无输出。
