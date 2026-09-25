import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserOrdersView from '../UserOrdersView.vue'
import OrderTable from '@/components/payment/OrderTable.vue'

const { getMyOrders, getRefundEligibleProviders, showError } = vi.hoisted(() => ({
  getMyOrders: vi.fn(),
  getRefundEligibleProviders: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/payment', () => ({ paymentAPI: { getMyOrders, getRefundEligibleProviders } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-router', async () => ({
  ...(await vi.importActual<typeof import('vue-router')>('vue-router')),
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('UserOrdersView loading states', () => {
  beforeEach(() => {
    getMyOrders.mockReset()
    getRefundEligibleProviders.mockReset().mockResolvedValue({ data: { provider_instance_ids: [] } })
    showError.mockReset()
  })

  it('does not render an empty order table on initial failure and retries into a real empty state', async () => {
    getMyOrders.mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ data: { items: [], total: 0 } })
    const wrapper = shallowMount(UserOrdersView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.findComponent(OrderTable).exists()).toBe(false)

    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.findComponent(OrderTable).exists()).toBe(true)
    expect(wrapper.findComponent(OrderTable).props('orders')).toEqual([])
  })
})
