<template>
<section class="route-history" aria-label="线路指标历史">
 <p v-if="loading" role="status">正在读取线路走势…</p>
 <p v-else-if="error" role="status">线路走势读取失败，请重新选择统计范围重试。</p>
 <p v-else-if="!points.length">暂无历史统计</p>
 <template v-else>
  <div class="chart-legend" role="group" aria-label="切换指标曲线"><button v-for="s in series" :key="s.key" type="button" :class="s.key" :aria-pressed="enabled[s.key]" :title="`${enabled[s.key] ? '隐藏' : '显示'}${s.label}${s.key==='success' ? '：后台对应分组 SLA，排除业务限制及客户端取消' : ''}`" @click="enabled[s.key]=!enabled[s.key]"><span class="legend-check" aria-hidden="true"><svg v-if="enabled[s.key]" viewBox="0 0 16 16" fill="none"><path d="m3.5 8 3 3 6-6" /></svg></span><i aria-hidden="true"></i><span class="legend-label">{{ s.label }}</span></button></div>
  <div class="chart-layout">
   <div class="chart-axis"><span>100%</span><span>50%</span><span>0%</span></div>
   <div class="chart-plot" @mouseleave="tooltipOpen=false" @focusout="tooltipOpen=false" @keydown.esc="tooltipOpen=false">
    <svg viewBox="0 0 400 140" preserveAspectRatio="none" aria-hidden="true">
     <path class="grid" d="M0 4H400 M0 70H400 M0 136H400" />
     <line v-if="tooltipOpen" class="cursor" :x1="x(selected)" :x2="x(selected)" y1="4" y2="136" />
     <g v-for="s in visibleSeries" :key="s.key" :class="s.key">
      <path :data-series="s.key" :d="path(s.key)" class="line" />
      <template v-for="(p,i) in points" :key="p.start"><circle v-if="value(p,s.key)!==null" :cx="x(i)" :cy="y(value(p,s.key)!,s.key)" :r="selected===i?3.5:2" /></template>
     </g>
    </svg>
    <div class="chart-targets" role="group" aria-label="按时间查看线路指标"><button v-for="(p,i) in points" :key="p.start" :data-point="i" :aria-label="describe(p)" :aria-pressed="selected===i" :tabindex="selected===i?0:-1" @mouseenter="select(i)" @focus="select(i)" @click="select(i)" @keydown="navigate($event,i)"></button></div>
    <div v-if="tooltipOpen && points[selected]" class="chart-tooltip" role="status" :style="tooltipStyle">
     <strong>{{ tooltipTime(points[selected]!.start) }}</strong>
     <div class="tooltip-readings"><div v-for="s in tooltipSeries" :key="s.key" class="tooltip-row" :class="s.key"><span>{{ s.label }}</span><b>{{ formatted(points[selected]!,s.key).split('（')[0] }}</b></div></div><div v-if="tooltipSeries.some(s=>formatted(points[selected]!,s.key).includes('无样本'))" class="tooltip-note">缓存、成功率及首字无样本时按 0 展示</div>
    </div>
   </div>
   <div class="chart-axis seconds"><span>{{ secondsMax }}s</span><span>{{ secondsMax/2 }}s</span><span>0s</span></div>
  </div>
  <div class="chart-times"><span>{{ tick(points[0]!.start) }}</span><span>{{ tick(points[Math.floor(points.length/2)]!.start) }}</span><span>{{ tick(points[points.length-1]!.end) }}</span></div>
 </template>
