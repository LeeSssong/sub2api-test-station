import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Dashboard from '../DashboardView.vue'
import { clearDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
const mocks=vi.hoisted(()=>({timeline:vi.fn().mockResolvedValue([]),groups:vi.fn(),rates:vi.fn(),keys:vi.fn(),snapshot:vi.fn(),models:vi.fn(),push:vi.fn(),authUser:{id:7}}))
vi.mock('vue-router',()=>({useRouter:()=>({push:mocks.push})}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({user:mocks.authUser})}))
vi.mock('@/api/groups',()=>({default:{getAvailable:mocks.groups,getUserGroupRates:mocks.rates}}))
vi.mock('@/api/keys',()=>({default:{list:mocks.keys}}))
vi.mock('@/features/monitor-v4/api',()=>({getHybridPerformanceSnapshot:mocks.snapshot}))
vi.mock('@/features/monitor-v4/lineSelection',()=>({getLineSelectionMetrics:vi.fn().mockResolvedValue({groups:[],generated_at:new Date().toISOString()})}))
vi.mock('@/features/ai-tools/api',()=>({getGroupModels:mocks.models}))
const groups=[{id:1,name:'GPT-Pro',platform:'openai',rate_multiplier:1,status:'active'},{id:2,name:'未关联线路',platform:'openai',rate_multiplier:.5,status:'active'}]
const metric=(id:number)=>({id,tool_ids:['codex'],success_rate:98,request_count:100,success_count:98,ttft_p50_ms:2160,latency_p50_ms:6500,sla_request_count:100,sla_success_count:98,source_updated_at:new Date().toISOString()})
const deferred=<T,>()=>{let resolve!:(value:T)=>void;let reject!:(reason?:unknown)=>void;const promise=new Promise<T>((res,rej)=>{resolve=res;reject=rej});return {promise,resolve,reject}}
const make=()=>mount(Dashboard,{global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})],stubs:{ToolRouteDetails:{name:'ToolRouteDetails',props:['toolId','toolName','groups'],template:'<section data-testid="shared-monitor"><button @click="$emit(\'title\', \'分组 · 模型表现\')">models</button></section>'},KeysView:{template:'<section id="my-keys" data-testid="merged-key-manager" />'},AppLayout:{template:'<div><slot/></div>'},BaseDialog:{props:['show','title','description'],template:'<div v-if="show" role="dialog"><h3><slot name="title">{{title}}</slot></h3><p v-if="description" class="modal-description">{{description}}</p><slot/><slot name="footer"/></div>'},CreateLineKeyDialog:{name:'CreateLineKeyDialog',props:['show','initialGroupId','recommendedGroupIds','groups'],template:'<div v-if="show" data-testid="create-key">{{initialGroupId}}</div>'}}}})
beforeEach(()=>{vi.clearAllMocks();clearDashboardWorkspaceSnapshot();mocks.models.mockResolvedValue([{group_id:1,supported_models:['gpt-5.4','gpt-5.2'],official_pricing:{'gpt-5.4':{input_price:2.5e-6,cache_read_price:.25e-6,output_price:15e-6}}},{group_id:2,supported_models:['gpt-5.4','custom-model']},{group_id:99,supported_models:['private-model']}]);mocks.groups.mockResolvedValue(groups);mocks.rates.mockResolvedValue({});mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'inactive',group:groups[0]}],total:1});mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))})})
describe('原型AI工具交互',()=>{
 it.each(['groups','rates'] as const)('keeps the key manager mounted when tool workspace %s fails',async(failure)=>{
  mocks[failure].mockRejectedValue(new Error('workspace offline'))
  const w=make();await flushPromises()
  expect(w.find('.workspace-error').exists()).toBe(true)
  expect(w.find('[data-testid="merged-key-manager"]').exists()).toBe(true)
  expect(mocks.keys).toHaveBeenCalled()
  w.unmount()
 })
 it('replaces the retired line checker with the complete key manager',async()=>{
  const w=make();await flushPromises()
  expect(w.find('[data-testid="merged-key-manager"]').exists()).toBe(true)
  expect(w.find('[aria-labelledby="routes-title"]').exists()).toBe(false)
  expect(w.find('button[aria-label="检查线路"]').exists()).toBe(false)
  w.unmount()
 })
 it('uses a 24-hour weighted score and includes active unlinked lines in the best route',async()=>{
  const candidateGroups=[
   {id:1,name:'已关联线路',platform:'openai',rate_multiplier:1,status:'active'},
   {id:2,name:'未关联最佳线路',platform:'openai',rate_multiplier:1,status:'active'},
   {id:3,name:'停用线路',platform:'openai',rate_multiplier:1,status:'inactive'},
  ]
  mocks.groups.mockResolvedValue(candidateGroups)
  mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'active',group:candidateGroups[0]}],total:1})
  const snapshot=(window:string)=>({generated_at:new Date().toISOString(),groups:candidateGroups.map(group=>({
   ...metric(group.id),
   cache_hit_rate: window==='24h' ? ({1:.9,2:1,3:1}[group.id]) : .5,
   sla_request_count:100,
   sla_success_count:window==='24h' ? ({1:100,2:99,3:100}[group.id]) : 98,
   ttft_p50_ms:window==='24h' ? ({1:3000,2:100,3:1}[group.id]) : 2160,
  }))})
  mocks.snapshot.mockImplementation((window:string)=>Promise.resolve(snapshot(window)))
  const w=make();await flushPromises()
  expect(mocks.snapshot.mock.calls.map(([window])=>window)).toContain('24h')
  expect(w.get('.tool-card .tool-best .best').text()).toContain('未关联最佳线路')
  expect(w.get('.tool-card .card-bottom').element.firstElementChild?.className).toBe('tool-best')
  await w.get('.tool-card .card-bottom button').trigger('click');await flushPromises()
  expect(w.get('[data-testid="create-key"]').text()).toBe('2')
  w.findComponent({name:'CreateLineKeyDialog'}).vm.$emit('close');await flushPromises()
  w.unmount()
 })
 it('withdraws the create recommendation when its 24-hour ranking expires',async()=>{
  vi.useFakeTimers({toFake:['Date','setInterval','clearInterval']})
  try {
   mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>({...metric(g.id),cache_hit_rate:.5,sla_success_count:g.id===1?99:80}))})
   const w=make();await flushPromises()
   const dialog=w.findComponent({name:'CreateLineKeyDialog'})
   expect((dialog.props('recommendedGroupIds') as Map<string,number>).get('codex')).toBe(1)
   vi.advanceTimersByTime(8*60*1000);await flushPromises()
   expect((dialog.props('recommendedGroupIds') as Map<string,number>).has('codex')).toBe(false)
   w.unmount()
  } finally { vi.useRealTimers() }
 })
 it('keeps the cached 24-hour best route when returning before statistics finish loading',async()=>{
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>({...metric(g.id),cache_hit_rate:.5}))})
  const first=make();await flushPromises()
  const best=first.get('.tool-card .tool-best .best').text();expect(best).toContain('GPT-Pro');first.unmount()
  const pending=deferred<unknown>();mocks.snapshot.mockReturnValue(pending.promise)
  const w=make();await flushPromises()
  expect(w.get('.tool-card .tool-best .best').text()).toBe(best)
  pending.resolve({generated_at:new Date().toISOString(),groups:[]});await flushPromises()
  w.unmount()
 })
 it('explains incomplete recommendation metrics and permits retry after a 24-hour failure',async()=>{
  mocks.snapshot.mockImplementation((window:string)=>window==='24h'
   ? Promise.reject(new Error('offline'))
   : Promise.resolve({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))}))
  const w=make();await flushPromises()
  expect(w.get('.ranking-error').text()).toContain('近 24 小时')
  expect(w.get('.tool-card .tool-best .best').text()).toBe('暂无请求数据')
  expect((w.findComponent({name:'CreateLineKeyDialog'}).props('recommendedGroupIds') as Map<string,number>).size).toBe(0)
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))})
  await w.get('.ranking-error button').trigger('click');await flushPromises()
  expect(w.find('.ranking-error').exists()).toBe(false)
  expect(w.get('.tool-card .tool-best .best').text()).toBe('统计不足')
  await w.get('.tool-card .card-bottom button').trigger('click');await flushPromises()
  expect(w.get('[data-testid="create-key"]').text()).toBe('')
  w.unmount()
 })
 it('shows all route states and exact counts on one clickable tool entry',async()=>{
  const fiveRoutes=Array.from({length:5},(_,index)=>({id:index+1,name:`线路 ${index+1}`,platform:'openai',rate_multiplier:1,status:'active'}))
  mocks.groups.mockResolvedValue(fiveRoutes)
  mocks.keys.mockResolvedValue({items:[],total:0})
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:[
   {...metric(1),sla_success_count:95},
   {...metric(2),sla_success_count:100},
   {...metric(3),sla_success_count:80},
   {...metric(4),sla_success_count:20},
   {...metric(5),sla_request_count:0,sla_success_count:0},
  ]})
  const w=make();await flushPromises()
  const status=w.get('button[aria-label="Codex 线路详情"]')
  expect(status.findAll('[data-health-count]').map(item=>item.text())).toEqual(['正常2','波动1','异常1','无数据1'])
  expect(status.attributes('title')).toBe('正常 2 条，波动 1 条，异常 1 条，无数据 1 条')
  expect(status.attributes('aria-expanded')).toBe('false')
  await status.trigger('click');await flushPromises()
  expect(status.attributes('aria-expanded')).toBe('true')
  expect(w.findComponent({name:'ToolRouteDetails'}).props('groups')).toHaveLength(5)
  w.unmount()
 })

 it('hides zero counts and distinguishes no routes from routes without request data',async()=>{
  const w=make();await flushPromises()
  const status=w.get('button[aria-label="Codex 线路详情"]')
  expect(status.findAll('[data-health-count]').map(item=>item.text())).toEqual(['正常2'])
  expect(w.get('button[aria-label="DeepSeek 线路详情"]').text()).toBe('暂无线路')
  w.unmount()
 })

 it('keeps unavailable statistics neutral instead of implying success',async()=>{
  mocks.snapshot.mockRejectedValue(new Error('offline'))
  const w=make();await flushPromises()
  expect(w.get('.tool-status').text()).toContain('无数据')
  w.unmount()
 })
 it('keeps associate action even with keys and opens creation without navigation',async()=>{const w=make();await flushPromises();const b=w.findAll('button').find(b=>b.text()==='关联密钥')!;await b.trigger('click');expect(w.find('[data-testid="create-key"]').exists()).toBe(true);expect(mocks.push).not.toHaveBeenCalled();w.unmount()})
 it('initial load failure shows retry instead of invented empty data',async()=>{mocks.groups.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('[role="alert"]').text()).toContain('重试');expect(w.find('.tool-grid').exists()).toBe(false);w.unmount()})
 it('shows the workspace before monitoring statistics finish loading',async()=>{const pending=deferred<{groups:ReturnType<typeof metric>[]}>();mocks.snapshot.mockReturnValue(pending.promise);const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.text()).not.toContain('正在读取 AI 工具');pending.resolve({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))});await flushPromises();w.unmount()})
 it('keeps the workspace visible when monitoring statistics fail',async()=>{mocks.snapshot.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.find('[role="alert"]').exists()).toBe(true);expect(w.text()).toContain('无数据');w.unmount()})
 it('restores the last successful workspace immediately when returning to the page',async()=>{const first=make();await flushPromises();first.unmount();const pendingGroups=deferred<typeof groups>();mocks.groups.mockReturnValue(pendingGroups.promise);const second=make();await flushPromises();expect(second.find('.tool-grid').exists()).toBe(true);expect(second.text()).not.toContain('正在读取 AI 工具');pendingGroups.resolve(groups);await flushPromises();second.unmount()})
 it('falls back to the group platform when no explicit tool mapping exists',async()=>{
  const anthropic={id:3,name:'default',platform:'anthropic',rate_multiplier:1,status:'active'}
  mocks.groups.mockResolvedValue([groups[0],anthropic])
  mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'inactive',group:groups[0]},{id:2,group_id:3,status:'active',group:anthropic}],total:2})
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:[metric(1),{...metric(3),tool_ids:[]}]})
  const w=make();await flushPromises()
  const cards=w.findAll('.tool-card')
  expect(cards[0].text()).not.toContain('已关联')
  expect(cards[1].text()).not.toContain('已关联')
  expect(cards[0].find('button').attributes('disabled')).toBeUndefined()
  expect(cards[1].find('button').attributes('disabled')).toBeUndefined()
  w.unmount()
 })
 it('shows current tool models newest first with native prices without changing group fees',async()=>{
  mocks.rates.mockResolvedValue({1:0.12})
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 价格与扣费说明"]').trigger('click');await flushPromises()
  const dialog=w.get('[role="dialog"]')
  expect(dialog.text()).toContain('OpenAI 官方参考价')
  const rows=dialog.findAll('.model-pricing-table tbody tr')
  expect(rows.map(r=>r.find('.model-name').text())).toEqual(['gpt-5.4','gpt-5.2','custom-model'])
  expect(dialog.text()).not.toContain('private-model')
  const gpt=()=>dialog.findAll('.model-pricing-table tbody tr').find(r=>r.find('.model-name').text()==='gpt-5.4')!
  expect(gpt().findAll('td')[0].text()).toBe('$2.50')
  expect(gpt().findAll('td')[1].text()).toBe('$0.25')
  expect(dialog.find('[role="tablist"]').exists()).toBe(false)
  expect(dialog.text()).not.toContain('待核对')
  const fees=dialog.get('.fee-table')
  expect(fees.findAll('thead th').map(th=>th.text())).toEqual(['分组','支持模型','倍率','扣费标准'])
  expect(fees.text()).toContain('gpt-5.4、gpt-5.2')
  expect(fees.text()).toContain('官方计费金额 × 0.12')
  expect(dialog.text()).not.toContain('Input')
  w.unmount()
 })
 it('recovers model loading failure through retry without hiding the workspace',async()=>{
  mocks.models.mockRejectedValueOnce(new Error('offline'))
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 价格与扣费说明"]').trigger('click');await flushPromises()
  expect(w.get('.tool-grid').exists()).toBe(true)
  expect(w.get('[role="dialog"]').text()).toContain('读取失败')
  await w.get('[role="dialog"]').findAll('button').find(b=>b.text()==='重试')!.trigger('click');await flushPromises()
  expect(w.get('[role="dialog"]').text()).toContain('gpt-5.4')
  expect(w.get('[role="dialog"]').text()).not.toContain('读取失败')
  w.unmount()
 })
 it('does not invent supported models for groups with no configuration',async()=>{
  mocks.models.mockResolvedValue([{group_id:1,supported_models:[]}])
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 价格与扣费说明"]').trigger('click');await flushPromises()
  expect(w.get('[role="dialog"]').text()).toContain('暂无可用模型')
  expect(w.get('.fee-table').text()).toContain('暂无可用模型')
  expect(w.find('.model-pricing-table').exists()).toBe(false)
  w.unmount()
 })

})


