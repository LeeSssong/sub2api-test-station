import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ReadModelStatus from '@/components/admin/ReadModelStatus.vue'
import { controlPlaneAPI, resolveControlPlaneReadMode } from '@/api/controlPlane'
import { apiClient } from '@/api/client'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() } }))

describe('control plane API', () => {
  it('uses same-origin routes, isolates session recovery, and normalizes freshness metadata', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      data: {
        items: [],
        generated_at: '2026-08-10T00:00:00Z',
        source_watermark: 'event-42',
        freshness_seconds: 12,
        completeness: 'complete',
        calculation_version: 'v1',
      },
    })
    const response = await controlPlaneAPI.monitor({ range: '24h' })
    expect(apiClient.get).toHaveBeenCalledWith('/xingqiao/accounts/monitor', {
      params: { range: '24h' },
      skipSessionRecovery: true,
    })
    expect(response.freshness?.source_watermark).toBe('event-42')
    expect(response.freshness?.calculation_version).toBe('v1')
  })

  it('requires an idempotency key for account refresh requests', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { account_id: 7, status: 'accepted' } })
    await controlPlaneAPI.refreshAccount(7, 'refresh-7')
    expect(apiClient.post).toHaveBeenCalledWith('/xingqiao/accounts/7/refresh', {}, {
      headers: { 'Idempotency-Key': 'refresh-7' },
      skipSessionRecovery: true,
    })
  })

  it('fails closed to legacy-only for unknown read modes', () => {
    expect(resolveControlPlaneReadMode('external_primary')).toBe('external_primary')
    expect(resolveControlPlaneReadMode('shadow')).toBe('shadow')
    expect(resolveControlPlaneReadMode('unexpected')).toBe('legacy_only')
  })

  it('renders control-plane failure as a local status with retry', async () => {
    const wrapper = mount(ReadModelStatus, {
      props: {
        generatedAt: '2026-08-10T00:00:00Z',
        completeness: 'partial',
        calculationVersion: 'accounts-v1',
        degraded: true,
        sourceLabel: '控制面',
      },
    })

    expect(wrapper.text()).toContain('控制面暂时不可用')
    expect(wrapper.text()).toContain('完整性：partial')
    expect(wrapper.text()).toContain('计算版本：accounts-v1')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})
