import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'

const { getRequestErrorDetail, getUpstreamErrorDetail, listRequestErrorUpstreamErrors } = vi.hoisted(() => ({
  getRequestErrorDetail: vi.fn(),
  getUpstreamErrorDetail: vi.fn(),
  listRequestErrorUpstreamErrors: vi.fn(),
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getRequestErrorDetail, getUpstreamErrorDetail, listRequestErrorUpstreamErrors },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
    t: (key: string) => ({
      'admin.ops.errorDetail.adminDiagnosis': '管理员诊断',
      'admin.ops.errorDetail.noUpstreamSelected': '未选择上游',
      'admin.ops.errorDetail.selectedUpstream': '已选择上游',
      'admin.ops.errorDetail.diagnosisClass': '分类/代码',
      'admin.ops.errorDetail.diagnosisStage': '阶段',
      'admin.ops.errorDetail.diagnosisOwner': '归属',
      'admin.ops.errorDetail.originalUpstreamStatus': '原始上游状态',
      'admin.ops.errorDetail.originalUpstreamMessage': '原始上游消息',
      'admin.ops.errorDetail.originalUpstreamDetail': '原始上游详情',
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

function makeDetail(selected: boolean) {
  return {
    id: 12,
    created_at: '2026-08-12T00:00:00Z',
    phase: selected ? 'upstream' : 'request',
    type: selected ? 'upstream_error' : 'invalid_request_error',
    error_owner: selected ? 'provider' : 'client',
    error_source: selected ? 'upstream_http' : 'client_request',
    severity: 'P1',
    status_code: selected ? 503 : 400,
    platform: 'openai',
    model: 'gpt-5',
    resolved: false,
    client_request_id: '',
    request_id: 'req-12',
    message: selected ? 'Upstream request failed' : 'Failed to read request body',
    user_email: 'user@example.com',
    account_id: selected ? 7 : null,
    account_name: selected ? 'provider-a' : '',
    group_id: selected ? 9 : null,
    group_name: selected ? 'paid' : '',
    error_body: '',
    is_business_limited: false,
    diagnosis: {
      class: selected ? 'upstream_failed' : 'upload_interrupted',
      code: selected ? 'UPSTREAM_FAILED' : 'UPLOAD_INTERRUPTED',
      stage: selected ? 'upstream' : 'request',
      ownership: selected ? 'provider' : 'client',
      upstream_account_selected: selected,
      selected_account_id: selected ? 7 : undefined,
      selected_account_name: selected ? 'provider-a' : undefined,
      group_id: selected ? 9 : undefined,
      group_name: selected ? 'paid' : undefined,
      original_upstream_status: selected ? 503 : undefined,
      original_upstream_message: selected ? 'provider unavailable' : undefined,
      original_upstream_detail: selected ? '{"message":"maintenance"}' : undefined,
    },
  }
}

function mountModal(errorType: 'request' | 'upstream') {
  return mount(OpsErrorDetailModal, {
    props: { show: true, errorId: 12, errorType },
    global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
  })
}

describe('OpsErrorDetailModal diagnosis', () => {
  beforeEach(() => {
    getRequestErrorDetail.mockReset()
    getUpstreamErrorDetail.mockReset()
    listRequestErrorUpstreamErrors.mockReset()
    listRequestErrorUpstreamErrors.mockResolvedValue({ items: [] })
  })

  it('renders not-selected upload diagnosis in the existing request detail modal', async () => {
    getRequestErrorDetail.mockResolvedValue(makeDetail(false))
    const wrapper = mountModal('request')
    await flushPromises()

    const diagnosis = wrapper.get('[data-testid="admin-error-diagnosis"]')
    expect(diagnosis.text()).toContain('管理员诊断')
    expect(diagnosis.text()).toContain('upload_interrupted')
    expect(diagnosis.text()).toContain('UPLOAD_INTERRUPTED')
    expect(diagnosis.text()).toContain('未选择上游')
  })

  it('reuses diagnosis for upstream context with selected account and sanitized evidence', async () => {
    getUpstreamErrorDetail.mockResolvedValue(makeDetail(true))
    const wrapper = mountModal('upstream')
    await flushPromises()

    const diagnosis = wrapper.get('[data-testid="admin-error-diagnosis"]')
    expect(diagnosis.text()).toContain('已选择上游')
    expect(diagnosis.text()).toContain('provider-a')
    expect(diagnosis.text()).toContain('paid')
    expect(diagnosis.text()).toContain('provider unavailable')
    expect(diagnosis.text()).toContain('maintenance')
  })
})
