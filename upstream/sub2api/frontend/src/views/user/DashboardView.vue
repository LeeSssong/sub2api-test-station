<template>
  <AppLayout>
    <section class="user-page xq-ai" data-testid="ai-tools-workspace" aria-labelledby="ai-tools-title">
      <header class="page-head"><h1 id="ai-tools-title">请选择你的 AI 工具</h1></header>
      <div v-if="workspaceError" class="workspace-error" role="alert">{{ workspaceError }} <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="statsError" class="workspace-error statistics-error" role="alert">{{ statsError }} <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="rankingFailed" class="workspace-error ranking-error" role="alert">近 24 小时推荐统计读取失败，请刷新重试。 <button class="xq-button" @click="loadWorkspace">重试</button></div>
      <div v-if="loading && !loaded" class="empty" role="status">正在读取 AI 工具…</div>
      <template v-if="loaded">
        <div class="tool-grid" :aria-busy="loading">
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
              <div :id="`tool-best-${tool.id}`" class="tool-best" title="近 24 小时：成功率 50% · 缓存命中率 30% · 首字速度 20%"><small>最佳线路</small><strong class="best" :class="{muted:!tool.best}">{{ tool.best?.name || (tool.active.length ? tool.bestEmptyText : '暂无可用线路') }}</strong></div>
              <button type="button" class="xq-button" :disabled="!tool.active.length" :aria-describedby="`tool-best-${tool.id}`" @click="openCreate(tool)">关联密钥</button>
            </div>
          </article>
        </div>
        <section class="lines-panel" aria-labelledby="routes-title">
          <div class="panel-head"><h2 id="routes-title">我的 AI 线路</h2><button class="xq-button route-check-button" aria-label="检查线路" title="每分钟最多检查 30 次" :disabled="checking || checkCooldown || !routeRows.some(g=>g.status==='active')" @click="runChecks"><img src="/xingqiao/refresh.svg" alt="" :class="{'is-checking':checking}" /><span>检查线路</span></button></div>
          <div v-if="checkError" class="workspace-error" role="alert">{{ checkError }}</div>
          <div class="route-table-scroll">
            <div class="route-table" role="table" aria-label="我的 AI 线路">
              <div class="route-header" role="row"><span role="columnheader">线路</span><span role="columnheader">近 1 小时成功率</span><span role="columnheader" title="本次检查首字耗时">本次检查结果</span><span role="columnheader">关联密钥</span><span role="columnheader">操作</span></div>
              <div v-for="group in routeRows" :key="group.id" class="route-row" role="row">
                <div class="route-name" role="cell"><span class="dot" :class="stateOf(group).kind" :aria-label="stateOf(group).text"></span><div><div class="route-identity"><img class="provider-logo route-provider-logo" :src="providerIcon(group.platform)" alt="" /><strong>{{ group.name }}</strong><span class="rate-badge">{{ rateLabel(group) }}</span></div><small>{{ platformLabel(group.platform) }} · {{ stateOf(group).text }}</small></div></div>
                <span role="cell" class="rate success-rate-tone" :data-tone="successTone(group,metricsById,true)" :title="statsHint(group)">{{ successLabel(group,metricsById,true) }}</span>
                <span role="cell" :class="checkOf(group).kind" :title="checkOf(group).text==='—'?'本次尚未检查':'本次检查首字耗时'"><Icon v-if="checkOf(group).kind==='success'" name="check" size="sm" />{{ checkOf(group).text }}</span>
                <span role="cell">{{ counts.get(group.id)||0 }} 把</span><button class="route-action" @click="openGroupKeys(group.id)">查看关联密钥</button>
              </div>
            </div>
          </div>
          <div v-if="!routeRows.length" class="empty">暂无已关联密钥的线路</div>
        </section>
      </template>
      <BaseDialog brand-theme :show="!!selectedTool && !createTool" :title="`${selectedTool?.label||''} 线路详情`" width="full" panel-class="xq-route-dialog" :close-on-click-outside="true" @close="closeDetails" @opened="restoreDetailFocus">
        <div class="detail-section-head"><div class="detail-period"><span>统计范围</span><div class="detail-period-segment" role="group" aria-label="线路统计时间"><button v-for="period in periods" :key="period.value" :aria-pressed="detailWindow===period.value" :class="{active:detailWindow===period.value}" @click="loadDetails(period.value)">{{ period.label }}</button></div><label class="detail-granularity">粒度 <select aria-label="线路图粒度" :value="detailWindow==='1h'?'5m':detailGranularity" :disabled="detailWindow==='1h'" @change="detailGranularity=($event.target as HTMLSelectElement).value as 'hour'|'day';loadDetails(detailWindow)"><option v-if="detailWindow==='1h'" value="5m">5 分钟</option><option value="hour">小时</option><option value="day">天</option></select></label></div></div>
        <div v-if="detailError" class="workspace-error" role="alert">{{ detailError }} <button class="xq-button" @click="loadDetails(detailWindow)">重试</button></div>
        <div v-if="detailLoading && !detailData.length" class="empty" role="status">正在读取统计…</div>
        <div v-else class="route-detail-grid" :aria-busy="detailLoading">
          <article v-for="group in detailRows" :key="group.id" class="route-detail-card" :aria-labelledby="`detail-route-${group.id}`">
            <span v-if="group.id===detailBest?.id" class="best-route-badge"><span class="best-route-ribbon">最佳线路</span></span>
            <header class="detail-card-header">
              <div class="detail-card-identity">
                <div class="detail-card-name">
                  <img class="provider-logo" :src="providerIcon(group.platform)" alt="" />
                  <h3 :id="`detail-route-${group.id}`">{{ group.name }}</h3>
                  <span class="rate-badge">{{ rateLabel(group) }}</span>
                  <span class="route-health" :class="detailSLAHealth(group).kind" title="所选统计范围的后台 SLA 状态"><span class="dot" :class="detailSLAHealth(group).kind" aria-hidden="true"></span>{{ detailSLAHealth(group).text }}</span>
                </div>
              </div>
              <button v-if="group.status==='active'" class="xq-button detail-associate-button" :data-detail-group-id="group.id" @click="openCreate(selectedTool!,group.id)">关联密钥</button>
              <span v-else class="muted detail-unavailable">不可配置</span>
            </header>
            <div class="detail-card-quality">
              <div class="detail-quality-value" role="group">
                <span class="detail-quality-label" title="与后台对应分组的 SLA 一致，排除业务限制及客户端取消">请求成功率</span>
                <strong class="success-rate success-rate-tone" :data-tone="routeHealthTone(detailSLAHealth(group))">{{ timelineLoading || timelineError ? '—' : routeSuccessLabel(detailSLAHealth(group)) }}</strong>
              </div>
            </div>
            <RouteHistoryStrip :points="timelinePoints.filter(point=>point.group_id===group.id)" :loading="timelineLoading" :error="timelineError" />
          </article>
          <div v-if="!detailRows.length" class="empty">暂无线路</div>
        </div>
      </BaseDialog>
      <CreateLineKeyDialog :show="!!createTool" :tool-name="createTool?.label||''" :tool-id="createTool?.id" :groups="createTool?.active||[]" :metrics="metricsById" :metrics-generated-at="metricsGeneratedAt" :metrics-error="statsFailed" @retry-metrics="loadWorkspace" :linked-counts="counts" :rates="rates" :initial-group-id="createGroupId" @close="closeCreate" @created="keyCreated" />
      <PricingDialog :tool="pricingTool" :lines="pricingTool?.groups || []" :rates="rates" @close="closePricing" />
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
import RouteHistoryStrip from '@/features/ai-tools/RouteHistoryStrip.vue'
import { aggregateRouteSLACounts, getRouteTimeline, type RouteTimelinePoint } from '@/features/ai-tools/routeTimeline'
import PricingDialog from '@/features/ai-tools/PricingDialog.vue'
import userGroupsAPI from '@/api/groups'
import keysAPI from '@/api/keys'
import { getHybridPerformanceSnapshot } from '@/features/monitor-v4/api'
import { formatLineRate, resolveLineRate } from '@/components/keys/lineOptions'
import { checkLines, type LineCheck } from '@/features/ai-tools/api'
import { tools, linkedCounts, configuredLines, routeCountHealth, routeHealth, routeHealthTone, routeSuccessLabel, compareQuality, rankWeightedRoutes, metricLabel, providerIcon, platformLabel, toolIdsForGroup } from '@/features/ai-tools/model'
import { getDashboardWorkspaceSnapshot, setDashboardWorkspaceSnapshot } from '@/features/ai-tools/workspaceCache'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group, MonitorV4Window } from '@/features/monitor-v4/types'
import '@/styles/xingqiao-ai.css'

