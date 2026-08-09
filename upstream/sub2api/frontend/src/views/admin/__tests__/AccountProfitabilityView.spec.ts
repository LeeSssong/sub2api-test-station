import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountProfitabilityView from '../AccountProfitabilityView.vue'

const { get, showError, controlPlaneProfitability, readMode } = vi.hoisted(() => ({
  get: vi.fn(),
  showError: vi.fn(),
  controlPlaneProfitability: vi.fn(),
  readMode: { value: 'legacy_only' as 'legacy_only' | 'shadow' | 'external_primary' },
}))

vi.mock('@/api/controlPlane', () => ({
  controlPlaneAPI: { profitability: controlPlaneProfitability },
  getControlPlaneReadMode: () => readMode.value,
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accountProfitability: { get } },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const response = {
  start_date: '2026-08-01',
  end_date: '2026-08-08',
  generated_at: '2026-08-08T12:00:00Z',
  summary: { revenue: 120, expense: 70, profit: 50, margin: 0.4167, account_count: 2, pending_count: 1 },
  rows: [
    { account_id: 1, name: 'Sub account', platform: 'openai', account_type: 'relay', source: 'sub2api', status: 'known', revenue: 100, expense: 60, profit: 40, margin: 0.4, expense_status: 'known', request_count: 10, tokens: 1000, cost_basis: 'account_cost' },
    { account_id: 2, name: 'Needs setup', platform: 'anthropic', account_type: 'self', source: 'pending', status: 'pending', revenue: 20, expense: null, profit: null, margin: null, expense_status: 'pending', request_count: 2, tokens: 200, cost_basis: null },
  ],
}

describe('AccountProfitabilityView', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue(response)
    showError.mockReset()
    controlPlaneProfitability.mockReset().mockResolvedValue({
      items: [],
      freshness: { completeness: 'complete', calculation_version: 'profitability-v1' },
    })
    readMode.value = 'legacy_only'
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-08T12:00:00+08:00'))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('loads the current month by default and renders summary totals', async () => {
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    expect(get).toHaveBeenCalledWith(expect.objectContaining({ start_date: '2026-08-01', end_date: '2026-08-08' }))
    expect(wrapper.find('[data-test="summary-revenue"]').text()).toContain('120')
    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
  })

  it('keeps legacy filter and CSV rows visible during a shadow read', async () => {
    readMode.value = 'shadow'
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    expect(controlPlaneProfitability).toHaveBeenCalledWith(expect.objectContaining({ start_date: '2026-08-01', end_date: '2026-08-08' }))
    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('来源：现有系统')
    expect(wrapper.text()).toContain('完整性：complete')
  })

  it.each([
    ['a complete-looking response', { items: response, freshness: { completeness: 'complete', calculation_version: 'profitability-v1' } }],
    ['an incomplete response', { items: { start_date: response.start_date, end_date: response.end_date, rows: [], summary: {} }, freshness: { completeness: 'partial', calculation_version: 'profitability-v1' } }],
  ])('keeps the legacy profitability source and degrades external_primary for %s', async (_label, externalResponse) => {
    readMode.value = 'external_primary'
    controlPlaneProfitability.mockResolvedValueOnce(externalResponse)
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('来源：现有系统')
    expect(wrapper.text()).toContain('控制面暂时不可用')
  })

  it.each([401, 403])('keeps profitability local when the control plane returns %s', async (status) => {
    readMode.value = 'external_primary'
    controlPlaneProfitability.mockRejectedValueOnce({ status, message: 'control plane rejected request' })
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('控制面暂时不可用')
    expect(showError).not.toHaveBeenCalledWith('control plane rejected request')
  })

  it('filters by source and pending status', async () => {
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    await wrapper.find('[data-test="source-filter"]').setValue('sub2api')
    expect(wrapper.find('[data-test="account-row-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-row-2"]').exists()).toBe(false)

    await wrapper.find('[data-test="source-filter"]').setValue('pending')
    expect(wrapper.find('[data-test="account-row-2"]').exists()).toBe(true)

    await wrapper.find('[data-test="status-filter"]').setValue('known')
    expect(wrapper.find('[data-test="account-row-2"]').exists()).toBe(false)
  })

  it('shows pending cost clearly and exports CSV', async () => {
    const createObjectURL = vi.fn(() => 'blob:csv')
    const revokeObjectURL = vi.fn()
    Object.assign(URL, { createObjectURL, revokeObjectURL })
    const click = vi.fn()
    const originalCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      const element = originalCreate(tagName)
      if (tagName === 'a') Object.assign(element, { click })
      return element
    })

    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    expect(wrapper.find('[data-test="pending-expense"]').text()).toContain('pending')
    await wrapper.find('[data-test="export-csv"]').trigger('click')
    expect(click).toHaveBeenCalled()
    expect(createObjectURL).toHaveBeenCalled()

  })

  it('normalizes available expense status and keeps CNY procurement costs out of USD profit', async () => {
    get.mockResolvedValue({
      ...response,
      summary: { revenue: 100, expense: 30, profit: null, margin: null, account_count: 1, pending_count: 0 },
      rows: [{
        account_id: 3,
        name: 'Self purchased',
        platform: 'anthropic',
        account_type: 'self',
        source: 'self_purchased',
        status: 'active',
        revenue: 100,
        expense: 30,
        expense_currency: 'CNY',
        procurement_expense_cny: 30,
        profit: null,
        margin: null,
        expense_status: 'available',
        request_count: 4,
        tokens: 400,
        cost_basis: 'allocated_procurement',
      }],
    })
    const createObjectURL = vi.fn(() => 'blob:csv')
    Object.assign(URL, { createObjectURL })
    const blobParts: unknown[] = []
    class BlobMock {
      constructor(parts: unknown[]) {
        blobParts.push(...parts)
      }
    }
    vi.stubGlobal('Blob', BlobMock)
    const click = vi.fn()
    const originalCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      const element = originalCreate(tagName)
      if (tagName === 'a') Object.assign(element, { click })
      return element
    })

    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()

    const row = wrapper.find('[data-test="account-row-3"]')
    expect(row.text()).toContain('¥30.00')
    expect(row.text()).toContain('admin.accountProfitability.pendingConversion')
    await wrapper.find('[data-test="status-filter"]').setValue('known')
    expect(wrapper.find('[data-test="account-row-3"]').exists()).toBe(true)

    await wrapper.find('[data-test="export-csv"]').trigger('click')
    const csv = String(blobParts[0] ?? '')
    expect(csv).toContain('¥30.00')
    expect(csv).toContain('CNY')
    expect(click).toHaveBeenCalled()
  })
})
