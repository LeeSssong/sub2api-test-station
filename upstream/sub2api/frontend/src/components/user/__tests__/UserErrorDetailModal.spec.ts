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
        'usage.detail.loading': 'Loading detail',
        'usage.detail.retry': 'Retry',
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
  error_class: 'upstream_failed',
  meaning: '上游请求失败',
  suggestion: '请稍后重试；持续失败请联系管理员并提供请求 ID',
  category: 'upstream_error',
  model: 'gpt-5.4',
  message: '上游请求失败',
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

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
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

  it('shows only safe meaning and suggestion even if an unsafe legacy payload is returned', async () => {
    getMyErrorDetail.mockResolvedValue({
      ...detail,
      message: 'RAW provider message sk-secret',
      error_body: '{"api_key":"sk-secret","body":"private prompt"}',
      upstream_status_code: 503,
    })
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.get('[data-testid="user-error-meaning"]').text()).toBe('上游请求失败')
    expect(wrapper.get('[data-testid="user-error-suggestion"]').text()).toContain('提供请求 ID')
    expect(wrapper.text()).not.toContain('RAW provider message')
    expect(wrapper.text()).not.toContain('sk-secret')
    expect(wrapper.text()).not.toContain('private prompt')
    expect(wrapper.text()).not.toContain('503')
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

  it('announces a load failure and lets the user retry', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getMyErrorDetail
      .mockRejectedValueOnce(new Error('network failed'))
      .mockResolvedValueOnce(detail)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    const alert = wrapper.get('[role="alert"]')
    expect(alert.text()).toContain('Failed to load detail')
    await wrapper.get('[data-testid="retry-user-error-detail"]').trigger('click')
    await flushPromises()

    expect(getMyErrorDetail).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="user-error-request-id"]').text()).toBe('req-user-error-7')
    consoleError.mockRestore()
  })

  it('allows only the latest changed-ID success to render and be copied', async () => {
    const first = deferred<typeof detail>()
    const second = deferred<typeof detail>()
    getMyErrorDetail
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await wrapper.setProps({ errorId: 8 })
    first.resolve({ ...detail, request_id: 'req-stale-error-7' })
    await flushPromises()

    expect(wrapper.find('[data-testid="user-error-request-id"]').exists()).toBe(false)
    expect(wrapper.find('.animate-spin').exists()).toBe(true)

    second.resolve({ ...detail, id: 8, request_id: 'req-current-error-8' })
    await flushPromises()
    expect(wrapper.get('[data-testid="user-error-request-id"]').text()).toBe('req-current-error-8')

    await wrapper.get('[data-testid="copy-user-error-request-id"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('req-current-error-8', 'Copied')
  })

  it('ignores a stale rejection while the latest changed ID is loading', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const first = deferred<typeof detail>()
    const second = deferred<typeof detail>()
    getMyErrorDetail
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await wrapper.setProps({ errorId: 8 })
    first.reject(new Error('stale request failed'))
    await flushPromises()

    expect(wrapper.text()).not.toContain('Failed to load detail')
    expect(wrapper.find('.animate-spin').exists()).toBe(true)

    second.resolve({ ...detail, id: 8, request_id: 'req-current-error-8' })
    await flushPromises()
    expect(wrapper.get('[data-testid="user-error-request-id"]').text()).toBe('req-current-error-8')
    consoleError.mockRestore()
  })

  it('invalidates a pending request across close and reopen', async () => {
    const first = deferred<typeof detail>()
    const second = deferred<typeof detail>()
    getMyErrorDetail
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    wrapper.getComponent({ name: 'BaseDialog' }).vm.$emit('close')
    await flushPromises()
    expect(wrapper.emitted('update:show')).toEqual([[false]])
    expect(wrapper.find('.animate-spin').exists()).toBe(false)

    await wrapper.setProps({ show: false, errorId: 8 })
    await wrapper.setProps({ show: true })
    first.resolve({ ...detail, request_id: 'req-pre-close-error-7' })
    await flushPromises()

    expect(wrapper.find('[data-testid="user-error-request-id"]').exists()).toBe(false)
    expect(wrapper.find('.animate-spin').exists()).toBe(true)

    second.resolve({ ...detail, id: 8, request_id: 'req-reopened-error-8' })
    await flushPromises()
    expect(wrapper.get('[data-testid="user-error-request-id"]').text()).toBe('req-reopened-error-8')
  })

  it('invalidates a pending request when the open dialog loses its valid ID', async () => {
    const pending = deferred<typeof detail>()
    getMyErrorDetail.mockReturnValueOnce(pending.promise)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await wrapper.setProps({ errorId: 0 })
    pending.resolve({ ...detail, request_id: 'req-invalidated-error-7' })
    await flushPromises()

    expect(getMyErrorDetail).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
    expect(wrapper.find('[data-testid="user-error-request-id"]').exists()).toBe(false)
  })

  it('invalidates a pending request before unmount', async () => {
    const pending = deferred<typeof detail>()
    getMyErrorDetail.mockReturnValueOnce(pending.promise)
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    const vm = wrapper.vm as unknown as {
      detail: typeof detail | null
    }
    wrapper.unmount()
    pending.resolve({ ...detail, request_id: 'req-after-unmount-error-7' })
    await flushPromises()

    expect(vm.detail).toBeNull()
  })
})