</section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { RouteTimelinePoint } from './routeTimeline'
const props = defineProps<{points:RouteTimelinePoint[];loading:boolean;error:boolean}>()
type Metric = 'cache'|'success'|'ttft'
const series: {key:Metric;label:string}[] = [{key:'success',label:'请求成功率'},{key:'ttft',label:'首字 P50'},{key:'cache',label:'缓存命中率'}]
const tooltipSeries=series
const enabled=ref<Record<Metric,boolean>>({cache:true,success:true,ttft:true})
const visibleSeries=computed(()=>series.filter(s=>enabled.value[s.key]))
const tooltipOpen=ref(false)
function select(i:number){selected.value=i;tooltipOpen.value=true}
const selected = ref(0)
watch(()=>props.points,()=>{tooltipOpen.value=false;selected.value=Math.max(0,props.points.length-1)},{immediate:true})
const secondsMax = computed(()=>Math.max(1,Math.ceil(Math.max(0,...props.points.map(p=>p.ttft_p50_ms??0))/1000)))
const tooltipStyle=computed(()=>{
 const anchor=(selected.value+.5)/props.points.length*100
 return {left:`clamp(0px, calc(${anchor}% ${anchor<50?'+ 10px':'- 250px'}), max(0px, calc(100% - 240px)))`}
})
const x = (i:number)=>(i+.5)*400/props.points.length
function value(p:RouteTimelinePoint,m:Metric):number|null {
 if(m==='cache')return (p.cache_hit_rate??0)*100
 if(m==='ttft')return (p.ttft_p50_ms??0)/1000
 return p.request_count?p.success_count/p.request_count*100:0
}
const y = (v:number,m:Metric)=>136-v/(m==='ttft'?secondsMax.value:100)*132
function path(m:Metric){
 let connected=false
 return props.points.map((p,i)=>{const v=value(p,m);if(v===null){connected=false;return ''}const command=connected?'L':'M';connected=true;return `${command}${x(i)},${y(v,m)}`}).join(' ')
}
const tooltipTime=(v:string)=>{const d=new Date(v);const pad=(n:number)=>String(n).padStart(2,'0');return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}  ${pad(d.getHours())}:${pad(d.getMinutes())}`}
const time=(v:string)=>new Date(v).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false})
const tick=(v:string)=>new Date(v).toLocaleString('zh-CN',props.points.length>1 && Date.parse(props.points[props.points.length-1]!.end)-Date.parse(props.points[0]!.start)>86400000?{month:'2-digit',day:'2-digit'}:{hour:'2-digit',minute:'2-digit',hour12:false})
function formatted(p:RouteTimelinePoint,m:Metric){const reading=value(p,m);const v=reading??0;const missing=m==='cache'?p.cache_hit_rate==null:m==='ttft'?p.ttft_p50_ms==null:!p.request_count;return (m==='ttft'?`${v.toFixed(2)}s`:`${Number(v.toFixed(1))}%`)+(missing?'（无样本，按 0 展示）':'')}
const describe=(p:RouteTimelinePoint)=>`${time(p.start)} – ${time(p.end)} · ${series.map(s=>`${s.label} ${formatted(p,s.key)}`).join(' · ')}`
function navigate(event:KeyboardEvent,i:number){
 let next=i
 if(event.key==='ArrowRight')next=Math.min(props.points.length-1,i+1)
 else if(event.key==='ArrowLeft')next=Math.max(0,i-1)
 else if(event.key==='Home')next=0
 else if(event.key==='End')next=props.points.length-1
 else return
 event.preventDefault();select(next)
 const container=(event.currentTarget as HTMLElement).parentElement
 ;(container?.children[next] as HTMLElement)?.focus()
}
</script>
<style scoped>
.route-history{min-width:0;--chart-cache:#b49aee;--chart-ttft:#82b7f3;color:var(--xq-secondary);font-size:12px}
:global(:root:not(.dark) .route-history){--chart-cache:#7651b5;--chart-ttft:#326ea9}
.cache{color:var(--chart-cache)}.success{color:var(--xq-success)}.ttft{color:var(--chart-ttft)}
.chart-legend{display:flex;flex-wrap:wrap;gap:4px 16px;margin-bottom:20px;font-size:11px}
.chart-legend button{font:inherit;color:var(--xq-secondary);min-height:36px;background:transparent;border:0;border-radius:4px;padding:6px 0;cursor:pointer;display:inline-flex;align-items:center;gap:6px;white-space:nowrap}
.chart-legend button:hover{color:var(--xq-text);background:var(--xq-raised)}
.legend-check{display:inline-flex;align-items:center;justify-content:center;width:14px;height:14px;flex-shrink:0;border:1px solid var(--xq-secondary);border-radius:3px;background:var(--xq-raised);color:var(--xq-text)}
.legend-check svg{width:12px;height:12px;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
.chart-legend button[aria-pressed="false"] .legend-check{background:transparent;border-color:var(--xq-secondary)}
.chart-legend i{width:14px;flex-shrink:0;border-top:2px solid currentColor}
.chart-legend .cache i{color:var(--chart-cache);border-top-style:dashed}
.chart-legend .success i{color:var(--xq-success)}
.chart-legend .ttft i{color:var(--chart-ttft)}
.chart-legend button[aria-pressed="false"] .legend-label{text-decoration:line-through;text-decoration-thickness:1px}
.chart-legend button[aria-pressed="false"] i{color:var(--xq-secondary);opacity:.4}
.chart-legend button:focus-visible{outline:2px solid var(--xq-accent);outline-offset:3px}
.chart-tooltip{position:absolute;top:8px;z-index:5;max-width:100%;box-sizing:border-box;width:240px;padding:10px 12px;border:1px solid var(--xq-border);border-radius:8px;background:var(--xq-surface);color:var(--xq-text);pointer-events:none;font-size:11px;line-height:1.6;box-shadow:0 4px 12px #0003}
.chart-tooltip strong{white-space:pre-wrap;display:block;font-size:10px;overflow-wrap:anywhere;margin-bottom:5px}.tooltip-readings{display:grid;grid-template-columns:max-content minmax(0,1fr);column-gap:8px;font-variant-numeric:tabular-nums}.tooltip-row{display:contents}.tooltip-row b{text-align:right;white-space:nowrap}.tooltip-row small{font-size:inherit;white-space:nowrap}.tooltip-note{margin-top:5px;color:var(--xq-secondary);font-size:10px}
.chart-layout{display:grid;grid-template-columns:34px minmax(0,1fr) 30px;gap:6px;height:140px}
.chart-axis{display:flex;flex-direction:column;justify-content:space-between;text-align:right;font-size:10px;line-height:12px;font-variant-numeric:tabular-nums}.seconds{text-align:left;color:var(--xq-secondary)}
.chart-plot{position:relative;min-width:0}.chart-plot svg{display:block;width:100%;height:100%;overflow:visible}
.grid{fill:none;stroke:var(--xq-border);stroke-width:1;vector-effect:non-scaling-stroke}
.cursor{stroke:var(--xq-secondary);stroke-dasharray:3 4;opacity:.5;vector-effect:non-scaling-stroke}
.line{fill:none;stroke:currentColor;stroke-width:2;stroke-linejoin:round;vector-effect:non-scaling-stroke}.cache .line{stroke-dasharray:5 4}
circle{fill:var(--xq-surface);stroke:currentColor;stroke-width:1.5;vector-effect:non-scaling-stroke}
.chart-targets{position:absolute;inset:0;display:flex}.chart-targets button{flex:1;min-width:0;padding:0;border:0;background:transparent;cursor:crosshair}.chart-targets button:focus-visible{outline:2px solid var(--xq-accent);outline-offset:2px}
.chart-times{display:flex;justify-content:space-between;gap:4px;margin:10px 36px 0 40px;font-size:10px;font-variant-numeric:tabular-nums}
@media(max-width:700px){.chart-legend{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 16px;margin-bottom:16px}.chart-legend button{min-height:44px}.chart-layout{height:128px}}
</style>
