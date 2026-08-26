import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getUserBalanceHistory, getUserQuotaLedger, getUserQuotaSummary } = vi.hoisted(() => ({
  getUserBalanceHistory: vi.fn(),
  getUserQuotaLedger: vi.fn(),
  getUserQuotaSummary: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getUserBalanceHistory, getUserQuotaLedger, getUserQuotaSummary } },
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
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
    getUserQuotaLedger.mockResolvedValue({ items: [], total: 0 })
    getUserQuotaSummary.mockResolvedValue({
      cash_balance_cny: '20.00000000',
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
})
