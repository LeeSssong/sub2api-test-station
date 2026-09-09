import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { updateBalance, getUserQuotaSummary, showError, showSuccess } = vi.hoisted(() => ({
  updateBalance: vi.fn(),
  getUserQuotaSummary: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      updateBalance,
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
    updateBalance.mockResolvedValue({})
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '21.00000000',
      gift_quota_balance_usd: '0.00000000',
      total_quota_balance_usd: '21.00000000',
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
    expect(wrapper.text()).toContain('admin.users.paidQuota')
    expect(wrapper.text()).toContain('admin.users.giftQuota')
  })

  it('limits legacy subtract to the remaining gift quota', async () => {
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '21.00000000',
      gift_quota_balance_usd: '3.00000000',
      total_quota_balance_usd: '24.00000000',
    })

    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'subtract' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    const amount = wrapper.findAll('input[type="number"]')[0]
    await amount.setValue('4')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.users.insufficientBalance')
  })

  it('maps backend gift quota insufficiency to the explicit user-facing error', async () => {
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '21.00000000',
      gift_quota_balance_usd: '3.00000000',
      total_quota_balance_usd: '24.00000000',
    })
    updateBalance.mockRejectedValue({
      reason: 'GIFT_QUOTA_INSUFFICIENT',
      message: 'gift quota is insufficient',
    })

    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'subtract' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.find('input[type="number"]').setValue('2')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.users.insufficientGiftQuota')
  })
})
