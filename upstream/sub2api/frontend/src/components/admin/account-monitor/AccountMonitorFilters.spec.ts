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
  it('仅提供搜索、平台和服务状态筛选，不重复提供分组范围选择', () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: { search: '', platform: '', status: '', accounts },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.find('[data-test="group-filter"]').exists()).toBe(false)
    expect(wrapper.findAll('select')).toHaveLength(2)
  })
})
