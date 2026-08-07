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
    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('1018 ms')
    expect(wrapper.get('[data-test="latency-metric"]').text()).toContain('1962 ms')
    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('成本待确认')
    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('3 / 10')

    for (const label of ['分组倍率', '营收', '利润', '对账', '账务', '上游真实扣费', '用户实际计费']) {
      expect(wrapper.text()).not.toContain(label)
    }
  })

  it('renders probe evidence instead of perfect real requests for an unavailable account', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        request_count: 33,
        error_count: 0,
        success_rate: 1,
        quality_score: 100,
        group_rank: 1,
        probe_sample_count: 24,
        probe_success_count: 0,
        probe_success_rate: 0,
        probe_ttft_p50_ms: null,
        probe_latency_p95_ms: null,
        availability_status: 'unavailable',
        score_status: 'ineligible',
      },
    })

		expect(wrapper.get('[data-test="status-badge"]').text()).toContain('不可用')
		expect(wrapper.get('[data-test="score-metric"] strong').text()).toBe('--')
    expect(wrapper.get('[data-test="rank-metric"]').text()).toContain('未排名')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('探测成功率')
		expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('0%')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('24 次探测样本')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).not.toContain('33 次真实请求')
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
