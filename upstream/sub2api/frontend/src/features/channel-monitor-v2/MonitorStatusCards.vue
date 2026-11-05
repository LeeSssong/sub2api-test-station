<template>
  <div class="space-y-7" data-testid="monitor-status-cards">
    <div v-if="loading && !items.length" class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      <div v-for="index in 6" :key="index" class="h-72 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800" />
    </div>
    <div v-else-if="!items.length" class="card py-16 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.empty.title') }}</div>
    <div v-if="items.length && showTimelineControls !== false" class="flex flex-wrap items-center justify-between gap-2" data-testid="card-timeline-mode">
      <div class="tabs inline-flex shrink-0" role="group" :aria-label="t('channelMonitorV2.cards.timelineMode.label')">
        <button type="button" class="tab !px-2.5 !py-1 text-xs" :class="timelineMode === 'windows' ? 'tab-active' : ''" data-testid="card-timeline-windows" @click="setTimelineMode('windows')">{{ t('channelMonitorV2.cards.timelineMode.windows') }}</button>
        <button type="button" class="tab !px-2.5 !py-1 text-xs" :class="timelineMode === 'samples' ? 'tab-active' : ''" data-testid="card-timeline-samples" @click="setTimelineMode('samples')">{{ t('channelMonitorV2.cards.timelineMode.samples') }}</button>
      </div>
    </div>
    <section v-for="platform in cardSections" :key="platform.name" class="space-y-3" :data-platform="platform.name">
      <h2 v-if="groupByPlatform !== false" class="flex items-center gap-2 text-sm font-semibold text-gray-700 dark:text-gray-200">
        <span class="grid h-7 w-7 place-items-center rounded-full bg-gray-100 dark:bg-dark-700"><ProviderIcon :provider="platform.name" :size="17" /></span>
        {{ providerLabel(platform.name) }}
        <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ platform.items.length }}</span>
      </h2>
      <div class="monitor-card-grid grid grid-cols-1 items-start gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        <article v-for="row in platform.items" :key="`${row.platform}-${row.group_id}`" class="min-w-0 rounded-3xl border border-gray-200/80 bg-white/80 p-5 shadow-sm dark:border-dark-700/70 dark:bg-dark-800/80" data-testid="monitor-group-card" :class="{ 'monitor-card--priority': prioritizeAvailability }">
          <div v-if="bestLineKeys.includes(`${row.platform}:${row.group_id}`)" class="monitor-card-best" data-testid="best-line-badge" :title="t(bestLineKeys.length > 1 ? 'channelMonitorV2.cards.bestLineTiedHint' : 'channelMonitorV2.cards.bestLineHint')"><span>{{ t('channelMonitorV2.cards.bestLine') }}</span></div>
          <header class="monitor-card-header" :class="{ 'monitor-card-header--associate': associateKeys }">
            <div v-if="showProvider" class="monitor-card-provider" data-testid="card-provider">
              <ProviderIcon :provider="row.platform" :size="24" />
              <span :title="providerLabel(row.platform)">{{ providerLabel(row.platform) }}</span>
            </div>
            <div class="monitor-card-identity" data-testid="card-group-meta">
              <span class="monitor-card-health rounded-full px-2 py-1 text-[10px] font-semibold" :class="healthBadge[row.health.overall]">{{ t(`channelMonitorV2.cards.health.${row.health.overall}`) }}</span>
              <div class="monitor-card-title-meta">
                <h3 class="min-w-0 text-base font-semibold text-gray-900 dark:text-gray-100" :title="row.group_name">{{ row.group_name || `#${row.group_id}` }}</h3>
                <span v-if="row.group_rate_multiplier != null" class="monitor-card-rate">{{ t('channelMonitorV2.cards.groupRate', { value: row.group_rate_multiplier.toFixed(2) }) }}</span>
              </div>
            </div>
            <button v-if="associateKeys" type="button" class="xq-button associate-key monitor-card-associate" data-testid="associate-group-key" :data-associate-group-id="row.group_id" :data-associate-platform="row.platform" :disabled="row.group_id == null || !associableGroupIds.includes(row.group_id)" :aria-label="t('channelMonitorV2.tool.associateGroupKey', { group: row.group_name || '#' + row.group_id })" @click="emit('associate', row)">{{ t('channelMonitorV2.tool.associateKey') }}</button>
          </header>

          <dl class="mt-5 grid grid-cols-3 gap-2" :class="{ 'monitor-card-metrics--priority': prioritizeAvailability }">
            <div v-for="metric in metrics(row)" :key="metric.name" class="min-w-0 rounded-2xl border border-gray-100 bg-gray-50/70 px-2.5 py-3 dark:border-dark-700/60 dark:bg-dark-900/40">
              <dt class="truncate text-[10px] tracking-wide text-gray-500 dark:text-gray-400" :title="metric.title || metric.name">{{ metric.name }}</dt>
              <dd class="mt-1.5 font-mono text-base font-semibold tabular-nums" :class="metric.color" :title="metric.status" :aria-label="`${metric.name} ${metric.value} · ${metric.status}`">{{ metric.value }}</dd>
            </div>
          </dl>
          <div class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700/70">
            <div class="mb-2 flex justify-between text-[10px] text-gray-500 dark:text-gray-400">
              <span>{{ timelineCaption(row) }}</span><span>{{ t('channelMonitorV2.cards.refreshIn', { seconds: countdown }) }}</span>
            </div>
            <p v-if="timelineMode === 'samples' && !timelineBars(row).length" class="text-[10px] text-gray-400" data-testid="card-timeline-empty">{{ t('channelMonitorV2.cards.noSamples') }}</p>
            <template v-else>
              <div class="flex h-5 items-end gap-1" :aria-label="t('channelMonitorV2.cards.trafficHistory')">
                <button v-for="(bar, index) in timelineBars(row)" :key="`${bar.start}-${index}`" type="button"
                  class="min-w-0 flex-1 cursor-help rounded-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-400"
                  :class="healthBar[bar.state]" :aria-label="trafficLabel(row, bar)" :aria-describedby="isTrafficDetail(row, bar) ? tooltipId : undefined"
                  data-testid="traffic-history-bar" @mouseenter="openDetail($event, { kind: 'traffic', row, bar })"
                  @focus="openDetail($event, { kind: 'traffic', row, bar })" @click="openDetail($event, { kind: 'traffic', row, bar })"
                  @mouseleave="scheduleClose" @blur="scheduleClose" />
              </div>
              <div class="mt-1 flex justify-between font-mono text-[9px] tracking-widest text-gray-400"><span>PAST</span><span>NOW</span></div>
            </template>
          </div>

          <section v-if="showCandy && candyHistories(row).length" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700/70" data-testid="candy-history">
              <h4 class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('channelMonitorV2.candy.title') }}</h4>
              <div v-for="(history, historyIndex) in candyHistories(row)" :key="`${history.model}-${history.reasoning_effort}-${historyIndex}`" class="mt-2" data-testid="candy-model-history">
                <div class="flex items-center justify-between gap-2 text-xs">
                  <p class="min-w-0 break-words text-[10px] leading-relaxed text-gray-500 dark:text-gray-400">{{ history.model }} · {{ history.reasoning_effort }} · {{ t('channelMonitorV2.candy.cadence', { minutes: history.interval_minutes }) }} · {{ t('channelMonitorV2.candy.window', { count: candyHistoryLimit(history) }) }}</p>
                  <span class="flex shrink-0 items-center gap-1.5 text-[10px] text-gray-500 dark:text-gray-400"><i class="h-2 w-2 rounded-full" :class="candyColor[candyDisplayState(history, now)]" />{{ t(`channelMonitorV2.candy.states.${candyDisplayState(history, now)}`) }}</span>
                </div>
                <div class="mt-2 flex h-5 gap-px" role="group" :aria-label="t('channelMonitorV2.candy.historyCount', { count: Math.min(history.results.length, candyHistoryLimit(history)), limit: candyHistoryLimit(history) })" data-testid="candy-history-slots">
                  <template v-for="(result, index) in candyHistorySlots(history)" :key="index">
                    <button v-if="result" type="button"
                    class="min-w-0 flex-1 cursor-help rounded-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-400"
                    :class="candyColor[result.verdict]" :aria-label="candyTooltip(result, history.model, history.reasoning_effort)" :aria-describedby="isCandyDetail(row, result, history.model, history.reasoning_effort) ? tooltipId : undefined"
                    data-testid="candy-history-bar" @mouseenter="openDetail($event, { kind: 'candy', row, result, model: history.model, effort: history.reasoning_effort })"
                    @focus="openDetail($event, { kind: 'candy', row, result, model: history.model, effort: history.reasoning_effort })" @click="openDetail($event, { kind: 'candy', row, result, model: history.model, effort: history.reasoning_effort })"
                    @mouseleave="scheduleClose" @blur="scheduleClose" />
                    <span v-else class="min-w-0 flex-1 rounded-sm bg-gray-200 dark:bg-dark-600" aria-hidden="true" :title="t('channelMonitorV2.candy.notChecked')" data-testid="candy-history-placeholder" />
                  </template>
                </div>
                <p v-if="!history.results.length" class="mt-3 text-[10px] text-gray-400">{{ t('channelMonitorV2.candy.waiting') }}</p>
              </div>
              <div class="mt-2 flex items-center justify-between gap-2 text-[9px] text-gray-500 dark:text-gray-400"><span>{{ t('channelMonitorV2.candy.legend') }}</span><span>{{ latestCandyTime(row) }}</span></div>
          </section>
          <button v-if="modelDetails" type="button" class="monitor-card-models" data-testid="view-group-models" :data-model-group-id="row.group_id" :data-model-platform="row.platform" :aria-label="`${row.group_name} · ${modelCountLabel(row)} · ${t('channelMonitorV2.tool.viewAllModels')}`" @click="emit('models', row)"><span data-testid="supported-model-count">{{ modelCountLabel(row) }}</span><span>{{ t('channelMonitorV2.tool.viewAllModels') }} →</span></button>
        </article>
      </div>
    </section>
    <Teleport to="body">
      <div v-if="detail" :id="tooltipId" ref="tooltipElement" role="tooltip" tabindex="0"
        class="monitor-card-tooltip fixed z-[100] w-max max-w-[calc(100vw-24px)] rounded-xl border border-gray-300 bg-white px-3 py-2 text-xs leading-relaxed text-gray-800 shadow-xl dark:border-gray-400 dark:bg-dark-900 dark:text-gray-100"
        :style="{ left: placement.left + 'px', top: placement.top + 'px', visibility: placement.ready ? 'visible' : 'hidden' }"
        @mouseenter="cancelClose" @mouseleave="scheduleClose" @focus="cancelClose" @blur="scheduleClose">
        <span aria-hidden="true" class="absolute h-2.5 w-2.5 rotate-45 border-gray-300 bg-white dark:border-gray-400 dark:bg-dark-900"
          :class="placement.below ? '-top-1.5 border-l border-t' : '-bottom-1.5 border-b border-r'" :style="{ left: placement.arrowLeft + 'px' }" />
        <div class="max-h-[min(18rem,calc(100vh-48px))] max-w-lg overflow-y-auto overscroll-contain break-words" data-testid="monitor-card-tooltip-content">
          <p class="font-semibold">{{ detail.row.group_name || '#' + detail.row.group_id }}</p>
          <template v-if="detail.kind === 'traffic'">
            <p class="text-gray-500 dark:text-gray-400">{{ dateTime(detail.bar.start) }} - {{ dateTime(detail.bar.end) }} · {{ t('channelMonitorV2.cards.health.' + detail.bar.state) }}</p>
            <p v-if="!detail.bar.buckets.length" class="mt-1">{{ t('channelMonitorV2.matrix.noTraffic') }}</p>
            <template v-else>
              <p class="mt-1 text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.cards.bucketDetails') }}</p>
              <p v-for="bucket in detail.bar.buckets" :key="bucket.bucket_start" class="mt-1" data-testid="monitor-card-tooltip-sample">{{ trafficSample(bucket) }}</p>
            </template>
          </template>
          <p v-else class="mt-1 whitespace-pre-wrap">{{ candyTooltip(detail.result, detail.model, detail.effort) }}</p>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { HealthState, MonitorCandyResult, MonitorCoverage, MonitorMatrixBucket, MonitorMatrixRow } from '@/api/channelMonitorV2'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { formatMonitorMs, formatMonitorPercent } from './monitorFormat'
