import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Dashboard from '../DashboardView.vue'
import { clearDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
const mocks=vi.hoisted(()=>({groups:vi.fn(),rates:vi.fn(),keys:vi.fn(),snapshot:vi.fn(),check:vi.fn(),push:vi.fn(),authUser:{id:7}}))
vi.mock('vue-router',()=>({useRouter:()=>({push:mocks.push})}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({user:mocks.authUser})}))
vi.mock('@/api/groups',()=>({default:{getAvailable:mocks.groups,getUserGroupRates:mocks.rates}}))
vi.mock('@/api/keys',()=>({default:{list:mocks.keys}}))
vi.mock('@/features/monitor-v4/api',()=>({getHybridPerformanceSnapshot:mocks.snapshot}))
vi.mock('@/features/ai-tools/api',()=>({checkLines:mocks.check}))
const groups=[{id:1,name:'GPT-Pro',platform:'openai',rate_multiplier:1,status:'active'},{id:2,name:'未关联线路',platform:'openai',rate_multiplier:.5,status:'active'}]
const metric=(id:number)=>({id,tool_ids:['codex'],success_rate:98,request_count:100,success_count:98,ttft_p50_ms:2160,latency_p50_ms:6500,current_operational:true,source_updated_at:new Date().toISOString()})
const deferred=<T,>()=>{let resolve!:(value:T)=>void;let reject!:(reason?:unknown)=>void;const promise=new Promise<T>((res,rej)=>{resolve=res;reject=rej});return {promise,resolve,reject}}
const make=()=>mount(Dashboard,{global:{stubs:{AppLayout:{template:'<div><slot/></div>'},BaseDialog:{props:['show','title'],template:'<div v-if="show" role="dialog"><h3>{{title}}</h3><slot/><slot name="footer"/></div>'},CreateLineKeyDialog:{props:['show','initialGroupId'],template:'<div v-if="show" data-testid="create-key">{{initialGroupId}}</div>'}}}})
beforeEach(()=>{vi.clearAllMocks();clearDashboardWorkspaceSnapshot();mocks.groups.mockResolvedValue(groups);mocks.rates.mockResolvedValue({});mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'inactive',group:groups[0]}],total:1});mocks.snapshot.mockResolvedValue({groups:groups.map(g=>metric(g.id))});mocks.check.mockResolvedValue([{group_id:1,status:'success',ttft_ms:1230}])})
describe('原型AI工具交互',()=>{
 it('keeps associate action even with keys and opens creation without navigation',async()=>{const w=make();await flushPromises();const b=w.findAll('button').find(b=>b.text()==='关联密钥')!;await b.trigger('click');expect(w.find('[data-testid="create-key"]').exists()).toBe(true);expect(mocks.push).not.toHaveBeenCalled();w.unmount()})
 it('only lists linked lines and starts checks without using historical latency',async()=>{const w=make();await flushPromises();const list=w.get('[aria-labelledby="routes-title"]');expect(list.text()).not.toContain('未关联线路');expect(list.text()).not.toContain('2.16s');await w.get('button[aria-label="检查线路"]').trigger('click');await flushPromises();expect(mocks.check.mock.calls[0][0]).toEqual([1]);expect(list.text()).toContain('1.23s');w.unmount()})
 it('shows all detail metrics and fetches selected period',async()=>{const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises();expect(w.get('[role="dialog"]').text()).toContain('首字 P50');expect(w.get('[role="dialog"]').text()).toContain('2.16s');await w.findAll('button').find(b=>b.text()==='近 7 天')!.trigger('click');await flushPromises();expect(mocks.snapshot.mock.calls.some(c=>c[0]==='7d')).toBe(true);w.unmount()})
 it('initial load failure shows retry instead of invented empty data',async()=>{mocks.groups.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('[role="alert"]').text()).toContain('重试');expect(w.find('.tool-grid').exists()).toBe(false);w.unmount()})
 it('shows the workspace before monitoring statistics finish loading',async()=>{const pending=deferred<{groups:ReturnType<typeof metric>[]}>();mocks.snapshot.mockReturnValue(pending.promise);const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.text()).not.toContain('正在读取 AI 工具');pending.resolve({groups:groups.map(g=>metric(g.id))});await flushPromises();w.unmount()})
 it('keeps the workspace visible when monitoring statistics fail',async()=>{mocks.snapshot.mockRejectedValue(new Error('offline'));const w=make();await flushPromises();expect(w.find('.tool-grid').exists()).toBe(true);expect(w.find('[role="alert"]').exists()).toBe(false);expect(w.text()).toContain('暂不可用');w.unmount()})
 it('restores the last successful workspace immediately when returning to the page',async()=>{const first=make();await flushPromises();first.unmount();const pendingGroups=deferred<typeof groups>();mocks.groups.mockReturnValue(pendingGroups.promise);const second=make();await flushPromises();expect(second.find('.tool-grid').exists()).toBe(true);expect(second.text()).not.toContain('正在读取 AI 工具');pendingGroups.resolve(groups);await flushPromises();second.unmount()})
 it('falls back to the group platform when no explicit tool mapping exists',async()=>{
  const anthropic={id:3,name:'default',platform:'anthropic',rate_multiplier:1,status:'active'}
  mocks.groups.mockResolvedValue([groups[0],anthropic])
  mocks.keys.mockResolvedValue({items:[{id:1,group_id:1,status:'inactive',group:groups[0]},{id:2,group_id:3,status:'active',group:anthropic}],total:2})
  mocks.snapshot.mockResolvedValue({groups:[metric(1),{...metric(3),tool_ids:[]}]})
  const w=make();await flushPromises()
  const cards=w.findAll('.tool-card')
  expect(cards[0].text()).toContain('已关联 1 把密钥')
  expect(cards[1].text()).toContain('已关联 1 把密钥')
  expect(cards[0].find('button').attributes('disabled')).toBeUndefined()
  expect(cards[1].find('button').attributes('disabled')).toBeUndefined()
  w.unmount()
 })
 it('preserves the last successful detail metrics after a same-window refresh fails',async()=>{const w=make();await flushPromises();await w.get('button[aria-label="Codex 线路详情"]').trigger('click');await flushPromises();mocks.snapshot.mockRejectedValue(new Error('offline'));await w.findAll('button').find(b=>b.text()==='近 1 小时')!.trigger('click');await flushPromises();expect(w.get('[role="dialog"]').text()).toContain('2.16s');expect(w.get('[role="dialog"]').text()).toContain('98%');w.unmount()})

})
