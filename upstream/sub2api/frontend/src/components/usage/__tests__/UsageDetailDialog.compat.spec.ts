import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUsageLog } from '@/types'

const { apiGet, userGetById, copyToClipboard } = vi.hoisted(() => ({
  apiGet: vi.fn(),
  userGetById: vi.fn(),
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get: apiGet,
    post: vi.fn(),
  },
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getById: userGetById,
  },
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key === 'usage.detail.copyRequestId' ? 'Copy request ID' : key,
  }),
}))

vi.mock('@/i18n', () => ({
  getLocale: () => 'en',
  i18n: { global: { t: (key: string) => key } },
}))

import UsageDetailDialog from '../UsageDetailDialog.vue'

const BaseDialogStub = defineComponent({
  props: { show: Boolean, title: String, width: String },
  template: '<section v-if="show"><slot /></section>',
})

const adminRecord = {
  id: 42,
  user_id: 5,
  api_key_id: 9,
  account_id: 11,
  request_id: 'req-admin-42',
  model: 'claude-sonnet-4',
  inbound_endpoint: '/v1/messages',
  group_id: 3,
  input_tokens: 1000,
  output_tokens: 250,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: 0.005,
  output_cost: 0.003,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  total_cost: 0.008,
  actual_cost: 0.00688,
  rate_multiplier: 0.8,
  long_context_billing_applied: false,
  billing_type: 1,
  request_type: 'sync',
  stream: false,
  openai_ws_mode: false,
  duration_ms: 800,
  first_token_ms: 250,
  image_count: 0,
  image_input_tokens: 0,
  image_output_tokens: 0,
  image_input_cost: 0,
  image_output_cost: 0,
  created_at: '2026-07-25T08:00:00Z',
  service_tier: null,
  reasoning_effort: null,
  cache_ttl_overridden: false,
  upstream_request_id: 'upstream-pascal-42',
  upstream_model: 'claude-sonnet-4-20250514',
  model_mapping_chain: 'sonnet-latest -> claude-sonnet-4-20250514',
  account_rate_multiplier: 0.25,
  account_stats_cost: 0.01,
  channel_id: 7,
  billing_tier: 'premium',
  account: { id: 11, name: 'Primary relay' },
} as AdminUsageLog

function valueForLabel(wrapper: ReturnType<typeof mount>, label: string): string {
  const term = wrapper.findAll('dt').find((item) => item.text() === label)
  expect(term, `missing detail label: ${label}`).toBeDefined()
  return term!.element.nextElementSibling?.textContent?.trim() ?? ''
}

describe('UsageDetailDialog PascalCase compatibility', () => {
  beforeEach(() => {
    apiGet.mockReset()
    userGetById.mockReset()
    copyToClipboard.mockReset().mockResolvedValue(true)
  })

  it('normalizes PascalCase evidence without replacing the native cost and profit', async () => {
    apiGet.mockImplementation((path: string) => {
      if (path === '/admin/usage/42') return Promise.resolve({ data: adminRecord })
      if (path === '/admin/usage/42/upstream-cost') {
        return Promise.resolve({
          data: {
            UsageLogID: 42,
            EvidenceStatus: 'confirmed',
            NormalizedCostCNY: 0.004,
          },
        })
      }
      throw new Error(`unexpected path: ${path}`)
    })

    const wrapper = mount(UsageDetailDialog, {
      props: { show: true, usageId: 42, scope: 'admin' },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Icon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('$0.002500')
    expect(valueForLabel(wrapper, 'admin.usageCostDetail.profit')).toBe('$0.004380')
    expect(apiGet).toHaveBeenCalledWith('/admin/usage/42/upstream-cost')
  })
})
