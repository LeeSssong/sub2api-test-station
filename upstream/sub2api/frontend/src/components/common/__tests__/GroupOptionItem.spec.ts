import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem description layout', () => {
  it('shows configured default and user rates with the same label', () => {
    const props = { name: 'GPT', platform: 'openai' as const, rateMultiplier: 1.2 }
    const regular = mount(GroupOptionItem, { props, global: { stubs: { GroupBadge: true } } })
    expect(regular.text()).toContain('1.2x倍率')
    const custom = mount(GroupOptionItem, { props: { ...props, userRateMultiplier: 0.12 }, global: { stubs: { GroupBadge: true } } })
    expect(custom.get('.line-through').text()).toBe('1.2x倍率')
    expect(custom.get('.font-bold').text()).toBe('0.12x倍率')
  })
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })
})
