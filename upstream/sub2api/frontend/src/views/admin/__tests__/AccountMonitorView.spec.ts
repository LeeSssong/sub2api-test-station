import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountMonitorView from '../AccountMonitorView.vue'

const {
  list,
  updateSettings,
  runAll,
  runOne,
  history,
} = vi.hoisted(() => ({
  list: vi.fn(),
  updateSettings: vi.fn(),
  runAll: vi.fn(),
  runOne: vi.fn(),
  history: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accountMonitor: {
      list,
      updateSettings,
      runAll,
      runOne,
      history,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

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
  name: 'Primary Claude',
  platform: 'anthropic',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  group_ids: [3],
  group_names: ['Production'],
  model_id: 'claude-sonnet-4-5',
  latest_status: 'success',
  sample_count: 10,
  success_rate: 0.9,
  ttft_p50_ms: 120,
  ttft_p95_ms: 190,
  latency_p95_ms: 840,
  multiplier: 0.1,
  request_count: 12,
  error_count: 1,
  today_stats: { requests: 12, tokens: 100, cost: 0.1, standard_cost: 0.2, user_cost: 0.15 },
  usage_windows: [{ name: '5h', utilization: 0.2, requests: 2, tokens: 100 }],
  checked_at: '2026-07-25T08:00:00Z',
  stale: false,
}

const projection = () => ({
  schema_version: 1,
  observed_at: '2026-07-25T08:01:00Z',
  stale: false,
  settings: {
    interval_seconds: 300,
    updated_by: 1,
    updated_at: '2026-07-25T07:55:00Z',
  },
  accounts: [account],
})

function mountView() {
  return mount(AccountMonitorView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        AccountMonitorCard: {
          props: ['account'],
          emits: ['refresh', 'settings', 'history'],
          template: `
            <article data-test="monitor-card">
              <span>{{ account.name }}</span>
              <span>{{ account.multiplier.toFixed(2) }}x</span>
              <span>{{ account.latest_status }}</span>
              <span>{{ account.error_code }}</span>
              <button data-test="card-refresh" @click="$emit('refresh', account.account_id)">refresh</button>
              <button data-test="card-settings" @click="$emit('settings')">settings</button>
              <button data-test="card-history" @click="$emit('history', account.account_id)">history</button>
            </article>
          `,
        },
        AccountMonitorFilters: {
          props: ['search', 'platform', 'status'],
          emits: ['update:search', 'update:platform', 'update:status'],
          template: '<div data-test="filters" />',
        },
        AccountMonitorSettingsDialog: {
          props: ['show', 'intervalSeconds'],
          emits: ['close', 'save'],
          template: `
            <div v-if="show" data-test="settings-dialog">
              <button data-test="save-settings" @click="$emit('save', 60)">save</button>
            </div>
          `,
        },
        AccountTodayStatsCell: true,
        AccountUsageCell: true,
        HelpTooltip: true,
        Icon: true,
      },
    },
  })
}

describe('admin account monitor view', () => {
  beforeEach(() => {
    list.mockReset().mockResolvedValue(projection())
    updateSettings.mockReset().mockResolvedValue({
      interval_seconds: 60,
      updated_by: 1,
      updated_at: '2026-07-25T08:02:00Z',
    })
    runAll.mockReset().mockResolvedValue({ completed: 1 })
    runOne.mockReset().mockResolvedValue({ account_id: 7, status: 'success' })
    history.mockReset().mockResolvedValue({ items: [] })
  })

  it('renders monitored account quality and today-stat projection', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Primary Claude')
    expect(wrapper.text()).toContain('0.10x')
    expect(wrapper.text()).toContain('success')
    expect(wrapper.get('[data-test="monitor-card"]').exists()).toBe(true)
  })

  it('updates the global interval from the header and card settings action', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="open-settings"]').trigger('click')
    await wrapper.get('[data-test="save-settings"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(60)

    await wrapper.get('[data-test="card-settings"]').trigger('click')
    expect(wrapper.get('[data-test="settings-dialog"]').exists()).toBe(true)
  })

  it('runs the whole pool and one account, including history loading', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="run-all"]').trigger('click')
    await wrapper.get('[data-test="card-refresh"]').trigger('click')
    await wrapper.get('[data-test="card-history"]').trigger('click')
    await flushPromises()

    expect(runAll).toHaveBeenCalledOnce()
    expect(runOne).toHaveBeenCalledWith(7)
    expect(history).toHaveBeenCalledWith(7, 25)
  })

  it('shows stale, failed, and no-history states without removing the card', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      stale: true,
      accounts: [
        { ...account, latest_status: 'failed', error_code: 'timeout', sample_count: 0, stale: true },
      ],
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="monitor-card"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('failed')
    expect(wrapper.text()).toContain('timeout')
  })
})
