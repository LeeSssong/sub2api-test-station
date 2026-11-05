<template>
  <AppLayout>
    <section class="user-page xq-ai" data-testid="ai-tools-workspace" aria-labelledby="ai-tools-title">
      <header class="page-head"><h1 id="ai-tools-title">请选择你的 AI 工具</h1></header>
      <div v-if="workspaceError" class="workspace-error" role="alert">{{ workspaceError }} <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="statsError" class="workspace-error statistics-error" role="alert">{{ statsError }} <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="rankingFailed" class="workspace-error ranking-error" role="alert">近 24 小时推荐统计读取失败，请刷新重试。 <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="loading && !loaded" class="empty" role="status">正在读取 AI 工具…</div>
      <template v-if="loaded">
        <ToolCarousel :item-count="toolCards.length" :aria-busy="loading">
          <article v-for="tool in toolCards" :key="tool.id" class="tool-card">
            <header class="tool-card-header">
              <div class="tool-card-identity"><h3 class="tool-title"><img class="provider-logo" :src="providerIcon(tool.platform)" alt="" /><span>{{ tool.label }}</span></h3><span class="tool-type">{{ tool.type }}</span></div>
              <button type="button" class="pricing-entry" :aria-label="`${tool.label} 价格与扣费说明`" @click="openPricing(tool)">扣费说明 <Icon name="externalLink" size="sm" /></button>
            </header>
            <button type="button" class="tool-status" :aria-label="`${tool.label} 线路详情`" :title="tool.healthSummary" aria-haspopup="dialog" :aria-expanded="selectedTool?.id===tool.id && !createTool" @click="openDetails(tool)">
              <span v-if="tool.healthItems.length" class="tool-status-grid">
                <span v-for="item in tool.healthItems" :key="item.kind" class="tool-status-value" :data-health-count="item.kind"><span class="dot" :class="item.kind" aria-hidden="true"></span><span>{{ item.text }}</span><span class="tool-status-count">{{ item.count }}</span></span>
              </span>
              <span v-else class="tool-status-value"><span class="dot muted" aria-hidden="true"></span>暂无线路</span>
              <Icon class="tool-status-arrow" name="chevronRight" size="sm" />
            </button>
            <div class="card-bottom tool-association" role="group" :aria-label="`${tool.label} 最佳线路与关联密钥`">
              <div :id="`tool-best-${tool.id}`" class="tool-best" title="近 24 小时：成功率 50% · 缓存命中率 30% · 首字速度 20%"><small>最佳线路</small><strong class="best" :class="{muted:!tool.best}">{{ tool.best?.name || (tool.active.length ? tool.bestEmptyText : tool.id === 'gemini' ? '暂无可用 Gemini 分组' : '暂无可用线路') }}</strong></div>
              <button type="button" class="xq-button associate-key" :disabled="!tool.active.length" :aria-describedby="`tool-best-${tool.id}`" @click="openCreate(tool)">关联密钥</button>
            </div>
          </article>
        </ToolCarousel>
      </template>
      <KeysView :refresh-version="keyRefreshVersion" :recommended-group-ids="recommendedGroupIds" @changed="loadWorkspace" />
      <BaseDialog brand-theme :show="!!selectedTool && !createTool" :title="detailTitle || `${selectedTool?.label||''} 线路详情`" :description="detailDescription" width="full" panel-class="xq-route-dialog" :close-on-click-outside="true" @close="closeDetails">
        <template #title><span class="route-dialog-title"><img v-if="selectedTool" class="provider-logo" :src="providerIcon(selectedTool.platform)" alt="" /><span>{{ detailTitle || `${selectedTool?.label || ''} 线路详情` }}</span></span></template>
        <ToolMonitorDetails v-if="selectedTool" :key="selectedTool.id" :tool-id="selectedTool.id" :tool-name="selectedTool.label" :groups="groups" :resume-association="detailResume" @title="detailTitle = $event" @description="detailDescription = $event" @associate="associateDetailGroup" />
      </BaseDialog>
      <CreateLineKeyDialog :show="!!createTool" :tool-name="createTool?.label||''" :tool-id="createTool?.id" :groups="createTool?.active||[]" :metrics="selectionMetrics" :metrics-generated-at="selectionGeneratedAt" :metrics-error="selectionError" :recommended-group-ids="recommendedGroupIds" :rates="rates" :initial-group-id="createGroupId" @close="closeCreate" @created="keyCreated" />
      <PricingDialog :tool="pricingTool" :lines="pricingTool?.groups || []" :rates="rates" @close="closePricing" />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import KeysView from './KeysView.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import CreateLineKeyDialog from '@/features/ai-tools/CreateLineKeyDialog.vue'
