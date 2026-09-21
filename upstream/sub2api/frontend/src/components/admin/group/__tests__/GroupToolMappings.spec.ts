import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupToolMappings from '../GroupToolMappings.vue'
import { apiClient } from '@/api/client'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), put: vi.fn() } }))
const record = (tool_ids: string[]) => ({ data: { group_id: 7, tool_ids, version: 1, effective_at: '', updated_by: 1 } })
const button = (wrapper: ReturnType<typeof mount>, label: string) => wrapper.findAll('button').find(item => item.text() === label)!

describe('GroupToolMappings', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(apiClient.get).mockResolvedValue(record(['codex'])) })
  it('loads explicit mappings and saves all four choices with an independent button', async () => {
    const wrapper = mount(GroupToolMappings, { props: { groupId: 7 } })
    await flushPromises()
    expect(apiClient.get).toHaveBeenCalledWith('/admin/groups/7/tool-mappings')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(4)
    expect(wrapper.get('input[value="codex"]').element).toHaveProperty('checked', true)
    expect(wrapper.get('input[value="claude"]').element).toHaveProperty('checked', false)
    await wrapper.get('input[value="grok"]').setValue(true)
    vi.mocked(apiClient.put).mockResolvedValue(record(['codex', 'grok']))
    expect(button(wrapper, '保存工具关联').attributes('type')).toBe('button')
    expect(wrapper.find('form').exists()).toBe(false)
    await button(wrapper, '保存工具关联').trigger('click'); await flushPromises()
    expect(apiClient.put).toHaveBeenCalledWith('/admin/groups/7/tool-mappings', { tool_ids: ['codex', 'grok'] })
    expect(wrapper.text()).toContain('工具关联已保存')
  })
  it('does not infer mappings and blocks writes after an unauthorized read until retry succeeds', async () => {
    vi.mocked(apiClient.get).mockRejectedValueOnce({ response: { status: 403 } })
    const wrapper = mount(GroupToolMappings, { props: { groupId: 7 } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('读取失败')
    expect(button(wrapper, '保存工具关联').attributes('disabled')).toBeDefined()
    await button(wrapper, '重试').trigger('click'); await flushPromises()
    expect(wrapper.get('input[value="codex"]').element).toHaveProperty('checked', true)
  })
  it('preserves selections when a write is denied and allows retry', async () => {
    const wrapper = mount(GroupToolMappings, { props: { groupId: 7 } })
    await flushPromises()
    await wrapper.get('input[value="deepseek"]').setValue(true)
    vi.mocked(apiClient.put).mockRejectedValueOnce({ response: { status: 403 } })
    await button(wrapper, '保存工具关联').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('保存失败')
    expect(wrapper.get('input[value="deepseek"]').element).toHaveProperty('checked', true)
    expect(wrapper.text()).not.toContain('工具关联已保存')
    expect(button(wrapper, '保存工具关联').attributes('disabled')).toBeUndefined()
  })
})
