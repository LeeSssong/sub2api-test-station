import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const account = {
  account_id: 7,
  name: 'Primary Codex',
  platform: 'openai',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  group_ids: [3],
  group_names: ['Production'],
  model_id: 'gpt-4o-mini',
  latest_status: 'success',
  sample_count: 4,
  success_rate: 1,
  ttft_p50_ms: 80,
  ttft_p95_ms: 100,
  latency_p95_ms: 240,
  multiplier: {
    value: 0.1,
    source: 'declared',
    status: 'ok',
    observed_at: '2026-07-25T08:00:00Z',
  },
  cost_guard: {
    upstream_multiplier: 0.2,
    upstream_multiplier_source: 'upstream_pricing',
    equivalent_site_multiplier: 0.204,
    cost_source: 'reconciled_bill',
    model: 'gpt-5.4',
    sample_count: 6,
    required_sample_count: 6,
    group_multiplier: 0.18,
    gap: 0.024,
    status: 'loss_confirmed',
    observed_at: '2026-07-25T08:00:00Z',
  },
  request_count: 12,
  error_count: 0,
  today_stats: { requests: 12, tokens: 3400, cost: 0.4, standard_cost: 1, user_cost: 0.6 },
  usage_windows: [{ name: '5h', utilization: 0.2, requests: 2, tokens: 100 }],
  checked_at: '2026-07-25T08:00:00Z',
  stale: false,
  quality_score: 92.5,
  group_rank: 1,
  eligible: true,
  evidence: {
    source: 'group',
    sample_count: 12,
    success_rate: 0.98,
    ttft_p50_ms: 80,
    latency_p95_ms: 240,
    observed_at: '2026-07-25T08:00:00Z',
  },
}

