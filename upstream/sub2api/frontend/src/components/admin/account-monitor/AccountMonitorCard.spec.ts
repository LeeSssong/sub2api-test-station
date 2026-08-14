import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => ({
        'admin.accountMonitor.status.success': '正常',
        'admin.accountMonitor.status.failed': '不可用',
        'admin.accountMonitor.status.pending': '待确认',
        'admin.accountMonitor.status.paused': '暂停',
        'admin.accountMonitor.status.unavailable': '不可用',
      }[key] ?? key),
    }),
  }
})

type Deferred = {
  promise: Promise<void>
  resolve: () => void
  reject: (reason?: unknown) => void
}

function deferred(): Deferred {
  let resolve!: () => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

const account = {
  account_id: 113,
  name: '93_dowel.paddler@icloud.com',
  platform: 'openai',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  management_state: 'enabled',
  service_state: 'available',
  group_eligibility: 'eligible',
  monitor_bucket: 'available',
  priority: 1,
  group_ids: [3],
  group_names: ['GPT-PLUS-内测'],
  model_id: 'gpt-4o-mini',
  latest_status: 'success',
  sample_count: 72,
  success_sample_count: 71,
  ttft_sample_count: 71,
  latency_sample_count: 71,
  success_rate: 0.986,
  ttft_p50_ms: 1018,
  ttft_p95_ms: 1400,
  latency_p95_ms: 1962,
  multiplier: { value: 0.58, source: 'declared', status: 'ok', sample_count: 72 },
  request_count: 72,
  error_count: 1,
  range: '24h',
  base_cost: 18,
  effective_multiplier: 0.48,
  cost_mode: 'multiplier',
  cost_score: 15,
  timeline: Array.from({ length: 24 }, (_, index) => ({
    status: index === 7 ? 'failed' : 'success',
    checked_at: `2026-08-04T04:${String(index).padStart(2, '0')}:00Z`,
    latency_ms: 900 + index * 10,
  })),
  stale: false,
  quality_score: 91,
  score_breakdown: { cost: 12, success: 43.5, ttft: 18, latency: 17.5 },
  evidence_source: 'monitor_probe',
  homepage_url: 'https://upstream.example.com/v1',
  group_rank: 1,
  eligible: true,
}

function mountCard(overrides: Record<string, unknown> = {}) {
  return mount(AccountMonitorCard, {
    attachTo: document.body,
    props: {
      account,
      concurrency: { account_id: 113, current: 3, limit: 10, delayed: false },
      ...overrides,
    },
    global: { stubs: { Icon: true } },
  })
}

afterEach(() => {
  document.body.replaceChildren()
})

describe('AccountMonitorCard', () => {
	const recommendation = {
		status: 'recommended',
		target: 'gpt_pro',
		target_name: 'GPT-Pro',
		action: 'migrate',
		reason_codes: ['codex_auth_default_pro'],
		sample_count: 72,
		observed_at: '2026-08-10T00:00:00Z',
		source: 'monitor_probe',
	}

	it('exposes native account entry points without changing the compact card projection', async () => {
		const accountInfo = vi.fn()
		const accountEdit = vi.fn()
		const accountDelete = vi.fn()
		const accountMore = vi.fn()
		const wrapper = mountCard({ onAccountInfo: accountInfo, onAccountEdit: accountEdit, onAccountDelete: accountDelete, onAccountMore: accountMore })

		expect(wrapper.get('[data-test="account-info"]').text()).toContain('账号信息')
		expect(wrapper.get('[data-test="account-edit"]').text()).toContain('编辑')
		expect(wrapper.get('[data-test="account-delete"]').text()).toContain('删除')
		expect(wrapper.get('[data-test="account-more"]').text()).toContain('更多')

		await wrapper.get('[data-test="account-info"]').trigger('click')
		await wrapper.get('[data-test="account-edit"]').trigger('click')
		await wrapper.get('[data-test="account-delete"]').trigger('click')
		await wrapper.get('[data-test="account-more"]').trigger('click')

		expect(accountInfo).toHaveBeenCalledWith(account)
		expect(accountEdit).toHaveBeenCalledWith(account)
		expect(accountDelete).toHaveBeenCalledWith(account)
		expect(accountMore).toHaveBeenCalledWith(account, expect.any(MouseEvent))
	})

	it('keeps platform, current group, schedulable state, and recommendation in one compact metadata row', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: recommendation } })
		const metadata = wrapper.get('[data-test="account-metadata"]')
		expect(metadata.text()).toContain('平台 openai')
		expect(metadata.text()).toContain('当前分组 GPT-测试分组')
		expect(metadata.text()).toContain('调度状态 可调度')
		expect(metadata.text()).toContain('推荐：GPT-Pro')
	})

	it.each([
		['observe', 'keep', '继续观察'],
		['blocked', 'hold', '暂缓迁入'],
		['not_recommended', 'none', '暂不建议入组'],
	])('shows the test-group %s state as %s', (status, action, label) => {
		const wrapper = mountCard({
			account: {
				...account,
				group_names: ['GPT-测试分组'],
				group_recommendation: { ...recommendation, status, action, target: status === 'blocked' ? 'gpt_pro' : '', target_name: status === 'blocked' ? 'GPT-Pro' : '' },
			},
		})
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain(label)
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
	})

	it('shows an accessible warning tooltip only for an explicit formal-group migration', async () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-Plus'], group_recommendation: recommendation } })
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('推荐：GPT-Pro')
		const warning = wrapper.get<HTMLElement>('[data-test="recommendation-warning"]')
		expect(warning.attributes('title')).toContain('推荐迁移至 GPT-Pro')
		expect(warning.attributes('aria-label')).toContain('推荐迁移至 GPT-Pro')

		warning.element.focus()
		await nextTick()
		const content = document.body.querySelector('[data-test="group-recommendation-tooltip"]')
		expect(content?.textContent).toContain('推荐迁移至 GPT-Pro')
		expect(content?.textContent).toContain('Codex Auth 默认进入 Pro')
		expect(content?.textContent).toContain('固定 7d 主动探测 72 次')
		expect(content?.textContent).toContain('2026')
		expect(content?.closest('[role="tooltip"]')?.getAttribute('style')).not.toContain('display: none')
	})

	it('normalizes test-group case and ASCII spaces before deciding whether to show a formal warning', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['  gPt - 测试 分组  '], group_recommendation: recommendation } })
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('推荐：GPT-Pro')
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
	})

	it('keeps formal matching or legacy rows free of recommendation text and warning icons', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-Pro'] } })
		expect(wrapper.find('[data-test="group-recommendation"]').exists()).toBe(false)
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
		expect(wrapper.get('[data-test="account-metadata"]').text()).not.toContain('继续观察')
	})

  it('shows only the projected equivalent site multiplier in the cost evidence area', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        equivalent_site_multiplier: 0.05,
        cost_guard: {
          upstream_multiplier: 0.2,
          equivalent_site_multiplier: 0.3,
          group_multiplier: 0.1,
          model: 'legacy-model',
          sample_count: 6,
          required_sample_count: 6,
          status: 'loss_confirmed',
        },
      },
    })

    const evidence = wrapper.get('[data-test="equivalent-cost-multiplier"]')
    expect(evidence.text()).toContain('成本折合本站倍率')
    expect(evidence.text()).toContain('0.05×')
    expect(evidence.text()).not.toContain('上游原生倍率')
    expect(evidence.text()).not.toContain('当前分组倍率')
    expect(evidence.text()).not.toContain('有效样本')
    expect(evidence.text()).not.toContain('成本状态')
    expect(wrapper.find('[data-test="cost-inversion-alert"]').exists()).toBe(false)
  })

  it('shows unavailable when the equivalent site multiplier cannot be priced', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        equivalent_site_multiplier: null,
      },
    })

    expect(wrapper.get('[data-test="equivalent-cost-multiplier"]').text()).toContain('--')
  })

  it('emits editCost and removes inline cost editors', async () => {
    const editCost = vi.fn()
    const wrapper = mountCard({ onEditCost: editCost })

    await wrapper.get('[data-test="edit-cost"]').trigger('click')
    expect(editCost).toHaveBeenCalledWith(account)
    expect(wrapper.find('[data-test="cost-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="multiplier-input"]').exists()).toBe(false)
  })

  it('shows a valid API Key balance and marks a failed refresh as delayed', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        balance: { value_usd: 12.5, status: 'failed', source: 'newapi' },
      },
    })

    expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('$12.50')
    expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('数据延迟')
  })

  it('does not render a balance placeholder for non-API-Key accounts', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'oauth', balance: undefined } })
    expect(wrapper.find('[data-test="balance-metric"]').exists()).toBe(false)
  })

  it('renders the V3 service, rank, cost, and concurrency information without operations labels', () => {
    const wrapper = mountCard()

    expect(wrapper.get('[data-test="monitor-card"]').text()).toContain('93_dowel.paddler@icloud.com')
    expect(wrapper.text()).toContain('#113')
    expect(wrapper.get('[data-test="status-badge"]').text()).toContain('正常')
    expect(wrapper.get('[data-test="score-metric"]').text()).toContain('91')
    expect(wrapper.get('[data-test="rank-metric"]').text()).toContain('第 1')
    expect(wrapper.get('[data-test="priority-control"]').text()).toContain('1')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('98.6%')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('72 次探测样本，1 次失败')
    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('1018 ms')
    expect(wrapper.get('[data-test="latency-metric"]').text()).toContain('1962 ms')
    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('成本待确认')
    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('3 / 10')

    for (const label of ['分组倍率', '营收', '利润', '对账', '账务', '上游真实扣费', '用户实际计费']) {
      expect(wrapper.text()).not.toContain(label)
    }
  })

  it('uses probe evidence wording, links the account homepage, and exposes score composition', () => {
    const wrapper = mountCard()
    expect(wrapper.get('[data-test="score-metric"]').text()).toContain('基于 72 次主动探测')
    expect(wrapper.get('[data-test="account-homepage-link"]').attributes()).toMatchObject({ href: 'https://upstream.example.com/v1', target: '_blank', rel: 'noopener noreferrer' })
    expect(wrapper.get('[data-test="score-metric"]').attributes('title')).toContain('成本优势 12.0')
    expect(wrapper.get('[data-test="score-metric"]').attributes('title')).toContain('探测成功率 43.5')
  })

  it('shows the score breakdown in an application tooltip on click', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-test="score-tooltip-trigger"]').trigger('click')
    await nextTick()
    const tooltip = document.body.querySelector('[data-test="score-breakdown-tooltip"]')
    expect(tooltip?.textContent).toContain('成本优势 12.0')
    expect(tooltip?.textContent).toContain('探测成功率 43.5')
    expect(tooltip?.textContent).toContain('总耗时 17.5')
  })

  it('opens score and cost details by click so later cards remain usable after scrolling', async () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })

    await wrapper.get('[data-test="score-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="score-breakdown-tooltip"]')?.textContent).toContain('成本优势 12.0')

    await wrapper.get('[data-test="cost-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="cost-source-tooltip"]')?.textContent).toContain('手工维护')
  })

  it('marks manually maintained cost with a warning and explains its source', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })
    expect(wrapper.get('[data-test="cost-tooltip-trigger"] button').attributes('title')).toContain('手工维护')
  })

  it('shows the cost source in an application tooltip on click', async () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })
    await wrapper.get('[data-test="cost-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="cost-source-tooltip"]')?.textContent).toContain('手工维护')
  })

  it('exposes the native billing cost source tooltip', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'declared', status: 'ok', sample_count: 72 } } })
    expect(wrapper.get('[data-test="cost-tooltip-trigger"] button').attributes('title')).toContain('上游原生')
  })

  it('restores the rejected V3 green service card shell, five colored metrics, probe bars, and service-only footer', async () => {
    const refresh = vi.fn()
    const wrapper = mountCard({
      statisticsCutoff: '2026-08-04T04:20:42Z',
      onRefresh: refresh,
    })

    const card = wrapper.get('[data-test="monitor-card"]')
    expect(card.classes()).toEqual(expect.arrayContaining(['border-l-4', 'border-emerald-500', 'rounded-lg']))
    expect(card.classes()).not.toContain('card')
    expect(wrapper.get('[data-test="monitor-card-header"]').classes()).toEqual(expect.arrayContaining(['bg-emerald-50', 'px-[18px]', 'py-4']))
    expect(wrapper.get('[data-test="score-metric"]').classes()).toEqual(expect.arrayContaining(['min-h-[121px]', 'p-[14px]']))
    expect(wrapper.findAll('.service-metric')).toHaveLength(5)
    expect(wrapper.get('[data-test="success-rate-metric"]').classes()).toContain('bg-emerald-50')
    expect(wrapper.get('[data-test="ttft-metric"]').classes()).toContain('bg-blue-50')
    expect(wrapper.get('[data-test="latency-metric"]').classes()).toContain('bg-amber-50')
    expect(wrapper.get('[data-test="cost-metric"]').classes()).toContain('bg-violet-50')
    expect(wrapper.get('[data-test="concurrency-metric"]').classes()).toContain('bg-gray-50')
    expect(wrapper.findAll('[data-test="probe-bar"]')).toHaveLength(24)
    expect(wrapper.get('[data-test="probe-summary"]').text()).toContain('72 次结果 · 71 成功 · 1 失败')
    expect(wrapper.get('[data-test="calls-disclosure"]').text()).toContain('24 小时调用')
    expect(wrapper.get('[data-test="calls-disclosure"]').text()).toContain('72 次请求 · 1 次失败')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('检查于')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('统计截止')

    await wrapper.get('[data-test="refresh-account"]').trigger('click')
    expect(refresh).toHaveBeenCalledWith(113)

    for (const label of ['营收', '利润', '经营', '账务', '对账', '流水', '历史', '异常', '调整']) {
      expect(wrapper.text()).not.toContain(label)
    }
  })

  it('keeps metric milliseconds on one line, wraps a mobile identity safely, and stacks the short cutoff footer', () => {
    const wrapper = mountCard({
      account: { ...account, name: 'a-very-long-account-name-that-must-not-be-truncated@example.com' },
      statisticsCutoff: '2026-08-04T04:20:42Z',
    })

    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('1018 ms')
    expect(wrapper.get('[data-test="ttft-metric-value"]').classes()).toContain('whitespace-nowrap')
    expect(wrapper.get('[data-test="latency-metric-value"]').classes()).toContain('whitespace-nowrap')
    const identity = wrapper.get('[data-test="account-identity"]')
    expect(identity.classes()).toContain('break-words')
    expect(identity.classes()).not.toContain('truncate')
    expect(identity.text()).toContain('a-very-long-account-name-that-must-not-be-truncated@example.com')
    expect(identity.text()).toContain('#113')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('统计截止 12:20')
    expect(wrapper.get('[data-test="card-footer"]').text()).not.toContain('统计截止 2026')
    expect(wrapper.get('[data-test="card-footer"]').classes()).toContain('max-[430px]:flex-col')
  })

  it('keeps the call disclosure aligned with the selected window instead of optional account range or probe totals', () => {
    const wrapper = mountCard({
      account: { ...account, range: '24h', request_count: 426, error_count: 8 },
      selectedRange: '7d',
    })

    const disclosure = wrapper.get('[data-test="calls-disclosure"]').text()
    expect(disclosure).toContain('7 天调用')
    expect(disclosure).toContain('426 次请求 · 8 次失败')
    expect(disclosure).not.toContain('24 次请求')
  })

  it('renders procurement cost with its expected usable quota and derived multiplier', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        cost_mode: 'procurement',
        effective_multiplier: 0.48,
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('¥120.00')
    expect(cost).toContain('预计可用额度 120 USD')
    expect(cost).toContain('预计成本倍率 1.00×')
    expect(cost).not.toContain('0.58×')
  })

  it('shows a saved manual multiplier for an OpenAI API Key account', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('renders a saved manual multiplier when the API Key type uses the underscored wire spelling', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'api_key',
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('updates the visible cost when a post-save range reload replaces the account snapshot', async () => {
    const wrapper = mountCard({
      account: { ...account, account_type: 'apikey', multiplier: { value: 0.58, source: 'declared', status: 'ok', sample_count: 72 } },
    })

    await wrapper.setProps({
      account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('uses the OpenAI API Key multiplier even when stale procurement fields exist', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('0.08×')
    expect(cost).not.toContain('¥120.00')
  })

  it('does not fall back to stale procurement fields when an OpenAI API Key multiplier is unavailable', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'api_key',
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        multiplier: { value: null, source: 'manual', status: 'failed', sample_count: 0 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('--')
    expect(cost).not.toContain('¥120.00')
  })

  it('uses procurement as the sole source for OpenAI non-API-Key accounts', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'oauth',
        procurement_cost_cny: null,
        estimated_usable_quota_usd: null,
        multiplier: { value: 0.08, source: 'declared', status: 'ok', sample_count: 72 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('--')
    expect(cost).toContain('成本待确认')
    expect(cost).not.toContain('0.08×')
  })

  it('keeps procurement cost actions below the metric detail so the V3 five-tile label does not collapse', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        procurement_cost_effective_at: '2026-08-04T00:00:00Z',
        expires_at: '2026-09-01T00:00:00Z',
        cost_mode: 'procurement',
      },
    })

    const detail = wrapper.get('[data-test="cost-detail"]')
    const actions = wrapper.get('[data-test="cost-actions"]')
    expect(detail.element.compareDocumentPosition(actions.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  it('marks procurement cost as pending when expected usable quota is missing', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: null,
        cost_mode: 'procurement',
        effective_multiplier: null,
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('成本待确认')
  })

  it('rejects priority values below one or with a fraction before asking the parent to save', async () => {
    const updatePriority = vi.fn()
    const wrapper = mountCard({ onUpdatePriority: updatePriority })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    const input = wrapper.get<HTMLInputElement>('[data-test="priority-input"]')
    await input.setValue('1.5')
    await wrapper.get('[data-test="save-priority"]').trigger('click')

    expect(updatePriority).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="priority-error"]').text()).toContain('请输入大于或等于 1 的整数')
    expect((wrapper.get<HTMLInputElement>('[data-test="priority-input"]').element).value).toBe('1.5')
  })

  it('keeps priority editing until the parent confirms the save, then shows the saved value', async () => {
    const pending = deferred()
    const wrapper = mountCard({
      onUpdatePriority: (_id: number, _value: number, completion: { resolve: () => void, reject: (reason?: unknown) => void }) => {
        pending.promise.then(completion.resolve, completion.reject)
      },
    })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-test="priority-input"]').setValue('2')
    const saveButton = wrapper.get<HTMLButtonElement>('[data-test="save-priority"]')
    saveButton.element.focus()
    await saveButton.trigger('click')

    expect(wrapper.find('[data-test="priority-input"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="save-priority"]').attributes('disabled')).toBeDefined()

    pending.resolve()
    await flushAsyncWork()

    expect(wrapper.find('[data-test="priority-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-control"]').text()).toContain('2')
  })

  it('preserves priority draft, focus, and local error when the parent rejects the save', async () => {
    const wrapper = mountCard({
      onUpdatePriority: (_id: number, _value: number, completion: { reject: (reason?: unknown) => void }) => completion.reject(new Error('保存失败')),
    })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-test="priority-input"]').setValue('2')
    await wrapper.get('[data-test="save-priority"]').trigger('click')
    await flushAsyncWork()

    const input = wrapper.get<HTMLInputElement>('[data-test="priority-input"]')
    expect(input.element.value).toBe('2')
    expect(wrapper.get('[data-test="priority-error"]').text()).toContain('保存失败')
    expect(document.activeElement).toBe(input.element)
  })

  it('retains the last concurrency snapshot and marks it delayed', () => {
    const wrapper = mountCard({ concurrency: { account_id: 113, current: 3, limit: 10, delayed: true } })

    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('3 / 10')
    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('数据延迟')
  })
})
