<template>
  <AppLayout>
    <section class="user-page xq-ai" data-testid="ai-tools-workspace" aria-labelledby="ai-tools-title">
      <header class="page-head"><h1 id="ai-tools-title">请选择你的 AI 工具</h1><p>比较可用线路、稳定性和价格倍率</p></header>
      <div v-if="workspaceError" class="workspace-error" role="alert">{{ workspaceError }} <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="loading && !loaded" class="empty" role="status">正在读取 AI 工具…</div>
      <template v-if="loaded">
        <div class="tool-grid" :aria-busy="loading">
          <article v-for="tool in toolCards" :key="tool.id" class="tool-card">
            <h3 class="tool-title"><img class="provider-logo" :src="providerIcon(tool.platform)" alt="" /><span>{{ tool.label }}</span></h3>
            <span class="tool-type">{{ tool.type }}</span>
            <div class="tool-status" :class="`status-${tool.statusKind}`"><span class="dot" :class="tool.statusKind"></span><span class="availability-pill">{{ tool.statusText }}</span><button class="xq-button icon-btn" :aria-label="`${tool.label} 线路详情`" @click="openDetails(tool)"><img src="/xingqiao/info.svg" alt="" /></button></div>
            <div class="separator"></div><small>最佳线路</small><strong class="best" :class="{muted:!tool.best}">{{ tool.best?.name || '暂无可用线路' }}</strong>
            <div class="card-bottom"><span>已关联 {{ tool.linked }} 把密钥</span><button class="xq-button" :disabled="!tool.active.length" @click="openCreate(tool)">关联密钥</button></div>
          </article>
        </div>
        <section class="lines-panel" aria-labelledby="routes-title">
          <div class="panel-head"><h2 id="routes-title">我的 AI 线路</h2><button class="xq-button icon-btn" aria-label="检查线路" title="检查线路" :disabled="checking || !routeRows.some(g=>g.status==='active')" @click="runChecks"><img src="/xingqiao/refresh.svg" alt="" :class="{'is-checking':checking}" /></button></div>
          <div v-if="checkError" class="workspace-error" role="alert">{{ checkError }}</div>
          <div class="route-table-scroll">
            <div class="route-table" role="table" aria-label="我的 AI 线路">
              <div class="route-header" role="row"><span role="columnheader">线路</span><span role="columnheader">近 1 小时成功率</span><span role="columnheader" title="本次检查首字耗时">本次检查结果</span><span role="columnheader">关联密钥</span><span role="columnheader">操作</span></div>
              <div v-for="group in routeRows" :key="group.id" class="route-row" role="row">
                <div class="route-name" role="cell"><span class="dot" :class="stateOf(group).kind" :aria-label="stateOf(group).text"></span><div><div class="route-identity"><img class="provider-logo route-provider-logo" :src="providerIcon(group.platform)" alt="" /><strong>{{ group.name }}</strong><span class="rate-badge">{{ rateLabel(group) }}</span></div><small>{{ platformLabel(group.platform) }} · {{ stateOf(group).text }}</small></div></div>
                <span role="cell" class="rate" :title="statsHint(group)">{{ successLabel(group,metricsById,true) }}</span>
                <span role="cell" :class="checkOf(group).kind" :title="checkOf(group).text==='—'?'本次尚未检查':'本次检查首字耗时'"><Icon v-if="checkOf(group).kind==='success'" name="check" size="sm" />{{ checkOf(group).text }}</span>
                <span role="cell">{{ counts.get(group.id)||0 }} 把</span><button class="route-action" @click="openGroupKeys(group.id)">查看关联密钥</button>
              </div>
            </div>
          </div>
          <div v-if="!routeRows.length" class="empty">暂无已关联密钥的线路</div>
        </section>
      </template>
      <BaseDialog :show="!!selectedTool && !createTool" :title="`${selectedTool?.label||''} 线路详情`" width="full" panel-class="xq-route-dialog" :close-on-click-outside="true" @close="closeDetails" @opened="restoreDetailFocus">
        <div class="detail-section-head"><p>已按线路质量从高到低排列；成功率、请求次数与 P50 按所选时间范围统计</p><div class="detail-period"><span>统计范围</span><div class="detail-period-segment" role="group" aria-label="线路统计时间"><button v-for="period in periods" :key="period.value" :aria-pressed="detailWindow===period.value" :class="{active:detailWindow===period.value}" @click="loadDetails(period.value)">{{ period.label }}</button></div></div></div>
        <div v-if="detailError" class="workspace-error" role="alert">{{ detailError }} <button class="xq-button" @click="loadDetails(detailWindow)">重试</button></div>
        <div v-if="detailLoading && !detailData.length" class="empty" role="status">正在读取统计…</div>
        <div v-else class="table-scroll"><table class="route-metrics-table"><thead><tr><th>线路</th><th>关联密钥</th><th>成功率</th><th>请求次数</th><th>首字 P50</th><th>耗时 P50</th><th title="本次检查首字耗时">本次检查</th></tr></thead><tbody>
          <tr v-for="group in detailRows" :key="group.id"><td><div class="detail-route"><div class="detail-route-title"><div class="route-identity"><img class="provider-logo route-provider-logo" :src="providerIcon(group.platform)" alt="" /><strong>{{group.name}}</strong><span class="rate-badge">{{rateLabel(group)}}</span></div><span v-if="group.id===detailBest?.id" class="best-route-badge">最佳线路</span></div><small class="route-availability" :class="stateOf(group).kind"><span class="dot" :class="stateOf(group).kind"></span>{{stateOf(group).text}}</small></div></td>
            <td><div class="linked-key-cell"><span>{{counts.get(group.id)?'已关联':'未关联'}} · {{counts.get(group.id)||0}} 把</span><button v-if="group.status==='active'" class="xq-button small" :data-detail-group-id="group.id" @click="openCreate(selectedTool!,group.id)">关联密钥</button><span v-else class="muted">不可配置</span></div></td>
            <td><strong class="success-rate">{{successLabel(group,detailMetrics,false)}}</strong></td><td><span class="request-count">{{group.status!=='active'?'—':detailMetrics.get(group.id)?.request_count??'—'}}</span></td><td><strong class="latency-metric">{{group.status!=='active'?'—':metricLabel(detailMetrics.get(group.id)?.ttft_p50_ms)}}</strong></td><td><strong class="latency-metric">{{group.status!=='active'?'—':metricLabel(detailMetrics.get(group.id)?.latency_p50_ms)}}</strong></td><td><span :class="checkOf(group).kind"><Icon v-if="checkOf(group).kind==='success'" name="check" size="sm" />{{checkOf(group).text}}</span></td>
          </tr>
        </tbody></table><div v-if="!detailRows.length" class="empty">暂无线路</div></div>
      </BaseDialog>
      <CreateLineKeyDialog :show="!!createTool" :tool-name="createTool?.label||''" :tool-id="createTool?.id" :groups="createTool?.active||[]" :metrics="metricsById" :linked-counts="counts" :rates="rates" :initial-group-id="createGroupId" @close="closeCreate" @created="keyCreated" />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import CreateLineKeyDialog from '@/features/ai-tools/CreateLineKeyDialog.vue'
