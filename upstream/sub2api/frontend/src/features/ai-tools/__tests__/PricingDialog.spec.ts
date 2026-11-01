import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PricingDialog from '../PricingDialog.vue'
import { createI18n } from 'vue-i18n'
import type { Group } from '@/types'

const mocks = vi.hoisted(() => ({ groups: vi.fn(), rates: vi.fn(), models: vi.fn(), popularity: vi.fn() }))
vi.mock('@/api/groups', () => ({ default: { getAvailable: mocks.groups, getUserGroupRates: mocks.rates } }))
vi.mock('../api', () => ({ getGroupModels: mocks.models, getModelPopularity: mocks.popularity }))
const group = (id: number, name: string, rate: number, platform = 'openai') => ({ id, name, rate_multiplier: rate, platform, status: 'active' }) as Group
const pending = <T,>() => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}
const make = () => mount(PricingDialog, {
  props: { tool: { id: 'codex', label: 'Codex' }, lines: [group(1, '旧分组', 9)], rates: { 1: 8 } },
  global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: {} })], stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot/></div>' } } },
})
beforeEach(() => {
  vi.clearAllMocks()
  mocks.popularity.mockResolvedValue({ models: [], start_time: '', end_time: '' })
  mocks.groups.mockResolvedValue([group(1, '最新分组', .2), group(2, '新分组', .3), group(3, 'Claude', 1, 'anthropic')])
  mocks.rates.mockResolvedValue({ 1: .12 })
  mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-5.4'] }, { group_id: 2, supported_models: ['gpt-image-2'] }, { group_id: 3, supported_models: ['claude-private'] }])
})
enableAutoUnmount(afterEach)
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })
describe('原生扣费标准同步', () => {
  it('keeps category filter alongside the heading and moves units above the table without explanatory copy', async () => {
    const w = make(); await flushPromises()
    expect(w.get('.official-pricing-heading').find('button[aria-label="模型类型"]').exists()).toBe(true)
    expect(w.get('.official-pricing-heading').text()).not.toContain('100 万 Token')
    expect(w.get('#official-pricing-panel').get('.pricing-tier-label').text()).toBe('Standard · 美元 / 100 万 Token')
    expect(w.text()).not.toContain('按发布时间从新到旧')
    expect(w.text()).not.toContain('参考价与模型广场')
  })
  it('uses site popularity, preserves it when refreshing stats fails, and updates it on recovery', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-6.1-sol', 'gpt-5.4', 'gpt-image-2'] }])
    mocks.popularity.mockResolvedValueOnce({ models: [{ model: 'gpt-5.4', requests: 20 }], start_time: '', end_time: '' })
    const w = make(); await flushPromises()
    const names = () => w.findAll('.model-name').map(e => e.text())
    expect(names()).toEqual(['gpt-5.4', 'gpt-6.1-sol', 'gpt-image-2'])
    mocks.popularity.mockRejectedValueOnce(new Error('offline'))
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    expect(names()[0]).toBe('gpt-5.4')
    expect(w.find('.fee-table').exists()).toBe(true)
    mocks.popularity.mockResolvedValueOnce({ models: [{ model: 'gpt-image-2', requests: 30 }], start_time: '', end_time: '' })
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    expect(names()[0]).toBe('gpt-image-2')
  })
  it('falls back to release order on first stats failure and keeps popularity order within a category', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-6.1-sol', 'gpt-5.4', 'gpt-image-2'] }])
    mocks.popularity.mockRejectedValueOnce(new Error('offline'))
    const w = make(); await flushPromises()
    expect(w.findAll('.model-name').map(e => e.text())).toEqual(['gpt-6.1-sol', 'gpt-image-2', 'gpt-5.4'])
    mocks.popularity.mockResolvedValueOnce({ models: [{ model: 'gpt-5.4', requests: 20 }], start_time: '', end_time: '' })
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    await w.get('button[aria-label="模型类型"]').trigger('click'); await flushPromises()
    ;[...document.querySelectorAll<HTMLElement>('[role="option"]')].filter(e => e.textContent?.includes('旗舰模型')).at(-1)!.click()
    await flushPromises()
    expect(w.findAll('.model-name').map(e => e.text())).toEqual(['gpt-5.4', 'gpt-6.1-sol'])
  })
  it('does not let slow or unavailable popularity block native prices and discards stale tool results', async () => {
    const stats = pending<{ models: { model: string; requests: number }[] }>()
    mocks.popularity.mockReturnValueOnce(stats.promise)
    const w = make(); await flushPromises()
    expect(w.find('.model-pricing-table').exists()).toBe(true)
    const signal = mocks.popularity.mock.calls[0][0] as AbortSignal
    await w.setProps({ tool: { id: 'claude', label: 'Claude' }, lines: [] }); await flushPromises()
    expect(signal.aborted).toBe(true)
    stats.resolve({ models: [{ model: 'gpt-5.4', requests: 20 }] }); await flushPromises()
    expect(w.get('.model-pricing-table').text()).toContain('claude-private')
    expect(w.get('.model-pricing-table').text()).not.toContain('gpt-5.4')
  })

  it('shows one cache-write price when native duration fields are equivalent or absent', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-6.1-sol', 'native-flat'], official_pricing: {
      'gpt-6.1-sol': { input_price: 2e-6, output_price: 10e-6, cache_read_price: .1e-6, cache_write_price: 2.5e-6, cache_write_1h_price: 2.5e-6 },
      'native-flat': { input_price: 1e-6, output_price: 5e-6, cache_read_price: null, cache_write_price: 1.25e-6 },
    } }])
    const w = make(); await flushPromises()
    const table = w.get('.model-pricing-table')
    expect(table.text()).toContain('$2.50')
    expect(table.text()).toContain('$1.25')
    expect(table.text()).not.toContain('5分钟')
    expect(table.text()).not.toContain('1小时')
  })
  it('compares short and long context prices under range headers in one model row', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-5.4'], official_pricing: {
      'gpt-5.4': { input_price: 1e-6, output_price: 2e-6, cache_read_price: null, cache_write_price: null, intervals: [
        { min_tokens: 0, max_tokens: 272000, tier_label: '≤272K', input_price: 1e-6, output_price: 2e-6, cache_read_price: 0, cache_write_price: 3e-6, cache_write_1h_price: 4e-6 },
        { min_tokens: 272000, max_tokens: null, tier_label: '>272K', input_price: 5e-6, output_price: 6e-6, cache_read_price: .5e-6, cache_write_price: 7e-6, cache_write_1h_price: 8e-6 },
      ] },
    } }])
    const w = make(); await flushPromises()
    expect(w.findAll('.model-pricing-table thead tr')).toHaveLength(2)
    const headers = w.findAll('.model-pricing-table thead [scope="colgroup"]')
    expect(headers.map(header => header.text())).toEqual(['短上下文≤272K', '长上下文>272K'])
    expect(headers.map(header => header.attributes('colspan'))).toEqual(['4', '4'])
    expect(w.get('.model-pricing-table thead th').attributes('rowspan')).toBe('2')
    const row = w.get('.model-pricing-table tbody tr')
    expect(row.findAll('.model-name')).toHaveLength(1)
    expect(row.findAll('td').map(cell => cell.text())).toEqual(['$1.00', '$0.00', '5分钟 $3.001小时 $4.00', '$2.00', '$5.00', '$0.50', '5分钟 $7.001小时 $8.00', '$6.00'])
    expect(row.findAll('td')[4].classes()).toContain('context-divider')
  })
  it('does not label different native thresholds as a universal 272K range', async () => {
    const priced = (threshold: number) => ({ input_price: 1e-6, output_price: 2e-6, cache_read_price: null, cache_write_price: null, intervals: [
      { min_tokens: 0, max_tokens: threshold, input_price: 1e-6 },
      { min_tokens: threshold, max_tokens: null, input_price: 3e-6 },
    ] })
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['model-a', 'model-b'], official_pricing: {
      'model-a': priced(272000), 'model-b': priced(200000),
    } }])
    const w = make(); await flushPromises()
    expect(w.findAll('.model-pricing-table thead [scope="colgroup"]').map(header => header.text())).toEqual(['短上下文', '长上下文'])
    const rows = w.findAll('.model-pricing-table tbody tr')
    expect(rows[0].text()).toContain('≤272K')
    expect(rows[0].text()).toContain('>272K')
    expect(rows[1].text()).toContain('≤200K')
    expect(rows[1].text()).toContain('>200K')
  })
  it('distinguishes all-context prices from a missing context tier', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['flat', 'tiered'], official_pricing: {
      flat: { input_price: 9e-6, output_price: 10e-6, cache_read_price: null, cache_write_price: null },
      tiered: { input_price: 1e-6, output_price: 2e-6, cache_read_price: null, cache_write_price: null, intervals: [
        { min_tokens: 0, max_tokens: 272000, tier_label: '≤272K', input_price: 1e-6 },
        { min_tokens: 272000, max_tokens: null, tier_label: '>272K', input_price: 3e-6 },
      ] },
    } }])
    const w = make(); await flushPromises()
    const flat = w.findAll('.model-pricing-table tbody tr').find(row => row.get('.model-name').text() === 'flat')!
    expect(flat.text()).toContain('全部上下文')
    expect(flat.text()).toContain('适用全部上下文')
    expect(flat.findAll('td').at(-1)!.attributes('colspan')).toBe('4')
    expect(flat.text()).not.toContain('无此档位')
  })
  it('orders newer models first and filters official types without changing group fees', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-5.2', 'gpt-image-2', 'gpt-5.4', 'gpt-4o-transcribe', 'gpt-realtime', 'gpt-5.6-cyber', 'gpt-6.1-sol'] }])
    const w = make(); await flushPromises()
    expect(w.findAll('.model-pricing-table tbody th')[0].text()).toContain('gpt-6.1-sol')
    const fees = w.get('.fee-table').text()
    await w.get('button[aria-label="模型类型"]').trigger('click'); await flushPromises()
        const candidates = [...document.querySelectorAll<HTMLElement>('[role="option"]')]
    ;candidates.filter(e => e.textContent?.includes('图像生成模型')).at(-1)!.click()
    await flushPromises()
    expect(w.findAll('.model-pricing-table tbody tr')).toHaveLength(1)
    expect(w.get('.model-pricing-table').text()).toContain('gpt-image-2')
    expect(w.get('.fee-table').text()).toBe(fees)
  })

  it('uses model plaza same-source prices rather than static model constants', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['gpt-5.4', 'gpt-5.4-2026-03-05'], official_pricing: {
      'gpt-5.4': { input_price: 7.34e-6, cache_read_price: 0, cache_write_price: null, output_price: 41e-6 },
      'gpt-5.4-2026-03-05': { input_price: 7.34e-6, cache_read_price: 0, cache_write_price: null, output_price: 41e-6 },
    } }])
    const w = make(); await flushPromises()
    const table = w.get('.model-pricing-table')
    expect(table.text()).toContain('$7.34')
    expect(table.text()).toContain('$41.00')
    expect(table.text()).toContain('$0.00')
    expect(table.text()).not.toContain('待核对')
    expect(table.text()).not.toContain('$2.50')
    expect(w.find('[role="tablist"]').exists()).toBe(false)
    expect(mocks.models.mock.calls[0][2]).toBe(true)
  })
  it('shows every native context interval including a third tier', async () => {
    mocks.models.mockResolvedValue([{ group_id: 1, supported_models: ['native-model'], official_pricing: {
      'native-model': { input_price: 1e-6, output_price: 2e-6, cache_read_price: null, cache_write_price: null, intervals: [
        { min_tokens: 0, max_tokens: 100000, tier_label: '≤100K', input_price: 1e-6, output_price: 2e-6, cache_read_price: .1e-6, cache_write_price: null },
        { min_tokens: 100000, max_tokens: 200000, tier_label: '100K–200K', input_price: 3e-6, output_price: 4e-6, cache_read_price: .3e-6, cache_write_price: null },
        { min_tokens: 200000, max_tokens: null, tier_label: '>200K', input_price: 5e-6, output_price: 6e-6, cache_read_price: .5e-6, cache_write_price: 7e-6, cache_write_1h_price: 8e-6 },
      ] },
    } }])
    const w = make(); await flushPromises()
    expect(w.findAll('.model-pricing-table tbody tr')).toHaveLength(1)
    expect(w.findAll('.model-pricing-table tbody th')).toHaveLength(1)
    expect(w.get('.model-pricing-table').text()).toContain('100K–200K')
    expect(w.get('.model-pricing-table').text()).toContain('$5.00')
    expect(w.get('.model-pricing-table').text()).toContain('$8.00')
  })
  it('uses fresh native groups and rates instead of dashboard props', async () => {
    const w = make(); await flushPromises()
    expect(mocks.groups).toHaveBeenCalledOnce()
    expect(mocks.rates).toHaveBeenCalledOnce()
    expect(w.get('.fee-table').text()).toContain('最新分组')
    expect(w.get('.fee-table').text()).toContain('新分组')
    expect(w.get('.fee-table').text()).not.toContain('旧分组')
    expect(w.get('.fee-table').text()).not.toContain('Claude')
    const rows = w.findAll('.fee-table tbody tr')
    expect(rows[0].findAll('td').map(cell => cell.text())).toEqual(['gpt-5.4', '0.12x倍率', '官方计费金额 × 0.12'])
    expect(rows[1].findAll('td').map(cell => cell.text())).toEqual(['gpt-image-2', '0.3x倍率', '官方计费金额 × 0.3'])
    expect(w.get('.model-pricing-table').text()).not.toContain('claude-private')
    w.unmount()
  })
  it('refreshes all fields together and retains a zero user multiplier', async () => {
    const w = make(); await flushPromises()
    mocks.groups.mockResolvedValue([group(2, '改名分组', .8)])
    mocks.rates.mockResolvedValue({ 2: 0 })
    mocks.models.mockResolvedValue([{ group_id: 2, supported_models: ['new-model'] }])
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    expect(w.findAll('.fee-table tbody tr')).toHaveLength(1)
    expect(w.get('.fee-table').text()).toContain('改名分组')
    expect(w.get('.fee-table').text()).toContain('new-model')
    expect(w.get('.fee-table tbody tr').findAll('td')[1].text()).toBe('0.0x倍率')
    expect(w.get('.fee-table').text()).toContain('官方计费金额 × 0')
    expect(mocks.groups).toHaveBeenCalledTimes(2)
    expect(mocks.rates).toHaveBeenCalledTimes(2)
    expect(mocks.models).toHaveBeenCalledTimes(2)
    w.unmount()
  })
  it.each(['groups', 'rates', 'models'] as const)('does not display stale fees after the %s request fails', async endpoint => {
    const w = make(); await flushPromises()
    mocks[endpoint].mockRejectedValueOnce(new Error('offline'))
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    expect(w.find('.fee-table').exists()).toBe(false)
    expect(w.text()).toContain('读取失败')
    await w.findAll('button').find(button => button.text() === '重试')!.trigger('click'); await flushPromises()
    expect(w.get('.fee-table').text()).toContain('官方计费金额 × 0.12')
    w.unmount()
  })
  it('keeps failed cached fees hidden while a retry is pending', async () => {
    const w = make(); await flushPromises()
    mocks.rates.mockRejectedValueOnce(new Error('offline'))
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    const retry = pending<Group[]>()
    mocks.groups.mockReturnValueOnce(retry.promise)
    await w.findAll('button').find(button => button.text() === '重试')!.trigger('click'); await flushPromises()
    expect(w.find('.fee-table').exists()).toBe(false)
    expect(w.find('.model-pricing-table').exists()).toBe(false)
    retry.resolve([group(1, '重新读取', .4)]); await flushPromises()
    expect(w.get('.fee-table').text()).toContain('重新读取')
  })
  it('shows empty native groups even when the dashboard has an old group', async () => {
    mocks.groups.mockResolvedValue([])
    const w = make(); await flushPromises()
    expect(w.find('.fee-table').exists()).toBe(false)
    expect(w.text()).toContain('暂无分组')
    w.unmount()
  })
  it('keeps the current cross-platform tool association using fresh native values', async () => {
    mocks.groups.mockResolvedValue([group(4, '混合新名称', .4, 'composite')])
    const w = make()
    await w.setProps({ lines: [group(4, '混合旧名称', 9, 'composite')] }); await flushPromises()
    expect(w.get('.fee-table').text()).toContain('混合新名称')
    expect(w.get('.fee-table').text()).toContain('官方计费金额 × 0.4')
    w.unmount()
  })
  it('honors explicit native tool mappings and discovers newly mapped composite groups', async () => {
    mocks.groups.mockResolvedValue([
      { ...group(1, '仅 Claude 的 OpenAI 分组', .2), tool_ids: ['claude'] },
      { ...group(4, '新增混合分组', .4, 'composite'), tool_ids: ['codex'] },
    ])
    mocks.models.mockResolvedValue([{ group_id: 4, supported_models: ['native-alias'] }])
    const w = make(); await flushPromises()
    expect(w.get('.fee-table').text()).not.toContain('仅 Claude')
    expect(w.get('.fee-table').text()).toContain('新增混合分组')
    expect(mocks.models.mock.calls[0][1]).toEqual([4])
    mocks.groups.mockResolvedValue([{ ...group(4, '新增混合分组', .4, 'composite'), tool_ids: ['claude'] }])
    await w.get('button[aria-label="刷新价格与扣费标准"]').trigger('click'); await flushPromises()
    expect(w.find('.fee-table').exists()).toBe(false)
    expect(w.text()).toContain('暂无分组')
    w.unmount()
  })
  it('syncs silently while open and aborts outstanding work on close', async () => {
    vi.useFakeTimers()
    const w = make(); await flushPromises()
    mocks.rates.mockResolvedValue({ 1: .45 })
    await vi.advanceTimersByTimeAsync(60_000); await flushPromises()
    expect(mocks.groups).toHaveBeenCalledTimes(2)
    expect(w.get('.fee-table').text()).toContain('官方计费金额 × 0.45')
    const request = pending<Group[]>()
    mocks.groups.mockReturnValue(request.promise)
    const table = w.get('.model-pricing-table').element
    const fees = w.get('.fee-table').element
    await vi.advanceTimersByTimeAsync(60_000)
    expect(w.get('.model-pricing-table').element).toBe(table)
    expect(w.get('.fee-table').element).toBe(fees)
    const signal = mocks.groups.mock.calls.at(-1)![0] as AbortSignal
    await w.setProps({ tool: null })
    expect(signal.aborted).toBe(true)
    request.resolve([group(9, '晚到分组', 1)]); await flushPromises()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(mocks.groups).toHaveBeenCalledTimes(3)
    await w.setProps({ tool: { id: 'codex', label: 'Codex' } }); await flushPromises()
    expect(mocks.groups).toHaveBeenCalledTimes(4)
    w.unmount()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(mocks.groups).toHaveBeenCalledTimes(4)
  })
  it('does not accept an older tool response after switching tools', async () => {
    const request = pending<Group[]>()
    mocks.groups.mockReturnValueOnce(request.promise)
    const w = make()
    await w.setProps({ tool: { id: 'claude', label: 'Claude Code' }, lines: [] }); await flushPromises()
    request.resolve([group(1, '晚到分组', 9)]); await flushPromises()
    expect(w.get('.fee-table').text()).toContain('Claude')
    expect(w.get('.fee-table').text()).not.toContain('晚到分组')
    expect(w.get('.fee-table').text()).not.toContain('最新分组')
    w.unmount()
  })
  it('does not refetch on a brief tab switch but syncs after stale data', async () => {
    vi.useFakeTimers()
    const w = make(); await flushPromises()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(10_000)
    expect(mocks.groups).toHaveBeenCalledOnce()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(mocks.groups).toHaveBeenCalledOnce()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60_000)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(mocks.groups).toHaveBeenCalledTimes(2)
    w.unmount()
    vi.restoreAllMocks()
  })
})

it('shows Gemini official source and native Gemini group models without unrelated fees', async () => {
  mocks.groups.mockResolvedValue([group(5,'Gemini 测试分组',.5,'gemini'),group(1,'Codex 分组',1)])
  mocks.models.mockResolvedValue([{group_id:5,supported_models:['gemini-test-model']}])
  const w=make();await w.setProps({tool:{id:'gemini',label:'Gemini'},lines:[]});await flushPromises()
  expect(w.get('.pricing-source a').attributes('href')).toBe('https://ai.google.dev/gemini-api/docs/pricing')
  expect(w.get('.fee-table').text()).toContain('Gemini 测试分组')
  expect(w.get('.fee-table').text()).not.toContain('Codex 分组')
  expect(w.text()).toContain('gemini-test-model')
})
