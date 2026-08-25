import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import HybridPerformanceView from '../HybridPerformanceView.vue'

const { getSnapshot } = vi.hoisted(() => ({
  getSnapshot: vi.fn().mockResolvedValue({
    contract_version: '1', window: '7d', refresh_interval_seconds: 0,
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
        'channelMonitorV2.hybrid.title': '渠道性能监控',
        'channelMonitorV2.hybrid.updated': `更新于 ${args?.time ?? ''}`,
        'channelMonitorV2.hybrid.empty': '暂无可见分组',
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
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('渠道性能监控')
    expect(wrapper.text()).toContain('暂无可见分组')
    expect(wrapper.text()).not.toContain('monitorV2.hybrid.title')
    expect(wrapper.text()).not.toContain('monitorV2.hybrid.empty')
    expect(wrapper.find('[data-test="codexradar-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="hybrid-group-status"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
