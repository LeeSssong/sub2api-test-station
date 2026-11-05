import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import V1 from '@/views/user/ChannelStatusV1View.vue'
import V3 from '@/views/user/ChannelStatusV3View.vue'
const api=vi.hoisted(()=>({list:vi.fn(),detail:vi.fn(),status:vi.fn(),incidents:vi.fn()}))
vi.mock('@/api/channelMonitor',()=>({list:api.list,status:api.detail}))
vi.mock('@/api/channelMonitorV3',()=>({getStatus:api.status,getIncidents:api.incidents}))
vi.mock('@/stores/app',()=>({useAppStore:()=>({cachedPublicSettings:{channel_monitor_enabled:true},showError:vi.fn()})}))
const stubs={AppLayout:{template:'<div data-layout><slot/></div>'},MonitorHero:{template:'<button @click="$emit(\'refresh\')">refresh</button>'},MonitorCardGrid:{props:['items'],template:'<div><button v-for="item in items" class="native-card" @click="$emit(\'card-click\',item)">{{item.name}}</button></div>'},MonitorDetailDialog:{name:'MonitorDetailDialog',props:['show','inline','toolId'],template:'<div v-if="show" data-detail><button @click="$emit(\'close\')">back</button></div>'},StatusPage:{props:['status'],template:'<div v-if="status" data-native-status><button @click="$emit(\'open-incidents\')">incidents</button></div>'},IncidentsDialog:{name:'IncidentsDialog',props:['show','inline','toolId'],template:'<div v-if="show" data-incidents />'}}
const mountView=(view:any,toolId='codex')=>mount(view,{props:{embedded:true,toolId},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})],stubs}})
beforeEach(()=>{vi.clearAllMocks();api.list.mockResolvedValue({items:[{id:1,name:'native probe'}]});api.status.mockResolvedValue({window:{latest:true},featured:null,categories:[]})})
it('embeds V1 native content with server tool scope and inline native detail',async()=>{
 const w=mountView(V1);await flushPromises()
 expect(w.find('[data-layout]').exists()).toBe(false)
 expect(api.list.mock.calls[0][0]).toMatchObject({toolId:'codex'})
 await w.get('.native-card').trigger('click');await flushPromises()
 expect(w.findComponent({name:'MonitorDetailDialog'}).props()).toMatchObject({inline:true,toolId:'codex',show:true})
 expect(w.find('.native-card').exists()).toBe(false)
 w.unmount()
})
it('embeds V3 native status and scopes incidents using the same tool',async()=>{
 const w=mountView(V3,'claude');await flushPromises()
 expect(w.find('[data-layout]').exists()).toBe(false)
 expect(api.status.mock.calls[0][3]).toBe('claude')
 await w.get('[data-native-status] button').trigger('click');await flushPromises()
 expect(w.findComponent({name:'IncidentsDialog'}).props()).toMatchObject({inline:true,toolId:'claude',show:true})
 w.unmount()
})
it.each([V1,V3])('clears old data on failure and reports mode recheck without toast-only failure',async(view)=>{
 const fn=view===V1?api.list:api.status
 fn.mockRejectedValueOnce(new Error('mode changed'))
 const w=mountView(view);await flushPromises()
 expect(w.find('[role="alert"]').exists()).toBe(true)
 expect(w.emitted('load-error')).toHaveLength(1)
 w.unmount()
})
it.each([V1,V3])('aborts pending native reads on unmount',async(view)=>{
 const fn=view===V1?api.list:api.status
 let resolve!:(v:unknown)=>void;fn.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 const w=mountView(view);await flushPromises()
 const signal=view===V1?fn.mock.calls[0][0].signal:fn.mock.calls[0][2]
 w.unmount();expect(signal.aborted).toBe(true)
 resolve(view===V1?{items:[]}:{window:{latest:true}});await flushPromises()
})

it('passes scoped stable identity and abort to native model detail without another dialog',async()=>{
 const {default:Detail}=await import('@/components/user/MonitorDetailDialog.vue')
 api.detail.mockResolvedValue({models:[{model:'probe-model',latest_status:'operational',latest_latency_ms:1200,availability_7d:99,availability_15d:98,availability_30d:97,avg_latency_7d_ms:1300}]})
 const w=mount(Detail,{props:{show:true,inline:true,toolId:'claude',monitorId:1,title:'native'},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})]}})
 await flushPromises();expect(api.detail.mock.calls[0]).toEqual([1,expect.objectContaining({toolId:'claude',signal:expect.any(AbortSignal)})])
 expect(w.find('[role="dialog"]').exists()).toBe(false);expect(w.text()).toContain('probe-model')
 const signal=api.detail.mock.calls[0][1].signal
 w.unmount();expect(signal.aborted).toBe(true)
})
it('scopes native incidents and isolates an old tool response on prop change',async()=>{
 const {default:Incidents}=await import('@/features/channel-monitor-v3/IncidentsDialog.vue')
 let resolve!:(v:unknown)=>void;api.incidents.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 api.incidents.mockResolvedValue({items:[],total:0,page:1,since:''})
 const w=mount(Incidents,{props:{show:true,inline:true,admin:false,toolId:'codex'},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})]}})
 await flushPromises();const oldSignal=api.incidents.mock.calls[0][3]
 expect(api.incidents.mock.calls[0][4]).toBe('codex')
 await w.setProps({toolId:'claude'});await flushPromises()
 expect(oldSignal.aborted).toBe(true);expect(api.incidents.mock.calls[1][4]).toBe('claude')
 resolve({items:[{component_id:1,component_name:'old forbidden tool',started_at:new Date().toISOString(),down_slots:1}],total:1,page:1});await flushPromises()
 expect(w.text()).not.toContain('old forbidden tool');expect(w.find('[role="dialog"]').exists()).toBe(false)
 w.unmount()
})
it('distinguishes an unobserved native probe from a measured zero availability',async()=>{
 const {default:Grid}=await import('@/components/user/monitor/MonitorCardGrid.vue')
 const w=mount(Grid,{props:{items:[{id:1,name:'no probe yet',primary_status:'',availability_7d:0,timeline:[]},{id:2,name:'all probes failed',primary_status:'failed',availability_7d:0,has_samples_7d:true,timeline:[{status:'failed'}]}] as any,window:'7d',countdownSeconds:60,loading:false,detailCache:{},hideUnobserved:true} as any,global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})],stubs:{MonitorCard:{name:'MonitorCard',props:['availabilityValue'],template:'<span>{{availabilityValue===null?"unknown":availabilityValue}}</span>'}}}})
 expect(w.findAllComponents({name:'MonitorCard'}).map(c=>c.props('availabilityValue'))).toEqual([null,0])
 w.unmount()
})
