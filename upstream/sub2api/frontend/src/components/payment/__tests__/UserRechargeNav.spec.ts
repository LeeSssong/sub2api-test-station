import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import UserRechargeNav from '@/components/payment/UserRechargeNav.vue'

describe('UserRechargeNav', () => {
  it('disables recharge navigation and explains why when payment is off', () => {
    const wrapper = mount(UserRechargeNav, {
      props: { active: 'redeem', balance: 2, paymentEnabled: false },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })

    expect(wrapper.find('[data-test="recharge-unavailable"]').text()).toContain('充值暂不可用')
    expect(wrapper.find('[data-test="recharge-tab"]').attributes('aria-disabled')).toBe('true')
    expect(wrapper.find('[data-test="recharge-tab"]').element.tagName).toBe('SPAN')
    expect(wrapper.find('[data-test="orders-link"]').exists()).toBe(false)
  })

  it('keeps recharge and orders navigation when payment is on', () => {
    const wrapper = mount(UserRechargeNav, {
      props: { active: 'redeem', balance: 2, paymentEnabled: true },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })

    expect(wrapper.find('[data-test="recharge-tab"]').element.tagName).toBe('A')
    expect(wrapper.find('[data-test="orders-link"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="recharge-unavailable"]').exists()).toBe(false)
  })

  it('lets the account metrics shrink inside the mobile user workspace', () => {
    const wrapper = mount(UserRechargeNav, {
      props: {
        active: 'recharge',
        balance: 2,
        concurrency: 5,
      },
      global: {
        stubs: {
          RouterLink: {
            template: '<a><slot /></a>',
          },
        },
      },
    })

    const metrics = wrapper.get('[data-test="account-metrics"]')
    expect(metrics.classes()).toEqual(expect.arrayContaining(['grid', 'min-w-0', 'grid-cols-2']))
    expect(metrics.findAll('[data-test="account-metric"]').every(metric => (
      metric.classes().includes('min-w-0') && metric.classes().includes('px-3')
    ))).toBe(true)
  })
})
