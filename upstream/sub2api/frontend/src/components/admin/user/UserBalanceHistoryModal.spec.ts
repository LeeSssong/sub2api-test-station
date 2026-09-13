import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getUserBalanceHistory, getUserQuotaSummary } = vi.hoisted(() => ({
  getUserBalanceHistory: vi.fn(),
  getUserQuotaSummary: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getUserBalanceHistory, getUserQuotaSummary } },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { t: (key: string) => key } }),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))

import UserBalanceHistoryModal from './UserBalanceHistoryModal.vue'

const user = {
  id: 37,
  email: 'wallet@example.test',
  balance: 0.33,
  created_at: '2026-08-26T00:00:00Z',
} as any

describe('UserBalanceHistoryModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getUserBalanceHistory.mockResolvedValue({ items: [], total: 0, total_recharged: 0 })
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '18.84689040',
      gift_quota_balance_usd: '0.00000000',
      total_quota_balance_usd: '18.84689040',
    })
  })

  it('loads fresh quota summary instead of showing stale user-list balance as current balance', async () => {
    const wrapper = shallowMount(UserBalanceHistoryModal, {
      props: { show: false, user },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getUserQuotaSummary).toHaveBeenCalledWith(37)
    expect(wrapper.text()).toContain('$18.8468904')
    expect(wrapper.text()).not.toContain('$0.33')
  })

  it('does not render a cash refund balance from the quota summary', async () => {
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '18.84689040',
      gift_quota_balance_usd: '2.00000000',
      total_quota_balance_usd: '20.84689040',
    })

    const wrapper = shallowMount(UserBalanceHistoryModal, {
      props: { show: false, user },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).not.toContain('refundableCashBalance')
  })

  it('keeps only balance and concurrency history actions', async () => {
    const wrapper = shallowMount(UserBalanceHistoryModal, {
      props: { show: false, user },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).not.toContain('quotaLedger')
    expect(wrapper.text()).not.toContain('paymentChannelRefund')
    expect(wrapper.findAll('button').map((button) => button.text())).toEqual(expect.arrayContaining([
      'admin.users.gift',
      'admin.users.deduct',
    ]))
  })

  it('renders paid and gift quota deltas for quota history items', async () => {
    getUserBalanceHistory.mockResolvedValue({
      items: [{
        id: 1,
        code: 'ADMIN-GIFT-1',
        type: 'admin_gift',
        value: 10,
        status: 'used',
        used_by: 37,
        used_at: '2026-09-09T01:00:00Z',
        created_at: '2026-09-09T01:00:00Z',
        notes: '',
        paid_quota_delta_usd: '0.00000000',
        gift_quota_delta_usd: '10.00000000',
      }],
      total: 1,
      total_recharged: 0,
    })

    const wrapper = shallowMount(UserBalanceHistoryModal, {
      props: { show: false, user },
      global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.users.paidQuota $0.00')
    expect(wrapper.text()).toContain('admin.users.giftQuota +$10.00')
    expect(wrapper.text()).toContain('admin.users.adminGiftBalance')
    expect(wrapper.find('.quota-summary-secondary-row').exists()).toBe(true)
  })
})
