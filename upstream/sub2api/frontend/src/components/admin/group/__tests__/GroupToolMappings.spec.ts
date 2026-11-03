import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupToolMappings from '../GroupToolMappings.vue'
import { apiClient } from '@/api/client'
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), put: vi.fn() } }))
const record=(tool_ids:string[])=>({data:{group_id:7,tool_ids,version:1,effective_at:'',updated_by:1}})
describe('platform-based tool classification',()=>{
 beforeEach(()=>{vi.clearAllMocks();vi.mocked(apiClient.get).mockResolvedValue(record([]))})
 it('shows platform default without a redundant edit or save step',async()=>{
  const w=mount(GroupToolMappings,{props:{groupId:7,platform:'openai'}});await flushPromises()
  expect(w.text()).toContain('Codex')
  expect(w.find('input[type="checkbox"]').exists()).toBe(false)
  expect(w.text()).not.toContain('保存工具关联')
  expect(apiClient.put).not.toHaveBeenCalled()
 })
 it('exposes a historical nondefault mapping without discarding it',async()=>{
  vi.mocked(apiClient.get).mockResolvedValueOnce(record(['claude']))
  const w=mount(GroupToolMappings,{props:{groupId:7,platform:'openai'}});await flushPromises()
  expect(w.text()).toContain('历史例外')
  expect(w.text()).toContain('Claude Code')
  expect(w.text()).toContain('Codex')
  expect(w.text()).toContain('负责人')
  expect(apiClient.put).not.toHaveBeenCalled()
 })
 it('shows mapping read error and allows retry',async()=>{
  vi.mocked(apiClient.get).mockRejectedValueOnce(new Error('offline'))
  const w=mount(GroupToolMappings,{props:{groupId:7,platform:'openai'}});await flushPromises()
  expect(w.get('[role="alert"]').text()).toContain('读取失败')
  await w.get('button').trigger('click');await flushPromises()
  expect(w.text()).toContain('Codex')
 })
})
