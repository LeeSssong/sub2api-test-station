import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorFilters from './AccountMonitorFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const accounts = [
  {
    account_id: 7,
    name: 'Primary',
    platform: 'openai',
    account_type: 'api_key',
    status: 'active',
    schedulable: true,
    management_state: 'enabled',
    service_state: 'available',
    group_eligibility: 'eligible',
    monitor_bucket: 'available',
    group_ids: [9, 3],
    group_names: ['Zulu', 'Alpha'],
    model_id: 'gpt-5.4',
    latest_status: 'success',
    sample_count: 1,
    success_rate: 1,
    multiplier: { value: 0.1, source: 'declared', status: 'ok' },
    request_count: 1,
    error_count: 0,
    stale: false,
  },
  {
    account_id: 8,
    name: 'Secondary',
    platform: 'openai',
    account_type: 'api_key',
    status: 'active',
    schedulable: true,
    management_state: 'enabled',
    service_state: 'available',
    group_eligibility: 'eligible',
    monitor_bucket: 'available',
    group_ids: [3, 5],
    group_names: ['Alpha', 'Beta'],
    model_id: 'gpt-5.4',
    latest_status: 'success',
    sample_count: 1,
    success_rate: 1,
    multiplier: { value: 0.2, source: 'measured', status: 'ok' },
    request_count: 1,
    error_count: 0,
    stale: false,
  },
]

describe('AccountMonitorFilters', () => {
  it('catches the rejected V3 platform selector regression by rendering only search and one status selector', () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: { search: '', platform: '', status: '', accounts },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.find('[data-test="group-filter"]').exists()).toBe(false)
    expect(wrapper.findAll('select')).toHaveLength(1)
    expect(wrapper.find('[data-test="platform-filter"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="status-filter"]').attributes('aria-label')).toBe('admin.accountMonitor.filters.status')
  })

  it('uses exactly the five Chinese monitor-bucket options for status filtering', () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: { search: '', platform: '', status: '', accounts },
      global: { stubs: { Icon: true } },
    })

    const options = wrapper.get('[data-test="status-filter"]').findAll('option').map((option) => ({ value: option.attributes('value'), text: option.text() }))
    expect(options).toEqual([
      { value: '', text: '全部状态' },
      { value: 'available', text: '可用' },
      { value: 'unavailable', text: '不可用' },
      { value: 'cost_ineligible', text: '成本不合格' },
      { value: 'pending', text: '待确认' },
      { value: 'paused', text: '暂停' },
    ])
  })

})
