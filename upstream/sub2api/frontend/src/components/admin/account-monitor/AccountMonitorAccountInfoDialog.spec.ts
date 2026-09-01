import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import AccountMonitorAccountInfoDialog from './AccountMonitorAccountInfoDialog.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, required: true }, title: { type: String, required: true } },
  emits: ['close'],
  template: '<div v-if="show" data-test="base-dialog"><h1>{{ title }}</h1><slot /></div>',
})

describe('AccountMonitorAccountInfoDialog', () => {
  it('shows the native account-management fields and excludes raw account payload fields', () => {
    const wrapper = mount(AccountMonitorAccountInfoDialog, {
      props: {
        show: true,
        account: {
          id: 17,
          name: 'Native account',
          platform: 'openai',
          type: 'apikey',
          status: 'active',
          schedulable: true,
          effective_schedulable: false,
          effective_schedulable_at: '2026-09-01T00:00:00Z',
          effective_unschedulable_reason: 'temp_unschedulable',
          priority: 2,
          proxy_id: 9,
          proxy: { id: 9, name: 'Primary proxy' },
          group_ids: [3],
          groups: [{ id: 3, name: 'GPT-Pro' }],
          notes: 'internal note',
          error_message: 'Bearer sk-secret-value from upstream',
          credentials: { api_key: 'sk-secret-value' },
          credentials_status: { has_api_key: true },
          rate_multiplier: 0.8,
          concurrency: 10,
          current_concurrency: 2,
          scheduler_score: { base_score: 0.8, sticky_score: 0.9, sticky_weighted_enabled: true },
          usage_windows: [{ name: '今日', utilization: 0.25, requests: 2, tokens: 100 }],
          last_used_at: null,
          expires_at: null,
          auto_pause_on_expired: false,
          created_at: '2026-08-01T00:00:00Z',
          updated_at: '2026-08-02T00:00:00Z',
        },
      } as never,
      global: { stubs: { BaseDialog: BaseDialogStub } },
    })

    expect(wrapper.text()).toContain('Native account')
    expect(wrapper.text()).toContain('Primary proxy')
    expect(wrapper.text()).toContain('2 / 10')
    expect(wrapper.text()).toContain('GPT-Pro')
    expect(wrapper.text()).toContain('25.0%')
    expect(wrapper.text()).toContain('人工调度开关')
    expect(wrapper.text()).toContain('有效调度状态')
    expect(wrapper.text()).toContain('临时不可调度')
    expect(wrapper.text()).toContain('有效状态快照')
    expect(wrapper.text()).toContain('internal note')
    expect(wrapper.text()).not.toContain('sk-secret-value')
    expect(wrapper.text()).not.toContain('Bearer')
    expect(wrapper.text()).not.toContain('credentials')
    expect(wrapper.text()).not.toContain('extra')
    expect(wrapper.find('[data-test="account-all-fields"]').exists()).toBe(false)
  })
})
