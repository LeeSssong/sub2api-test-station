import { describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import BatchImageGuideView from '../BatchImageGuideView.vue'

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ locale: { value: 'en' }, t: (key: string, params?: { amount: string }) => params?.amount ?? key }),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), fetchPublicSettings: vi.fn().mockResolvedValue(undefined) }) }))
vi.mock('@/api', () => ({ keysAPI: { list: vi.fn().mockResolvedValue({ items: [] }) } }))
vi.mock('@/api/batchImage', () => ({ listBatchImageJobs: vi.fn().mockResolvedValue({ items: [] }), listBatchImageModels: vi.fn().mockResolvedValue([]) }))

describe('batch image job cost display', () => {
  it('shows two decimals for actual and held cost, but does not invent zero for missing amounts', () => {
    const wrapper = shallowMount(BatchImageGuideView, { global: { stubs: { AppLayout: true, TablePageLayout: true, 'i18n-t': true } } })
    const { costLabel } = (wrapper.vm as any).$?.setupState
    expect(costLabel({ status: 'succeeded', actual_cost: 90.5, hold_amount: 0 })).toBe('$90.50')
    expect(costLabel({ status: 'succeeded', actual_cost: 0.001, hold_amount: 0 })).toBe('$0.00')
    expect(costLabel({ status: 'running', actual_cost: null, hold_amount: 1.2 })).toBe('$1.20')
    expect(costLabel({ status: 'running', actual_cost: null, hold_amount: null })).not.toBe('$0.00')
    expect(costLabel({ status: 'failed', actual_cost: null, hold_amount: null })).toBe('$0.00')
    wrapper.unmount()
  })
})
