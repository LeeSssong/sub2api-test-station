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

const account = (id: number, groupIds: number[], groupNames: string[]) => ({
  account_id: id,
  name: `Account ${id}`,
  platform: 'openai',
  account_type: 'api_key',
  status: 'active',
  schedulable: true,
  group_ids: groupIds,
  group_names: groupNames,
  model_id: 'gpt-5.4',
  latest_status: 'success',
  sample_count: 3,
  success_rate: 1,
  multiplier: { value: 0.1, source: 'declared', status: 'ok' },
  request_count: 0,
  error_count: 0,
  stale: false,
})

describe('AccountMonitorFilters', () => {
  it('derives unique, stable group options with ID fallback labels', async () => {
    const wrapper = mount(AccountMonitorFilters, {
      props: {
        search: '',
        platform: '',
        status: '',
        group: '',
        accounts: [
          account(1, [9, 3], ['Zulu', 'Alpha']),
          account(2, [3, 7], ['Alpha', '']),
        ],
      },
      global: { stubs: { Icon: true } },
    })

    const options = wrapper.get('[data-test="group-filter"]').findAll('option')
    expect(options.map((option) => [option.attributes('value'), option.text()])).toEqual([
      ['', 'admin.accountMonitor.filters.allGroups'],
      ['7', '#7'],
      ['3', 'Alpha'],
      ['9', 'Zulu'],
    ])

    await wrapper.get('[data-test="group-filter"]').setValue('3')
    expect(wrapper.emitted('update:group')).toEqual([['3']])
  })
})
