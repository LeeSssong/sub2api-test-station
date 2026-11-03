import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import LineSelect from '../LineSelect.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const group = { id: 1, name: 'GPT Plus', platform: 'openai', status: 'active', rate_multiplier: 1.2 } as Group
const metric = { sla_request_count:100,sla_success_count:97,success_rate: 97, ttft_p50_ms: 2160 } as MonitorV4Group
const wrappers: Array<ReturnType<typeof mount>> = []
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = ''; vi.useRealTimers() })

describe('shared line selector', () => {
  it('uses the time of a newly received snapshot before the next clock tick', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-05T00:00:00Z'))
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: {}, metrics: new Map(), linkedCounts: new Map() } })
    wrappers.push(wrapper)
    vi.setSystemTime(new Date('2026-10-05T00:00:01Z'))
    await wrapper.setProps({ metrics: new Map([[1, metric]]), metricsGeneratedAt: new Date().toISOString() })
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    expect(document.querySelector('.line-status')?.textContent).toContain('正常运行')
  })
  it('expires displayed health without requiring a prop update', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-05T00:00:00Z'))
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: {}, metrics: new Map([[1, metric]]), metricsGeneratedAt: new Date().toISOString(), linkedCounts: new Map() } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    expect(document.querySelector('.line-status')?.textContent).toContain('正常运行')
    vi.advanceTimersByTime(8 * 60 * 1000)
    await nextTick()
    expect(document.querySelector('.line-status')?.textContent).toContain('统计已过期')
  })
  it('shows failed statistics only inside the dropdown without stale values or a retry button', async () => {
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: {}, metrics: new Map([[1, metric]]), linkedCounts: new Map(), metricsError: true } })
    wrappers.push(wrapper)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('.line-metrics-error').exists()).toBe(false)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    expect(document.querySelector('.line-status')?.textContent).toContain('统计读取失败')
    expect(document.querySelector('.line-option-meta')?.textContent).toContain('可用性 —')
    expect(document.querySelector('.line-option-meta')?.textContent).toContain('首字 —')
    expect(document.querySelector('.xq-line-select-dropdown')?.textContent).not.toContain('重试')
  })

  it('uses only a matching eligible tool recommendation, regardless of selection or rate', async () => {
    const expensive = { ...group, id: 2, name: '推荐线路', rate_multiplier: 2 }
    const cheaper = { ...group, name: '便宜线路', rate_multiplier: 0.1 }
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: 1, groups: [cheaper, expensive], rates: {}, metrics: new Map(), recommendedGroupIds: new Map([['codex', 2]]) } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    const options = [...document.querySelectorAll('.line-option')]
    expect(options[0]?.querySelector('.line-recommended')).toBeNull()
    expect(options[1]?.querySelector('.line-recommended')?.textContent).toBe('推荐最佳线路')
    await wrapper.setProps({ modelValue: 2 })
    expect(document.querySelectorAll('.line-recommended')).toHaveLength(1)
    expect(options[1]?.querySelector('.line-recommended')?.textContent).toBe('推荐最佳线路')
    await wrapper.setProps({ recommendedGroupIds: new Map([['gemini', 2]]) })
    expect(document.querySelector('.line-recommended')).toBeNull()
    await wrapper.setProps({ recommendedGroupIds: new Map([['codex', 2]]), groups: [cheaper, { ...expensive, status: 'inactive' }] })
    expect(document.querySelector('.line-recommended')).toBeNull()
  })
  it('shows only the current tool recommendation for a cross-tool legacy line', async () => {
    const shared = { ...group, id: 2, name: '跨工具线路', tool_ids: ['codex', 'gemini'] }
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, toolId: 'codex', groups: [group, shared], rates: {}, metrics: new Map(), recommendedGroupIds: new Map([['codex', 1], ['gemini', 2]]) } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    const options = [...document.querySelectorAll('.line-option')]
    expect(options).toHaveLength(2)
    expect(options[0]?.querySelector('.line-recommended')?.textContent).toBe('推荐最佳线路')
    expect(options[1]?.querySelector('.line-recommended')).toBeNull()
  })

  it('displays the effective rate and the same metrics in its searchable options', async () => {
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: { 1: 0.8 }, metrics: new Map([[1, metric]]), linkedCounts: new Map([[1, 2]]) } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('0.8x倍率')
    expect(document.body.textContent).toContain('正常运行')
    expect(document.body.textContent).not.toContain('关联密钥')
    expect(document.body.textContent).toContain('97%')
    expect(document.body.textContent).toContain('2.16s')
    expect(document.querySelector('.xq-line-select-dropdown')).not.toBeNull()
    expect(document.querySelector('.line-status')?.textContent).toContain('正常运行')
    expect(document.querySelector('.line-rate')?.textContent).toContain('0.8x倍率')
    const option = document.querySelector('[role="option"]') as HTMLElement
    option.click()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([1])
  })

  it('filters the options and keeps the popup inside a narrow viewport', async () => {
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: {}, metrics: new Map(), linkedCounts: new Map() } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    await nextTick()
    const popup = document.querySelector('[role="listbox"]') as HTMLElement
    expect(popup.getAttribute('style')).toContain('max-width')
    expect(popup.classList.contains('xq-line-select-dropdown')).toBe(true)
    const input = popup.querySelector('input') as HTMLInputElement
    input.value = 'missing'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(popup.querySelector('[role="option"]')).toBeNull()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(wrapper.get('[aria-label="选择线路"]').attributes('aria-expanded')).toBe('false')
  })
})

it('renders full description and metric hover text including no samples', async () => {
 const wrapper=mount(LineSelect,{attachTo:document.body,props:{modelValue:null,groups:[{...group,description:'完整描述，不截断'}],rates:{},metrics:new Map(),linkedCounts:null}})
 wrappers.push(wrapper)
 await wrapper.get('[aria-label="选择线路"]').trigger('click')
 expect(document.querySelector('.line-description')?.textContent).toBe('完整描述，不截断')
 expect(document.querySelector('[title="近24小时可用性"]')?.textContent).toContain('可用性 —')
 expect(document.querySelector('[title="近24小时首字响应中位数（P50）"]')?.textContent).toContain('首字 —')
 expect(document.body.textContent).not.toContain('关联密钥')
})
