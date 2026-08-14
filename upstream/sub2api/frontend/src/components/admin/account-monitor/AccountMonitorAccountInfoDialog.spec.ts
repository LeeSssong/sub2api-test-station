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
  it('shows account-level fields absent from the compact monitor projection without credential plaintext', () => {
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
          priority: 2,
          proxy_id: 9,
          proxy: { id: 9, name: 'Primary proxy' },
          group_ids: [3],
          notes: 'internal note',
          credentials: { api_key: 'sk-secret-value' },
          credentials_status: { has_api_key: true },
          rate_multiplier: 0.8,
          concurrency: 0,
          error_message: null,
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
    expect(wrapper.text()).toContain('已配置（1 项）')
    expect(wrapper.text()).toContain('internal note')
    expect(wrapper.text()).not.toContain('sk-secret-value')
    expect(wrapper.get('[data-test="account-info-security-note"]').text()).toContain('不展示任何凭据原文')
  })
})
