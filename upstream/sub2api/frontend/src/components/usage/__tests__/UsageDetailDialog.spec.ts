import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUsageLog, UserUsageDetail } from '@/types'

const { adminGetById, adminGetCostEvidence, copyToClipboard, userGetById } = vi.hoisted(() => ({
  adminGetById: vi.fn(),
  adminGetCostEvidence: vi.fn(),
  copyToClipboard: vi.fn().mockResolvedValue(true),
  userGetById: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key === 'usage.detail.copyRequestId' ? 'Copy request ID' : key,
  }),
}))

vi.mock('@/i18n', () => ({
  getLocale: () => 'en',
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getById: userGetById,
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    getById: adminGetById,
    getCostEvidence: adminGetCostEvidence,
  },
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

import UsageDetailDialog from '../UsageDetailDialog.vue'
import enAdmin from '@/i18n/locales/en/admin'
import enDashboard from '@/i18n/locales/en/dashboard'
import zhAdmin from '@/i18n/locales/zh/admin'
import zhDashboard from '@/i18n/locales/zh/dashboard'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: Boolean,
    title: String,
    width: String,
  },
  emits: ['close'],
  template: `
    <section v-if="show" data-testid="base-dialog" :data-width="width">
      <h1>{{ title }}</h1>
      <slot />
      <button data-testid="dialog-close" type="button" @click="$emit('close')">close</button>
    </section>
  `,
})

const userRecord: UserUsageDetail = {
  id: 42,
  user_id: 5,
  api_key_id: 9,
  request_id: 'req-user-42',
  model: 'claude-sonnet-4',
  service_tier: 'priority',
  reasoning_effort: 'high',
  inbound_endpoint: '/v1/messages',
  group_id: 3,
  input_tokens: 1000,
  output_tokens: 250,
  cache_creation_tokens: 50,
  cache_read_tokens: 100,
  cache_creation_5m_tokens: 20,
  cache_creation_1h_tokens: 30,
  input_cost: 0.005,
  output_cost: 0.003,
  cache_creation_cost: 0.0005,
  cache_read_cost: 0.0001,
  total_cost: 0.0086,
  actual_cost: 0.00688,
  rate_multiplier: 0.8,
  long_context_billing_applied: false,
  billing_type: 1,
  request_type: 'stream',
  stream: true,
  openai_ws_mode: false,
  duration_ms: 1234,
  first_token_ms: 245,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  media_type: null,
  user_agent: 'usage-detail-test-agent',
  ip_address: '203.0.113.8',
  cache_ttl_overridden: false,
  billing_mode: 'token',
  created_at: '2026-07-25T08:30:00Z',
  api_key: { id: 9, name: 'Primary key' },
  group: { id: 3, name: 'Production group' },
}

const adminRecord: AdminUsageLog = {
  ...userRecord,
  account_id: 11,
  subscription_id: null,
  request_id: 'req-admin-42',
  upstream_endpoint: 'https://relay.example.com/v1/messages',
  upstream_model: 'claude-sonnet-4-20250514',
  model_mapping_chain: 'sonnet-latest -> claude-sonnet-4-20250514',
  account_rate_multiplier: 0.25,
  account_stats_cost: 0.01,
  channel_id: 7,
  billing_tier: 'premium',
  account: {
    id: 11,
    name: 'Primary relay',
  },
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, reject, resolve }
}

function mountDialog(props: {
  show?: boolean
  usageId?: number | null
  scope?: 'user' | 'admin'
} = {}) {
  return mount(UsageDetailDialog, {
    props: {
      show: props.show ?? true,
      usageId: props.usageId ?? 42,
      scope: props.scope ?? 'user',
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: {
          template: '<span />',
        },
      },
    },
  })
}

function valueForLabel(wrapper: ReturnType<typeof mountDialog>, label: string): string {
  const term = wrapper.findAll('dt').find((item) => item.text() === label)
  expect(term, `missing detail label: ${label}`).toBeDefined()
  return term!.element.nextElementSibling?.textContent?.trim() ?? ''
}

function usageDetailMessages(locale: unknown): Record<string, string> {
  return (locale as { usage: { detail: Record<string, string> } }).usage.detail
}

