import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import LineSelect from '../LineSelect.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const group = { id: 1, name: 'GPT Plus', platform: 'openai', status: 'active', rate_multiplier: 1.2 } as Group
const metric = { success_rate: 97, ttft_p50_ms: 2160 } as MonitorV4Group
const wrappers: Array<ReturnType<typeof mount>> = []
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = '' })

describe('shared line selector', () => {
  it('displays the effective rate and the same metrics in its searchable options', async () => {
    const wrapper = mount(LineSelect, { attachTo: document.body, props: { modelValue: null, groups: [group], rates: { 1: 0.8 }, metrics: new Map([[1, metric]]), linkedCounts: new Map([[1, 2]]) } })
    wrappers.push(wrapper)
    await wrapper.get('[aria-label="选择线路"]').trigger('click')
    await nextTick()
    expect(document.body.textContent).toContain('0.8倍率')
    expect(document.body.textContent).toContain('管理正常')
    expect(document.body.textContent).toContain('关联密钥 2 把')
    expect(document.body.textContent).toContain('97%')
    expect(document.body.textContent).toContain('2.16s')
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
