import { beforeEach, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getLineSelectionMetrics } from '../lineSelection'
import { getPublicSettings } from '@/api/auth'
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))
vi.mock('@/api/auth', () => ({ getPublicSettings: vi.fn() }))
const coverage = { computed_at: '2026-11-06T09:00:00Z' }
const row = (id = 1) => ({ platform: 'openai', group_id: id, health: { overall: 'warning', error_rate: 'healthy' }, metrics: { has_samples: true, request_count: 0, error_rate: .005, ttft: { p50_ms: 5000 } } })
beforeEach(() => {
  vi.mocked(apiClient.get).mockReset()
  vi.mocked(getPublicSettings).mockReset().mockResolvedValue({ channel_monitor_enabled: true, channel_monitor_mode: 'v2' } as never)
})
it('reads the same 24-hour V2 matrix as route details, preserving redacted-count rates and server status', async () => {
  vi.mocked(apiClient.get).mockResolvedValue({ data: { coverage, items: [row(), row(99)] } })
  const c = new AbortController()
  const result = await getLineSelectionMetrics(c.signal, [1], 'codex')
  expect(apiClient.get).toHaveBeenCalledWith('/channel-monitor-v2/matrix', expect.objectContaining({ signal: c.signal, params: expect.objectContaining({ range: '24h', tool_id: 'codex', group_id: [1], group_by: 'platform_group' }) }))
  expect(result).toEqual({ generated_at: coverage.computed_at, groups: [{ id: 1, has_samples: true, health: 'warning', availability_health: 'healthy', error_rate: .005, ttft_p50_ms: 5000 }] })
})

it('follows fresh V1 settings and uses probe status and seven-day availability without inventing TTFT', async () => {
  vi.mocked(getPublicSettings).mockResolvedValue({ channel_monitor_enabled: true, channel_monitor_mode: 'v1' } as never)
  vi.mocked(apiClient.get).mockResolvedValue({ data: { items: [
    { id: 70, group_id: 1, primary_status: 'operational', has_samples_7d: true, availability_7d: 99.5, primary_latency_ms: 1800 },
    { id: 71, group_id: 2, primary_status: '', has_samples_7d: false, availability_7d: 0 },
    { id: 72, group_id: 99, primary_status: 'failed', has_samples_7d: true, availability_7d: 0 },
  ] } })
  const result = await getLineSelectionMetrics(undefined, [1, 2], 'codex')
  expect(apiClient.get).toHaveBeenCalledWith('/channel-monitors', expect.objectContaining({ params: { tool_id: 'codex' } }))
  expect(result.groups).toHaveLength(2)
  expect(result.groups[0]).toMatchObject({ id: 1, health: 'healthy', status_label: '正常', availability_percent: 99.5, availability_label: '可用性 · 近7天', ttft_p50_ms: null })
  expect(result.groups[1]).toMatchObject({ id: 2, has_samples: false, availability_percent: null })
})

const v3Component = (id = 10, group_id: number | undefined = 1) => ({ id, group_id, status: 'degraded', availability: 97.46, model: '', cells: [{ status: 'degraded', ttft_p50_ms: 15000 }] })
it('uses V3 status and configured availability range, not a slot P50 as a whole-route statistic', async () => {
  vi.mocked(getPublicSettings).mockResolvedValue({ channel_monitor_enabled: true, channel_monitor_mode: 'v3' } as never)
  vi.mocked(apiClient.get).mockResolvedValue({ data: { generated_at: coverage.computed_at, availability_range: '30d', featured: v3Component(), categories: [{ components: [v3Component(), v3Component(11, 99)] }] } })
  const result = await getLineSelectionMetrics(undefined, [1], 'codex')
  expect(apiClient.get).toHaveBeenCalledWith('/channel-monitor-v3/status', expect.objectContaining({ params: { tool_id: 'codex' } }))
  expect(result.groups).toHaveLength(1)
  expect(result.groups[0]).toMatchObject({ id: 1, status_label: '性能下降', availability_percent: 97.46, availability_label: '可用性 · 近30天', ttft_p50_ms: null })
})

it('does not guess group mappings from component ids/names or average multiple monitor records', async () => {
  vi.mocked(getPublicSettings).mockResolvedValue({ channel_monitor_enabled: true, channel_monitor_mode: 'v3' } as never)
  vi.mocked(apiClient.get).mockResolvedValue({ data: { generated_at: coverage.computed_at, availability_range: '7d', featured: null, categories: [{ components: [{ ...v3Component(1), group_id: undefined }, v3Component(2, 2), v3Component(3, 2)] }] } })
  const result = await getLineSelectionMetrics(undefined, [1, 2])
  expect(result.groups.find(g => g.id === 1)).toBeUndefined()
  expect(result.groups.find(g => g.id === 2)).toMatchObject({ has_samples: false, status_label: '多项监控，查看详情', availability_percent: null })
})