import { candyDisplayState, candyHistoryLimit, candyHistorySlots, hasMonitorSamples, MONITOR_CARD_SLOT_COUNT, monitorCandyHistories, monitorCardTimeline, type MonitorCardTimelineMode } from './monitorCards'

const props = withDefaults(defineProps<{ items: MonitorMatrixRow[]; coverage?: MonitorCoverage; countdown: number; loading: boolean; now: number; platformOrder?: string[]; showCandy?: boolean; modelDetails?: boolean; initialTimelineMode?: MonitorCardTimelineMode; timelineMode?: MonitorCardTimelineMode; showTimelineControls?: boolean; groupByPlatform?: boolean; supportedModelCounts?: Record<number, number>; modelCountsLoading?: boolean; showProvider?: boolean; prioritizeAvailability?: boolean; bestLineKeys?: string[]; associateKeys?: boolean; associableGroupIds?: number[] }>(), { bestLineKeys: () => [], showProvider: true, showTimelineControls: true, groupByPlatform: true, associableGroupIds: () => [] })
const emit = defineEmits<{ models: [MonitorMatrixRow]; associate: [MonitorMatrixRow] }>()
const { t, locale } = useI18n()
type TrafficBar = ReturnType<typeof monitorCardTimeline>[number]
type CardDetail = { kind: 'traffic'; row: MonitorMatrixRow; bar: TrafficBar } | { kind: 'candy'; row: MonitorMatrixRow; result: MonitorCandyResult; model: string; effort: string }
const localTimelineMode = ref<MonitorCardTimelineMode>(props.initialTimelineMode || 'windows')
const timelineMode = computed(() => props.timelineMode || localTimelineMode.value)
const tooltipId = useId()
const detail = shallowRef<CardDetail | null>(null)
const anchor = shallowRef<HTMLElement | null>(null)
const tooltipElement = ref<HTMLElement | null>(null)
const placement = ref({ left: 0, top: 0, arrowLeft: 0, below: false, ready: false })
let closeTimer: ReturnType<typeof setTimeout> | undefined

