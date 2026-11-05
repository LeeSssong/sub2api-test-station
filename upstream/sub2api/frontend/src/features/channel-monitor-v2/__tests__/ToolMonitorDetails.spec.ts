import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'
const compiled=(value:unknown):any => typeof value==='string' ? new Function('return '+baseCompile(value).code)() : Object.fromEntries(Object.entries(value as object).map(([key,item])=>[key,compiled(item)]))
import ToolMonitorDetails from '../ToolMonitorDetails.vue'
import zh from '@/i18n/locales/zh'
import type { Group } from '@/types'
const api = vi.hoisted(() => ({ matrix: vi.fn(), snapshot: vi.fn(), models: vi.fn(), catalog: vi.fn() }))
vi.mock('@/api/channelMonitorV2', () => ({ getMatrix: api.matrix, getSnapshot: api.snapshot, getModels: api.models }))
vi.mock('@/features/ai-tools/api', () => ({ getGroupModels: api.catalog }))
const groups = [
 { id: 1, name: 'Codex', platform: 'openai', tool_ids: ['codex'], tool_mapping_source: 'platform' },
 { id: 2, name: '组合', platform: 'composite', tool_ids: ['codex','claude'], tool_mapping_source: 'explicit' },
 { id: 3, name: 'Claude', platform: 'anthropic', tool_ids: ['claude'], tool_mapping_source: 'platform' },
] as Group[]
const metric = { has_samples:true,request_count:0,success_rate:0,success_requests:0,error_requests:10,error_rate:1,cache_rate:0,ttft:{p50_ms:0,avg_ms:0,p90_ms:0},duration:{} }
const row = (id:number) => ({ platform: id===3?'anthropic':'openai', group_id:id,group_name:groups.find(g=>g.id===id)?.name,metrics:metric,health:{overall:'healthy',error_rate:'healthy'},buckets:[] })
const make = (toolId='codex', attached=false) => mount(ToolMonitorDetails,{attachTo:attached?document.body:undefined,props:{toolId,toolName:toolId==='codex'?'Codex':'Claude Code',groups},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{zh:compiled(zh)}})],stubs:{ProviderIcon:true}}})
beforeEach(()=>{vi.clearAllMocks();api.catalog.mockResolvedValue([{group_id:1,supported_models:['a','b','c']},{group_id:2,supported_models:[]}]);api.snapshot.mockResolvedValue({config:{enabled:true,refresh_interval_seconds:300,platforms:[],group_ids:[]}});api.matrix.mockImplementation(async ()=>({items:[row(1),row(2),row(3)],coverage:{}}));api.models.mockResolvedValue({items:[{platform:'openai',model:'unlisted-one',metrics:metric,health:{}},{platform:'openai',model:'unlisted-two',metrics:metric,health:{}}]})})
it('keeps refresh and range requests in the full tool mapping without group filtering',async()=>{
 const w=make();await flushPromises()
 expect(api.matrix.mock.calls[0][0]).toMatchObject({groupIds:[1,2],platforms:[],models:[]})
 expect(w.text()).not.toContain('Claude')
 expect(w.findAll('[data-testid="monitor-group-card"]')).toHaveLength(2)
 expect(w.find('[data-testid="tool-monitor-group"]').exists()).toBe(false)
 expect(w.find('[data-testid="tool-monitor-clear"]').exists()).toBe(false)
 await w.get('[data-range="7d"]').trigger('click');await flushPromises()
 await w.findAll('button').find(b=>b.text()==='刷新')!.trigger('click');await flushPromises()
 expect(api.matrix.mock.calls.every(([f])=>JSON.stringify(f.groupIds)==='[1,2]')).toBe(true)
 expect(api.snapshot.mock.calls.every(([f])=>JSON.stringify(f.groupIds)==='[1,2]')).toBe(true)
 w.unmount()
})
it('shows full group model data in the same surface and returns with period and focus intact',async()=>{
 const w=make();await flushPromises()
 await w.get('[data-range="7d"]').trigger('click');await flushPromises()
 const trigger=w.findAll('[data-testid="view-group-models"]')[1]
 await trigger.trigger('click');await flushPromises()
 expect(api.models.mock.calls.at(-1)?.[0]).toEqual({range:'7d',groupIds:[2],platforms:[],models:[]})
 expect(w.text()).toContain('unlisted-one');expect(w.text()).toContain('unlisted-two')
 expect(w.find('[role="dialog"]').exists()).toBe(false)
 expect(w.text()).toContain('0.00%');expect(w.text().indexOf('unlisted-one')).toBeLessThan(w.text().indexOf('unlisted-two'))
 await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
 expect(w.get('[data-range="7d"]').attributes('aria-pressed')).toBe('true')
 expect(w.emitted('title')?.at(-1)).toEqual(['Codex 线路详情'])
 w.unmount()
})
it('restores the second platform entry when a composite group has multiple platform cards',async()=>{
 api.matrix.mockResolvedValueOnce({items:[row(2),{...row(2),platform:'anthropic'}],coverage:{}})
 const w=make('codex',true)
 try {
  await flushPromises()
  const entries=w.findAll('[data-model-group-id="2"]');expect(entries).toHaveLength(2)
  await entries[1].trigger('click');await flushPromises()
  await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
  const restored=w.findAll('[data-model-group-id="2"]')
  expect(document.activeElement).toBe(restored[1].element)
 } finally { w.unmount() }
})
it('rejects late tool and model responses even if transport ignores abort',async()=>{
 let resolve!:(value:unknown)=>void
 api.matrix.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 const w=make();await flushPromises()
 await w.setProps({toolId:'claude',toolName:'Claude Code'});await flushPromises()
 resolve({items:[row(1)],coverage:{}});await flushPromises()
 expect(w.text()).not.toContain('Codex')
 expect(w.findAll('[data-testid="monitor-group-card"]').map(x=>x.text()).join(' ')).toContain('Claude')
 w.unmount()
})
it('distinguishes no mapped groups, no monitored groups, no samples and retry',async()=>{
 const w=make('gemini');await flushPromises();expect(w.text()).toContain('暂无纳入监控的线路');expect(api.matrix).not.toHaveBeenCalled();expect(api.catalog).not.toHaveBeenCalled();w.unmount()
 api.matrix.mockResolvedValueOnce({items:[],coverage:{}})
 const empty=make();await flushPromises();expect(empty.text()).toContain('暂无纳入监控的线路');empty.unmount()
 api.matrix.mockRejectedValueOnce(new Error('offline'))
 const failed=make();await flushPromises();expect(failed.get('[role="alert"]').text()).toContain('读取失败');await failed.get('[data-testid="tool-monitor-retry"]').trigger('click');await flushPromises();expect(failed.find('[role="alert"]').exists()).toBe(false);failed.unmount()
})
it.each(['windows','samples'])('uses the saved %s mode and one card grid without platform headings or local switches',async(mode)=>{
 api.snapshot.mockResolvedValueOnce({config:{card_timeline_mode:mode,platforms:[{platform:'anthropic',enabled:true},{platform:'openai',enabled:true}]}})
 api.matrix.mockResolvedValueOnce({items:[row(1),{...row(2),platform:'anthropic',sample_buckets:[{bucket_start:'2026-11-05T00:00:00Z',metrics:metric,health:{overall:'healthy'}}]}],coverage:{requested_start:'2026-11-05T00:00:00Z',requested_end:'2026-11-05T01:30:00Z',bucket_seconds:300}})
 const w=make();await flushPromises()
 expect(w.find('[data-testid="card-timeline-mode"]').exists()).toBe(false)
 expect(w.find('h2').exists()).toBe(false)
 const cards=w.findAll('[data-testid="monitor-group-card"]')
 expect(cards).toHaveLength(2)
 expect(w.find('[data-testid="card-provider"]').exists()).toBe(false)
 expect(cards[0].element.parentElement).toBe(cards[1].element.parentElement)
 expect(cards[0].findAll('[data-testid="traffic-history-bar"]')).toHaveLength(mode==='windows'?18:1)
 w.unmount()
})
it('clears pending model loading when returning so manual and timed refresh resume',async()=>{
 vi.useFakeTimers()
 let resolve!:(value:unknown)=>void
 api.models.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 const w=make();await flushPromises()
 await w.get('[data-testid="view-group-models"]').trigger('click');await flushPromises()
 await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
 const refresh=w.findAll('button').find(button=>button.text()==='刷新')!
 expect(refresh.attributes('disabled')).toBeUndefined()
 const before=api.matrix.mock.calls.length
 await refresh.trigger('click');await flushPromises();expect(api.matrix).toHaveBeenCalledTimes(before+1)
 await vi.advanceTimersByTimeAsync(300000);await flushPromises();expect(api.matrix.mock.calls.length).toBeGreaterThan(before+1)
 resolve({items:[]});await flushPromises();expect(w.text()).not.toContain('正在读取模型')
 w.unmount();vi.useRealTimers()
})
it('clears pending model state on tool changes and keeps the new tool refresh available',async()=>{
 api.models.mockImplementationOnce(()=>new Promise(()=>{}))
 const w=make();await flushPromises();await w.get('[data-testid="view-group-models"]').trigger('click');await flushPromises()
 await w.setProps({toolId:'claude',toolName:'Claude Code'});await flushPromises()
 const refresh=w.findAll('button').find(button=>button.text()==='刷新')!
 expect(refresh.attributes('disabled')).toBeUndefined();expect(w.text()).toContain('Claude');w.unmount()
})
it('does not confuse ignored errors with real model success or reorder redacted model rows',async()=>{
 api.models.mockResolvedValueOnce({items:['z-server-first','a-server-second'].map(model=>({platform:'openai',model,metrics:{...metric,has_samples:true,request_count:0,success_requests:0,error_requests:0,success_rate:0,error_rate:0},health:{}}))})
 const w=make();await flushPromises();await w.get('[data-testid="view-group-models"]').trigger('click');await flushPromises()
 expect(w.findAll('.model-main')[0].text()).toBe('0.00%')
 expect(w.text().indexOf('z-server-first')).toBeLessThan(w.text().indexOf('a-server-second'))
 await w.get('[data-range="7d"]').trigger('click');await flushPromises();await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
 expect(api.matrix.mock.calls.at(-1)?.[0].range).toBe('7d');w.unmount()
})


