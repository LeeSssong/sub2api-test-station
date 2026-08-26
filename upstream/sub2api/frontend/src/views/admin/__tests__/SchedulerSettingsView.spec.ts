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
          priority: { profit: 1, ttft: 2, latency: 3 },
          operations: { balance: 'standard', peak_protection: 'strict', session_continuity: 'standard' },
        },
      },
    })
    getGroups.mockResolvedValue([
      { id: 11, name: 'GPT-特惠', status: 'active' },
      { id: 12, name: 'GPT-Pro', status: 'active' },
    ])
    updateSettings.mockImplementation(async (payload) => payload)
  })

  it('shows a selected state after choosing a priority and operational option', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-priority-profit-3"]').trigger('click')
    await wrapper.get('[data-testid="scheduler-operation-balance-high"]').trigger('click')

    expect(wrapper.get('[data-testid="scheduler-priority-profit-3"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="scheduler-priority-profit-3"]').classes()).toContain('scheduler-option-selected')
    expect(wrapper.get('[data-testid="scheduler-operation-balance-high"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="scheduler-operation-balance-high"]').classes()).toContain('scheduler-option-selected')
  })

  it('updates the normal-traffic preview when account balance changes', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-operation-balance-high"]').trigger('click')

    expect(wrapper.get('[data-testid="scheduler-preview-normal"]').text()).toContain('优先补齐长期未参与的健康账号')
  })

  it('saves the existing native policy together with the current business draft', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-operation-balance-high"]').trigger('click')
    await wrapper.get('[data-testid="scheduler-save"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      openai_advanced_scheduler_enabled: true,
      openai_advanced_scheduler_group_policies: expect.objectContaining({
        11: expect.objectContaining({
          operations: expect.objectContaining({ balance: 'high' }),
        }),
      }),
    }))
    expect(showSuccess).toHaveBeenCalledWith('调度策略已保存')
  })

  it('keeps the draft and shows an error when saving fails', async () => {
    updateSettings.mockRejectedValueOnce(new Error('network'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="scheduler-operation-balance-high"]').trigger('click')
    await wrapper.get('[data-testid="scheduler-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="scheduler-operation-balance-high"]').attributes('aria-pressed')).toBe('true')
    expect(showError).toHaveBeenCalledWith('保存调度策略失败')
  })
})
