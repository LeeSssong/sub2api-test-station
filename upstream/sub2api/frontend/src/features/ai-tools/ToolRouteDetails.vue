<template>
  <section class="tool-route-details" data-testid="tool-route-details">
    <div v-if="settingsError" class="route-message" role="alert">
      {{ t('channelMonitorV2.tool.settingsError') }}
      <button class="btn btn-secondary" data-testid="tool-settings-retry" :disabled="refreshing" @click="refreshSettings">{{ t('channelMonitorV2.tool.retry') }}</button>
    </div>
    <div v-else-if="!ready" class="route-message" role="status">{{ t('common.loading') }}</div>
    <div v-else-if="!enabled" class="route-message" role="status" data-testid="tool-monitor-disabled">{{ t('channelMonitorV2.tool.disabled') }}</div>
    <div v-else-if="!hasGroups" class="route-message" role="status" data-testid="tool-monitor-no-groups">{{ t('channelMonitorV2.tool.noGroups') }}</div>
    <ToolMonitorDetails v-else-if="mode === 'v2'" :key="`v2:${toolId}`" v-bind="props" @title="emit('title', $event)" @description="emit('description', $event)" @associate="(id, resume) => emit('associate', id, resume)" @load-error="refreshSettings" />
    <ChannelStatusV1View v-else-if="mode === 'v1' || mode === 'native_probe'" :key="`v1:${toolId}`" :embedded="true" :tool-id="toolId" @load-error="refreshSettings" />
    <ChannelStatusV3View v-else-if="mode === 'v3'" :key="`v3:${toolId}`" :embedded="true" :tool-id="toolId" @load-error="refreshSettings" />
    <div v-else class="route-message" role="status">{{ t('channelMonitorV2.tool.unsupportedMode') }}</div>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import type { Group } from '@/types'
import type { MonitorRange } from '@/api/channelMonitorV2'
import { toolIdsForGroup } from './model'
import ToolMonitorDetails from '@/features/channel-monitor-v2/ToolMonitorDetails.vue'
import ChannelStatusV1View from '@/views/user/ChannelStatusV1View.vue'
import ChannelStatusV3View from '@/views/user/ChannelStatusV3View.vue'
const props=defineProps<{toolId:string;toolName:string;groups:Group[];resumeAssociation?:{range:MonitorRange;groupId:number;platform:string;scrollTop:number}}>()
const emit=defineEmits<{title:[string];description:[string];associate:[number,{range:MonitorRange;groupId:number;platform:string;scrollTop:number}]}>()
const {t}=useI18n()
const app=useAppStore()
const ready=ref(false), settingsError=ref(false), refreshing=ref(false)
const enabled=computed(()=>app.cachedPublicSettings?.channel_monitor_enabled!==false)
const mode=computed(()=>app.cachedPublicSettings?.channel_monitor_mode||'v1')
const hasGroups=computed(()=>props.groups.some(group=>toolIdsForGroup(group).includes(props.toolId)))
let alive=true
async function refreshSettings(){
  if(refreshing.value || !alive)return
  refreshing.value=true
  try {
    const settings=await app.fetchPublicSettings(true)
    if(!alive)return
    settingsError.value=!settings
    ready.value=!!settings
  }catch {if(alive){settingsError.value=true;ready.value=false}}
  finally{if(alive)refreshing.value=false}
}
watch([mode,enabled,()=>props.toolId],()=>{
  emit('title',`${props.toolName} ${t('channelMonitorV2.tool.lines')}`)
  emit('description',enabled.value?t(mode.value==='v1'||mode.value==='native_probe'?'channelMonitorV2.tool.activeHint':mode.value==='v3'?'channelMonitorV2.tool.componentHint':'channelMonitorV2.tool.passiveHint'):'')
},{immediate:true})
function onFocus(){void refreshSettings()}
function onVisible(){if(!document.hidden)onFocus()}
let timer:ReturnType<typeof setInterval>|undefined
onMounted(()=>{void refreshSettings();window.addEventListener('focus',onFocus);document.addEventListener('visibilitychange',onVisible);timer=setInterval(onVisible,30000)})
onBeforeUnmount(()=>{alive=false;window.removeEventListener('focus',onFocus);document.removeEventListener('visibilitychange',onVisible);if(timer)clearInterval(timer)})
</script>
<style scoped>
.route-message{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:12px;min-height:160px;color:var(--xq-secondary);font-size:14px}
.tool-route-details{min-width:0}
</style>