it('returns monitoring disabled without calling a version API', async () => {
  vi.mocked(getPublicSettings).mockResolvedValue({ channel_monitor_enabled: false, channel_monitor_mode: 'v3' } as never)
  const result = await getLineSelectionMetrics(undefined, [1])
  expect(result.groups[0]).toMatchObject({ id: 1, has_samples: false, status_label: '监控未开启' })
  expect(apiClient.get).not.toHaveBeenCalled()
})

it('rechecks configuration only once when the backend switches mode between settings and data', async () => {
  vi.mocked(getPublicSettings).mockResolvedValueOnce({ channel_monitor_enabled: true, channel_monitor_mode: 'v2' } as never).mockResolvedValueOnce({ channel_monitor_enabled: true, channel_monitor_mode: 'v3' } as never)
  vi.mocked(apiClient.get).mockRejectedValueOnce({ status: 403, code: 'CHANNEL_MONITOR_MODE_MISMATCH' }).mockResolvedValueOnce({ data: { generated_at: coverage.computed_at, availability_range: '24h', categories: [], featured: v3Component() } })
  expect((await getLineSelectionMetrics(undefined, [1])).groups[0].availability_label).toBe('可用性 · 近24小时')
  expect(getPublicSettings).toHaveBeenCalledTimes(2)
  expect(vi.mocked(apiClient.get).mock.calls.map(call => call[0])).toEqual(['/channel-monitor-v2/matrix', '/channel-monitor-v3/status'])
})

it('stops after cancellation while reading settings and never calls an old mode API', async () => {
  const c = new AbortController()
  vi.mocked(getPublicSettings).mockImplementationOnce(async () => { c.abort(); return { channel_monitor_enabled: true, channel_monitor_mode: 'v2' } as never })
  await expect(getLineSelectionMetrics(c.signal, [1])).rejects.toThrow()
  expect(apiClient.get).not.toHaveBeenCalled()
})
it('never sends an unscoped request for no visible groups', async () => {
  expect((await getLineSelectionMetrics(undefined, [])).groups).toEqual([])
  expect(apiClient.get).not.toHaveBeenCalled()
})
it('keeps zero success and zero TTFT, but hides metrics for no samples', async () => {
  const zero = row(); zero.metrics.error_rate = 1; zero.metrics.ttft.p50_ms = 0
  const empty = row(2); empty.metrics.has_samples = false
  vi.mocked(apiClient.get).mockResolvedValue({ data: { coverage, items: [zero, empty] } })
  const result = await getLineSelectionMetrics(undefined, [1, 2])
  expect(result.groups[0]).toMatchObject({ has_samples: true, error_rate: 1, ttft_p50_ms: 0 })
  expect(result.groups[1]).toMatchObject({ has_samples: false, ttft_p50_ms: null })
})
it('does not turn failures, duplicate groups or invalid metric values into healthy statistics', async () => {
  vi.mocked(apiClient.get).mockRejectedValueOnce(new Error('offline'))
  await expect(getLineSelectionMetrics(undefined, [1])).rejects.toThrow('offline')
  for (const data of [{ coverage, items: [row(), row()] }, { coverage, items: [{ ...row(), metrics: { ...row().metrics, error_rate: NaN } }] }, { coverage: {}, items: [row()] }]) {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data })
    await expect(getLineSelectionMetrics(undefined, [1])).rejects.toThrow()
  }
})

it('does not retry unrelated failures or loop when mode keeps changing', async () => {
  vi.mocked(apiClient.get).mockRejectedValueOnce({status:500,code:'INTERNAL_ERROR'})
  await expect(getLineSelectionMetrics(undefined,[1])).rejects.toMatchObject({status:500})
  expect(getPublicSettings).toHaveBeenCalledTimes(1)
  vi.mocked(getPublicSettings).mockClear().mockResolvedValueOnce({channel_monitor_enabled:true,channel_monitor_mode:'v2'} as never).mockResolvedValueOnce({channel_monitor_enabled:true,channel_monitor_mode:'v3'} as never)
  vi.mocked(apiClient.get).mockReset().mockRejectedValue({status:403,code:'CHANNEL_MONITOR_MODE_MISMATCH'})
  await expect(getLineSelectionMetrics(undefined,[1])).rejects.toMatchObject({code:'CHANNEL_MONITOR_MODE_MISMATCH'})
  expect(getPublicSettings).toHaveBeenCalledTimes(2)
  expect(apiClient.get).toHaveBeenCalledTimes(2)
})
it('keeps unknown V3 status independent of historical zero availability and hides unavailable rates',async()=>{
  vi.mocked(getPublicSettings).mockResolvedValue({channel_monitor_enabled:true,channel_monitor_mode:'v3'} as never)
  vi.mocked(apiClient.get).mockResolvedValue({data:{generated_at:coverage.computed_at,availability_range:'7d',categories:[{components:[{...v3Component(),status:'unknown',availability:0},{...v3Component(11,2),status:'unknown',availability:null}]}]}})
  const result=await getLineSelectionMetrics(undefined,[1,2])
  expect(result.groups[0]).toMatchObject({has_samples:true,health:'unknown',status_label:'暂无近期数据',availability_percent:0})
  expect(result.groups[1]).toMatchObject({has_samples:false,availability_percent:null})
})
