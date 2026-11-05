import { mount, flushPromises } from '@vue/test-utils'
import { reactive, defineComponent } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import ToolRouteDetails from '../ToolRouteDetails.vue'
import type { Group } from '@/types'
const api=vi.hoisted(()=>({fetch:vi.fn(),state:null as any}))
vi.mock('@/stores/app',()=>({useAppStore:()=>api.state}))
const child=(name:string)=>defineComponent({name,props:['toolId','embedded','resumeAssociation'],emits:['load-error','title','description','associate'],template:`<div data-mode="${name}"><button class="fail" @click="$emit('load-error')">fail</button></div>`})
const stubs={ToolMonitorDetails:child('v2'),ChannelStatusV1View:child('v1'),ChannelStatusV3View:child('v3')}
const groups=[{id:1,platform:'openai',tool_ids:['codex'],tool_mapping_source:'platform'}] as Group[]
const wrappers:any[]=[]
function make(toolId='codex'){const w=mount(ToolRouteDetails,{props:{toolId,toolName:toolId,groups},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{}})],stubs}});wrappers.push(w);return w}
beforeEach(()=>{api.state=reactive({cachedPublicSettings:{channel_monitor_enabled:true,channel_monitor_mode:'v2'},fetchPublicSettings:api.fetch});api.fetch.mockReset().mockImplementation(async()=>api.state.cachedPublicSettings)})
afterEach(()=>{wrappers.forEach(w=>w.unmount());wrappers.length=0;vi.useRealTimers()})
it.each(['v1','v2','v3'])('waits for fresh settings before mounting %s only',async(mode)=>{
 api.state.cachedPublicSettings.channel_monitor_mode=mode
 let resolve!:(v:unknown)=>void;api.fetch.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 const w=make();expect(w.find('[data-mode]').exists()).toBe(false)
 resolve(api.state.cachedPublicSettings);await flushPromises()
 expect(api.fetch).toHaveBeenCalledWith(true)
 expect(w.get('[data-mode]').attributes('data-mode')).toBe(mode)
 if(mode!=='v2'){expect(w.findComponent({name:mode}).props()).toMatchObject({toolId:'codex',embedded:true})}
})
it('unmounts the old mode on public-setting changes and handles disabled and unmapped tools',async()=>{
 const w=make();await flushPromises();expect(w.find('[data-mode="v2"]').exists()).toBe(true)
 api.state.cachedPublicSettings.channel_monitor_mode='v3';await flushPromises()
 expect(w.find('[data-mode="v2"]').exists()).toBe(false);expect(w.find('[data-mode="v3"]').exists()).toBe(true)
 api.state.cachedPublicSettings.channel_monitor_enabled=false;await flushPromises()
 expect(w.find('[data-mode]').exists()).toBe(false);expect(w.find('[data-testid="tool-monitor-disabled"]').exists()).toBe(true)
 api.state.cachedPublicSettings.channel_monitor_enabled=true
 await w.setProps({toolId:'gemini'});await flushPromises()
 expect(w.find('[data-mode]').exists()).toBe(false);expect(w.find('[data-testid="tool-monitor-no-groups"]').exists()).toBe(true)
})
it('treats null settings refresh as a failure and retries without mounting stale mode',async()=>{
 api.fetch.mockResolvedValueOnce(null)
 const w=make();await flushPromises()
 expect(w.find('[role="alert"]').exists()).toBe(true);expect(w.find('[data-mode]').exists()).toBe(false)
 await w.get('[data-testid="tool-settings-retry"]').trigger('click');await flushPromises()
 expect(w.find('[data-mode="v2"]').exists()).toBe(true)
})
it('refreshes settings once after a mode rejection without looping on the same mode',async()=>{
 const w=make();await flushPromises()
 api.fetch.mockImplementationOnce(async()=>{api.state.cachedPublicSettings.channel_monitor_mode='v1';return api.state.cachedPublicSettings})
 await w.get('.fail').trigger('click');await flushPromises()
 expect(w.find('[data-mode="v1"]').exists()).toBe(true)
 await w.get('.fail').trigger('click');await flushPromises()
 expect(api.fetch).toHaveBeenCalledTimes(3)
 await flushPromises();expect(api.fetch).toHaveBeenCalledTimes(3)
})
it('checks on focus and ignores a settings response after unmount',async()=>{
 const w=make();await flushPromises()
 window.dispatchEvent(new Event('focus'));await flushPromises();expect(api.fetch).toHaveBeenCalledTimes(2)
 let resolve!:(v:unknown)=>void;api.fetch.mockImplementationOnce(()=>new Promise(r=>resolve=r))
 window.dispatchEvent(new Event('focus'));await flushPromises();w.unmount();wrappers.pop()
 resolve(null);await flushPromises();expect(w.emitted('description')?.at(-1)).not.toEqual(['stale'])
})

it('keeps native probe compatibility and avoids forcing unsupported hybrid mode into V2',async()=>{
 api.state.cachedPublicSettings.channel_monitor_mode='native_probe'
 const w=make();await flushPromises();expect(w.find('[data-mode="v1"]').exists()).toBe(true)
 api.state.cachedPublicSettings.channel_monitor_mode='hybrid_performance';await flushPromises()
 expect(w.find('[data-mode]').exists()).toBe(false)
})

it('polls public settings while open and stops polling after close',async()=>{
 vi.useFakeTimers()
 const w=make();await flushPromises()
 api.fetch.mockImplementationOnce(async()=>{api.state.cachedPublicSettings.channel_monitor_mode='v3';return api.state.cachedPublicSettings})
 await vi.advanceTimersByTimeAsync(30000);await flushPromises()
 expect(w.find('[data-mode="v3"]').exists()).toBe(true)
 const calls=api.fetch.mock.calls.length
 w.unmount();wrappers.pop();await vi.advanceTimersByTimeAsync(60000)
 expect(api.fetch).toHaveBeenCalledTimes(calls)
})
