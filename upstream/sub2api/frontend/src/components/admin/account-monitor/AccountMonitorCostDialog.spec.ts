import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountMonitorCostDialog from './AccountMonitorCostDialog.vue'

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show" data-test="base-dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
}

const openAIAccount = (overrides: Record<string, unknown> = {}) => ({
  account_id: 10,
  name: 'OpenAI account',
  platform: 'openai',
  account_type: 'oauth',
  procurement_cost_cny: 4,
  estimated_usable_quota_usd: null,
  multiplier: { value: 0.08, source: 'declared', status: 'ok', sample_count: 1 },
  ...overrides,
})

const mountDialog = (account: Record<string, unknown>, overrides: Record<string, unknown> = {}) => mount(AccountMonitorCostDialog, {
  props: { show: true, account, ...overrides },
  global: { stubs: { BaseDialog: BaseDialogStub } },
})

describe('AccountMonitorCostDialog', () => {
  it('shows procurement fields for OpenAI non-api-key accounts', () => {
    const wrapper = mountDialog(openAIAccount({ account_type: 'oauth', estimated_usable_quota_usd: null }))

    expect(wrapper.get('[data-test="procurement-cost-input"]').exists()).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-test="estimated-quota-input"]').element.value).toBe('60')
    expect(wrapper.find('[data-test="cost-mode-select"]').exists()).toBe(false)
  })

  it('derives a procurement multiplier and saves both fields atomically', async () => {
    const wrapper = mountDialog(openAIAccount({ procurement_cost_cny: 4 }))

    await wrapper.get<HTMLInputElement>('[data-test="estimated-quota-input"]').setValue('120')
    expect(wrapper.get('[data-test="derived-multiplier"]').text()).toContain('0.0333')
    await wrapper.get('[data-test="save-procurement"]').trigger('click')

    expect(wrapper.emitted('saveProcurement')).toEqual([[4, 120]])
  })

  it('shows multiplier controls for OpenAI API Key accounts without a mode selector', async () => {
    const wrapper = mountDialog(openAIAccount({ account_type: 'apikey', multiplier: { value: 0.11, source: 'manual', status: 'ok', sample_count: 1 } }))

    expect(wrapper.get('[data-test="multiplier-input"]').element.value).toBe('0.11')
    expect(wrapper.find('[data-test="procurement-cost-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="cost-mode-select"]').exists()).toBe(false)

    await wrapper.get<HTMLInputElement>('[data-test="multiplier-input"]').setValue('0.2')
    await wrapper.get('[data-test="save-multiplier"]').trigger('click')
    expect(wrapper.emitted('saveMultiplier')).toEqual([[0.2]])
  })

  it('treats the underscored API Key wire spelling as a multiplier account', async () => {
    const wrapper = mountDialog(openAIAccount({ account_type: 'api_key', multiplier: { value: 0.11, source: 'manual', status: 'ok', sample_count: 1 } }))

    expect(wrapper.get('[data-test="multiplier-input"]').element.value).toBe('0.11')
    expect(wrapper.find('[data-test="procurement-cost-input"]').exists()).toBe(false)
    await wrapper.get('[data-test="save-multiplier"]').trigger('click')
    expect(wrapper.emitted('saveMultiplier')).toEqual([[0.11]])
  })

  it('uses procurement mode for OAuth accounts on every platform', () => {
    const wrapper = mountDialog(openAIAccount({ platform: 'anthropic', account_type: 'oauth', procurement_cost_cny: null }))
    expect(wrapper.find('[data-test="multiplier-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="procurement-cost-input"]').exists()).toBe(true)
  })

  it('emits restoreAuto and clear actions', async () => {
    const apiKey = mountDialog(openAIAccount({ account_type: 'apikey' }))
    await apiKey.get('[data-test="restore-auto"]').trigger('click')
    expect(apiKey.emitted('restoreAuto')).toHaveLength(1)

    const procurement = mountDialog(openAIAccount({ procurement_cost_cny: 4, estimated_usable_quota_usd: 60 }))
    await procurement.get('[data-test="clear-cost"]').trigger('click')
    expect(procurement.emitted('clear')).toHaveLength(1)
  })

  it('rejects invalid multiplier, cost, and quota values before emitting saves', async () => {
    const wrapper = mountDialog(openAIAccount({ account_type: 'oauth' }))
    await wrapper.get<HTMLInputElement>('[data-test="procurement-cost-input"]').setValue('-1')
    await wrapper.get('[data-test="save-procurement"]').trigger('click')
    expect(wrapper.emitted('saveProcurement')).toBeUndefined()
    expect(wrapper.get('[data-test="cost-error"]').text()).toContain('大于或等于 0')

    const apiKey = mountDialog(openAIAccount({ account_type: 'apikey' }))
    await apiKey.get<HTMLInputElement>('[data-test="multiplier-input"]').setValue('-0.1')
    await apiKey.get('[data-test="save-multiplier"]').trigger('click')
    expect(apiKey.emitted('saveMultiplier')).toBeUndefined()
    expect(apiKey.get('[data-test="multiplier-error"]').text()).toContain('大于或等于 0')
  })

  it('preserves draft values and displays parent errors while saving', async () => {
    const onSave = vi.fn()
    const wrapper = mountDialog(openAIAccount(), { error: '保存失败', saving: true, onSaveProcurement: onSave })
    expect(wrapper.get('[data-test="dialog-error"]').text()).toContain('保存失败')
    expect(wrapper.get('[data-test="save-procurement"]').attributes('disabled')).toBeDefined()
  })
})
