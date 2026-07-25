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
  it('derives unique group options ordered by name and id', () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: { search: '', platform: '', status: '', groupId: '', accounts },
      global: { stubs: { Icon: true } },
    })

    const options = wrapper.findAll('[data-test="group-filter"] option')
    expect(options.map((option) => option.text())).toEqual([
      'admin.accountMonitor.filters.allGroups',
      'Alpha',
      'Beta',
      'Zulu',
    ])
    expect(options.map((option) => option.attributes('value'))).toEqual(['', '3', '5', '9'])
  })

  it('emits the selected group id', async () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: { search: '', platform: '', status: '', groupId: '', accounts },
      global: { stubs: { Icon: true } },
    })

    await wrapper.get('[data-test="group-filter"]').setValue('5')
    expect(wrapper.emitted('update:groupId')).toEqual([['5']])
  })
})
