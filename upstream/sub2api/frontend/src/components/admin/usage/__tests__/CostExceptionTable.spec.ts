import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { listCostExceptions, reviewOne, reviewSelected, reviewFiltered, saveAs, blobParts } = vi.hoisted(() => ({
  listCostExceptions: vi.fn(), reviewOne: vi.fn(), reviewSelected: vi.fn(), reviewFiltered: vi.fn(),
  saveAs: vi.fn(),
  blobParts: [] as BlobPart[][],
}))
vi.mock('@/api/admin/usage', () => ({ adminUsageAPI: { listCostExceptions, reviewOne, reviewSelected, reviewFiltered } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('file-saver', () => ({ saveAs }))

import CostExceptionTable from '../CostExceptionTable.vue'
import enAdmin from '@/i18n/locales/en/admin'
import zhAdmin from '@/i18n/locales/zh/admin'

const response = {
  generated_at: '2026-08-13T08:00:00Z', total: 2, page: 1, page_size: 20,
  items: [
    { usage_log_id: 11, account_id: 42, request_id: 'req-11', model: 'gpt-5', created_at: '2026-08-13T07:00:00Z', revenue_cny: 3, source: 'newapi', evidence_status: 'unavailable', reason_code: 'record_not_found', review_status: 'pending', cost_trace: { sub_actual_cost: null, new_api_quota: null, new_api_quota_per_unit: null, normalized_cost_cny: null } },
    { usage_log_id: 12, account_id: 42, request_id: 'req-12', model: 'gpt-5', created_at: '2026-08-13T07:01:00Z', revenue_cny: 4, source: 'sub', evidence_status: 'confirmed_zero', reason_code: 'confirmed_zero', review_status: 'pending', cost_trace: { sub_actual_cost: 0, new_api_quota: null, new_api_quota_per_unit: null, normalized_cost_cny: 0 } },
  ],
}

describe('CostExceptionTable', () => {
  beforeEach(() => {
    vi.clearAllMocks(); blobParts.length = 0; listCostExceptions.mockResolvedValue(response)
    vi.stubGlobal('Blob', class MockBlob {
      constructor(parts: BlobPart[]) { blobParts.push(parts) }
    })
    reviewSelected.mockResolvedValue([{ usage_log_id: 11 }, { usage_log_id: 12 }])
    reviewFiltered.mockResolvedValue({ cutoff: 12, matched: 2, updated: 1, skipped: 1 })
  })

  afterEach(() => vi.unstubAllGlobals())

  it('provides localized loading, empty, error, and retry labels', () => {
    expect(zhAdmin.costExceptions).toMatchObject({ loading: expect.any(String), empty: expect.any(String), loadError: expect.any(String), retry: '重试' })
    expect(enAdmin.costExceptions).toMatchObject({ loading: expect.any(String), empty: expect.any(String), loadError: expect.any(String), retry: 'Retry' })
  })

  it('shows loading while the list request is pending', async () => {
    let resolve!: (value: typeof response) => void
    listCostExceptions.mockReturnValueOnce(new Promise((done) => { resolve = done }))
    const wrapper = mount(CostExceptionTable, { props: { filters: { review_status: 'pending' } } })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="cost-exceptions-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="cost-exceptions-empty"]').exists()).toBe(false)
    resolve(response)
    await flushPromises()
  })

  it('shows an explicit empty state for zero matching rows', async () => {
    listCostExceptions.mockResolvedValueOnce({ ...response, total: 0, items: [] })
    const wrapper = mount(CostExceptionTable, { props: { filters: { review_status: 'pending' } } })
    await flushPromises()
    expect(wrapper.find('[data-test="cost-exceptions-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.costExceptions.empty')
  })

  it('shows a retryable error and reloads with the routed filters', async () => {
    listCostExceptions.mockRejectedValueOnce(new Error('unavailable')).mockResolvedValueOnce(response)
    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42, review_status: 'pending' } } })
    await flushPromises()
    expect(wrapper.find('[data-test="cost-exceptions-error"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="cost-exceptions-review"]').element).toHaveProperty('value', 'pending')
    await wrapper.get('[data-test="cost-exceptions-retry"]').trigger('click')
    await flushPromises()
    expect(listCostExceptions).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 42, review_status: 'pending' }))
    expect(wrapper.find('[data-test="select-11"]').exists()).toBe(true)
  })

  it('lets an explicit all selection override routed filters across unrelated prop changes', async () => {
    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42, review_status: 'pending' } } })
    await flushPromises()
    await wrapper.get('[data-test="cost-exceptions-review"]').setValue('')
    await flushPromises()
    expect(listCostExceptions).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 42, review_status: undefined }))

    await wrapper.setProps({ filters: { account_id: 43, review_status: 'pending' } })
    await flushPromises()
    expect(wrapper.get('[data-test="cost-exceptions-review"]').element).toHaveProperty('value', '')
    expect(listCostExceptions).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 43, review_status: undefined }))
  })

  it('keeps the latest reload authoritative while an older request is pending', async () => {
    let resolveFirst!: (value: typeof response) => void
    let resolveSecond!: (value: typeof response) => void
    listCostExceptions
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve }))
    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42 } } })
    await wrapper.vm.$nextTick()
    await wrapper.setProps({ filters: { account_id: 43 } })
    await wrapper.vm.$nextTick()

    resolveFirst(response)
    await flushPromises()
    expect(wrapper.find('[data-test="cost-exceptions-loading"]').exists()).toBe(true)

    resolveSecond({ ...response, total: 1, items: [{ ...response.items[0], usage_log_id: 22, account_id: 43 }] })
    await flushPromises()
    expect(wrapper.find('[data-test="cost-exceptions-loading"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="select-22"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="select-11"]').exists()).toBe(false)
  })

  it('shows provenance and reviews selected rows', async () => {
    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42 } } })
    await flushPromises()
    expect(listCostExceptions).toHaveBeenCalledWith(expect.objectContaining({ account_id: 42 }))
    expect(wrapper.text()).toContain('record_not_found')
    expect(wrapper.text()).toContain('newapi')
    await wrapper.get('[data-test="select-11"]').setValue(true)
    await wrapper.get('[data-test="select-12"]').setValue(true)
    await wrapper.get('[data-test="review-selected"]').trigger('click')
    await flushPromises()
    expect(reviewSelected).toHaveBeenCalledWith({ usage_log_ids: [11, 12] })
  })

  it('freezes and reports filtered review counts', async () => {
    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42 } } })
    await flushPromises()
    await wrapper.get('[data-test="review-filtered"]').trigger('click')
    await flushPromises()
    expect(reviewFiltered).toHaveBeenCalledWith({ filter: expect.objectContaining({ account_id: 42 }), max_usage_log_id: 0 })
    expect(wrapper.text()).toContain('admin.costExceptions.cutoff: 12')
    expect(wrapper.text()).toContain('admin.costExceptions.matched: 2')
  })

  it('exports persisted source without inferring NewAPI from empty quota fields', async () => {
    listCostExceptions
      .mockResolvedValueOnce(response)
      .mockResolvedValueOnce({ ...response, page: 1, page_size: 100 })

    const wrapper = mount(CostExceptionTable, { props: { filters: { account_id: 42 } } })
    await flushPromises()
    await wrapper.get('[data-test="export-cost-exceptions"]').trigger('click')
    await flushPromises()

    expect(listCostExceptions).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 42, page: 1, page_size: 100 }))
    expect(saveAs).toHaveBeenCalledWith(expect.anything(), 'cost-exceptions.csv')
    expect(String(blobParts[0][0])).toContain('newapi')
  })
})
