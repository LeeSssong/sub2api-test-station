import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'

const { createMonitor, updateMonitor, listTemplates, getAllGroups } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  updateMonitor: vi.fn(),
  listTemplates: vi.fn(),
  getAllGroups: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: updateMonitor,
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
    groups: {
      getAll: getAllGroups,
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const SelectStub = defineComponent({
  name: 'MonitorGroupSelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean], default: null },
    options: { type: Array, default: () => [] },
    id: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  template: `
    <button
      v-if="id === 'monitor-group'"
      type="button"
      data-testid="monitor-group-select"
      @click="$emit('update:modelValue', options[0]?.value ?? null)"
    >
      select group
    </button>
    <div v-else />
  `,
})

function monitor(overrides: Partial<ChannelMonitor> = {}): ChannelMonitor {
  return {
    id: 13,
    name: 'GPT monitor',
    provider: 'openai',
    api_mode: 'responses',
    endpoint: 'https://example.com',
    api_key_masked: 'sk-a***',
    primary_model: 'gpt-5.6-sol',
    extra_models: [],
    group_name: 'GPT-PLUS-内测',
    group_id: 16,
    enabled: true,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-07-30T00:00:00Z',
    updated_at: '2026-07-30T00:00:00Z',
    primary_status: '',
    primary_latency_ms: null,
    availability_7d: 0,
    extra_models_status: [],
    template_id: null,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
    ...overrides,
  }
}

function mountDialog(existing: ChannelMonitor | null = null) {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: existing },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        Select: SelectStub,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
}

describe('channel monitor group association', () => {
  beforeEach(() => {
    createMonitor.mockReset().mockResolvedValue({})
    updateMonitor.mockReset().mockResolvedValue({})
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    getAllGroups.mockReset().mockResolvedValue([
      { id: 16, name: 'GPT-PLUS-内测', platform: 'openai', status: 'active' },
    ])
  })

  it('submits stable group_id together with the selected group name', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    await wrapper.get('[data-testid="monitor-group-select"]').trigger('click')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.namePlaceholder"]').setValue('GPT monitor')
    await wrapper.get('input[data-testid="monitor-endpoint"]').setValue('https://example.com')
    await wrapper.get('input[type="password"]').setValue('sk-test')
    await wrapper.get('input[data-testid="monitor-primary-model"]').setValue('gpt-5.6-sol')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      group_id: 16,
      group_name: 'GPT-PLUS-内测',
    }))
  })

  it('sends clear_group when an existing stable association is cleared', async () => {
    const wrapper = mountDialog(monitor())
    await flushPromises()

    const select = wrapper.getComponent(SelectStub)
    select.vm.$emit('update:modelValue', null)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).toHaveBeenCalledWith(13, expect.objectContaining({
      clear_group: true,
      group_name: '',
    }))
    expect(updateMonitor.mock.calls[0]?.[1]).not.toHaveProperty('group_id')
  })

  it('resolves a legacy name-only association to the active stable group ID', async () => {
    const wrapper = mountDialog(monitor({ group_id: null }))
    await flushPromises()

    expect(wrapper.getComponent(SelectStub).props('modelValue')).toBe(16)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateMonitor).toHaveBeenCalledWith(13, expect.objectContaining({
      group_id: 16,
      group_name: 'GPT-PLUS-内测',
    }))
  })
})