it('counts distinct supported models from the group catalog, independent of sampled model rows', async () => {
 api.catalog.mockResolvedValue([{group_id:1,supported_models:['a','b','c','a']},{group_id:2,supported_models:[]}])
 api.matrix.mockResolvedValueOnce({items:[row(1),row(2),{...row(2),platform:'anthropic'}],coverage:{}})
 const w=make();await flushPromises()
 expect(api.catalog).toHaveBeenCalledWith(expect.any(AbortSignal),[1,2])
 const links=w.findAll('[data-testid="view-group-models"]')
 expect(links[0].text()).toContain('支持 3 个模型')
 expect(links[1].text()).toContain('支持 0 个模型')
 expect(links[2].text()).toContain('支持 0 个模型')
 expect(links.every(link=>link.text().includes('查看全部模型表现'))).toBe(true)
 expect(api.models).not.toHaveBeenCalled()
 await links[0].trigger('click');await flushPromises()
 expect(w.text()).toContain('unlisted-one')
 await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
 expect(w.findAll('[data-testid="view-group-models"]')[0].text()).toContain('支持 3 个模型')
 await w.get('[data-range="7d"]').trigger('click');await flushPromises()
 expect(w.findAll('[data-testid="view-group-models"]')[0].text()).toContain('支持 3 个模型')
 w.unmount()
})
it('keeps line statistics usable when model counts fail, then retries on refresh', async () => {
 api.catalog.mockRejectedValueOnce(new Error('offline'))
 const w=make();await flushPromises()
 expect(w.find('[role="alert"]').exists()).toBe(false)
 expect(w.findAll('[data-testid="monitor-group-card"]')).toHaveLength(2)
 expect(w.findAll('[data-testid="view-group-models"]')[0].text()).toContain('模型数量暂不可用')
 expect(w.text()).not.toContain('支持 0 个模型')
 await w.findAll('button').find(b=>b.text()==='刷新')!.trigger('click');await flushPromises()
 expect(w.findAll('[data-testid="view-group-models"]')[0].text()).toContain('支持 3 个模型')
 w.unmount()
})
it('does not treat a missing group catalog entry as zero supported models', async () => {
 api.catalog.mockResolvedValueOnce([{group_id:1,supported_models:['a']}])
 const w=make();await flushPromises()
 expect(w.findAll('[data-testid="view-group-models"]')[1].text()).toContain('模型数量暂不可用')
 w.unmount()
})
it('shows count loading without blocking cards and ignores late catalog results after changing tools', async () => {
 let resolve!:(value:unknown)=>void
 api.catalog.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 const w=make();await flushPromises()
 expect(w.findAll('[data-testid="monitor-group-card"]')).toHaveLength(2)
 expect(w.findAll('[data-testid="view-group-models"]')[0].text()).toContain('模型数量加载中')
 api.catalog.mockResolvedValueOnce([{group_id:3,supported_models:['claude-a']}])
 await w.setProps({toolId:'claude',toolName:'Claude Code'});await flushPromises()
 resolve([{group_id:3,supported_models:['wrong1','wrong2','wrong3']}]);await flushPromises()
 expect(w.get('[data-model-group-id="3"]').text()).toContain('支持 1 个模型')
 expect(w.text()).not.toContain('支持 3 个模型')
 w.unmount()
})


