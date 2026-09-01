import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { AccountMonitorAccount } from '@/api/admin/accountMonitor'
import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'admin.accounts.modelDetection.section': '模型监测',
      'admin.accounts.modelDetection.detectNow': '手动探测',
      'admin.accounts.modelDetection.detecting': '探测中',
      'admin.accounts.modelDetection.editConnectionProbeModel': '连接测试模型',
      'admin.accounts.modelDetection.status.untested': '未检测',
      'admin.accounts.modelDetection.status.normal': '正常',
      'admin.accounts.modelDetection.viewRecent': '查看最近结果',
    }[key] ?? key),
  }),
}))

const account = {
  account_id: 278,
  name: 'xian-plus',
  platform: 'OpenAI',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  priority: 1,
  group_names: ['GPT-Pro'],
  request_count: 12846,
  lifetime_real_request_count: 54231,
  success_rate: 0.9913,
  ttft_p95_ms: 4120,
  multiplier: { value: 0.12, source: 'declared', status: 'ok', sample_count: 12846 },
  group_profitability: { status: 'confirmed', profit_rate: 0.618 },
  scheduler_explanation: { eligible: true, policy_label: '服务质量优先' },
  scheduler_rank: 2,
  scheduler_rank_total: 12,
  model_detection: {
    status: 'normal',
    settings: { account_id: 278, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
  },
  real_request_timeline: Array.from({ length: 24 }, (_, index) => ({
    start_at: `2026-08-30T00:${String(index).padStart(2, '0')}:00Z`,
    request_count: index + 1,
    success_count: index,
    failure_count: index === 8 ? 1 : 0,
    ttft_p95_ms: index === 8 ? 6200 : 4000,
  })),
} as unknown as AccountMonitorAccount

function mountCard(overrides: Record<string, unknown> = {}) {
  return mount(AccountMonitorCard, {
    attachTo: document.body,
    props: { account, ...overrides },
    global: { stubs: { Icon: true } },
  })
}

afterEach(() => document.body.replaceChildren())

describe('AccountMonitorCard R2', () => {
  it('renders the approved identity, scheduler rank, four metrics, chart and two footer actions', () => {
    const wrapper = mountCard()

    expect(wrapper.get('[data-test="account-identity"]').text()).toBe('xian-plus #278')
    expect(wrapper.get('[data-test="scheduler-column"]').text()).toContain('第 2 / 12')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('99.1%')
    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('4120 ms')
    expect(wrapper.get('[data-test="profit-rate-metric"]').text()).toContain('61.8%')
    expect(wrapper.get('[data-test="native-priority-metric"]').text()).toContain('1')
    expect(wrapper.get('[data-test="upstream-multiplier-metric"]').text()).toContain('0.12×')
    expect(wrapper.findAll('[data-test="real-request-bar"]').length).toBe(24)
    expect(wrapper.get('[data-test="account-info"]').text()).toContain('账号详情')
    expect(wrapper.get('[data-test="account-more"]').text()).toContain('账号操作')
  })

  it('uses persisted real-request evidence and does not render legacy explanatory surfaces', () => {
    const wrapper = mountCard()
    const text = wrapper.text()

    expect(text).toContain('12,846 次窗口真实请求')
    expect(text).toContain('累计 54,231 次')
    expect(text).not.toContain('质量评分')
    expect(text).not.toContain('全站质量排名')
    expect(text).not.toContain('组内质量排名')
    expect(text).not.toContain('成本折合本站倍率')
    expect(text).not.toContain('事实源：')
    expect(text).not.toContain('柱越高表示综合表现越好')
    expect(wrapper.find('[data-test="quality-column"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="ranking-explanation"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="cost-metric"]').exists()).toBe(false)
  })

  it('shows multiplier-estimated profit before real requests exist', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        request_count: 0,
        real_request_evidence: {
          request_count: 0,
          success_count: 0,
          failure_count: 0,
          success_rate: 0,
          ttft_sample_count: 0,
          observed_at: '2026-08-30T00:00:00Z',
        },
        group_profitability: { status: 'estimated', profit_rate: 0.25 },
      },
    })

    expect(wrapper.get('[data-test="profit-rate-metric"]').text()).toContain('25%')
  })

  it('routes all-site profitability to group views', () => {
    expect(mountCard({ rankingScope: 'global' }).get('[data-test="profit-rate-metric"]').text()).toContain('按分组查看')
    expect(mountCard({ rankingScope: 'group' }).get('[data-test="profit-rate-metric"]').text()).toContain('61.8%')
  })

  it('renders 24 empty real-request buckets and labels probe refresh separately', () => {
    const wrapper = mountCard({ account: { ...account, real_request_timeline: [] } })

    expect(wrapper.findAll('[data-test="real-request-bar"]')).toHaveLength(24)
    expect(wrapper.get('[data-test="timeline-section"]').text()).toContain('真实性能 · 真实请求')
    expect(wrapper.get('[data-test="refresh-account"]').text()).toContain('刷新探测状态')
    expect(wrapper.get('[data-test="refresh-account"]').attributes('title')).toContain('不生成真实请求样本')
  })

  it('labels active-probe fallback without adding it to real-request counts', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        request_count: 0,
        lifetime_real_request_count: 51,
        real_request_timeline: [{
          start_at: '2026-08-30T00:00:00Z', end_at: '2026-08-30T01:00:00Z',
          request_count: 0, success_count: 0, failure_count: 0,
          probe_count: 1, probe_success_count: 1, probe_failure_count: 0, source: 'probe', ttft_p95_ms: 900,
        }],
      },
    })

    expect(wrapper.get('[data-test="account-metadata"]').text()).toContain('0 次窗口真实请求 · 累计 51 次')
    expect(wrapper.get('[data-test="real-request-bar"]').attributes('title')).toContain('主动探测兜底')
    expect(wrapper.get('[data-test="real-request-bar"]').attributes('title')).toContain('真实请求 0')
  })

  it('keeps manual model detection and account action entry points', async () => {
    const detect = vi.fn()
    const info = vi.fn()
    const more = vi.fn()
    const editCost = vi.fn()
    const wrapper = mountCard({ onDetectModelDetection: detect, onAccountInfo: info, onAccountMore: more, onEditCost: editCost })

    await wrapper.get('[data-test="detect-model-detection"]').trigger('click')
    await wrapper.get('[data-test="account-info"]').trigger('click')
    await wrapper.get('[data-test="account-more"]').trigger('click')
    await wrapper.get('[data-test="upstream-multiplier-metric"] button').trigger('click')

    expect(detect).toHaveBeenCalledWith(278)
    expect(info).toHaveBeenCalledWith(account)
    expect(more).toHaveBeenCalledWith(account, expect.any(MouseEvent))
    expect(editCost).toHaveBeenCalledWith(account)
  })

  it('refreshes active probe from the chart action without adding a second control rail', async () => {
    const refresh = vi.fn()
    const wrapper = mountCard({ onRefresh: refresh })

    await wrapper.get('[data-test="refresh-account"]').trigger('click')
    expect(refresh).toHaveBeenCalledWith(278)
    expect(wrapper.findAll('[data-test="account-actions"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="account-actions"] [data-test="account-edit"]').classes()).toContain('sr-only')
    expect(wrapper.get('[data-test="account-actions"] [data-test="account-delete"]').classes()).toContain('sr-only')
  })

  it('renders the mobile-safe card structure', () => {
    const wrapper = mountCard()
    expect(wrapper.get('[data-test="monitor-card"]').classes()).toContain('monitor-card-shell')
    expect(wrapper.get('[data-test="monitor-card-header"]').classes()).toContain('monitor-card-layout')
    expect(wrapper.get('[data-test="timeline-section"]').attributes('aria-label')).toBe('真实性能')
  })
})