describe('Gemini 第五工具',()=>{
 it('offers only active Gemini groups through the existing key creation dialog',async()=>{
  const gemini={id:5,name:'Gemini 示例',platform:'gemini',rate_multiplier:1,status:'active'}
  mocks.groups.mockResolvedValue([...groups,gemini,{...gemini,id:6,status:'inactive'}])
  const w=make();await flushPromises()
  expect(w.findAll('.tool-title').map(c=>c.text())).toEqual(['Codex','Claude','Gemini','DeepSeek','Grok'])
  const card=w.findAll('.tool-card').find(c=>c.text().includes('Gemini'))!
  await card.get('.card-bottom button').trigger('click');await flushPromises()
  const dialog=w.findComponent({name:'CreateLineKeyDialog'})
  expect(dialog.attributes('tool-id')).toBe('gemini')
  expect((dialog.attributes('tool-name'))).toBe('Gemini')
  expect((dialog.props('groups') as {id:number}[]).map(g=>g.id)).toEqual([5])
  w.unmount()
 })
 it('keeps Gemini discoverable without available groups and explains disabled creation',async()=>{
  const w=make();await flushPromises()
  const card=w.findAll('.tool-card').find(c=>c.text().includes('Gemini'))!
  expect(card.get('.card-bottom button').attributes('disabled')).toBeDefined()
  expect(card.text()).toContain('暂无可用 Gemini 分组')
  w.unmount()
 })
 it('hosts V2 route and model content inside the existing tool dialog without requiring keys',async()=>{
  mocks.keys.mockResolvedValue({items:[],total:0})
  const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  const monitor=w.findComponent({name:'ToolRouteDetails'})
  expect(monitor.props('toolId')).toBe('codex');expect(monitor.props('groups')).toEqual(groups)
  expect(w.findAll('[role="dialog"]')).toHaveLength(1)
  await w.get('[data-testid="shared-monitor"] button').trigger('click');await flushPromises()
  expect(w.get('[role="dialog"] h3').text()).toBe('分组 · 模型表现')
  expect(mocks.timeline).not.toHaveBeenCalled();w.unmount()
 })

 it('opens the existing key dialog with the chosen detail group and returns on cancellation',async()=>{
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  w.findComponent({name:'ToolRouteDetails'}).vm.$emit('associate',2);await flushPromises()
  expect(w.get('[data-testid="create-key"]').text()).toBe('2')
  expect(w.findAll('[role="dialog"]')).toHaveLength(0)
  w.findComponent({name:'CreateLineKeyDialog'}).vm.$emit('close');await flushPromises()
  expect(w.find('[data-testid="create-key"]').exists()).toBe(false)
  expect(w.get('[role="dialog"] h3').text()).toBe('Codex 线路详情')
  expect(mocks.push).not.toHaveBeenCalled()
  w.unmount()
 })
 it('rejects unauthorized and stopped detail groups without opening a fallback key route',async()=>{
  mocks.groups.mockResolvedValue([...groups,{id:3,name:'停用',platform:'openai',status:'inactive'}])
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  for(const id of [99,3]){
   w.findComponent({name:'ToolRouteDetails'}).vm.$emit('associate',id);await flushPromises()
   expect(w.find('[data-testid="create-key"]').exists()).toBe(false)
   expect(w.find('[role="dialog"]').exists()).toBe(true)
  }
  w.unmount()
 })

})

it('places tool helper events under the dialog title rather than inside its heading',async()=>{
 const w=make();await flushPromises()
 await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
 w.findComponent({name:'ToolRouteDetails'}).vm.$emit('description','线路辅助说明');await flushPromises()
 expect(w.get('[role="dialog"] .modal-description').text()).toBe('线路辅助说明')
 expect(w.get('[role="dialog"] h3').text()).toBe('Codex 线路详情')
 w.unmount()
})

it('uses the native radial Claude mark in both tool and detail heading',async()=>{
 const w=make();await flushPromises()
 const claude=w.findAll('.tool-card').find(card=>card.find('.tool-title').text()==='Claude')!
 expect(claude.find('.provider-logo').element.tagName.toLowerCase()).toBe('svg')
 expect(claude.find('.provider-logo path').exists()).toBe(true)
 await claude.get('button[aria-label="Claude 线路详情"]').trigger('click');await flushPromises()
 expect(w.get('.route-dialog-title').text()).toBe('Claude 线路详情')
 expect(w.get('.route-dialog-title .provider-logo').element.tagName.toLowerCase()).toBe('svg')
 w.unmount()
})
