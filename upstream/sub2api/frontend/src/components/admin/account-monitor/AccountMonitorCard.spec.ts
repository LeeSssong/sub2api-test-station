import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key === 'admin.accountMonitor.status.paused' ? '暂停' : key }),
  }
})

const account = {
  account_id: 7,
  name: 'Primary Codex',
  platform: 'openai',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  management_state: 'enabled',
  service_state: 'available',
  group_eligibility: 'eligible',
  monitor_bucket: 'available',
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

  it('hides quality score and group rank in the all-site scope', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account,
        scope: 'all',
      },
      global: {
        stubs: {
          Icon: true,
          AccountTodayStatsCell: true,
          AccountUsageCell: true,
        },
      },
    })

    expect(wrapper.find('[data-test="quality-summary"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('质量评分')
    expect(wrapper.text()).not.toContain('组内第 1')
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

  it('shows paused accounts as 暂停 without a failed service badge', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account: {
          ...account,
          status: 'paused',
          schedulable: false,
          management_state: 'paused',
          service_state: 'not_monitored',
          group_eligibility: 'not_applicable',
          monitor_bucket: 'paused',
          latest_status: 'failed',
          eligible: false,
          checked_at: null,
          latest: null,
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

    expect(wrapper.text()).toContain('暂停')
    expect(wrapper.text()).not.toContain('admin.accountMonitor.status.failed')
    expect(wrapper.classes()).not.toContain('border-red-500')
  })

  it('shows service-available accounts as healthy even when cost-ineligible', () => {
    const wrapper = mount(AccountMonitorCard, {
      props: {
        account: {
          ...account,
          service_state: 'available',
          group_eligibility: 'cost_ineligible',
          latest_status: 'failed',
          eligible: false,
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

    expect(wrapper.text()).toContain('admin.accountMonitor.status.success')
    expect(wrapper.text()).not.toContain('admin.accountMonitor.status.failed')
    expect(wrapper.classes()).toContain('border-emerald-500')
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
