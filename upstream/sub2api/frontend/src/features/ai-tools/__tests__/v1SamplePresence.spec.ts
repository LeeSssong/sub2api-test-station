import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import V1 from '@/views/user/ChannelStatusV1View.vue'
import Detail from '@/components/user/MonitorDetailDialog.vue'
import Grid from '@/components/user/monitor/MonitorCardGrid.vue'
import Hero from '@/components/user/monitor/MonitorHero.vue'
const api=vi.hoisted(()=>({list:vi.fn(),detail:vi.fn()}))
vi.mock('@/api/channelMonitor',()=>({list:api.list,status:api.detail}))
vi.mock('@/stores/app',()=>({useAppStore:()=>({cachedPublicSettings:{},showError:vi.fn()})}))
const i18n=()=>createI18n({legacy:false,locale:'zh',missingWarn:false,fallbackWarn:false,messages:{zh:{common:{unknown:()=> '未知'},monitorCommon:{status:{unknown:'未知'}},channelStatus:{overall:{operational:'正常',degraded:'降级'}}}}})
const stubs={AppLayout:{template:'<div><slot/></div>'},MonitorCardGrid:true,MonitorDetailDialog:true}
beforeEach(()=>{vi.clearAllMocks()})
it.each([
 [[], 'unknown'],
 [[{primary_status:''}], 'unknown'],
 [[{primary_status:'operational'},{primary_status:''}], 'unknown'],
 [[{primary_status:'failed'},{primary_status:''}], 'degraded'],
 [[{primary_status:'operational'}], 'operational'],
])('embedded overall status preserves observed failures and unknown coverage: %j',async(items,status)=>{
 api.list.mockResolvedValue({items})
 const w=mount(V1,{props:{embedded:true,toolId:'codex'},global:{plugins:[i18n()],stubs}});await flushPromises()
 expect(w.findComponent(Hero).props('overallStatus')).toBe(status);w.unmount()
})
it('renders unknown overall with a neutral chip',()=>{
 const w=mount(Hero,{props:{overallStatus:'unknown' as any,window:'7d',intervalSeconds:60,loading:false},global:{plugins:[i18n()]}})
 expect(w.text()).toContain('未知');expect(w.html()).not.toContain('bg-amber-100');w.unmount()
})
it.each([
 [{latest_status:'',has_samples_7d:false,has_samples_15d:false,has_samples_30d:false},['-','-','-']],
 [{latest_status:'operational',has_samples_7d:false,has_samples_15d:true,has_samples_30d:true,availability_15d:100,availability_30d:100},['-','100.00%','100.00%']],
 [{latest_status:'failed',has_samples_7d:true,has_samples_15d:true,has_samples_30d:true},['0.00%','0.00%','0.00%']],
 [{latest_status:'operational'},['-','-','-']],
])('inline detail uses independent window sample flags: %j',async(fixture,expected)=>{
 api.detail.mockResolvedValue({models:[{model:'model',availability_7d:0,availability_15d:0,availability_30d:0,latest_latency_ms:null,avg_latency_7d_ms:null,...fixture}]})
 const w=mount(Detail,{props:{inline:true,show:true,toolId:'codex',monitorId:1,title:'model'},global:{plugins:[i18n()]}});await flushPromises()
 expect(w.findAll('tbody tr td').slice(3,6).map(td=>td.text())).toEqual(expected);w.unmount()
})
it.each(['7d','15d','30d'] as const)('grid does not infer %s from latest status or an old timeline',(window)=>{
 const items=[{id:1,primary_model:'model',primary_status:'operational',availability_7d:0,has_samples_7d:false,timeline:[{status:'operational'}]},{id:2,primary_model:'model',primary_status:'failed',availability_7d:0,has_samples_7d:true,timeline:[{status:'failed'}]}]
 const w=mount(Grid,{props:{hideUnobserved:true,items:items as any,window,countdownSeconds:60,loading:false,detailCache:{1:{models:[{model:'model',availability_15d:100,has_samples_15d:false,availability_30d:100,has_samples_30d:false}]} as any,2:{models:[{model:'model',availability_15d:0,has_samples_15d:true,availability_30d:0,has_samples_30d:true}]} as any}},global:{plugins:[i18n()],stubs:{MonitorCard:{name:'MonitorCard',props:['availabilityValue'],template:'<span />'}}}})
 const values=()=>w.findAllComponents({name:'MonitorCard'}).map(c=>c.props('availabilityValue'))
 expect(values()).toEqual([null,0]);w.unmount()
})
it('independent detail retains its prior number display without new sample fields',async()=>{
 api.detail.mockResolvedValue({models:[{model:'model',latest_status:'',availability_7d:0,availability_15d:0,availability_30d:0}]})
 const w=mount(Detail,{props:{show:true,monitorId:1,title:'model'},global:{plugins:[i18n()],stubs:{BaseDialog:{template:'<div><slot/></div>'}}}});await flushPromises()
 expect(w.findAll('tbody tr td').slice(3,6).map(td=>td.text())).toEqual(['0.00%','0.00%','0.00%']);w.unmount()
})
