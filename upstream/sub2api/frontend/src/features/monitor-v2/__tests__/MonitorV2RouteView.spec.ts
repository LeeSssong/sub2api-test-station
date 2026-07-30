import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMonitorV2Snapshot } = vi.hoisted(() => ({
  getMonitorV2Snapshot: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof import('../api')>('../api')
  return {
    ...actual,
    getMonitorV2Snapshot,
  }
})

vi.mock('@/views/user/ChannelStatusView.vue', () => ({
  default: {
    name: 'ChannelStatusView',
    template: '<section data-test="native-channel-status">原生渠道状态</section>',
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) =>
        ({
          'monitorV2.loading': '正在读取分组状态',
          'monitorV2.fallbackNotice': '新版监控暂不可用，已切换到基础状态页。',
          'monitorV2.title': '服务监控',
          'monitorV2.description': '可见分组状态',
          'monitorV2.updatedAt': '更新于现在',
          'monitorV2.refresh': '刷新',
          'monitorV2.overall.noData': '等待监控数据',
          'monitorV2.window.24h': '24 小时',
          'monitorV2.window.7d': '7 天',
          'monitorV2.window.30d': '30 天',
          'monitorV2.empty.title': '暂无可见分组',
          'monitorV2.empty.description': '管理员尚未开放可展示的服务分组。',
          'monitorV2.notes.metrics': '样本说明',
          'monitorV2.notes.privacy': '隐私说明',
        })[key] ?? key,
    }),
  }
})

import MonitorV2RouteView from '../MonitorV2RouteView.vue'

const emptySnapshot = {
  contract_version: '2' as const,
  window: '7d' as const,
  generated_at: '2026-07-29T12:00:00Z',
  groups: [],
}

function mountRoute() {
  return mount(MonitorV2RouteView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<span aria-hidden="true" />' },
      },
    },
  })
}

describe('MonitorV2RouteView', () => {
  beforeEach(() => {
    getMonitorV2Snapshot.mockReset()
  })

  it('renders Monitor V2 after the initial contract succeeds', async () => {
    getMonitorV2Snapshot.mockResolvedValue(emptySnapshot)

    const wrapper = mountRoute()
    await flushPromises()

    expect(getMonitorV2Snapshot).toHaveBeenCalledWith('7d', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('暂无可见分组')
    expect(wrapper.find('[data-test="native-channel-status"]').exists()).toBe(false)
  })

  it('falls back to the preserved native status page without leaking the error', async () => {
    getMonitorV2Snapshot.mockRejectedValue(
      new Error('GET /api/v1/monitor-v2 failed: postgres password=secret')
    )

    const wrapper = mountRoute()
    await flushPromises()

    expect(wrapper.text()).toContain('新版监控暂不可用，已切换到基础状态页。')
    expect(wrapper.text()).toContain('原生渠道状态')
    expect(wrapper.text()).not.toContain('postgres')
    expect(wrapper.text()).not.toContain('password')
    expect(wrapper.text()).not.toContain('/api/v1/monitor-v2')
  })
})
