import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountModelDetectionHistoryPanel from './AccountModelDetectionHistoryPanel.vue'

const { history } = vi.hoisted(() => ({ history: vi.fn() }))

vi.mock('@/api/admin/accountMonitor', async () => {
  const actual = await vi.importActual<typeof import('@/api/admin/accountMonitor')>('@/api/admin/accountMonitor')
  return { ...actual, modelDetectionHistory: history }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => ({
    'admin.accounts.modelDetection.historyTitle': '检测记录',
    'admin.accounts.modelDetection.close': '关闭',
    'admin.accounts.modelDetection.historyEmpty': '暂无检测记录',
    'admin.accounts.modelDetection.loadMore': '加载更多',
    'admin.accounts.modelDetection.loading': '加载中',
    'admin.accounts.modelDetection.profile': '档位',
    'admin.accounts.modelDetection.profileNames.low': 'low',
    'admin.accounts.modelDetection.profileNames.medium': 'medium',
    'admin.accounts.modelDetection.profileNames.high': 'high',
    'admin.accounts.modelDetection.profileNames.unknown': '历史记录',
    'admin.accounts.modelDetection.modeNames.monitor': 'monitor',
    'admin.accounts.modelDetection.modeNames.manual': 'manual',
    'admin.accounts.modelDetection.modeNames.escalation': 'escalation',
    'admin.accounts.modelDetection.modeNames.historical': 'historical',
    'admin.accounts.modelDetection.reasonValue.model_conflict': 'model_conflict',
    'admin.accounts.modelDetection.historyEyebrow': 'MODEL EVIDENCE',
    'admin.accounts.modelDetection.historySummaryTitle': '检测轨迹',
    'admin.accounts.modelDetection.historySummaryHint': '查看每次检测',
    'admin.accounts.modelDetection.historyCountSuffix': ' 条',
    'admin.accounts.modelDetection.statusFilter': '结论',
    'admin.accounts.modelDetection.profileFilter': '档位',
    'admin.accounts.modelDetection.juiceFilter': 'Juice 结果',
    'admin.accounts.modelDetection.fingerprintFilter': '指纹结果',
    'admin.accounts.modelDetection.conclusionFilter': '综合结论',
    'admin.accounts.modelDetection.profileAndSamples': '档位与样本',
    'admin.accounts.modelDetection.fingerprintShort': '指纹',
    'admin.accounts.modelDetection.historyDetailsHint': '点击任意记录行可展开查看该次检测详情',
    'admin.accounts.modelDetection.juiceStatus.pass': '通过',
    'admin.accounts.modelDetection.juiceStatus.mismatch': '与申报不一致',
    'admin.accounts.modelDetection.juiceStatus.insufficient': '证据不足',
    'admin.accounts.modelDetection.juiceStatus.non_gpt': '可能非 GPT',
    'admin.accounts.modelDetection.juiceStatus.unavailable': '未取得证据',
    'admin.accounts.modelDetection.fingerprintStatus.strong_match': '强烈指向',
    'admin.accounts.modelDetection.fingerprintStatus.unclear': '证据不明确',
    'admin.accounts.modelDetection.fingerprintStatus.unavailable': '无已知指纹',
    'admin.accounts.modelDetection.conclusion.normal': '与申报一致',
    'admin.accounts.modelDetection.conclusion.abnormal': '检测异常',
    'admin.accounts.modelDetection.conclusion.insufficient': '证据不足',
    'admin.accounts.modelDetection.conclusion.failed': '检测失败',
    'admin.accounts.modelDetection.time': '时间',
    'admin.accounts.modelDetection.statusLabel': '结论',
    'admin.accounts.modelDetection.mode': '模式',
    'admin.accounts.modelDetection.reason': '触发原因',
    'admin.accounts.modelDetection.samples': '有效样本',
    'admin.accounts.modelDetection.samplesUnavailable': '历史记录',
    'admin.accounts.modelDetection.evidenceUnavailable': '未取得证据',
    'admin.accounts.modelDetection.juice': 'Juice',
    'admin.accounts.modelDetection.fingerprint': '行为指纹',
    'admin.accounts.modelDetection.details': '查看详情',
    'admin.accounts.modelDetection.historical': '历史记录',
    'admin.accounts.modelDetection.status.normal': '通过',
    'admin.accounts.modelDetection.status.abnormal': '异常',
    'admin.accounts.modelDetection.status.insufficient': '证据不足',
  }[key] ?? key) }) }
})

const account = { account_id: 7, name: 'ops@example.com' } as never

afterEach(() => vi.clearAllMocks())

