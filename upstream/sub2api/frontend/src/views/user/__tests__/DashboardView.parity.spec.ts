vi.mock('@/features/ai-tools/routeTimeline',async importOriginal=>({...await importOriginal<typeof import('@/features/ai-tools/routeTimeline')>(),getRouteTimeline:(...args:unknown[])=>mocks.timeline(...args)}))
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Dashboard from '../DashboardView.vue'
import { clearDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
const mocks=vi.hoisted(()=>({timeline:vi.fn().mockResolvedValue([]),groups:vi.fn(),rates:vi.fn(),keys:vi.fn(),snapshot:vi.fn(),check:vi.fn(),models:vi.fn(),push:vi.fn(),authUser:{id:7}}))
vi.mock('vue-router',()=>({useRouter:()=>({push:mocks.push})}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({user:mocks.authUser})}))
vi.mock('@/api/groups',()=>({default:{getAvailable:mocks.groups,getUserGroupRates:mocks.rates}}))
vi.mock('@/api/keys',()=>({default:{list:mocks.keys}}))
vi.mock('@/features/monitor-v4/api',()=>({getHybridPerformanceSnapshot:mocks.snapshot}))
vi.mock('@/features/ai-tools/api',()=>({checkLines:mocks.check,getGroupModels:mocks.models}))
const groups=[{id:1,name:'GPT-Pro',platform:'openai',rate_multiplier:1,status:'active'},{id:2,name:'未关联线路',platform:'openai',rate_multiplier:.5,status:'active'}]
const metric=(id:number)=>({id,tool_ids:['codex'],success_rate:98,request_count:100,success_count:98,ttft_p50_ms:2160,latency_p50_ms:6500,sla_request_count:100,sla_success_count:98,source_updated_at:new Date().toISOString()})
const slaPoint=(group_id:number,request_count=100,success_count=98)=>({group_id,start:'2026-10-08T10:00:00Z',end:'2026-10-08T11:00:00Z',request_count,success_count,cache_hit_rate:null,ttft_p50_ms:null})
const deferred=<T,>()=>{let resolve!:(value:T)=>void;let reject!:(reason?:unknown)=>void;const promise=new Promise<T>((res,rej)=>{resolve=res;reject=rej});return {promise,resolve,reject}}
const make=()=>mount(Dashboard,{global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})],stubs:{AppLayout:{template:'<div><slot/></div>'},BaseDialog:{props:['show','title'],template:'<div v-if="show" role="dialog"><h3>{{title}}</h3><slot/><slot name="footer"/></div>'},CreateLineKeyDialog:{name:'CreateLineKeyDialog',props:['show','initialGroupId'],template:'<div v-if="show" data-testid="create-key">{{initialGroupId}}</div>'}}}})
beforeEach(()=>{vi.clearAllMocks();clearDashboardWorkspaceSnapshot();mocks.models.mockResolvedValue([{group_id:1,supported_models:['gpt-5.4','gpt-5.2'],official_pricing:{'gpt-5.4':{input_price:2.5e-6,cache_read_price:.25e-6,output_price:15e-6}}},{group_id:2,supported_models:['gpt-5.4','custom-model']},{group_id:99,supported_models:['private-model']}]);mocks.groups.mockResolvedValue(groups);mocks.rates.mockResolvedValue({});mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'inactive',group:groups[0]}],total:1});mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))});mocks.check.mockResolvedValue([{group_id:1,status:'success',ttft_ms:1230}])})
describe('原型AI工具交互',()=>{
 beforeEach(()=>{mocks.timeline.mockReset();mocks.timeline.mockResolvedValue(groups.map(g=>slaPoint(g.id)))})
 it('uses group SLA timeline counts for the detail total instead of the logical snapshot',async()=>{
  const point=(group_id:number,request_count:number,success_count:number,start='2026-10-08T10:00:00Z',end='2026-10-08T11:00:00Z')=>({group_id,start,end,request_count,success_count,cache_hit_rate:.5,ttft_p50_ms:1000})
  mocks.timeline.mockResolvedValueOnce([point(1,2,1),point(1,10,9,'2026-10-08T11:00:00Z','2026-10-08T12:00:00Z'),point(2,2,0)])
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  const cards=w.findAll('.route-detail-card')
  const firstCard=()=>w.findAll('.route-detail-card').find(c=>c.get('h3').text()==='GPT-Pro')!
  const first=firstCard()
  expect(firstCard().get('.success-rate').text()).toBe('83.33%')
  expect(first.find('.detail-request-sample').exists()).toBe(false)
  expect(first.get('.detail-quality-label').attributes('title')).toContain('SLA')
  expect(cards.find(c=>c.get('h3').text()==='未关联线路')!.get('.success-rate').text()).toBe('0%')
  // Detail status follows the same selected-window SLA counts as its percentage.
  expect(first.get('.route-health').text()).toBe('波动')
  mocks.timeline.mockResolvedValueOnce([point(1,12,10,'2026-10-08T00:00:00Z','2026-10-09T00:00:00Z'),point(2,2,0)])
  await w.get('select[aria-label="线路图粒度"]').setValue('day');await flushPromises()
  expect(firstCard().get('.success-rate').text()).toBe('83.33%')
  const pending=deferred<unknown[]>()
  mocks.timeline.mockImplementationOnce(()=>pending.promise)
  await w.findAll('button').find(b=>b.text()==='近 7 天')!.trigger('click');await flushPromises()
  expect(firstCard().element).not.toBe(first.element)
  expect(firstCard().get('.success-rate').text()).toBe('—')
  pending.reject(new Error('offline'));await flushPromises()
  expect(firstCard().get('.success-rate').text()).toBe('—')
  expect(w.text()).toContain('线路走势读取失败')
  w.unmount()
 })
 it('shows the line check limit and allows retry after the server cooldown',async()=>{
  vi.useFakeTimers()
  const w=make()
  try {
   await flushPromises()
   mocks.check.mockRejectedValueOnce({status:429,code:'LINE_CHECK_RATE_LIMITED',metadata:{retry_after_seconds:3}})
   const button=w.get('button[aria-label="检查线路"]')
   await button.trigger('click');await flushPromises()
   expect(w.text()).toContain('每分钟最多检查 30 次，请在 3 秒后重试。')
   expect(button.attributes('disabled')).toBeDefined()
   await button.trigger('click');await flushPromises()
   expect(mocks.check).toHaveBeenCalledTimes(1)
   await vi.advanceTimersByTimeAsync(3000);await flushPromises()
   expect(button.attributes('disabled')).toBeUndefined()
   await button.trigger('click');await flushPromises()
   expect(mocks.check).toHaveBeenCalledTimes(2)
   expect(w.text()).not.toContain('每分钟最多检查')
  } finally { w.unmount();vi.useRealTimers() }
 })
 it('orders detail cards by effective multiplier across periods without changing the best route',async()=>{
  const candidates=[
   {id:1,name:'高倍率最佳线路',platform:'openai',rate_multiplier:1,status:'active'},
   {id:2,name:'专属低倍率',platform:'openai',rate_multiplier:.8,status:'active'},
   {id:3,name:'同倍率正常线路',platform:'openai',rate_multiplier:.2,status:'active'},
   {id:4,name:'同倍率异常线路',platform:'openai',rate_multiplier:.2,status:'inactive'},
   {id:5,name:'未知倍率',platform:'openai',rate_multiplier:NaN,status:'active'},
   {id:6,name:'零倍率',platform:'openai',rate_multiplier:1,status:'active'},
  ]
  mocks.groups.mockResolvedValue(candidates)
  mocks.keys.mockResolvedValue({items:[],total:0})
  mocks.rates.mockResolvedValue({2:.1,6:0})
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:candidates.map(g=>({
   ...metric(g.id),cache_hit_rate:.5,ttft_p50_ms:1000,
   sla_success_count:g.id===1?100:g.id===3?90:20,
  }))})
  const w=make();await flushPromises()
  expect(w.get('.tool-card .tool-best .best').text()).toBe('高倍率最佳线路')
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  for(const period of ['近 24 小时','近 1 小时','近 7 天']){
   await w.findAll('.detail-period-segment button').find(b=>b.text()===period)!.trigger('click');await flushPromises()
   const cards=w.findAll('.route-detail-card')
   expect(cards.map(card=>card.get('h3').text())).toEqual(['零倍率','专属低倍率','同倍率正常线路','同倍率异常线路','高倍率最佳线路','未知倍率'])
   expect(cards.slice(0,5).map(card=>card.get('.rate-badge').text())).toEqual(['0.0x倍率','0.1x倍率','0.2x倍率','0.2x倍率','1.0x倍率'])
   expect(cards.find(card=>card.find('.best-route-badge').exists())!.get('h3').text()).toBe('高倍率最佳线路')
  }
  w.unmount()
 })
 it('hides request counts from route detail while keeping the selected route',async()=>{
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  const cards=w.findAll('article.route-detail-card')
  expect(cards).toHaveLength(2)
  expect(cards[0].find('.detail-request-sample').exists()).toBe(false)
  expect(cards[0].find('.detail-success-hover').exists()).toBe(false)
  expect(cards[0].find('.detail-card-metrics').exists()).toBe(false)
  expect(cards[0].text()).not.toContain('耗时 P50')
  expect(cards[0].text()).not.toContain('本次检查')
  expect(cards[0].get('.success-rate').text()).toBe('98%')
  const second=cards.find(c=>c.text().includes('未关联线路'))!
  await second.get('button[data-detail-group-id="2"]').trigger('click');await flushPromises()
  expect(w.get('[data-testid="create-key"]').text()).toBe('2')
  expect(mocks.push).not.toHaveBeenCalled()
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
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  const bestCard=()=>w.findAll('.route-detail-card').find(card=>card.find('.best-route-badge').exists())!
  expect(bestCard().get('h3').text()).toBe('未关联最佳线路')
  expect(bestCard().element.firstElementChild?.classList.contains('best-route-badge')).toBe(true)
  expect(bestCard().find('.detail-card-quality .best-route-badge').exists()).toBe(false)
  await w.findAll('button').find(button=>button.text()==='近 7 天')!.trigger('click');await flushPromises()
  expect(bestCard().get('h3').text()).toBe('未关联最佳线路')
  mocks.snapshot.mockImplementation((window:string)=>Promise.resolve({
   ...snapshot(window),groups:snapshot(window).groups.map(g=>({...g,cache_hit_rate:1,ttft_p50_ms:1000,sla_success_count:g.id===1?100:90})),
  }))
  await w.findAll('button').find(button=>button.text()==='近 24 小时')!.trigger('click');await flushPromises()
  expect(w.get('.tool-card .tool-best .best').text()).toContain('已关联线路')
  expect(bestCard().get('h3').text()).toBe('已关联线路')
  w.unmount()
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
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))})
  await w.get('.ranking-error button').trigger('click');await flushPromises()
  expect(w.find('.ranking-error').exists()).toBe(false)
  expect(w.get('.tool-card .tool-best .best').text()).toBe('统计不足')
  await w.get('.tool-card .card-bottom button').trigger('click');await flushPromises()
  expect(w.get('[data-testid="create-key"]').text()).toBe('')
  w.unmount()
 })
 it('distinguishes zero SLA requests from missing timeline points in cards',async()=>{
  mocks.timeline.mockResolvedValue([slaPoint(1,0,0)])
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:[{...metric(1),sla_request_count:0,sla_success_count:0}]})
  const w=make();await flushPromises()
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  const cards=w.findAll('.route-detail-card')
  const zero=cards.find(c=>c.text().includes('GPT-Pro'))!
  const missing=cards.find(c=>c.text().includes('未关联线路'))!
  expect(zero.find('.detail-request-sample').exists()).toBe(false)
  expect(missing.find('.detail-request-sample').exists()).toBe(false)
  expect(zero.get('.success-rate').text()).toBe('暂无数据')
  expect(missing.get('.success-rate').text()).toBe('暂无数据')
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
  expect(w.findAll('.route-detail-card')).toHaveLength(5)
  w.unmount()
 })

 it('hides zero counts and distinguishes no routes from routes without request data',async()=>{
  const w=make();await flushPromises()
  const status=w.get('button[aria-label="Codex 线路详情"]')
  expect(status.findAll('[data-health-count]').map(item=>item.text())).toEqual(['正常2'])
  expect(w.get('button[aria-label="DeepSeek 线路详情"]').text()).toBe('暂无线路')
  w.unmount()
 })

 it('invalidates one-hour health after detail failure and clears only the statistics error on recovery',async()=>{
  const w=make();await flushPromises()
  expect(w.get('.tool-status').text()).toBe('正常2')
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  mocks.snapshot.mockRejectedValueOnce(new Error('offline'))
  await w.findAll('button').find(b=>b.text()==='近 1 小时')!.trigger('click');await flushPromises()
  expect(w.get('.tool-status').text()).toBe('无数据2')
  expect(w.get('.route-health').text()).toBe('正常运行')
  expect(w.get('.statistics-error').text()).toContain('统计读取失败')
  await w.get('[role="dialog"]').findAll('button').find(b=>b.text()==='重试')!.trigger('click');await flushPromises()
  expect(w.get('.tool-status').text()).toBe('正常2')
  expect(w.find('.statistics-error').exists()).toBe(false)
  w.unmount()
 })

 it.each([[0,'red'],[69.9,'red'],[70,'amber'],[89.9,'amber'],[90,'green'],[100,'green'],[null,'muted']] as const)('uses the shared success-rate tone for %s in the route list and detail cards',async(rate,tone)=>{
  mocks.timeline.mockResolvedValue(groups.map(g=>slaPoint(g.id,rate===null?0:1000,rate===null?0:rate*10)))
  mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>({...metric(g.id),success_rate:rate,request_count:rate===null?0:100,sla_request_count:rate===null?0:1000,sla_success_count:rate===null?0:rate*10}))})
  const w=make();await flushPromises()
  expect(w.get('.route-row .rate').attributes('data-tone')).toBe(tone)
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  expect(w.get('.success-rate').attributes('data-tone')).toBe(tone)
  w.unmount()
 })
 it('keeps unavailable statistics neutral instead of implying success',async()=>{
  mocks.snapshot.mockRejectedValue(new Error('offline'))
  const w=make();await flushPromises()
  expect(w.get('.route-row .rate').text()).toBe('暂无数据')
  expect(w.get('.route-row .rate').attributes('data-tone')).toBe('muted')
  w.unmount()
 })
 it('keeps the route list on one hour and detail status on its selected SLA window',async()=>{
  mocks.snapshot.mockImplementation((window:string)=>Promise.resolve({generated_at:new Date().toISOString(),groups:groups.map(g=>({...metric(g.id),sla_request_count:g.id===1?1:99,sla_success_count:window==='7d'?0:g.id===1?0:99}))}))
  mocks.timeline.mockImplementation((window:string)=>Promise.resolve(groups.map(g=>slaPoint(g.id,window==='7d'?100:g.id===1?1:99,window==='7d'?0:g.id===1?0:99))))
  const w=make();await flushPromises()
  expect(w.get('.tool-status').findAll('[data-health-count]').map(item=>item.text())).toEqual(['正常1','异常1'])
  expect(w.get('.route-row .route-name').text()).toContain('异常')
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  await w.findAll('button').find(b=>b.text()==='近 7 天')!.trigger('click');await flushPromises()
  expect(w.findAll('.route-detail-card').find(r=>r.text().includes('GPT-Pro'))!.get('.route-health').text()).toBe('异常')
  expect(w.findAll('.route-detail-card').find(r=>r.text().includes('未关联线路'))!.get('.route-health').text()).toBe('异常')
  expect(w.text()).not.toMatch(/可用 ·|不可用 ·|0\/2 条/)
  w.unmount()
 })
 it('uses the effective multiplier with its suffix in both line tables',async()=>{
  mocks.rates.mockResolvedValue({1:0.12})
  const w=make();await flushPromises()
  expect(w.get('.route-row .rate-badge').text()).toBe('0.12x倍率')
  await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  expect(w.get('[role="dialog"]').text()).toContain('0.12x倍率')
  expect(w.get('[role="dialog"]').text()).toContain('0.5x倍率')
  w.unmount()
 })
 it('keeps associate action even with keys and opens creation without navigation',async()=>{const w=make();await flushPromises();const b=w.findAll('button').find(b=>b.text()==='关联密钥')!;await b.trigger('click');expect(w.find('[data-testid="create-key"]').exists()).toBe(true);expect(mocks.push).not.toHaveBeenCalled();w.unmount()})
 it('only lists linked lines and starts checks without using historical latency',async()=>{const w=make();await flushPromises();const list=w.get('[aria-labelledby="routes-title"]');expect(list.text()).not.toContain('未关联线路');expect(list.text()).not.toContain('2.16s');await w.get('button[aria-label="检查线路"]').trigger('click');await flushPromises();expect(mocks.check.mock.calls[0][0]).toEqual([1]);expect(list.text()).toContain('1.23s');w.unmount()})
 it('keeps the chart and fetches selected period without standalone latency metrics',async()=>{const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises();expect(w.find('.route-history').exists()).toBe(true);expect(w.get('[role="dialog"]').text()).not.toContain('2.16s');await w.findAll('button').find(b=>b.text()==='近 7 天')!.trigger('click');await flushPromises();expect(mocks.snapshot.mock.calls.some(c=>c[0]==='7d')).toBe(true);w.unmount()})
 it('defaults to 24 hours and forwards granularity while one hour uses five minutes',async()=>{
  const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
  expect(w.findAll('button').find(b=>b.text()==='近 24 小时')!.attributes('aria-pressed')).toBe('true')
  expect(mocks.timeline).toHaveBeenLastCalledWith('24h',expect.anything(),'hour')
  await w.get('select[aria-label="线路图粒度"]').setValue('day');await flushPromises()
  expect(mocks.timeline).toHaveBeenLastCalledWith('24h',expect.anything(),'day')
  await w.findAll('button').find(b=>b.text()==='近 1 小时')!.trigger('click');await flushPromises()
  expect(w.get('select').attributes('disabled')).toBeDefined();expect((w.get('select').element as HTMLSelectElement).value).toBe('5m');w.unmount()
 })
 it('initial load failure shows retry instead of invented empty data',async()=>{mocks.groups.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('[role="alert"]').text()).toContain('重试');expect(w.find('.tool-grid').exists()).toBe(false);w.unmount()})
 it('shows the workspace before monitoring statistics finish loading',async()=>{const pending=deferred<{groups:ReturnType<typeof metric>[]}>();mocks.snapshot.mockReturnValue(pending.promise);const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.text()).not.toContain('正在读取 AI 工具');pending.resolve({generated_at:new Date().toISOString(),groups:groups.map(g=>metric(g.id))});await flushPromises();w.unmount()})
 it('keeps the workspace visible when monitoring statistics fail',async()=>{mocks.snapshot.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.find('[role="alert"]').exists()).toBe(true);expect(w.text()).toContain('暂无数据');w.unmount()})
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
 it('preserves the last successful detail metrics after a same-window refresh fails',async()=>{const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises();mocks.snapshot.mockRejectedValue(new Error('offline'));await w.findAll('button').find(b=>b.text()==='近 24 小时')!.trigger('click');await flushPromises();expect(w.get('[role="dialog"]').text()).not.toContain('次请求成功');expect(w.get('[role="dialog"]').text()).toContain('98%');w.unmount()})

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

it('shows SLA in the route list and keeps detail status on the selected SLA window',async()=>{
 mocks.snapshot.mockResolvedValue({generated_at:new Date().toISOString(),groups:groups.map(g=>({...metric(g.id),real_request_count:841,real_success_count:660,sla_request_count:667,sla_success_count:660}))})
 mocks.timeline.mockImplementation((window:string)=>Promise.resolve(groups.map(g=>slaPoint(g.id,window==='7d'?100:667,window==='7d'?80:660))))
 const w=make();await flushPromises()
 expect(w.get('.route-row .rate').text()).toBe('98.95%')
 expect(w.get('.route-row .rate').attributes('title')).toContain('667')
 await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises()
 expect(w.get('.route-health').text()).toBe('正常运行')
 expect(w.get('.success-rate').text()).toBe('98.95%')
 await w.findAll('button').find(b=>b.text()==='近 7 天')!.trigger('click');await flushPromises()
 expect(w.get('.route-health').text()).toBe('波动')
 expect(w.get('.success-rate').text()).toBe('80%')
 w.unmount()
})
