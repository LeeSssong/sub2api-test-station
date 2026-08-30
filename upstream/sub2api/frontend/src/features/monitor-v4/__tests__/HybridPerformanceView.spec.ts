import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import HybridPerformanceView from '../HybridPerformanceView.vue'

const { getSnapshot } = vi.hoisted(() => ({
  getSnapshot: vi.fn().mockResolvedValue({
    contract_version: '2', window: '7d', refresh_interval_seconds: 0,
    generated_at: '2026-08-25T00:00:00Z', groups: [],
  }),
}))

vi.mock('../api', () => ({ getHybridPerformanceSnapshot: getSnapshot }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
    t: (key: string, args?: Record<string, unknown>) => {
      const messages: Record<string, string> = {
        'channelMonitorV2.hybrid.title': '分组性能监控',
        'channelMonitorV2.hybrid.updated': `更新于 ${args?.time ?? ''}`,
        'channelMonitorV2.hybrid.empty': '暂无可见分组',
        'channelMonitorV2.hybrid.loadError': '该时间范围加载失败，请重试',
        'channelMonitorV2.hybrid.retry': '重试',
        'monitorV2.window.24h': '24h',
        'monitorV2.window.7d': '7d',
        'monitorV2.window.30d': '30d',
      }
      return messages[key] ?? key
    },
    }),
  }
})

describe('HybridPerformanceView', () => {
  it('renders Chinese title and empty copy instead of translation keys', async () => {
    const wrapper = mount(HybridPerformanceView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          CodexRadarRecommendations: { template: '<section data-test="codexradar-panel" />' },
        },
      },
    })
    await vi.waitFor(() => expect(getSnapshot).toHaveBeenCalled())
    expect(getSnapshot).toHaveBeenCalledWith('24h', expect.any(AbortSignal))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('分组性能监控')
    expect(wrapper.text()).toContain('暂无可见分组')
    expect(wrapper.text()).not.toContain('monitorV2.hybrid.title')
    expect(wrapper.text()).not.toContain('monitorV2.hybrid.empty')
    expect(wrapper.find('[data-test="codexradar-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="hybrid-group-status"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows a retryable error while keeping the last successful window', async () => {
    getSnapshot.mockReset()
    getSnapshot.mockRejectedValueOnce(new Error('timeout'))
    const wrapper = mount(HybridPerformanceView, {
      global: { stubs: { AppLayout: { template: '<main><slot /></main>' }, CodexRadarRecommendations: { template: '<section />' } } },
    })
    await vi.waitFor(() => expect(wrapper.find('[data-test="hybrid-load-error"]').exists()).toBe(true))
    expect(wrapper.get('[data-test="hybrid-load-error"]').text()).toContain('该时间范围加载失败')
    expect(wrapper.get('[data-test="hybrid-retry"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('requests the 30-day window when its tab is selected', async () => {
    getSnapshot.mockReset()
    getSnapshot.mockResolvedValue({ contract_version: '2', window: '24h', refresh_interval_seconds: 0, generated_at: '2026-08-25T00:00:00Z', groups: [] })
    const wrapper = mount(HybridPerformanceView, {
      global: { stubs: { AppLayout: { template: '<main><slot /></main>' }, CodexRadarRecommendations: { template: '<section />' } } },
    })
    await vi.waitFor(() => expect(getSnapshot).toHaveBeenCalledWith('24h', expect.any(AbortSignal)))
    await wrapper.get('[data-test="hybrid-window-30d"]').trigger('click')
    await vi.waitFor(() => expect(getSnapshot).toHaveBeenCalledWith('30d', expect.any(AbortSignal)))
    wrapper.unmount()
  })
})
