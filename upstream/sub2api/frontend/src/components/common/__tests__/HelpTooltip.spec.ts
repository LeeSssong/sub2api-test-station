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

	it('clamps hover-click details within the viewport on narrow screens', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: {
				content: 'clamped hover-click details',
				trigger: 'hover-click',
			},
			slots: {
				trigger: '<button type="button">!</button>',
			},
		})

		vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(320)
		vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(256)
		vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
			top: 120,
			left: 280,
			width: 40,
			height: 20,
			right: 320,
			bottom: 140,
			x: 280,
			y: 120,
			toJSON: () => ({}),
		})

		await wrapper.get('.group').trigger('mouseenter')
		await nextTick()
		await new Promise((resolve) => setTimeout(resolve, 0))

		expect(getTooltipElement().style.left).toBe('180px')

		wrapper.unmount()
	})

	it('closes hover-click details when focus leaves the close button for another control', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: {
				content: 'tab close details',
				trigger: 'hover-click',
			},
			slots: {
				trigger: '<button type="button">!</button>',
			},
		})

		const triggerButton = wrapper.get('button')
		triggerButton.element.focus()
		await nextTick()

		const closeButton = getTooltipElement().querySelector('button[aria-label="Close"]')
		if (!(closeButton instanceof HTMLButtonElement)) {
			throw new Error('close button not found')
		}

		const nextControl = document.createElement('button')
		nextControl.type = 'button'
		nextControl.textContent = 'next'
		document.body.append(nextControl)

		closeButton.focus()
		await nextTick()
		nextControl.focus()
		await nextTick()

		expect(getTooltipElement().style.display).toBe('none')
		expect(document.activeElement).toBe(nextControl)

		wrapper.unmount()
		nextControl.remove()
	})

	it('returns focus to the trigger after close button and Escape', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: {
				content: 'return focus details',
				trigger: 'hover-click',
			},
			slots: {
				trigger: '<button type="button">!</button>',
			},
		})

		const triggerButton = wrapper.get('button')
		triggerButton.element.focus()
		await nextTick()

		const closeButton = getTooltipElement().querySelector('button[aria-label="Close"]')
		if (!(closeButton instanceof HTMLButtonElement)) {
			throw new Error('close button not found')
		}

		closeButton.click()
		await nextTick()
		expect(document.activeElement).toBe(triggerButton.element)
		expect(getTooltipElement().style.display).toBe('none')

		triggerButton.element.focus()
		await nextTick()
		const reopenedCloseButton = getTooltipElement().querySelector('button[aria-label="Close"]')
		if (!(reopenedCloseButton instanceof HTMLButtonElement)) {
			throw new Error('close button not found')
		}

		reopenedCloseButton.focus()
		await nextTick()
		document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
		await nextTick()

		expect(document.activeElement).toBe(triggerButton.element)
		expect(getTooltipElement().style.display).toBe('none')

		wrapper.unmount()
	})

	it('resets hover-click pinning when the reset key changes', async () => {
		const wrapper = mount(HelpTooltip, {
			attachTo: document.body,
			props: {
				content: 'refreshable details',
				trigger: 'hover-click',
				resetKey: '2026-08-10T00:00:00Z',
			},
		})

		const trigger = wrapper.get('.group')
		const tooltip = getTooltipElement()

		await trigger.trigger('click')
		await nextTick()
		expect(tooltip.style.display).not.toBe('none')

		await wrapper.setProps({ resetKey: '2026-08-11T00:00:00Z' })
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