describe('AccountModelDetectionHistoryPanel', () => {
  it('loads a structured history list and never renders sensitive summary fields', async () => {
    history.mockResolvedValueOnce({ items: [{
      run_id: 'run-1', status: 'abnormal', profile: 'high', mode: 'escalation', trigger_reason: 'model_conflict',
      planned_requests: 158, valid_samples: 157, evidence_state: 'complete', fingerprint_status: 'strong_match',
      claimed_model: 'gpt-5.6-sol', detector_version: '4.1.1', finished_at: '2026-08-26T02:00:00Z',
      juice_status: 'mismatch', juice_summary: { score: 0.2, api_key: 'sk-secret', prompt: 'hidden' },
      fingerprint_candidate: 'gpt-5.6-luna', fingerprint_similarity: { 'gpt-5.6-luna': 0.98 },
    }], next_cursor: 'next-1' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()
    expect(history).toHaveBeenCalledWith(7, expect.objectContaining({ limit: 25 }))
    expect(wrapper.get('[data-test="detection-history-panel"]').text()).toContain('high')
    expect(wrapper.get('[data-test="detection-history-panel"]').text()).toContain('异常')
    expect(wrapper.get('[data-test="detection-history-panel"]').text()).not.toContain('sk-secret')
    expect(wrapper.get('[data-test="detection-history-panel"]').text()).not.toContain('hidden')
    expect(wrapper.find('[data-test="detection-history-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="detection-history-timeline"]').exists()).toBe(true)
  })

  it('loads the next cursor and applies status filters', async () => {
    history.mockResolvedValue({ items: [], next_cursor: '' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()
    await wrapper.get('[data-test="detection-history-conclusion-filter"]').setValue('abnormal')
    await flushPromises()
    expect(history).toHaveBeenLastCalledWith(7, expect.objectContaining({ status: 'abnormal' }))
  })

  it('uses a centered modal and sends Juice, fingerprint, and conclusion filters to the complete history query', async () => {
    history.mockResolvedValue({ items: [{
      run_id: 'run-evidence', status: 'abnormal', profile: 'high', mode: 'escalation',
      planned_requests: 158, valid_samples: 158, evidence_state: 'complete',
      juice_status: 'mismatch', fingerprint_status: 'strong_match', fingerprint_candidate: 'gpt-5.6-terra',
      finished_at: '2026-08-26T02:00:00Z',
    }], next_cursor: '' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()

    expect(wrapper.get('[data-test="detection-history-panel"]').attributes('class')).toContain('items-center')
    expect(wrapper.get('[data-test="detection-history-table"]').text()).toContain('Juice 结果')
    expect(wrapper.get('[data-test="detection-history-table"]').text()).toContain('行为指纹')
    expect(wrapper.get('[data-test="detection-history-table"]').text()).toContain('与申报不一致')
    expect(wrapper.get('[data-test="detection-history-table"]').text()).toContain('强烈指向')
    expect(wrapper.get('[data-test="detection-history-timeline"] strong').classes()).not.toContain('truncate')

    await wrapper.get('[data-test="detection-history-juice-filter"]').setValue('mismatch')
    await wrapper.get('[data-test="detection-history-fingerprint-filter"]').setValue('strong_match')
    await wrapper.get('[data-test="detection-history-conclusion-filter"]').setValue('abnormal')
    await flushPromises()

    expect(history).toHaveBeenLastCalledWith(7, expect.objectContaining({
      juice_status: 'mismatch', fingerprint_status: 'strong_match', status: 'abnormal',
    }))
  })

  it('renders the detector verified Juice result as passed and filters with the raw detector value', async () => {
    history.mockResolvedValue({ items: [{
      run_id: 'run-verified', status: 'normal', source: 'current', profile: 'medium', mode: 'monitor',
      trigger_reason: 'scheduled', planned_requests: 49, valid_samples: 49, evidence_state: 'complete',
      juice_status: 'verified', fingerprint_status: 'unavailable', finished_at: '2026-08-26T10:01:00Z',
    }], next_cursor: '' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()

    const tableText = wrapper.get('[data-test="detection-history-table"]').text()
    expect(tableText).toContain('通过')
    expect(tableText).not.toContain('admin.accounts.modelDetection.juiceStatus.verified')

    await wrapper.get('[data-test="detection-history-juice-filter"]').setValue('verified')
    await flushPromises()
    expect(history).toHaveBeenLastCalledWith(7, expect.objectContaining({ juice_status: 'verified' }))
  })

  it('renders legacy rows as historical records instead of zero-sample evidence', async () => {
    history.mockResolvedValueOnce({ items: [{
      run_id: 'legacy-1', status: 'normal', source: 'historical', profile: 'unknown', mode: 'historical',
      queued_at: '2026-08-26T00:00:00Z', finished_at: '2026-08-26T00:00:00Z',
    }], next_cursor: '' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()
    const text = wrapper.get('[data-test="detection-history-panel"]').text()
    expect(text).toContain('历史记录')
    expect(text).not.toContain('0/0')
    expect(text).not.toContain('unknown')
    expect(text).not.toContain('--')
  })

  it('does not turn missing detector samples into a zero-sample result', async () => {
    history.mockResolvedValueOnce({ items: [{
      run_id: 'missing-samples-1', status: 'normal', source: 'current', profile: 'medium', mode: 'monitor',
      planned_requests: 49, valid_samples: 0, evidence_state: 'complete', juice_status: 'verified',
      fingerprint_status: 'unavailable', finished_at: '2026-08-26T00:00:00Z',
    }], next_cursor: '' })
    const wrapper = mount(AccountModelDetectionHistoryPanel, { props: { show: true, account } })
    await flushPromises()
    const text = wrapper.get('[data-test="detection-history-panel"]').text()
    expect(text).toContain('未取得证据')
    expect(text).not.toContain('0/49')
  })
})
