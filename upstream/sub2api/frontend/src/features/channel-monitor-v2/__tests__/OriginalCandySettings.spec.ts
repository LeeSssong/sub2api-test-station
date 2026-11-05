import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'
import zh from '@/i18n/locales/zh'
const compiled=(value:unknown):any=>typeof value==='string'?new Function('return '+baseCompile(value).code)():Object.fromEntries(Object.entries(value as object).map(([k,v])=>[k,compiled(v)]))
import OriginalCandySettings from '../OriginalCandySettings.vue'
const api=vi.hoisted(()=>({get:vi.fn(),update:vi.fn(),groups:vi.fn()}))
vi.mock('@/api/channelMonitorV2',()=>({getConfig:api.get,updateConfig:api.update}))
vi.mock('@/api/admin',()=>({adminAPI:{groups:{getAllIncludingInactive:api.groups}}}))
const config=()=>({version:17,card_timeline_mode:"windows",enabled:true,platforms:[{platform:'openai',models:['old'],enabled:true}],group_ids:[7],refresh_interval_seconds:300,exclude_sync_requests:true,health_thresholds:{minimum_sample:4},ignored_error_categories:['timeout'],candy_probes:[{group_id:7,enabled:true,model:'old-probe',reasoning_effort:'high',interval_minutes:1}],candy_history_limit:100})
const make=()=>mount(OriginalCandySettings,{global:{plugins:[createI18n({legacy:false,locale:"zh",messages:{zh:compiled(zh)}})],stubs:{Icon:true,MonitorCandySettings:{props:['modelValue','historyLimit','groups'],template:'<div data-testid="candy-form"><button @click="$emit(\'update:historyLimit\',30)">change</button></div>'}}}})
beforeEach(()=>{vi.clearAllMocks();api.get.mockResolvedValue(config());api.groups.mockResolvedValue([{id:7,name:'group',platform:'openai'}]);api.update.mockImplementation(async c=>({...c,version:18}))})
it('loads only when expanded and preserves every unrelated V2 setting on save',async()=>{
 const w=make();expect(api.get).not.toHaveBeenCalled();await w.get('summary').trigger('click');await flushPromises();await w.get('[data-testid="candy-form"] button').trigger('click');await w.get('[data-testid="original-candy-save"]').trigger('click');await flushPromises()
 expect(api.update).toHaveBeenCalledWith({...config(),candy_history_limit:30})
 expect(w.text()).toContain('已保存');w.unmount()
})
it('keeps edited values after conflicts and requires explicit reload',async()=>{
 api.update.mockRejectedValue({status:409})
 const w=make();await w.get('summary').trigger('click');await flushPromises();await w.get('[data-testid="candy-form"] button').trigger('click');await w.get('[data-testid="original-candy-save"]').trigger('click');await flushPromises()
 expect(w.get('[role="alert"]').text()).toContain('重新载入');expect(api.get).toHaveBeenCalledTimes(1)
 expect(w.get('[data-testid="original-candy-save"]').attributes('disabled')).toBeDefined()
 await w.get('[data-testid="original-candy-reload"]').trigger('click');await flushPromises();expect(api.get).toHaveBeenCalledTimes(2);w.unmount()
})
