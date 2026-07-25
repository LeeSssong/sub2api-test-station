import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
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
  request_count: 12,
  error_count: 0,
  today_stats: { requests: 12, tokens: 3400, cost: 0.4, standard_cost: 1, user_cost: 0.6 },
  usage_windows: [{ name: '5h', utilization: 0.2, requests: 2, tokens: 100 }],
  checked_at: '2026-07-25T08:00:00Z',
  stale: false,
}

describe('AccountMonitorCard', () => {
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