describe('AccountMonitorCard', () => {
  const operations = {
    total_attempts: 12,
    matched_attempts: 12,
    pending_attempts: 0,
    conflict_attempts: 0,
    coverage_known: true,
    coverage_ratio: 1,
    upstream_cost: '0.02',
    user_charge: '1.00',
    paper_profit: '0.98',
    profit_margin: '0.98',
    currency: 'USD',
    observed_at: '2026-07-25T08:01:00Z',
  }

  it('renders quality rank, global priority, and real upstream economics', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account,
        operations,
        groupOperationalState: 'operational',
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    expect(wrapper.text()).toContain('质量评分')
    expect(wrapper.text()).toContain('92.5')
    expect(wrapper.text()).toContain('组内第 1')
    expect(wrapper.text()).toContain('全局调度优先级')
    expect(wrapper.text()).toContain('上游真实扣费')
    expect(wrapper.text()).toContain('用户实际计费')
    expect(wrapper.text()).toContain('纸面利润')
    expect(wrapper.text()).toContain('利润率')
    expect(wrapper.text()).toContain('$0.02')
    expect(wrapper.text()).toContain('$1.00')
    expect(wrapper.text()).toContain('$0.98')
    expect(wrapper.text()).toContain('98.0%')
  })

  it('shows pending reconciliation instead of a profit margin when coverage is unknown', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account,
        operations: { ...operations, coverage_known: false, pending_attempts: 2, profit_margin: null },
        groupOperationalState: 'operational',
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    expect(wrapper.text()).toContain('待对账')
    expect(wrapper.text()).not.toContain('98.0%')
  })

  it('renders the lightweight cost fields and a strong confirmed-loss alert without changing economics', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: { account, operations, groupOperationalState: 'operational' },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    const summary = wrapper.get('[data-test="cost-guard-summary"]')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.upstreamMultiplier')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.multiplierSource')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.equivalentSiteMultiplier')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.costSource')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.groupMultiplier')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.status')
    expect(summary.text()).toContain('0.20x')
    expect(summary.text()).toContain('0.204x')
    expect(summary.text()).toContain('0.18x')
    expect(summary.text()).toContain('gpt-5.4')
    expect(summary.text()).toContain('6/6')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.multiplierSources.upstreamPricing')
    expect(summary.text()).toContain('admin.accountMonitor.costGuard.costSources.reconciledBill')

    const alert = wrapper.get('[data-test="cost-inversion-alert"]')
    expect(alert.classes()).toContain('border-red-300')
    expect(alert.text()).toContain('admin.accountMonitor.costGuard.alerts.inversion')
    expect(alert.text()).toContain('admin.accountMonitor.costGuard.statuses.lossConfirmed')
    expect(alert.text()).toContain('0.204x')
    expect(alert.text()).toContain('0.18x')

    expect(wrapper.get('[data-test="economics-summary"]').text()).toContain('$0.02')
  })

  it('keeps one to five real billed samples in loss observation instead of confirming loss', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account: {
          ...account,
          cost_guard: { ...account.cost_guard, sample_count: 5, status: 'loss_observing' },
        },
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    const alert = wrapper.get('[data-test="cost-inversion-alert"]')
    expect(alert.classes()).toContain('border-orange-300')
    expect(alert.text()).toContain('admin.accountMonitor.costGuard.statuses.lossObserving:{"count":5,"required":6}')
    expect(alert.text()).not.toContain('admin.accountMonitor.costGuard.alerts.inversion')
  })

  it('labels pricing-only evidence as possible loss and never as confirmed loss', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account: {
          ...account,
          cost_guard: {
            ...account.cost_guard,
            cost_source: 'upstream_pricing',
            sample_count: 0,
            status: 'pricing_risk',
          },
        },
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    const alert = wrapper.get('[data-test="cost-inversion-alert"]')
    expect(alert.text()).toContain('admin.accountMonitor.costGuard.statuses.pricingRisk')
    expect(alert.text()).not.toContain('admin.accountMonitor.costGuard.statuses.lossConfirmed')
    expect(wrapper.get('[data-test="cost-guard-summary"]').text()).toContain('admin.accountMonitor.costGuard.costSources.upstreamPricing')
  })

  it('renders a quiet covered state without a cost inversion alert', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account: {
          ...account,
          cost_guard: {
            ...account.cost_guard,
            equivalent_site_multiplier: 0.12,
            gap: -0.06,
            status: 'cost_covered',
          },
        },
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    expect(wrapper.find('[data-test="cost-inversion-alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="cost-status-badge"]').text()).toContain('admin.accountMonitor.costGuard.statuses.costCovered')
  })

  it('keeps a closed group informational instead of using a failure red border', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: { account: { ...account, eligible: false }, groupOperationalState: 'closed' },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    expect(wrapper.text()).toContain('已关闭')
    expect(wrapper.classes()).not.toContain('border-red-500')
  })

  it('reuses account-management today stats and usage-window components', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: { account },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: {
            props: ['stats'],
            template: '<div data-test="today-stats">{{ stats.requests }}/{{ stats.tokens }}</div>',
          },
          AccountUsageCell: {
            props: ['account', 'todayStats'],
            template: '<div data-test="usage-cell">{{ account.id }}:{{ account.type }}:{{ todayStats.requests }}</div>',
          },
        },
      },
    })

    expect(wrapper.get('[data-test="today-stats"]').text()).toBe('12/3400')
    expect(wrapper.get('[data-test="usage-cell"]').text()).toBe('7:oauth:12')
  })

  it.each([
    [{ value: 0.07, source: 'declared', status: 'ok' }, '0.07x', 'admin.accountMonitor.multiplier.declared'],
    [{ value: 0.25, source: 'measured', status: 'ok' }, '0.25x', 'admin.accountMonitor.multiplier.measured'],
    [{ source: 'measured', status: 'stale' }, 'admin.accountMonitor.multiplier.stale', ''],
    [{ status: 'unsupported' }, 'admin.accountMonitor.multiplier.unsupported', ''],
    [{ source: 'measured', status: 'failed' }, 'admin.accountMonitor.multiplier.failed', ''],
    [{ status: 'unavailable' }, 'admin.accountMonitor.multiplier.unavailable', ''],
  ])('renders trusted multiplier state %#', (multiplier, value, source) => {
    const wrapper = mount(AccountMonitorCard, {
      props: { account: { ...account, multiplier } },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    const metric = wrapper.get('[data-test="multiplier-metric"]')
    expect(metric.text()).toContain(value)
    if (source) expect(metric.text()).toContain(source)
    if (multiplier.status !== 'ok') expect(metric.text()).not.toContain('1.00x')
  })
})
