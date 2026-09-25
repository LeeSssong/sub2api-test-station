import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UsageStatsCards from '../UsageStatsCards.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('usage overview unavailable state', () => {
  it('does not fabricate zero values before a successful response', () => {
    const wrapper = mount(UsageStatsCards, { props: { stats: null }, global: { stubs: { Icon: true } } })
    expect(wrapper.text()).not.toContain('$—')
    expect(wrapper.text()).not.toContain('usage.totalRequests0')
    expect(wrapper.text()).not.toContain('usage.totalTokens0')
    expect(wrapper.text()).not.toContain('usage.avgDuration0ms')
    expect(wrapper.text()).toContain('usage.totalCost—')
  })
})