import userGroupsAPI from '@/api/groups'
import keysAPI from '@/api/keys'
import { getHybridPerformanceSnapshot } from '@/features/monitor-v4/api'
import { checkLines, type LineCheck } from '@/features/ai-tools/api'
import { tools, linkedCounts, configuredLines, availability, compareQuality, metricLabel, providerIcon, platformLabel, toolIdsForGroup } from '@/features/ai-tools/model'
import { getDashboardWorkspaceSnapshot, setDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group, MonitorV4Window } from '@/features/monitor-v4/types'
import '@/styles/xingqiao-ai.css'

const router=useRouter()
const authStore=useAuthStore()
const cacheUserId=String(authStore.user?.id||'')
const cachedWorkspace=getDashboardWorkspaceSnapshot(cacheUserId)
const clock=ref(Date.now())
const groups=ref<Group[]>(cachedWorkspace?.groups||[]), keys=ref<ApiKey[]>(cachedWorkspace?.keys||[]), rates=ref<Record<number,number>>(cachedWorkspace?.rates||{}), metrics=ref<MonitorV4Group[]>(cachedWorkspace?.metrics||[])
const metricsGeneratedAt=ref<string|null>(cachedWorkspace?.metricsGeneratedAt||null)
const loading=ref(false), loaded=ref(!!cachedWorkspace), workspaceError=ref(''), statsFailed=ref(false), checking=ref(false), checkError=ref('')
const checks=ref<Record<number,LineCheck>>({}), detailWindow=ref<MonitorV4Window>('1h'), detailLoading=ref(false), detailError=ref(''), detailData=ref<MonitorV4Group[]>([])
let loadController:AbortController|undefined, detailController:AbortController|undefined, checkController:AbortController|undefined
const detailCache=new Map<MonitorV4Window,MonitorV4Group[]>()
let returnToDetailGroup:number|undefined
let detailsTrigger:HTMLElement|null=null, createTrigger:HTMLElement|null=null
const counts=computed(()=>linkedCounts(keys.value))
const metricsById=computed(()=>new Map(metrics.value.map(m=>[m.id,m])))
const detailMetrics=computed(()=>new Map(detailData.value.map(m=>[m.id,m])))
const allGroups=computed(()=>{const all=new Map(groups.value.map(g=>[g.id,g]));for(const g of configuredLines(groups.value,keys.value))all.set(g.id,g);return [...all.values()]})
const sort=(list:Group[], ms=metricsById.value)=>[...list].sort((a,b)=>compareQuality(a,b,ms,rates.value,clock.value,metricsGeneratedAt.value))
const stateOf=(g:Group)=>availability(g,statsFailed.value?undefined:metricsById.value.get(g.id),clock.value,metricsGeneratedAt.value)
const toolCards=computed(()=>tools.map(tool=>{
  const matching=sort(allGroups.value.filter(g=>toolIdsForGroup(g,metricsById.value.get(g.id)).includes(tool.id)))
  const active=matching.filter(g=>g.status==='active'),available=active.filter(g=>stateOf(g).available)
  const unknown=active.some(g=>stateOf(g).text==='暂不可用')
  return {...tool,groups:matching,active,best:available[0],linked:matching.reduce((n,g)=>n+(counts.value.get(g.id)||0),0),statusKind:!active.length||unknown&&!available.length?'muted':available.length?'success':'danger',statusText:!active.length?'暂无可用线路':available.length?`可用 · ${available.length}/${active.length} 条`:unknown?'暂不可用':`不可用 · 0/${active.length} 条`}
}))
type ToolCard=typeof toolCards.value[number]
const selectedTool=ref<ToolCard|null>(null), createTool=ref<ToolCard|null>(null),createGroupId=ref<number>()
const routeRows=computed(()=>sort(configuredLines(groups.value,keys.value)))
const detailRows=computed(()=>sort(selectedTool.value?.groups||[],detailMetrics.value))
const detailBest=computed(()=>detailRows.value.find(g=>stateOf(g).available))
const periods=[{value:'1h' as const,label:'近 1 小时'},{value:'24h' as const,label:'近 24 小时'},{value:'7d' as const,label:'近 7 天'}]
const rateLabel=(g:Group)=>`${rates.value[g.id]??g.rate_multiplier}x`
function successLabel(g:Group,ms:Map<number,MonitorV4Group>,hour:boolean){
  if(g.status!=='active')return '—'
  const m=ms.get(g.id)
  if(!m || hour&&statsFailed.value)return '暂不可用'
  return m.request_count===0?(hour?'近 1 小时无请求':'无请求'):m.success_rate==null?'暂不可用':`${Number(m.success_rate.toFixed(1))}%`
}
function statsHint(g:Group){const m=metricsById.value.get(g.id);return g.status!=='active'||!m||statsFailed.value?'':'最近 1 小时\n成功请求：'+m.success_count+' 次\n总请求：'+m.request_count+' 次'}
function checkOf(g:Group){
  if(g.status!=='active')return {kind:'muted',text:'线路已停用'}
  if(checking.value&&counts.value.has(g.id))return {kind:'warning',text:'检查中…'}
  const r=checks.value[g.id]
  if(!r)return {kind:'muted',text:'—'}
  if(r.status==='success'&&r.ttft_ms!=null)return {kind:'success',text:metricLabel(r.ttft_ms)}
  return {kind:r.status==='disabled'?'muted':'danger',text:r.status==='timeout'?'响应超时':r.status==='disabled'?'线路已停用':'检查失败'}
}
function cacheWorkspace(){setDashboardWorkspaceSnapshot({userId:cacheUserId,groups:groups.value,keys:keys.value,rates:rates.value,metrics:metrics.value,metricsGeneratedAt:metricsGeneratedAt.value})}
async function loadAllKeys(signal:AbortSignal){const first=await keysAPI.list(1,100,undefined,{signal});const items=[...first.items];for(let page=2;page<=Math.ceil(first.total/100);page++){const next=await keysAPI.list(page,100,undefined,{signal});items.push(...next.items)}return items}
async function loadWorkspace(){
  loadController?.abort();const c=new AbortController();loadController=c;loading.value=true;workspaceError.value=''
  try {
    const statsRequest=getHybridPerformanceSnapshot('1h',c.signal).then(value=>({ok:true as const,value}),()=>({ok:false as const}))
    const [gs,rs,ks]=await Promise.allSettled([userGroupsAPI.getAvailable(),userGroupsAPI.getUserGroupRates(),loadAllKeys(c.signal)])
    if(c.signal.aborted)return
    if(gs.status==='rejected'||ks.status==='rejected'||rs.status==='rejected') {workspaceError.value=loaded.value?'刷新失败，保留上次成功数据。':'AI 工具数据暂时不可用，请重试。';return}
    clock.value=Date.now()
    groups.value=gs.value;rates.value=rs.value;keys.value=ks.value
    loaded.value=true
    cacheWorkspace()
    if(selectedTool.value) selectedTool.value=toolCards.value.find(t=>t.id===selectedTool.value?.id)||null
    const stats=await statsRequest
    if(c.signal.aborted)return
    if(stats.ok){statsFailed.value=false;metrics.value=stats.value.groups;metricsGeneratedAt.value=stats.value.generated_at;cacheWorkspace()}
    else statsFailed.value=!metricsGeneratedAt.value
  }finally{if(!c.signal.aborted)loading.value=false}
}
async function openDetails(tool:ToolCard){detailsTrigger=document.activeElement as HTMLElement;selectedTool.value=tool;await loadDetails('1h')}
function closeDetails(){detailController?.abort();selectedTool.value=null;nextTick(()=>detailsTrigger?.focus())}
async function loadDetails(window:MonitorV4Window){detailController?.abort();const c=new AbortController();detailController=c;detailWindow.value=window;detailLoading.value=true;detailError.value='';detailData.value=detailCache.get(window)||[];try{const result=await getHybridPerformanceSnapshot(window,c.signal);if(!c.signal.aborted){detailData.value=result.groups;detailCache.set(window,result.groups)}}catch{if(!c.signal.aborted)detailError.value=detailCache.has(window)?'统计刷新失败，保留上次成功数据。':'统计读取失败，请重试。'}finally{if(!c.signal.aborted)detailLoading.value=false}}
function openCreate(tool:ToolCard,id?:number){createTrigger=document.activeElement as HTMLElement;createTool.value=tool;createGroupId.value=id}
function restoreDetailFocus(){
  if(returnToDetailGroup){document.querySelector<HTMLElement>(`[data-detail-group-id="${returnToDetailGroup}"]`)?.focus();returnToDetailGroup=undefined}
}
function closeCreate(){
  returnToDetailGroup=selectedTool.value?createGroupId.value:undefined
  createTool.value=null
  if(!selectedTool.value)nextTick(()=>createTrigger?.focus())
}
async function keyCreated(key:ApiKey){keys.value=[...keys.value.filter(k=>k.id!==key.id),key];createTool.value=null;await loadWorkspace();if(selectedTool.value)await loadDetails(detailWindow.value)}
async function runChecks(){
  if(checking.value)return
  const ids=routeRows.value.filter(g=>g.status==='active').map(g=>g.id);if(!ids.length)return
  checking.value=true;checkError.value='';checkController=new AbortController()
  try{const results=await checkLines(ids,checkController.signal);if(checkController.signal.aborted)return;for(const id of ids)checks.value[id]=results.find(r=>r.group_id===id)||{group_id:id,status:'failed',ttft_ms:null,checked_at:''};await loadWorkspace()}
  catch{if(!checkController.signal.aborted){checkError.value='线路检查请求未完成，请稍后重试。'}}
  finally{checking.value=false}
}
function openGroupKeys(id:number){void router.push({path:'/keys',query:{group_id:String(id)}})}
onMounted(()=>{void loadWorkspace()})
onBeforeUnmount(()=>{loadController?.abort();detailController?.abort();checkController?.abort()})
</script>
