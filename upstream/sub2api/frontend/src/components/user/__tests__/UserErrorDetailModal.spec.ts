import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserErrorDetailModal from '../UserErrorDetailModal.vue'

const { getMyErrorDetail, copyToClipboard } = vi.hoisted(() => ({
  getMyErrorDetail: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/api/usage', () => ({
  getMyErrorDetail,
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => ({
        'usage.detail.requestId': 'Request ID',
        'usage.detail.copyRequestId': 'Copy request ID',
        'usage.detail.copied': 'Copied',
        'usage.errors.detail.loadFailed': 'Failed to load detail',
      })[key] ?? key,
    }),
  }
})

const BaseDialogStub = {
  name: 'BaseDialog',
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><slot /></div>',
}

const detail = {
  id: 7,
  user_id: 11,
  request_id: 'req-user-error-7',
  status_code: 500,
  upstream_status_code: 502,
  category: 'upstream_error',
  model: 'gpt-5.4',
  message: 'upstream failed',
  error_body: '{"error":"failed"}',
  created_at: '2026-07-25T12:00:00Z',
  client_ip: '203.0.113.7',
  inbound_endpoint: '/v1/responses',
  key_name: 'owned-key',
  key_deleted: false,
  group_name: 'default',
  request_type: 1,
  stream: false,
  platform: 'openai',
  user_agent: 'test-agent',
}

function mountModal() {
  return mount(UserErrorDetailModal, {
    props: {
      show: false,
      errorId: 7,
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: true,
      },
    },
  })
}

describe('UserErrorDetailModal', () => {
  beforeEach(() => {
    getMyErrorDetail.mockReset()
    copyToClipboard.mockReset()
  })

  it('renders and copies only the owned request ID', async () => {
    getMyErrorDetail.mockResolvedValue({
      ...detail,
      account_id: 99,
      client_request_id: 'internal-client-id',
      upstream_endpoint: 'https://internal.example/v1/responses',
      upstream_model: 'internal-model',
    })
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getMyErrorDetail).toHaveBeenCalledWith(7)
    const requestId = wrapper.get('[data-testid="user-error-request-id"]')
    expect(requestId.text()).toBe('req-user-error-7')
    expect(requestId.classes()).toEqual(expect.arrayContaining(['font-mono', 'break-all']))

    const copyButton = wrapper.get('[data-testid="copy-user-error-request-id"]')
    expect(copyButton.attributes('title')).toBe('Copy request ID')
    expect(copyButton.attributes('aria-label')).toBe('Copy request ID')
    await copyButton.trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('req-user-error-7', 'Copied')

    expect(wrapper.text()).not.toContain('internal-client-id')
    expect(wrapper.text()).not.toContain('https://internal.example/v1/responses')
    expect(wrapper.text()).not.toContain('internal-model')
    expect(wrapper.text()).not.toContain('99')
  })

  it('omits the request-ID row for historical responses without the field', async () => {
    const { request_id: _requestId, ...historicalDetail } = detail
    getMyErrorDetail.mockResolvedValue(historicalDetail)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="user-error-request-id"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="copy-user-error-request-id"]').exists()).toBe(false)
  })

  it('shows the existing load failure state without stale request-ID actions', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getMyErrorDetail.mockRejectedValue(new Error('network failed'))
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('Failed to load detail')
    expect(wrapper.find('[data-testid="user-error-request-id"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="copy-user-error-request-id"]').exists()).toBe(false)
    consoleError.mockRestore()
  })
})
