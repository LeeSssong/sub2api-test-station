import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UserErrorRequestsTable from '../UserErrorRequestsTable.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'usage.detail.action' ? 'Details' : key,
    }),
  }
})

const DataTableStub = {
  name: 'DataTable',
  props: ['columns', 'data'],
  emits: ['rowClick', 'sort'],
  template: `
    <div data-testid="error-row" @click="$emit('rowClick', data[0])">
      <span>row</span>
      <slot name="cell-detail" :row="data[0]" />
    </div>
  `,
}

const UserErrorDetailModalStub = {
  name: 'UserErrorDetailModal',
  props: ['show', 'errorId'],
  emits: ['update:show'],
  template: '<div data-testid="user-error-detail-modal" />',
}

const row = {
  id: 7,
  user_id: 11,
  status_code: 500,
  category: 'upstream_error',
  model: 'gpt-5.4',
  message: 'upstream failed',
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

function mountTable(visibleColumnKeys?: string[]) {
  return mount(UserErrorRequestsTable, {
    props: {
      rows: [row],
      total: 1,
      loading: false,
      page: 1,
      pageSize: 20,
      visibleColumnKeys,
    },
    global: {
      stubs: {
        DataTable: DataTableStub,
        EmptyState: true,
        Pagination: true,
        UserErrorDetailModal: UserErrorDetailModalStub,
        IpGeoCell: true,
        IpGeoBatchToolbar: true,
        Icon: true,
      },
    },
  })
}

describe('UserErrorRequestsTable', () => {
  it('opens the same owned detail from the explicit action and the row', async () => {
    const wrapper = mountTable()

    const detailAction = wrapper.get('[data-testid="user-error-detail-action"]')
    expect(detailAction.text()).toBe('Details')
    expect(detailAction.attributes('title')).toBe('Details')
    expect(detailAction.attributes('aria-label')).toBe('Details')
    expect(detailAction.classes()).toEqual(expect.arrayContaining(['min-h-9', 'min-w-24']))

    await detailAction.trigger('click')
    expect(wrapper.getComponent({ name: 'DataTable' }).emitted('rowClick')).toBeUndefined()
    expect(wrapper.getComponent({ name: 'UserErrorDetailModal' }).props('errorId')).toBe(7)

    await wrapper.get('[data-testid="error-row"]').trigger('click')
    expect(wrapper.getComponent({ name: 'DataTable' }).emitted('rowClick')).toHaveLength(1)
    expect(wrapper.getComponent({ name: 'UserErrorDetailModal' }).props('errorId')).toBe(7)
  })

  it('keeps only the user detail column visible when optional columns omit it', () => {
    const wrapper = mountTable(['model'])
    const columns = wrapper.getComponent({ name: 'DataTable' }).props('columns') as Array<{
      key: string
      class?: string
    }>

    expect(columns.map((column) => column.key)).toEqual(['model', 'detail'])
    expect(columns.at(-1)).toEqual(expect.objectContaining({
      key: 'detail',
      class: 'w-24 min-w-24',
    }))
    expect(columns.map((column) => column.key)).not.toContain('actions')
  })
})
