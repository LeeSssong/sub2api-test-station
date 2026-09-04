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
    createQuotaLedgerEntry.mockResolvedValue({
      ledger_entry_id: 1,
      idempotent: false,
      summary: {},
    })
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
    await wrapper.find('input[type="text"]').setValue('TRX-FAIL')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('quota wallet persistence failed')
    expect(showError).not.toHaveBeenCalledWith('common.error')
  })

  it('submits a recharge containing only gifted quota', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: true, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    const inputs = wrapper.findAll('input[type="number"]')
    await inputs[0].setValue('0')
    await inputs[1].setValue('5')
    await wrapper.find('input[type="text"]').setValue('TRX-GIFT')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(createQuotaLedgerEntry).toHaveBeenCalledWith(1, {
      record_type: 'recharge',
      amount_cny: 0,
      gift_quota_usd: 5,
      payment_trade_no: 'TRX-GIFT',
      note: '',
    })
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

  it('shows only the refundable cash that is backed by non-negative paid quota', async () => {
    getUserQuotaSummary.mockResolvedValue({
      cash_balance_cny: '50.00000000',
      paid_quota_balance_usd: '-0.01097834',
      gift_quota_balance_usd: '0.00000000',
      total_quota_balance_usd: '-0.01097834',
    })

    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'subtract' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('refundableCashBalance')
    expect(wrapper.text()).toContain('¥0.00')
    expect(wrapper.text()).not.toContain('¥50.00000000')
  })
})
