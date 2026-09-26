import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupBadge from '../GroupBadge.vue'

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: null }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('GroupBadge multiplier', () => {
  it('labels the configured and current-user rate without changing either value', () => {
    const wrapper = mount(GroupBadge, {
      props: { name: 'GPT', rateMultiplier: 1.2, userRateMultiplier: 0.12 },
      global: { stubs: { PlatformIcon: true } }
    })
    expect(wrapper.get('.line-through').text()).toBe('1.2x倍率')
    expect(wrapper.get('.font-bold').text()).toBe('0.12x倍率')
  })
})