import ToolMonitorDetails from '@/features/channel-monitor-v2/ToolMonitorDetails.vue'
import PricingDialog from '@/features/ai-tools/PricingDialog.vue'
import ToolCarousel from '@/features/ai-tools/ToolCarousel.vue'
import userGroupsAPI from '@/api/groups'
import keysAPI from '@/api/keys'
import { getLineSelectionMetrics, type LineSelectionMetric } from '@/features/monitor-v4/lineSelection'
import { getHybridPerformanceSnapshot } from '@/features/monitor-v4/api'
import { tools, configuredLines, routeHealth, compareQuality, rankWeightedRoutes, providerIcon, toolIdsForGroup } from '@/features/ai-tools/model'
import { getDashboardWorkspaceSnapshot, setDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
import type { ApiKey, Group } from '@/types'
import type { MonitorRange } from '@/api/channelMonitorV2'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import '@/styles/xingqiao-ai.css'
import '@/styles/xingqiao-merged-keys.css'

const authStore=useAuthStore()
const cacheUserId=String(authStore.user?.id||'')
const cachedWorkspace=getDashboardWorkspaceSnapshot(cacheUserId)
const clock=ref(Date.now())
let freshnessTimer: ReturnType<typeof setInterval> | undefined
const groups=ref<Group[]>(cachedWorkspace?.groups||[]), keys=ref<ApiKey[]>(cachedWorkspace?.keys||[]), rates=ref<Record<number,number>>(cachedWorkspace?.rates||{}), metrics=ref<MonitorV4Group[]>(cachedWorkspace?.metrics||[]), rankingMetrics=ref<MonitorV4Group[]>(cachedWorkspace?.rankingMetrics||[])
const metricsGeneratedAt=ref<string|null>(cachedWorkspace?.metricsGeneratedAt||null)
const rankingMetricsGeneratedAt=ref<string|null>(cachedWorkspace?.rankingMetricsGeneratedAt||null)
const loading=ref(false), loaded=ref(!!cachedWorkspace), workspaceError=ref(''), statsFailed=ref(false), statsError=ref(''), rankingFailed=ref(false)
const keyRefreshVersion=ref(0)
const detailTitle=ref('')
const detailDescription=ref('')
const detailResume=ref<{range:MonitorRange;groupId:number;platform:string;scrollTop:number}>()
let loadController:AbortController|undefined
let detailsTrigger:HTMLElement|null=null, createTrigger:HTMLElement|null=null
const metricsById=computed(()=>new Map(metrics.value.map(m=>[m.id,m])))
const rankingMetricsById=computed(()=>new Map(rankingMetrics.value.map(m=>[m.id,m])))
const allGroups=computed(()=>{const all=new Map(groups.value.map(g=>[g.id,g]));for(const g of configuredLines(groups.value,keys.value))all.set(g.id,g);return [...all.values()]})
const sort=(list:Group[], ms=metricsById.value)=>[...list].sort((a,b)=>compareQuality(a,b,ms,rates.value,clock.value,metricsGeneratedAt.value))
const stateOf=(g:Group)=>routeHealth(statsFailed.value?undefined:metricsById.value.get(g.id),clock.value,metricsGeneratedAt.value)
const HEALTH_ORDER=['success','warning','danger','muted'] as const
const HEALTH_LABELS={success:'正常',warning:'波动',danger:'异常',muted:'无数据'} as const
const toolCards=computed(()=>tools.map(tool=>{
  const matching=sort(allGroups.value.filter(g=>toolIdsForGroup(g,rankingMetricsById.value.get(g.id)||metricsById.value.get(g.id)).includes(tool.id)))
  const active=matching.filter(g=>g.status==='active')
  const ranked=rankingFailed.value?[]:rankWeightedRoutes(matching,rankingMetricsById.value,clock.value,rankingMetricsGeneratedAt.value)
  const bestEmptyText=!rankingFailed.value&&active.some(g=>routeHealth(rankingMetricsById.value.get(g.id),clock.value,rankingMetricsGeneratedAt.value).rate!==null)?'统计不足':'暂无请求数据'
  const healthCounts={success:0,warning:0,danger:0,muted:0}
  for(const group of matching) healthCounts[stateOf(group).kind] += 1
  const healthItems=HEALTH_ORDER.map(kind=>({kind,text:HEALTH_LABELS[kind],count:healthCounts[kind]})).filter(item=>item.count>0)
  const healthSummary=healthItems.map(item=>`${item.text} ${item.count} 条`).join('，')||'暂无线路'
  return {...tool,groups:matching,active,best:ranked[0]?.group,bestEmptyText,healthItems,healthSummary}
}))
type ToolCard=typeof toolCards.value[number]
const recommendedGroupIds=computed(()=>new Map(toolCards.value.flatMap(tool=>tool.best ? [[tool.id,tool.best.id] as const] : [])))
const pricingTool=ref<ToolCard|null>(null)
let pricingTrigger:HTMLElement|null=null
function openPricing(tool:ToolCard){pricingTrigger=document.activeElement as HTMLElement;pricingTool.value=tool}
function closePricing(){pricingTool.value=null;nextTick(()=>pricingTrigger?.focus())}
const selectedTool=ref<ToolCard|null>(null), createTool=ref<ToolCard|null>(null),createGroupId=ref<number>()
function cacheWorkspace(){setDashboardWorkspaceSnapshot({userId:cacheUserId,groups:groups.value,keys:keys.value,rates:rates.value,metrics:metrics.value,metricsGeneratedAt:metricsGeneratedAt.value,rankingMetrics:rankingMetrics.value,rankingMetricsGeneratedAt:rankingMetricsGeneratedAt.value})}
async function loadAllKeys(signal:AbortSignal){const first=await keysAPI.list(1,100,undefined,{signal});const items=[...first.items];for(let page=2;page<=Math.ceil(first.total/100);page++){const next=await keysAPI.list(page,100,undefined,{signal});items.push(...next.items)}return items}
async function loadWorkspace(){
  loadController?.abort();const c=new AbortController();loadController=c;loading.value=true;workspaceError.value=''
  try {
    const statsRequest=getHybridPerformanceSnapshot('1h',c.signal).then(value=>({ok:true as const,value}),()=>({ok:false as const}))
    const rankingRequest=getHybridPerformanceSnapshot('24h',c.signal).then(value=>({ok:true as const,value}),()=>({ok:false as const}))
    const [gs,rs,ks]=await Promise.allSettled([userGroupsAPI.getAvailable(),userGroupsAPI.getUserGroupRates(),loadAllKeys(c.signal)])
    if(c.signal.aborted)return
    if(gs.status==='rejected'||ks.status==='rejected'||rs.status==='rejected') {workspaceError.value=loaded.value?'刷新失败，保留上次成功数据。':'AI 工具数据暂时不可用，请重试。';return}
    clock.value=Date.now()
    groups.value=gs.value;rates.value=rs.value;keys.value=ks.value
    loaded.value=true
    cacheWorkspace()
    if(selectedTool.value) selectedTool.value=toolCards.value.find(t=>t.id===selectedTool.value?.id)||null
    const [stats,ranking]=await Promise.all([statsRequest,rankingRequest])
    if(c.signal.aborted)return
    if(stats.ok){clock.value=Date.now();statsFailed.value=false;statsError.value='';metrics.value=stats.value.groups;metricsGeneratedAt.value=stats.value.generated_at;cacheWorkspace()}
    else {statsFailed.value=true;statsError.value='近 1 小时统计读取失败，请刷新重试。'}
    if(ranking.ok){rankingFailed.value=false;rankingMetrics.value=ranking.value.groups;rankingMetricsGeneratedAt.value=ranking.value.generated_at}
    else {rankingFailed.value=true;rankingMetrics.value=[];rankingMetricsGeneratedAt.value=null}
    cacheWorkspace()
  }finally{if(!c.signal.aborted)loading.value=false}
}
function openDetails(tool:ToolCard){detailDescription.value='';detailResume.value=undefined;detailsTrigger=document.activeElement as HTMLElement;detailTitle.value=`${tool.label} 线路详情`;selectedTool.value=tool}
function closeDetails(){detailResume.value=undefined;selectedTool.value=null;nextTick(()=>detailsTrigger?.focus())}
const selectionMetrics=ref(new Map<number,LineSelectionMetric>())
const selectionGeneratedAt=ref<string|null>(null), selectionError=ref(false)
let selectionController:AbortController|undefined
async function loadSelectionMetrics(){
 selectionController?.abort();const c=new AbortController();selectionController=c
 selectionMetrics.value=new Map();selectionGeneratedAt.value=null;selectionError.value=false
 try{const s=await getLineSelectionMetrics(c.signal);if(c.signal.aborted)return;selectionMetrics.value=new Map(s.groups.map(g=>[g.id,g]));selectionGeneratedAt.value=s.generated_at}
 catch{if(!c.signal.aborted)selectionError.value=true}
}
function openCreate(tool:ToolCard,id?:number){void loadSelectionMetrics();createTrigger=document.activeElement as HTMLElement;createTool.value=tool;createGroupId.value=id??tool.best?.id}
function associateDetailGroup(id:number,resume?:{range:MonitorRange;groupId:number;platform:string;scrollTop:number}){
  const tool=selectedTool.value
  if(!tool || !tool.active.some(group=>group.id===id))return
  detailResume.value=resume
  openCreate(tool,id)
}
function closeCreate(){
  createTool.value=null
  if(!selectedTool.value)nextTick(()=>createTrigger?.focus())
}
async function keyCreated(key:ApiKey){keyRefreshVersion.value++;keys.value=[...keys.value.filter(k=>k.id!==key.id),key];createTool.value=null;await loadWorkspace();}
onMounted(()=>{void loadWorkspace();freshnessTimer=setInterval(()=>{clock.value=Date.now()},30000)})
onBeforeUnmount(()=>{clearInterval(freshnessTimer);selectionController?.abort();loadController?.abort();})
</script>

<style>
.xq-route-dialog > .modal-body { background: var(--xq-void); }
</style>
