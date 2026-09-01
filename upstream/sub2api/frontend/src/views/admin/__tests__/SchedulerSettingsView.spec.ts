import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SchedulerSettingsView from '../SchedulerSettingsView.vue'

const { getSettings, getGroups, updateSettings, showError, showSuccess, fetchPublicSettings, adminSettingsFetch } = vi.hoisted(() => ({
  getSettings: vi.fn(),
  getGroups: vi.fn(),
  updateSettings: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  fetchPublicSettings: vi.fn(),
  adminSettingsFetch: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    settings: { getSettings, updateSettings },
    groups: { getAll: getGroups },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError, showSuccess, fetchPublicSettings }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ fetch: adminSettingsFetch }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  const translations: Record<string, string> = {
    'admin.schedulerSettings.saved': '调度策略已保存',
    'admin.schedulerSettings.saveFailed': '保存调度策略失败',
  }

  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => translations[key] ?? key }),
  }
})

function mountPage() {
  return mount(SchedulerSettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
      },
    },
  })
}

describe('SchedulerSettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getSettings.mockResolvedValue({
      openai_advanced_scheduler_enabled: true,
      openai_advanced_scheduler_group_policies: {
        11: {
          mode: 'custom',
          extra_retry_count: 1,
          priority: { profit: 1, ttft: 2, latency: 3 },
          operations: { balance: 'standard', peak_protection: 'strict', session_continuity: 'standard' },
          weight_overrides: { ttft: 2 },
        },
      },
    })
    getGroups.mockResolvedValue([
      { id: 11, name: 'GPT-特惠', status: 'active' },
      { id: 12, name: 'GPT-Pro', status: 'active' },
    ])
    updateSettings.mockImplementation(async (payload) => payload)
  })

  it('shows the group recovery control and defaults missing values to zero', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-testid="scheduler-extra-retry-count"]').element.value).toBe('1')

    await wrapper.get('[data-testid="scheduler-group-12"]').trigger('click')
    expect(wrapper.get('[data-testid="scheduler-extra-retry-count"]').element.value).toBe('0')
    expect(wrapper.find('[data-testid="scheduler-priority-profit-3"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="scheduler-operation-balance-high"]').exists()).toBe(false)
  })

  it('limits recovery attempts to zero through three', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const options = wrapper.get('[data-testid="scheduler-extra-retry-count"]').findAll('option')
    expect(options.map((option) => option.element.value)).toEqual(['0', '1', '2', '3'])
  })

  it('saves extra recovery count while preserving legacy policy fields', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-extra-retry-count"]').setValue('3')
    await wrapper.get('[data-testid="scheduler-save"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      openai_advanced_scheduler_enabled: true,
      openai_advanced_scheduler_group_policies: expect.objectContaining({
        11: expect.objectContaining({
          mode: 'custom',
          extra_retry_count: 3,
          priority: { profit: 1, ttft: 2, latency: 3 },
          operations: { balance: 'standard', peak_protection: 'strict', session_continuity: 'standard' },
          weight_overrides: { ttft: 2 },
        }),
      }),
    }))
    expect(showSuccess).toHaveBeenCalledWith('调度策略已保存')
  })

  it('keeps the draft and shows an error when saving fails', async () => {
    updateSettings.mockRejectedValueOnce(new Error('network'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-extra-retry-count"]').setValue('2')
    await wrapper.get('[data-testid="scheduler-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="scheduler-extra-retry-count"]').element.value).toBe('2')
    expect(showError).toHaveBeenCalledWith('保存调度策略失败')
  })
})
