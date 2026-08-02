import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorGroupScoreDialog from './AccountMonitorGroupScoreDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('AccountMonitorGroupScoreDialog', () => {
  const weights = { cost: 15, success: 45, ttft: 20, latency: 20 }

  it('disables save until four weights sum to 100', async () => {
    const wrapper = mount(AccountMonitorGroupScoreDialog, {
      props: { show: true, groupId: 3, groupName: 'Production', weights },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    const inputs = wrapper.findAll('input')
    expect(inputs).toHaveLength(4)
    await inputs[3].setValue(19)

    expect(wrapper.get('[data-test="score-total"]').text()).toContain('99')
    expect(wrapper.get('[data-test="save-score-weights"]').attributes('disabled')).toBeDefined()

    await inputs[3].setValue(20)
    expect(wrapper.get('[data-test="save-score-weights"]').attributes('disabled')).toBeUndefined()
  })

  it('emits save and reset actions with the current group weights', async () => {
    const wrapper = mount(AccountMonitorGroupScoreDialog, {
      props: { show: true, groupId: 3, groupName: 'Production', weights },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.get('[data-test="save-score-weights"]').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual([{ cost: 15, success: 45, ttft: 20, latency: 20 }])

    await wrapper.get('[data-test="reset-score-weights"]').trigger('click')
    expect(wrapper.emitted('reset')).toHaveLength(1)
  })
})
