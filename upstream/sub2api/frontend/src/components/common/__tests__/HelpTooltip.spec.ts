import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
	afterEach(() => {
		vi.restoreAllMocks()
		document.body.innerHTML = ''
	})

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('opens and closes hover-click details from pointer enter and leave', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover-click details',
        trigger: 'hover-click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('pins hover-click details on click and toggles on the second click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'pinned hover-click details',
        trigger: 'hover-click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('closes a pinned hover-click tooltip on outside click and Escape', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'dismissible hover-click details',
        trigger: 'hover-click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('opens hover-click details from a focusable trigger and closes on blur when unpinned', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'focusable hover-click details',
        trigger: 'hover-click',
      },
      slots: {
        trigger: '<button type="button">!</button>',
      },
    })

    const button = wrapper.get('button')
    button.element.focus()
    await nextTick()
    expect(getTooltipElement().style.display).not.toBe('none')

    button.element.blur()
    await nextTick()
    expect(getTooltipElement().style.display).toBe('none')

    wrapper.unmount()
  })

	it('keeps hover-click details open when focus moves into the close button', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: {
				content: 'keyboard close details',
				trigger: 'hover-click',
			},
			slots: {
				trigger: '<button type="button">!</button>',
			},
		})

		const triggerButton = wrapper.get('button')
		triggerButton.element.focus()
		await nextTick()

		const tooltip = getTooltipElement()
		expect(tooltip.style.display).not.toBe('none')

		const closeButton = tooltip.querySelector('button[aria-label="Close"]')
		if (!(closeButton instanceof HTMLButtonElement)) {
			throw new Error('close button not found')
		}

		closeButton.focus()
		await nextTick()
		expect(tooltip.style.display).not.toBe('none')

		closeButton.click()
		await nextTick()
		expect(tooltip.style.display).toBe('none')

		wrapper.unmount()
	})

  it('resets hover-click pinning when switching to and from a legacy trigger', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'transition details',
        trigger: 'hover-click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await wrapper.setProps({ trigger: 'hover' })
    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await wrapper.setProps({ trigger: 'hover-click' })
    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

	it('opens hover details when a keyboard-focusable trigger receives focus', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: { content: 'keyboard details' },
			slots: { trigger: '<button type="button">!</button>' },
		})

		const button = wrapper.get('button')
		button.element.focus()
		await nextTick()
		expect(getTooltipElement().style.display).not.toBe('none')

		button.element.blur()
		await nextTick()
		 expect(getTooltipElement().style.display).toBe('none')
	})

	it('keeps fixed tooltip coordinates viewport-relative after scrolling', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: { content: 'scroll-safe' },
		})
		vi.spyOn(window, 'scrollY', 'get').mockReturnValue(640)
		vi.spyOn(window, 'scrollX', 'get').mockReturnValue(32)
		vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
			top: 120,
			left: 80,
			width: 40,
			height: 20,
			right: 120,
			bottom: 140,
			x: 80,
			y: 120,
			toJSON: () => ({}),
		})
		await wrapper.get('.group').trigger('mouseenter')
		await nextTick()
		expect(getTooltipElement().style.top).toBe('calc(112px)')
		expect(getTooltipElement().style.left).toBe('100px')
		wrapper.unmount()
	})
})
