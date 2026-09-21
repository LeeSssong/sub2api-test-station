import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import UserRechargeNav from '@/components/payment/UserRechargeNav.vue'

describe('UserRechargeNav', () => {
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