function cancelClose() {
  if (closeTimer) clearTimeout(closeTimer)
  closeTimer = undefined
}
function closeDetail() {
  cancelClose()
  detail.value = null
  anchor.value = null
}
function scheduleClose() {
  cancelClose()
  // Allow the pointer to cross the arrow gap and scroll multi-bucket details.
  closeTimer = setTimeout(() => {
    if (document.activeElement !== anchor.value && !tooltipElement.value?.contains(document.activeElement)) closeDetail()
  }, 120)
}
function openDetail(event: Event, value: CardDetail) {
  cancelClose()
  anchor.value = event.currentTarget as HTMLElement
  detail.value = value
  placement.value.ready = false
  void nextTick(positionDetail)
}
function positionDetail() {
  if (!anchor.value || !tooltipElement.value || !detail.value) return
  const rect = anchor.value.getBoundingClientRect()
  const box = tooltipElement.value.getBoundingClientRect()
  const center = rect.left + rect.width / 2
  const left = Math.max(12, Math.min(window.innerWidth - box.width - 12, center - box.width / 2))
  const below = rect.top < box.height + 22
  const top = Math.max(12, Math.min(window.innerHeight - box.height - 12, below ? rect.bottom + 10 : rect.top - box.height - 10))
  placement.value = { left, top, below, ready: true, arrowLeft: Math.max(8, Math.min(box.width - 18, center - left - 5)) }
}
function onOutsidePointer(event: Event) {
  if (!(event.target instanceof Node)) return
  if (!anchor.value?.contains(event.target) && !tooltipElement.value?.contains(event.target)) closeDetail()
}
function onViewportChange(event: Event) {
  if (event.target instanceof Node && tooltipElement.value?.contains(event.target)) return
  closeDetail()
}
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeDetail()
}
function isTrafficDetail(row: MonitorMatrixRow, bar: TrafficBar) {
  return detail.value?.kind === 'traffic' && detail.value.row === row && detail.value.bar.start === bar.start
}
function isCandyDetail(row: MonitorMatrixRow, result: MonitorCandyResult, model: string, effort: string) {
  return detail.value?.kind === 'candy' && detail.value.row === row && detail.value.model === model && detail.value.effort === effort && detail.value.result.checked_at === result.checked_at
}
function setTimelineMode(mode: MonitorCardTimelineMode) {
  localTimelineMode.value = mode
  closeDetail()
}
function timelineBars(row: MonitorMatrixRow) {
  return monitorCardTimeline(row, props.coverage, timelineMode.value)
}
function timelineCaption(row: MonitorMatrixRow) {
  if (timelineMode.value === 'samples') return t('channelMonitorV2.cards.sampledWindows', { count: timelineBars(row).length, limit: MONITOR_CARD_SLOT_COUNT })
  return t('channelMonitorV2.cards.windows', { count: MONITOR_CARD_SLOT_COUNT })
}
function candyHistories(row: MonitorMatrixRow) {
  return monitorCandyHistories(row)
}
function latestCandyTime(row: MonitorMatrixRow) {
  // Each history is ordered oldest-first. Compare instants, not ISO text:
  // fractional seconds do not sort lexicographically.
  let latest = Number.NEGATIVE_INFINITY
  for (const history of candyHistories(row)) {
    const parsed = Date.parse(history.results.at(-1)?.checked_at || '')
    if (Number.isFinite(parsed) && parsed > latest) latest = parsed
  }
  return Number.isFinite(latest) ? time(latest) : '—'
}
watch([() => props.items, () => props.coverage, timelineMode], closeDetail)
onMounted(() => {
  document.addEventListener('pointerdown', onOutsidePointer, true)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})
