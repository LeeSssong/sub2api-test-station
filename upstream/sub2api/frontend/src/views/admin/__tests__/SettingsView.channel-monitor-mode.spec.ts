import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  cachedPublicSettings: null as Record<string, unknown> | null,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => state,
}))

import {
  getChannelMonitorMode,
  isChannelMonitorHybridMode,
  isChannelMonitorNativeProbeMode,
  isChannelMonitorV1Mode,
  isChannelMonitorV2Mode,
} from '@/utils/featureFlags'
import zhSettings from '@/i18n/locales/zh/admin/settings'

describe('channel monitor mode selection', () => {
  beforeEach(() => {
    state.cachedPublicSettings = { channel_monitor_enabled: true, channel_monitor_mode: 'v1' }
  })

  it('preserves legacy modes and exposes the two new explicit modes', () => {
    const modes = ['v1', 'v2', 'native_probe', 'hybrid_performance'] as const
    for (const mode of modes) {
      state.cachedPublicSettings = { channel_monitor_enabled: true, channel_monitor_mode: mode }
      expect(getChannelMonitorMode()).toBe(mode)
    }
  })

  it('reports runtime predicates without conflating the four choices', () => {
    state.cachedPublicSettings = { channel_monitor_enabled: true, channel_monitor_mode: 'v1' }
    expect(isChannelMonitorV1Mode()).toBe(true)
    expect(isChannelMonitorV2Mode()).toBe(false)
    expect(isChannelMonitorNativeProbeMode()).toBe(false)
    expect(isChannelMonitorHybridMode()).toBe(false)

    state.cachedPublicSettings.channel_monitor_mode = 'v2'
    expect(isChannelMonitorV2Mode()).toBe(true)

    state.cachedPublicSettings.channel_monitor_mode = 'native_probe'
    expect(isChannelMonitorNativeProbeMode()).toBe(true)

    state.cachedPublicSettings.channel_monitor_mode = 'hybrid_performance'
    expect(isChannelMonitorHybridMode()).toBe(true)
  })

  it('provides the approved Chinese fourth-mode copy', () => {
    const channelMonitor = (zhSettings as any).settings.features.channelMonitor
    expect(channelMonitor.modeHybrid).toContain('混合')
    expect(channelMonitor.modeHybridHint).toContain('主动探测')
    expect(channelMonitor.modeHybridHint).toContain('真实请求')
  })
})