it('associates the clicked active authorized group instead of the best route or another platform', async () => {
 const active = groups.map(g => ({...g,status:'active'})) as Group[]
 const w=make();await w.setProps({groups:active});await flushPromises()
 const button=w.findAll('[data-testid="associate-group-key"]')[1]
 expect(button.attributes('aria-label')).toContain('组合')
 await button.trigger('click')
 expect(w.emitted('associate')?.[0]).toEqual([2,expect.objectContaining({range:'24h',platform:'openai'})])
 expect(api.models).not.toHaveBeenCalled()
 w.unmount()
})
it('does not enable association for stopped or missing authorized groups', async () => {
 const w=make();await w.setProps({groups:groups.map(g=>({...g,status:g.id===1?'active':'inactive'})) as Group[]});await flushPromises()
 const buttons=w.findAll('[data-testid="associate-group-key"]')
 expect(buttons[0].attributes('disabled')).toBeUndefined()
 expect(buttons[1].attributes('disabled')).toBeDefined()
 await buttons[1].trigger('click')
 expect(w.emitted('associate')).toBeUndefined()
 w.unmount()
})

it('resumes the range and clicked group action after the create dialog is cancelled', async () => {
 const w=make('codex',true)
 try {
  await w.setProps({groups:groups.map(g=>({...g,status:'active'})) as Group[],resumeAssociation:{range:'7d',groupId:2,platform:'openai',scrollTop:120}})
  await flushPromises()
  expect(w.get('[data-range="7d"]').attributes('aria-pressed')).toBe('true')
  expect(document.activeElement).toBe(w.findAll('[data-testid="associate-group-key"]')[1].element)
 } finally {w.unmount()}
})

