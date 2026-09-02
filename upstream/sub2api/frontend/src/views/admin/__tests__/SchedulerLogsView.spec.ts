import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SchedulerLogsView from '../SchedulerLogsView.vue'

const { listLogs, getDetail } = vi.hoisted(() => ({
  listLogs: vi.fn(),
  getDetail: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { schedulerLogs: { list: listLogs, getDetail } },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const summary = {
  logical_request_id: 'req_7a91f03cd12f',
  started_at: '2026-09-03T01:46:32Z',
  canonical_model: 'gpt-5.6',
  group_id: 2,
  selected_account_id: 131,
  algorithm_version: 'openai-multi-window-quality-v2',
  runtime_retry_budget: 4,
  switch_count: 1,
  final_outcome: 'success',
}

function mountPage() {
  return mount(SchedulerLogsView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
  })
}

describe('SchedulerLogsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listLogs.mockResolvedValue({ items: [summary], next_cursor: null, incomplete: true, dropped_count: 2 })
    getDetail.mockResolvedValue({
      logical_request_id: summary.logical_request_id,
      algorithm_version: summary.algorithm_version,
      final_outcome: 'success',
      runtime_retry_budget: 4,
      switch_count: 1,
      events: [
        { id: 1, event_at: summary.started_at, attempt_number: 1, event_name: 'openai.scheduler_selection', account_id: 131, selection_layer: 'unified_quality', outcome: 'selected', decision: { selected_rank: 1, quality_score: 87.4 } },
        { id: 2, event_at: summary.started_at, attempt_number: 1, event_name: 'openai.account_model_soft_failure', account_id: 131, outcome: 'failure', decision: { status_code: 502, safe_to_replay: true, switch_reason: 'upstream_failure' } },
      ],
    })
  })

  it('renders actual runtime facts without legacy scheduler controls', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('openai-multi-window-quality-v2')
    expect(wrapper.text()).toContain('4')
    expect(wrapper.text()).toContain('1')
    expect(wrapper.find('[data-testid="scheduler-extra-retry-count"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="scheduler-save"]').exists()).toBe(false)
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
  })

  it('loads attempt details on demand and exposes failures and log gaps', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(getDetail).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="scheduler-log-row"]').trigger('click')
    await flushPromises()

    expect(getDetail).toHaveBeenCalledWith('req_7a91f03cd12f')
    expect(wrapper.text()).toContain('502')
    expect(wrapper.text()).toContain('admin.schedulerLogs.incomplete')
  })
})