describe('UsageDetailDialog', () => {
  beforeEach(() => {
    adminGetById.mockReset()
    adminGetCostEvidence.mockReset()
    copyToClipboard.mockReset().mockResolvedValue(true)
    userGetById.mockReset()
  })

  it('uses only the ownership-protected user endpoint in user scope', async () => {
    userGetById.mockResolvedValue(userRecord)

    mountDialog({ scope: 'user' })
    await flushPromises()

    expect(userGetById).toHaveBeenCalledWith(42)
    expect(adminGetById).not.toHaveBeenCalled()
    expect(adminGetCostEvidence).not.toHaveBeenCalled()
  })

  it('uses only the administrator endpoint in admin scope', async () => {
    adminGetById.mockResolvedValue(adminRecord)

    mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(adminGetById).toHaveBeenCalledWith(42)
    expect(userGetById).not.toHaveBeenCalled()
  })

  it('renders scan-friendly request, latency, token, and user-billing details', async () => {
    userGetById.mockResolvedValue(userRecord)

    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.get('[data-testid="base-dialog"]').attributes('data-width')).toBe('wide')
    expect(wrapper.text()).toContain('req-user-42')
    expect(wrapper.text()).toContain('/v1/messages')
    expect(wrapper.text()).toContain('Primary key')
    expect(wrapper.text()).toContain('Production group')
    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('1.23s')
    expect(wrapper.text()).toContain('245ms')
    expect(wrapper.text()).toContain('1,000')
    expect(wrapper.text()).toContain('$0.005000')
    expect(wrapper.text()).toContain('$0.006880')
    expect(wrapper.text()).toContain('$5.000000')
  })

  it('leaves vertical scrolling to BaseDialog', async () => {
    userGetById.mockResolvedValue(userRecord)

    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.find('[class*="overflow-y-auto"]').exists()).toBe(false)
    expect(wrapper.find('[class*="max-h-"]').exists()).toBe(false)
  })

  it('never renders administrator fields in user scope even when the payload is malicious', async () => {
    userGetById.mockResolvedValue({
      ...adminRecord,
      account: { id: 999, name: 'secret-admin-account' },
      channel_id: 999,
      upstream_endpoint: 'https://secret-upstream.invalid',
      upstream_model: 'secret-upstream-model',
      model_mapping_chain: 'secret-model-mapping',
      billing_tier: 'secret-billing-tier',
      account_rate_multiplier: 9.75,
      account_stats_cost: 123,
      upstream_request_id: 'secret-upstream-request-id',
    } as AdminUsageLog)

    const wrapper = mountDialog({ scope: 'user' })
    await flushPromises()

    for (const hiddenText of [
      'usage.detail.adminSection',
      'usage.detail.account',
      'usage.detail.channelId',
      'usage.detail.upstreamEndpoint',
      'usage.detail.upstreamModel',
      'usage.detail.modelMappingChain',
      'usage.detail.billingTier',
      'usage.detail.accountMultiplier',
      'usage.detail.accountCost',
      'admin.usageCostDetail.siteRequestId',
      'admin.usageCostDetail.upstreamRequestId',
      'admin.usageCostDetail.siteStandardCost',
      'admin.usageCostDetail.siteActualCost',
      'admin.usageCostDetail.upstreamActualCost',
      'admin.usageCostDetail.costSource',
      'admin.usageCostDetail.includedCost',
      'admin.usageCostDetail.grossMargin',
      'secret-admin-account',
      'https://secret-upstream.invalid',
      'secret-upstream-model',
      'secret-model-mapping',
      'secret-billing-tier',
      '9.75x',
      'secret-upstream-request-id',
    ]) {
      expect(wrapper.text()).not.toContain(hiddenText)
    }
  })

  it('renders available safe administrator fields only in admin scope', async () => {
    adminGetById.mockResolvedValue(adminRecord)

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(wrapper.text()).toContain('usage.detail.adminSection')
    expect(valueForLabel(wrapper, 'usage.detail.account')).toBe('Primary relay (#11)')
    expect(valueForLabel(wrapper, 'usage.detail.channelId')).toBe('7')
    expect(valueForLabel(wrapper, 'usage.detail.upstreamEndpoint'))
      .toBe('https://relay.example.com/v1/messages')
    expect(valueForLabel(wrapper, 'usage.detail.upstreamModel'))
      .toBe('claude-sonnet-4-20250514')
    expect(valueForLabel(wrapper, 'usage.detail.modelMappingChain'))
      .toBe('sonnet-latest -> claude-sonnet-4-20250514')
    expect(valueForLabel(wrapper, 'usage.detail.billingTier')).toBe('premium')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('-')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('-')
  })

  it('renders confirmed native cost evidence and confirmed gross margin for administrators', async () => {
    adminGetById.mockResolvedValue({ ...adminRecord, upstream_request_id: 'upstream-req-42' })
    adminGetCostEvidence.mockResolvedValue({
      usage_log_id: 42,
      normalized_cost_cny: 0.004,
      evidence_status: 'confirmed',
      reason_code: '',
    })

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(adminGetCostEvidence).toHaveBeenCalledWith(42)
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamRequestId')).toBe('upstream-req-42')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.siteActualCost')).toBe('$0.006880')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('$0.004000')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('$0.002880')
    const administratorSection = wrapper.get('section[aria-labelledby="usage-detail-admin-heading"]')
    expect(administratorSection.text()).toContain('admin.usageCostDetail.siteActualCost')
    expect(administratorSection.text()).toContain('admin.usageCostDetail.upstreamActualCost')
    expect(administratorSection.text()).toContain('admin.usageCostDetail.profit')
    expect(wrapper.text()).not.toContain('admin.usageCostDetail.siteRequestId')
    expect(wrapper.text()).not.toContain('admin.usageCostDetail.costSource')
    expect(wrapper.text()).not.toContain('admin.usageCostDetail.includedCost')
    expect(wrapper.text()).not.toContain('admin.usageCostDetail.grossMarginStatus')
  })

  it('labels price-table cost and gross margin as estimated', async () => {
    adminGetById.mockResolvedValue(adminRecord)
    adminGetCostEvidence.mockResolvedValue({
      usage_log_id: 42,
      normalized_cost_cny: null,
      evidence_status: 'unavailable',
      reason_code: 'response_unavailable',
    })

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('-')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('-')
  })

  it('keeps cost and margin pending when native evidence is unavailable', async () => {
    adminGetById.mockResolvedValue(adminRecord)
    adminGetCostEvidence.mockResolvedValue({
      usage_log_id: 42,
      source: 'newapi',
      normalized_cost_cny: null,
      evidence_status: 'unavailable',
      reason_code: 'response_unavailable',
    })

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('-')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('-')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.costSource')).toBe('newapi')
  })

  it('explains when the upstream does not expose a compatible usage ledger', async () => {
    adminGetById.mockResolvedValue(adminRecord)
    adminGetCostEvidence.mockResolvedValue({
      usage_log_id: 42,
      normalized_cost_cny: null,
      evidence_status: 'unavailable',
      reason_code: 'endpoint_unsupported',
    })

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('-')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('-')
    expect(wrapper.text()).toContain('admin.usageCostDetail.unavailableReasons.endpointUnsupported')
  })

  it('shows a placeholder for a missing upstream request ID while querying by local ID', async () => {
    adminGetById.mockResolvedValue({ ...adminRecord, upstream_request_id: null })
    adminGetCostEvidence.mockResolvedValue({
      usage_log_id: 42,
      normalized_cost_cny: null,
      evidence_status: 'unavailable',
      reason_code: 'record_not_found',
    })

    const wrapper = mountDialog({ scope: 'admin' })
    await flushPromises()

    expect(adminGetCostEvidence).toHaveBeenCalledWith(42)
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamRequestId')).toBe('-')
  })

  it('clears the previous record while a changed ID is loading', async () => {
    const nextRecord = deferred<UserUsageDetail>()
    userGetById
      .mockResolvedValueOnce(userRecord)
      .mockReturnValueOnce(nextRecord.promise)
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.text()).toContain('req-user-42')

    await wrapper.setProps({ usageId: 43 })

    expect(wrapper.text()).toContain('usage.detail.loading')
    expect(wrapper.text()).not.toContain('req-user-42')
  })

  it('shows a retryable error and retries the same scoped endpoint', async () => {
    userGetById.mockRejectedValueOnce(new Error('network'))
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.text()).toContain('usage.detail.loadFailed')

    userGetById.mockResolvedValueOnce(userRecord)
    await wrapper.get('[data-testid="usage-detail-retry"]').trigger('click')
    await flushPromises()

    expect(userGetById).toHaveBeenNthCalledWith(1, 42)
    expect(userGetById).toHaveBeenNthCalledWith(2, 42)
    expect(wrapper.text()).toContain('req-user-42')
  })

  it('emits close, clears state, and ignores a late response', async () => {
    const pending = deferred<UserUsageDetail>()
    userGetById.mockReturnValue(pending.promise)
    const wrapper = mountDialog()
    await nextTick()

    await wrapper.get('[data-testid="dialog-close"]').trigger('click')
    expect(wrapper.emitted('update:show')).toEqual([[false]])

    pending.resolve(userRecord)
    await flushPromises()
    expect(wrapper.text()).not.toContain('req-user-42')
  })

  it('does not let a pre-close response populate a reopened dialog', async () => {
    const beforeClose = deferred<UserUsageDetail>()
    const afterReopen = deferred<UserUsageDetail>()
    userGetById
      .mockReturnValueOnce(beforeClose.promise)
      .mockReturnValueOnce(afterReopen.promise)
    const wrapper = mountDialog()
    await nextTick()

    await wrapper.get('[data-testid="dialog-close"]').trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    beforeClose.resolve({ ...userRecord, request_id: 'pre-close-request' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('pre-close-request')
    expect(wrapper.text()).toContain('usage.detail.loading')

    afterReopen.resolve({ ...userRecord, request_id: 'reopened-request' })
    await flushPromises()
    expect(wrapper.text()).toContain('reopened-request')
  })

  it('does not let an old admin response appear after switching to user scope', async () => {
    const oldAdminRequest = deferred<AdminUsageLog>()
    const currentUserRequest = deferred<UserUsageDetail>()
    adminGetById.mockReturnValue(oldAdminRequest.promise)
    userGetById.mockReturnValue(currentUserRequest.promise)
    const wrapper = mountDialog({ scope: 'admin' })
    await nextTick()

    await wrapper.setProps({ scope: 'user' })
    oldAdminRequest.resolve({
      ...adminRecord,
      account: { id: 999, name: 'stale-admin-account' },
      billing_tier: 'stale-admin-tier',
    })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-admin-account')
    expect(wrapper.text()).not.toContain('stale-admin-tier')
    expect(wrapper.text()).toContain('usage.detail.loading')

    currentUserRequest.resolve({ ...userRecord, request_id: 'current-user-request' })
    await flushPromises()
    expect(wrapper.text()).toContain('current-user-request')
    expect(wrapper.text()).not.toContain('usage.detail.adminSection')
  })

  it('allows only the latest changed-ID response to render', async () => {
    const first = deferred<UserUsageDetail>()
    const second = deferred<UserUsageDetail>()
    userGetById
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const wrapper = mountDialog()
    await nextTick()

    await wrapper.setProps({ usageId: 43 })
    first.resolve({ ...userRecord, request_id: 'stale-request' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-request')

    second.resolve({ ...userRecord, id: 43, request_id: 'latest-request' })
    await flushPromises()
    expect(wrapper.text()).toContain('latest-request')
  })

  it('invalidates an in-flight response when the open dialog loses its ID', async () => {
    const pending = deferred<UserUsageDetail>()
    userGetById.mockReturnValue(pending.promise)
    const wrapper = mountDialog()
    await nextTick()

    await wrapper.setProps({ usageId: null })
    pending.resolve(userRecord)
    await flushPromises()

    expect(wrapper.text()).not.toContain('req-user-42')
    expect(wrapper.text()).toContain('usage.noRecords')
  })

  it('uses a localized request-ID name and copies the exact identifier', async () => {
    userGetById.mockResolvedValue(userRecord)
    const wrapper = mountDialog()
    await flushPromises()

    const copyButton = wrapper.get('[data-testid="usage-detail-copy-request-id"]')
    expect(copyButton.attributes('title')).toBe('Copy request ID')
    expect(copyButton.attributes('aria-label')).toBe('Copy request ID')
    await copyButton.trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith('req-user-42', 'usage.detail.copied')
  })

  it('defines request-ID copy labels in both supported locales', () => {
    expect(usageDetailMessages(enDashboard).copyRequestId).toBe('Copy request ID')
    expect(usageDetailMessages(zhDashboard).copyRequestId).toBe('复制请求 ID')
  })

  it('defines administrator cost-detail labels in both supported locales', () => {
    const en = (enAdmin as { usageCostDetail: Record<string, string> }).usageCostDetail
    const zh = (zhAdmin as { usageCostDetail: Record<string, string> }).usageCostDetail

    expect(en.siteActualCost).toBe('Site Actual Charge')
    expect(en.profit).toBe('Profit')
    expect(zh.siteActualCost).toBe('本站实际扣费')
    expect(zh.profit).toBe('利润')
  })
})