onBeforeUnmount(() => {
  cancelClose()
  document.removeEventListener('pointerdown', onOutsidePointer, true)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
const { providerLabel } = useChannelMonitorFormat()
const platforms = computed(() => {
  const groups = new Map<string, MonitorMatrixRow[]>()
  for (const row of props.items) {
    if (!row.group_id) continue
    if (!groups.has(row.platform)) groups.set(row.platform, [])
    groups.get(row.platform)!.push(row)
  }
  const order = props.platformOrder?.length ? props.platformOrder : ['openai', 'anthropic', 'grok', 'gemini', 'antigravity', 'kiro']
  const rank = (name: string) => {
    const index = order.indexOf(name)
    return index < 0 ? order.length : index
  }
  return [...groups].map(([name, items]) => ({ name, items })).sort((a, b) => rank(a.name) - rank(b.name) || a.name.localeCompare(b.name))
})
const cardSections = computed(() => props.groupByPlatform === false
  ? [{ name: 'all', items: platforms.value.flatMap(platform => platform.items) }]
  : platforms.value)
const healthBadge = { healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300', warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300', critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300', unknown: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400' }
const healthBar = { healthy: 'h-5 bg-emerald-400', warning: 'h-3 bg-amber-400', critical: 'h-2 bg-red-400', unknown: 'h-1 bg-gray-300 dark:bg-dark-600' }
const candyColor = { correct: 'bg-emerald-400', incorrect: 'bg-amber-400', error: 'bg-red-400', unknown: 'bg-gray-400', stale: 'bg-gray-400' }
const time = (value: number | string) => new Date(value).toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit', hour12: false })
function modelCountLabel(row: MonitorMatrixRow) {
  const count = row.group_id == null ? undefined : props.supportedModelCounts?.[row.group_id]
  if (count != null) return t('channelMonitorV2.tool.supportedModelCount', { count })
  return t(props.modelCountsLoading ? 'channelMonitorV2.tool.modelCountLoading' : 'channelMonitorV2.tool.modelCountUnavailable')
}
const metricColors = {
  healthy: 'text-emerald-800 dark:text-emerald-300',
  warning: 'text-amber-800 dark:text-amber-300',
  critical: 'text-red-700 dark:text-red-300',
  unknown: 'text-gray-600 dark:text-gray-400',
}
function metricAppearance(state: HealthState | undefined, available: boolean) {
  const effectiveState = available && state ? state : 'unknown'
  return { color: metricColors[effectiveState], status: t(`channelMonitorV2.cards.health.${effectiveState}`) }
}
function metrics(row: MonitorMatrixRow): Array<{ name: string; title?: string; value: string; color: string; status: string }> {
  const sampled = hasMonitorSamples(row.metrics)
  const values = [
    { name: t('channelMonitorV2.cards.cache'), value: sampled ? formatMonitorPercent(row.metrics.cache_rate) : '—', ...metricAppearance(row.health.cache, sampled) },
    { name: t('channelMonitorV2.cards.availability'), value: sampled ? formatMonitorPercent(1 - row.metrics.error_rate) : '—', ...metricAppearance(row.health.error_rate, sampled) },
    { name: t('channelMonitorV2.cards.ttft'), title: t('channelMonitorV2.metrics.ttftP50'), value: formatMonitorMs(row.metrics.ttft.p50_ms), ...metricAppearance(row.health.ttft, sampled && row.metrics.ttft.p50_ms != null) }
  ]
  return props.prioritizeAvailability ? [values[1], values[2], values[0]] : values
}
function dateTime(value: number | string) {
  return new Date(value).toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}
function trafficLabel(row: MonitorMatrixRow, bar: TrafficBar) {
  return (row.group_name || '#' + row.group_id) + ' · ' + dateTime(bar.start) + ' - ' + dateTime(bar.end) + ' · ' + t('channelMonitorV2.cards.health.' + bar.state)
}
function trafficSample(bucket: MonitorMatrixBucket) {
  return t('channelMonitorV2.cards.sampleDetails', {
    time: dateTime(bucket.bucket_start), availability: formatMonitorPercent(1 - bucket.metrics.error_rate),
    cache: formatMonitorPercent(bucket.metrics.cache_rate), ttft: formatMonitorMs(bucket.metrics.ttft.p50_ms),
  })
}
function candyTooltip(result: MonitorCandyResult, model?: string, effort?: string) {
  const label = [model, effort].filter(Boolean).join(' · ')
  return (label ? label + ' · ' : '') + dateTime(result.checked_at) + ' · ' + t('channelMonitorV2.candy.states.' + result.verdict)
    + ' · ' + t('channelMonitorV2.metrics.durationValue', { value: formatMonitorMs(result.latency_ms) })
    + (result.answer_preview ? '\n' + result.answer_preview : '')
}
</script>

<style scoped>
.monitor-card--priority{position:relative}
.monitor-card-best{position:absolute;top:0;left:0;width:64px;height:64px;overflow:hidden;border-top-left-radius:12px}
.monitor-card-best>span{position:absolute;top:8px;left:-26px;display:block;width:88px;line-height:20px;text-align:center;transform:rotate(-45deg);background:var(--xq-success);color:var(--xq-canvas);font-size:10px;font-weight:600;white-space:nowrap}
.monitor-card-metrics--priority{grid-template-columns:minmax(0,1.12fr) minmax(0,1fr);grid-template-rows:repeat(2,minmax(54px,auto));gap:8px}
.monitor-card-metrics--priority>div{padding:10px}
.monitor-card-metrics--priority>div:first-child{grid-row:span 2;display:flex;flex-direction:column;justify-content:center}
.monitor-card-metrics--priority>div:first-child dt{font-size:11px}
.monitor-card-metrics--priority>div:first-child dd{margin-top:12px;font-size:30px;line-height:1.1}
.monitor-card-metrics--priority>div:not(:first-child) dd{margin-top:4px;font-size:18px;line-height:1.2}
.monitor-card-header{display:flex;flex-wrap:wrap;align-items:center;gap:10px}
.monitor-card-provider{display:flex;width:40px;flex:none;flex-direction:column;align-items:center;gap:5px;color:var(--xq-secondary)}
.monitor-card-provider>span{max-width:48px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:9px;line-height:1.2;color:var(--xq-secondary)}
.monitor-card-identity{display:flex;flex:1;min-width:0;align-items:center;gap:8px}
.monitor-card-title-meta{display:flex;min-width:0;flex-wrap:wrap;align-items:center;gap:6px 8px}
.monitor-card-title-meta>h3{max-width:100%;overflow-wrap:anywhere;line-height:1.4}
.monitor-card-header:has(.monitor-card-provider){align-items:flex-start}
.monitor-card-header:has(.monitor-card-provider) .monitor-card-identity{flex-wrap:wrap}
.monitor-card-header:has(.monitor-card-provider) .monitor-card-title-meta{display:contents}
.monitor-card-header:has(.monitor-card-provider) h3{flex-basis:100%;order:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.monitor-card-header:has(.monitor-card-provider) .monitor-card-rate{order:1}
.monitor-card-header:has(.monitor-card-provider) .monitor-card-health{order:2}
.monitor-card-health{flex:none;white-space:nowrap}
.monitor-card-rate{flex:none;border-radius:999px;padding:4px 8px;background:color-mix(in srgb,var(--xq-accent) 13%,var(--xq-surface));color:var(--xq-accent);font-size:11px;font-weight:500;line-height:1.5;white-space:nowrap;font-variant-numeric:tabular-nums}
.monitor-card-associate{margin-left:auto;white-space:nowrap}
.monitor-card-header--associate{flex-wrap:nowrap;align-items:flex-start}
.monitor-card-header--associate .monitor-card-identity{align-items:flex-start;padding-top:5px}
.monitor-card-header--associate .monitor-card-health{margin-top:0}
.monitor-card-models{display:flex;align-items:center;justify-content:space-between;gap:12px;width:100%;margin-top:12px;padding:10px 0 0;border-top:1px solid color-mix(in srgb,var(--xq-border) 55%,transparent);color:var(--xq-muted);font-size:11px;line-height:1.6;text-align:left;cursor:pointer;transition:color .15s}
.monitor-card-models:hover,.monitor-card-models:focus-visible{color:var(--xq-accent)}
.monitor-card-models:focus-visible{outline:2px solid var(--xq-accent);outline-offset:3px}

</style>