it('marks the highest server score in the current tool, including distinct platform entries of a composite group',async()=>{
 api.matrix.mockResolvedValueOnce({items:[{...row(1),health:{overall:'healthy',score:86}},{...row(2),health:{overall:'healthy',score:92}},{...row(2),platform:'anthropic',health:{overall:'healthy',score:97}},{...row(3),health:{overall:'healthy',score:100}}],coverage:{}})
 const w=make();await flushPromises()
 const best=w.findAll('[data-testid="best-line-badge"]')
 expect(best).toHaveLength(1)
 expect(best[0].element.closest('article')?.querySelector('[data-associate-platform]')?.getAttribute('data-associate-platform')).toBe('anthropic')
 expect(best[0].element.closest('article')?.querySelector('[data-associate-group-id]')?.getAttribute('data-associate-group-id')).toBe('2')
 w.unmount()
})
it.each([
 ['missing',undefined,'healthy',true],['null',null,'healthy',true],['not finite',Infinity,'healthy',true],['NaN',NaN,'healthy',true],
 ['unscored',100,'unknown',true],['no samples',100,'healthy',false],['out of range',101,'healthy',true]
])('excludes %s from best lines while accepting a valid zero score with redacted counts',async(_label,score,overall,hasSamples)=>{
 api.matrix.mockResolvedValueOnce({items:[{...row(1),metrics:{...metric,has_samples:hasSamples},health:{overall,score}},{...row(2),health:{overall:'critical',score:0}}],coverage:{}})
 const w=make();await flushPromises()
 expect(w.findAll('[data-testid="best-line-badge"]')).toHaveLength(1)
 expect(w.get('[data-testid="best-line-badge"]').element.closest('article')?.querySelector('[data-associate-group-id]')?.getAttribute('data-associate-group-id')).toBe('2')
 w.unmount()
})
it('marks ties without inventing a price or traffic tie breaker, and updates on range changes',async()=>{
 api.matrix.mockResolvedValueOnce({items:[1,2].map(id=>({...row(id),health:{overall:'healthy',score:94}})),coverage:{}})
 const w=make();await flushPromises()
 expect(w.findAll('[data-testid="best-line-badge"]')).toHaveLength(2)
 api.matrix.mockResolvedValueOnce({items:[{...row(1),health:{overall:'warning',score:70}},{...row(2),health:{overall:'healthy',score:95}}],coverage:{}})
 await w.get('[data-range="7d"]').trigger('click');await flushPromises()
 expect(w.findAll('[data-testid="best-line-badge"]')).toHaveLength(1)
 expect(w.get('[data-testid="best-line-badge"]').element.closest('article')?.querySelector('[data-associate-group-id]')?.getAttribute('data-associate-group-id')).toBe('2')
 w.unmount()
})
it('shows no best label when all scores are unavailable and reads availability before the two secondary metrics',async()=>{
 const w=make();await flushPromises()
 expect(w.find('[data-testid="best-line-badge"]').exists()).toBe(false)
 expect(w.get('[data-testid="monitor-group-card"]').findAll('dl dt').map(dt=>dt.text())).toEqual(['可用率','首 TOKEN','缓存率'])
 w.unmount()
})

it('passes the current helper text to the dialog header without repeating it in the body',async()=>{
 const w=make();await flushPromises()
 expect(w.emitted('description')?.at(-1)).toEqual(['基于真实请求的缓存率、可用率与首 Token 表现。'])
 expect(w.text()).not.toContain('基于真实请求')
 await w.get('[data-testid="view-group-models"]').trigger('click');await flushPromises()
 expect(w.emitted('description')?.at(-1)).toEqual([zh.channelMonitorV2.tool.modelHint])
 expect(w.text()).not.toContain(zh.channelMonitorV2.tool.modelHint)
 await w.get('[data-testid="tool-monitor-back"]').trigger('click');await flushPromises()
 expect(w.emitted('description')?.at(-1)).toEqual([zh.channelMonitorV2.tool.passiveHint])
 w.unmount()
})
