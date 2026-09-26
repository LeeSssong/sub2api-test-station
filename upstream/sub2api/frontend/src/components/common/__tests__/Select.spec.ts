import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const originalInnerWidth = window.innerWidth
let unmountWrapper: (() => void) | undefined

const setViewportWidth = (width: number) => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

const mockTriggerRect = (left: number, width: number) => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: left,
    y: 20,
    top: 20,
    right: left + width,
    bottom: 60,
    left,
    width,
    height: 40,
    toJSON: () => ({}),
  })
}

const openSelect = async (attachTo?: HTMLElement) => {
  const wrapper = mount(Select, {
    attachTo,
    props: {
      modelValue: null,
      options: [
        {
          value: 'example',
          label: 'very-long-unbroken-option-value-that-must-not-overflow',
        },
      ],
    },
  })
  unmountWrapper = () => wrapper.unmount()

  await wrapper.get('button').trigger('click')
  await nextTick()

  return document.body.querySelector<HTMLElement>('.select-dropdown-portal')
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  setViewportWidth(originalInnerWidth)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Select dropdown viewport constraints', () => {
  it('supplies brand colors when a teleported trigger has no theme variables', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: null, options: [{ value: 1, label: 'GPT Plus' }], brand: true, searchable: true },
    })
    unmountWrapper = () => wrapper.unmount()

    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-brand')
    expect(dropdown?.style.getPropertyValue('--xq-depth')).toBe('#091a2b')
    expect(dropdown?.style.getPropertyValue('--xq-raised')).toBe('#10283d')
    expect(dropdown?.style.getPropertyValue('--xq-text')).toBe('#f1f9f9')
    expect(dropdown?.style.getPropertyValue('--xq-border')).toBe('#1b4055')
    expect(dropdown?.style.getPropertyValue('--xq-muted')).toBe('#708c9e')
  })

  it('carries brand colors into a dropdown teleported outside the themed shell', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: null, options: [{ value: 1, label: 'GPT Plus' }], brand: true, searchable: true },
    })
    unmountWrapper = () => wrapper.unmount()
    const trigger = wrapper.get('button').element as HTMLElement
    trigger.style.setProperty('--xq-depth', '#091a2b')
    trigger.style.setProperty('--xq-text', '#f1f9f9')
    trigger.style.setProperty('--xq-raised', '#10283d')

    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-brand')
    expect(dropdown?.style.getPropertyValue('--xq-depth')).toBe('#091a2b')
    expect(dropdown?.style.getPropertyValue('--xq-text')).toBe('#f1f9f9')
    expect(dropdown?.style.getPropertyValue('--xq-raised')).toBe('#10283d')
  })

  it('preserves the existing 200px minimum width when space is available', async () => {
    setViewportWidth(1024)
    mockTriggerRect(20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('996px')
  })

  it('opens leftward near the right viewport edge', async () => {
    setViewportWidth(320)
    mockTriggerRect(220, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('112px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })

  it('clamps a trigger left of the viewport to the safe padding', async () => {
    setViewportWidth(320)
    mockTriggerRect(-20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('8px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('304px')
  })

  it('clamps an offscreen-right trigger position to the viewport boundary', async () => {
    setViewportWidth(320)
    mockTriggerRect(400, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('112px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })

  it('opens to the left at the right edge without narrowing a readable menu', async () => {
    setViewportWidth(390)
    mockTriggerRect(280, 96)

    const dropdown = await openSelect()

    expect(dropdown?.style.left).toBe('182px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })

  it('keeps a right-aligned menu within its card rather than the wider viewport', async () => {
    setViewportWidth(1024)
    const panel = document.createElement('div')
    panel.className = 'card'
    document.body.append(panel)
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue({ left: 100, right: 400 } as DOMRect)
    mockTriggerRect(330, 64)

    const dropdown = await openSelect(panel)

    expect(dropdown?.style.left).toBe('192px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('200px')
  })
})

describe('Select keyboard navigation', () => {
  it('moves focus into a non-searchable menu and selects with arrows and Enter', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: 'active', searchable: false, options: [{ value: 'active', label: 'Active' }, { value: 'inactive', label: 'Inactive' }] },
    })
    unmountWrapper = () => wrapper.unmount()
    const trigger = wrapper.get('button')
    trigger.element.focus()
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const dropdown = document.querySelector<HTMLElement>('[role="listbox"]')!
    expect(document.activeElement).toBe(dropdown)
    dropdown.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    await nextTick()
    dropdown.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([['inactive']])
    expect(document.activeElement).toBe(trigger.element)
  })

  it('closes only the open dropdown on Escape and restores trigger focus', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: null, options: [{ value: 1, label: 'Example' }], searchable: true },
    })
    unmountWrapper = () => wrapper.unmount()
    const outerEscape = vi.fn()
    document.addEventListener('keydown', outerEscape)
    try {
      await wrapper.get('button').trigger('click')
      await nextTick()
      const search = document.querySelector<HTMLInputElement>('.select-search-input')!
      search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await nextTick()
      expect(outerEscape).not.toHaveBeenCalled()
      expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
      expect(document.activeElement).toBe(wrapper.get('button').element)
    } finally {
      document.removeEventListener('keydown', outerEscape)
    }
  })
})

describe('Select remote search', () => {
  const mountRemoteSelect = (props: Record<string, unknown> = {}) => {
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        remote: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
        ...props,
      },
    })
    unmountWrapper = () => wrapper.unmount()
    return wrapper
  }

  const openDropdown = async () => {
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
    expect(dropdown).not.toBeNull()
    return dropdown as HTMLElement
  }

  const typeSearchQuery = async (query: string) => {
    const dropdown = await openDropdown()
    const input = dropdown.querySelector<HTMLInputElement>('.select-search-input')
    expect(input).not.toBeNull()
    input!.value = query
    input!.dispatchEvent(new Event('input'))
    await nextTick()
  }

  it('emits debounced search events and skips local filtering in remote mode', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('zzz')

    // 防抖窗口内不触发。
    expect(wrapper.emitted('search')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toEqual([['zzz']])
    // 远程模式不做本地过滤：无命中的 query 下选项仍完整展示（由父组件更新 options）。
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toContain('Alpha account')
    expect(labels).toContain('Beta account')
  })

  it('does not emit search when the dropdown closes and the query resets', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('hidden')

    // 关闭下拉：排队中的防抖定时器应被取消，也不应因 query 重置而尾随 emit。
    await wrapper.get('button').trigger('click')
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
  })

  it('shows the loading text instead of empty text while loading with no options', async () => {
    const wrapper = mountRemoteSelect({ options: [], loading: true })
    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = await openDropdown()
    expect(dropdown.querySelector('.select-empty')?.textContent).toContain('common.loading')
  })

  it('keeps local filtering and emits nothing when remote is not set', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        searchable: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
      },
    })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('alpha')
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toEqual(['Alpha account'])
  })
})
