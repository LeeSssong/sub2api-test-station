import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { createQuotaLedgerEntry, getUserQuotaSummary, showError, showSuccess } = vi.hoisted(() => ({
  createQuotaLedgerEntry: vi.fn(),
  getUserQuotaSummary: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      createQuotaLedgerEntry,
      getUserQuotaSummary,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

import UserBalanceModal from './UserBalanceModal.vue'

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

const user = {
  id: 1,
  email: 'admin-lab@example.test',
  balance: 21,
} as any

describe('UserBalanceModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getUserQuotaSummary.mockResolvedValue({
      cash_balance_cny: '21.00000000',
      paid_quota_balance_usd: '21.00000000',
      gift_quota_balance_usd: '0.00000000',
      total_quota_balance_usd: '21.00000000',
    })
  })

  it('shows the backend message when recharge fails with the API client error shape', async () => {
    createQuotaLedgerEntry.mockRejectedValue({
      status: 500,
      code: 500,
      reason: 'QUOTA_WALLET_WRITE_FAILED',
      message: 'quota wallet persistence failed',
      metadata: { request_id: 'lab-request-123' },
    })

    const wrapper = mount(UserBalanceModal, {
      props: { show: true, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.findAll('input[type="number"]')[0].setValue('10')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('quota wallet persistence failed')
    expect(showError).not.toHaveBeenCalledWith('common.error')
  })

  it('shows refreshed quota summary rather than the stale users-list balance', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user: { ...user, balance: 0.33 }, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getUserQuotaSummary).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('$21.00')
    expect(wrapper.text()).not.toContain('$0.33')
  })

  it('labels received recharge separately from spendable quota', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.users.currentSpendableBalance')
    expect(wrapper.text()).toContain('admin.users.refundableCashBalance')
    expect(wrapper.text()).toContain('admin.users.paidQuota')
    expect(wrapper.text()).toContain('admin.users.giftQuota')
  })
})
