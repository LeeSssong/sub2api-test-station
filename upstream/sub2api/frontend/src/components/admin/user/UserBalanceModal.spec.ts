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
  createI18n: () => ({ global: { t: (key: string) => key } }),
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

  it('shows the backend message when balance update fails', async () => {
    updateBalance.mockRejectedValue({
      status: 500,
      code: 500,
      reason: 'QUOTA_WALLET_WRITE_FAILED',
      message: 'quota wallet persistence failed',
      metadata: { request_id: 'lab-request-123' },
    })

    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.find('input[type="number"]').setValue('10')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('quota wallet persistence failed')
    expect(showError).not.toHaveBeenCalledWith('common.error')
  })

  it('displays current and projected quotas with two decimals without changing submitted amount', async () => {
    getUserQuotaSummary.mockResolvedValue({
      paid_quota_balance_usd: '18.84689040',
      gift_quota_balance_usd: '0.00000000',
      total_quota_balance_usd: '18.84689040',
    })
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).toContain('$18.85')
    expect(wrapper.text()).toContain('$0.00')
    await wrapper.find('input[type="number"]').setValue('0.12345678')
    expect(wrapper.text()).toContain('$18.97')
    await wrapper.get('#balance-form').trigger('submit')
    expect(updateBalance).toHaveBeenCalledWith(1, 0.12345678, 'add', '')
  })

  it('submits the entered amount through the native balance API', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.find('input[type="number"]').setValue('5')
    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(updateBalance).toHaveBeenCalledWith(1, 5, 'add', '')
  })

  it('keeps the confirmation disabled until an amount is entered', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    const confirm = wrapper.find('button[type="submit"]')
    expect(confirm.attributes('disabled')).toBeDefined()
    await wrapper.find('input[type="number"]').setValue('10')
    expect(confirm.attributes('disabled')).toBeUndefined()
  })

  it('enables confirmation after opening and entering a valid amount', async () => {
    const wrapper = mount(UserBalanceModal, {
      props: { show: false, user, operation: 'add' },
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    await wrapper.setProps({ show: true })
    await wrapper.find('input[type="number"]').setValue('110')
    const confirm = wrapper.find('button[type="submit"]')
    expect(confirm.attributes('disabled')).toBeUndefined()

    await wrapper.get('#balance-form').trigger('submit')
    await flushPromises()

    expect(updateBalance).toHaveBeenCalledWith(1, 110, 'add', '')
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