const timelinePoints=ref<RouteTimelinePoint[]>([]),timelineLoading=ref(false),timelineError=ref(false)
async function loadTimeline(window:MonitorV4Window,signal:AbortSignal){
 timelinePoints.value=[];timelineLoading.value=true;timelineError.value=false
 try{const points=await getRouteTimeline(window,signal,detailGranularity.value);if(!signal.aborted)timelinePoints.value=points}
 catch{if(!signal.aborted)timelineError.value=true}
 finally{if(!signal.aborted)timelineLoading.value=false}
}
const router=useRouter()
const authStore=useAuthStore()
const cacheUserId=String(authStore.user?.id||'')
const cachedWorkspace=getDashboardWorkspaceSnapshot(cacheUserId)
const clock=ref(Date.now())
let freshnessTimer: ReturnType<typeof setInterval> | undefined
const groups=ref<Group[]>(cachedWorkspace?.groups||[]), keys=ref<ApiKey[]>(cachedWorkspace?.keys||[]), rates=ref<Record<number,number>>(cachedWorkspace?.rates||{}), metrics=ref<MonitorV4Group[]>(cachedWorkspace?.metrics||[]), rankingMetrics=ref<MonitorV4Group[]>(cachedWorkspace?.rankingMetrics||[])
const metricsGeneratedAt=ref<string|null>(cachedWorkspace?.metricsGeneratedAt||null)
const rankingMetricsGeneratedAt=ref<string|null>(cachedWorkspace?.rankingMetricsGeneratedAt||null)
const loading=ref(false), loaded=ref(!!cachedWorkspace), workspaceError=ref(''), statsFailed=ref(false), statsError=ref(''), rankingFailed=ref(false), checking=ref(false), checkError=ref('')
const checks=ref<Record<number,LineCheck>>({}), detailWindow=ref<MonitorV4Window>('24h'), detailGranularity=ref<'hour'|'day'>('hour'), detailLoading=ref(false), detailError=ref(''), detailData=ref<MonitorV4Group[]>([])
let loadController:AbortController|undefined, detailController:AbortController|undefined, checkController:AbortController|undefined
const detailCache=new Map<MonitorV4Window,MonitorV4Group[]>()
let returnToDetailGroup:number|undefined
let detailsTrigger:HTMLElement|null=null, createTrigger:HTMLElement|null=null
const counts=computed(()=>linkedCounts(keys.value))
const metricsById=computed(()=>new Map(metrics.value.map(m=>[m.id,m])))
const rankingMetricsById=computed(()=>new Map(rankingMetrics.value.map(m=>[m.id,m])))
const detailSLACounts=computed(()=>timelineLoading.value || timelineError.value ? new Map<number,{request_count:number;success_count:number}>() : aggregateRouteSLACounts(timelinePoints.value))
function detailSLAHealth(group:Group){const counts=detailSLACounts.value.get(group.id);return routeCountHealth(counts?.success_count,counts?.request_count)}
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
const pricingTool=ref<ToolCard|null>(null)
let pricingTrigger:HTMLElement|null=null
function openPricing(tool:ToolCard){pricingTrigger=document.activeElement as HTMLElement;pricingTool.value=tool}
function closePricing(){pricingTool.value=null;nextTick(()=>pricingTrigger?.focus())}
const selectedTool=ref<ToolCard|null>(null), createTool=ref<ToolCard|null>(null),createGroupId=ref<number>()
const routeRows=computed(()=>sort(configuredLines(groups.value,keys.value)))
const detailRows=computed(()=>[...(selectedTool.value?.groups||[])].sort((a,b)=>(resolveLineRate(a,rates.value)??Infinity)-(resolveLineRate(b,rates.value)??Infinity)))
const detailBest=computed(()=>toolCards.value.find(tool=>tool.id===selectedTool.value?.id)?.best)
const periods=[{value:'1h' as const,label:'近 1 小时'},{value:'24h' as const,label:'近 24 小时'},{value:'7d' as const,label:'近 7 天'}]
const rateLabel=(g:Group)=>formatLineRate(resolveLineRate(g,rates.value))
function metricHealth(g:Group,ms:Map<number,MonitorV4Group>,hour:boolean){
  return routeHealth(hour&&statsFailed.value?undefined:ms.get(g.id),clock.value,hour?metricsGeneratedAt.value:undefined)
}
function successTone(g:Group,ms:Map<number,MonitorV4Group>,hour:boolean){return routeHealthTone(metricHealth(g,ms,hour))}
function successLabel(g:Group,ms:Map<number,MonitorV4Group>,hour:boolean){
  const health=metricHealth(g,ms,hour)
  return routeSuccessLabel(health)
}
function statsHint(g:Group){const m=metricsById.value.get(g.id);return !m||statsFailed.value||m.sla_request_count==null||m.sla_success_count==null?'SLA 统计暂不可用，请刷新重试':'最近 1 小时后台 SLA（排除业务限制及客户端取消）\n成功请求：'+m.sla_success_count+' 次\n总请求：'+m.sla_request_count+' 次'}
function checkOf(g:Group){
  if(g.status!=='active')return {kind:'muted',text:'线路已停用'}
  if(checking.value&&counts.value.has(g.id))return {kind:'warning',text:'检查中…'}
  const r=checks.value[g.id]
  if(!r)return {kind:'muted',text:'—'}
  if(r.status==='success'&&r.ttft_ms!=null)return {kind:'success',text:metricLabel(r.ttft_ms)}
  return {kind:r.status==='disabled'?'muted':'danger',text:r.status==='timeout'?'响应超时':r.status==='disabled'?'线路已停用':'检查失败'}
}
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
async function openDetails(tool:ToolCard){detailsTrigger=document.activeElement as HTMLElement;selectedTool.value=tool;detailGranularity.value='hour';await loadDetails('24h')}
function closeDetails(){detailController?.abort();selectedTool.value=null;nextTick(()=>detailsTrigger?.focus())}
async function loadDetails(window:MonitorV4Window){
  detailController?.abort();const c=new AbortController();detailController=c
  detailWindow.value=window;void loadTimeline(window,c.signal)
  detailLoading.value=true;detailError.value='';detailData.value=detailCache.get(window)||[]
  try{
    const result=await getHybridPerformanceSnapshot(window,c.signal)
    if(c.signal.aborted)return
    detailData.value=result.groups;detailCache.set(window,result.groups)
    clock.value=Date.now()
    if(window==='1h'){
      metrics.value=result.groups;metricsGeneratedAt.value=result.generated_at
      statsFailed.value=false;statsError.value=''
    }
    if(window==='24h'){
      rankingMetrics.value=result.groups;rankingMetricsGeneratedAt.value=result.generated_at
      rankingFailed.value=false
    }
    cacheWorkspace()
  }catch{
    if(!c.signal.aborted){
      detailError.value=detailCache.has(window)?'统计刷新失败，保留上次成功数据。':'统计读取失败，请重试。'
      if(window==='1h'){statsFailed.value=true;statsError.value='近 1 小时统计读取失败，请刷新重试。'}
      if(window==='24h'){
        rankingFailed.value=true;rankingMetrics.value=[];rankingMetricsGeneratedAt.value=null;cacheWorkspace()
      }
    }
  }finally{if(!c.signal.aborted)detailLoading.value=false}
}
function openCreate(tool:ToolCard,id?:number){createTrigger=document.activeElement as HTMLElement;createTool.value=tool;createGroupId.value=id??tool.best?.id}
function restoreDetailFocus(){
  if(returnToDetailGroup){document.querySelector<HTMLElement>(`[data-detail-group-id="${returnToDetailGroup}"]`)?.focus();returnToDetailGroup=undefined}
}
function closeCreate(){
  returnToDetailGroup=selectedTool.value?createGroupId.value:undefined
  createTool.value=null
  if(!selectedTool.value)nextTick(()=>createTrigger?.focus())
}
async function keyCreated(key:ApiKey){keys.value=[...keys.value.filter(k=>k.id!==key.id),key];createTool.value=null;await loadWorkspace();if(selectedTool.value)await loadDetails(detailWindow.value)}
const checkCooldown=ref(false)
let checkCooldownTimer:ReturnType<typeof setTimeout>|undefined
async function runChecks(){
  if(checking.value||checkCooldown.value)return
  const ids=routeRows.value.filter(g=>g.status==='active').map(g=>g.id);if(!ids.length)return
  checking.value=true;checkError.value='';checkController=new AbortController()
  try{const results=await checkLines(ids,checkController.signal);if(checkController.signal.aborted)return;for(const id of ids)checks.value[id]=results.find(r=>r.group_id===id)||{group_id:id,status:'failed',ttft_ms:null,checked_at:''};await loadWorkspace()}
  catch(error:unknown){if(!checkController.signal.aborted){
    const failure=error as {status?:number;code?:string;metadata?:{retry_after_seconds?:unknown}}|null
    if(failure?.status===429&&failure.code==='LINE_CHECK_RATE_LIMITED'){
      const retry=Number(failure.metadata?.retry_after_seconds)
      const seconds=Number.isFinite(retry)?Math.max(1,Math.min(60,Math.ceil(retry))):60
      checkError.value=`每分钟最多检查 30 次，请在 ${seconds} 秒后重试。`
      checkCooldown.value=true
      clearTimeout(checkCooldownTimer)
      checkCooldownTimer=setTimeout(()=>{checkCooldown.value=false;checkError.value=''},seconds*1000)
    }else if(failure?.status===429){checkError.value='检查过于频繁，请稍后重试。'}
    else if(failure?.code==='LINE_CHECK_UNAVAILABLE'){checkError.value='线路检查暂不可用，请稍后重试。'}
    else{checkError.value='线路检查请求未完成，请稍后重试。'}
  }}
  finally{checking.value=false}
}
function openGroupKeys(id:number){void router.push({path:'/keys',query:{group_id:String(id)}})}
onMounted(()=>{void loadWorkspace();freshnessTimer=setInterval(()=>{clock.value=Date.now()},30000)})
onBeforeUnmount(()=>{clearTimeout(checkCooldownTimer);clearInterval(freshnessTimer);loadController?.abort();detailController?.abort();checkController?.abort()})
</script>
