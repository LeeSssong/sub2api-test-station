<template>
  <section ref="surface" class="tool-monitor-details" data-testid="tool-monitor-details">
    <div class="tool-monitor-toolbar">
      <button v-if="selectedGroup" type="button" class="btn btn-secondary" data-testid="tool-monitor-back" @click="backToLines"><Icon name="arrowLeft" size="sm" />{{ t('channelMonitorV2.tool.back', { tool: toolName }) }}</button>
      <div class="tabs" role="group" :aria-label="t('channelMonitorV2.tool.period')">
        <button v-for="period in periods" :key="period.value" type="button" class="tab" :class="range === period.value ? 'tab-active' : ''" :data-range="period.value" :aria-pressed="range === period.value" @click="setRange(period.value)">{{ t(period.label) }}</button>
      </div>
      <button type="button" class="btn tool-monitor-refresh" :disabled="loading || modelsLoading" @click="selectedGroup ? loadModels() : loadLines()"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading || modelsLoading }" />{{ t('common.refresh') }}</button>
    </div>
    <template v-if="selectedGroup">
      <div v-if="modelsError" class="tool-monitor-message" role="alert">{{ t('channelMonitorV2.tool.loadError') }}<button class="btn btn-secondary" data-testid="tool-model-retry" @click="loadModels">{{ t('channelMonitorV2.tool.retry') }}</button></div>
      <div v-else-if="modelsLoading" class="tool-monitor-message" role="status">{{ t('channelMonitorV2.tool.loadingModels') }}</div>
      <div v-else-if="!modelRows.length" class="tool-monitor-message" role="status">{{ t('channelMonitorV2.tool.noModels') }}</div>
      <MonitorModelPerformance v-else :items="modelRows" />
    </template>
    <template v-else>
      <div v-if="error" class="tool-monitor-message" role="alert">{{ t('channelMonitorV2.tool.loadError') }}<button type="button" class="btn btn-secondary" data-testid="tool-monitor-retry" @click="loadLines">{{ t('channelMonitorV2.tool.retry') }}</button></div>
      <div v-else-if="loading" class="tool-monitor-message" role="status">{{ t('channelMonitorV2.tool.loading') }}</div>
      <div v-else-if="!rows.length" class="tool-monitor-message" role="status">{{ t('channelMonitorV2.tool.noGroups') }}</div>
      <template v-else>
        <p v-if="!rows.some(row => hasMonitorSamples(row.metrics))" class="tool-monitor-description" role="status">{{ t('channelMonitorV2.tool.noSamples') }}</p>
        <MonitorStatusCards :items="rows" :coverage="coverage" :countdown="countdown" :loading="false" :now="now" :platform-order="platformOrder" :timeline-mode="timelineMode" :show-timeline-controls="false" :group-by-platform="false" :show-provider="false" :supported-model-counts="supportedModelCounts" :model-counts-loading="modelCountsLoading" model-details associate-keys prioritize-availability :best-line-keys="bestLineKeys" :associable-group-ids="associableGroupIds" @models="openModels" @associate="associateGroup" />
      </template>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { getMatrix, getModels, getSnapshot, type MonitorCoverage, type MonitorFilter, type MonitorMatrixRow, type MonitorModelRow, type MonitorRange } from '@/api/channelMonitorV2'
import type { Group } from '@/types'
import { toolIdsForGroup } from '@/features/ai-tools/model'
import { getGroupModels } from '@/features/ai-tools/api'
import Icon from '@/components/icons/Icon.vue'
import MonitorStatusCards from './MonitorStatusCards.vue'
import MonitorModelPerformance from './MonitorModelPerformance.vue'
import { monitorPeriods } from './ranges'
import { type MonitorCardTimelineMode, hasMonitorSamples } from './monitorCards'
const props = defineProps<{ toolId: string; toolName: string; groups: Group[]; resumeAssociation?: { range: MonitorRange; groupId: number; platform: string; scrollTop: number } }>()
const emit = defineEmits<{ 'load-error': []; title: [string]; description: [string]; associate: [number, { range: MonitorRange; groupId: number; platform: string; scrollTop: number }] }>()
const { t } = useI18n()
const surface = ref<HTMLElement>()
const range = ref<MonitorRange>(props.resumeAssociation?.range || '24h')
const timelineMode = ref<MonitorCardTimelineMode>('samples')
const supportedModelCounts = ref<Record<number, number>>({}), modelCountsLoading = ref(false)
const rows = ref<MonitorMatrixRow[]>([]), coverage = ref<MonitorCoverage>(), platformOrder = ref<string[]>([])
const loading = ref(false), error = ref(false), selectedGroup = ref<MonitorMatrixRow | null>(null)
const modelRows = ref<MonitorModelRow[]>([]), modelsLoading = ref(false), modelsError = ref(false)
watchEffect(() => emit('description', t(selectedGroup.value ? 'channelMonitorV2.tool.modelHint' : 'channelMonitorV2.tool.passiveHint')))
const now = ref(Date.now()), countdown = ref(300)
let refreshSeconds = 300, timer: ReturnType<typeof setInterval> | undefined
let lineRequest = 0, modelRequest = 0, lineController: AbortController | undefined, modelController: AbortController | undefined
let savedScroll = 0, returnGroupId: number | undefined, returnPlatform: string | undefined, loadedRange: MonitorRange = '24h'
const periods = monitorPeriods
// Use authorized native group metadata and the shared mapping. Keys are irrelevant.
const toolGroups = computed(() => props.groups.filter(group => toolIdsForGroup(group).includes(props.toolId)))
const associableGroupIds = computed(() => toolGroups.value.filter(group => group.status === 'active').map(group => group.id))
// Scores are already computed by the server with its saved weights and sample gate.
// User-facing absolute request counts may be redacted, so use has_samples rather than a local count threshold.
const bestLineKeys = computed(() => {
  const scored = rows.value.filter(row => hasMonitorSamples(row.metrics) && row.health.overall !== 'unknown'
    && typeof row.health.score === 'number' && Number.isFinite(row.health.score) && row.health.score >= 0 && row.health.score <= 100)
  const highest = Math.max(...scored.map(row => row.health.score!))
  return scored.filter(row => row.health.score === highest).map(row => `${row.platform}:${row.group_id}`)
})
let restoreAssociation = !!props.resumeAssociation
function associateGroup(row: MonitorMatrixRow) {
  if (!row.group_id || !associableGroupIds.value.includes(row.group_id)) return
  emit('associate', row.group_id, { range: range.value, groupId: row.group_id, platform: row.platform, scrollTop: scrollContainer()?.scrollTop || 0 })
}
async function restoreAssociationEntry() {
  const resume = props.resumeAssociation
  if (!restoreAssociation || !resume) return
  restoreAssociation = false
  await nextTick()
  const scroller = scrollContainer(); if (scroller) scroller.scrollTop = resume.scrollTop
  const entries = surface.value?.querySelectorAll<HTMLButtonElement>('[data-associate-group-id]')
  Array.from(entries || []).find(entry => entry.dataset.associateGroupId === String(resume.groupId) && entry.dataset.associatePlatform === resume.platform)?.focus({ preventScroll: true })
}
function filter(groupIds = toolGroups.value.map(group => group.id)): MonitorFilter {
  return { range: range.value, toolId: props.toolId, platforms: [], groupIds, models: [] }
}
function scrollContainer() { return surface.value?.closest<HTMLElement>('.modal-body') || surface.value }
async function loadSupportedModelCounts(groupIds: number[], signal: AbortSignal, request: number) {
  modelCountsLoading.value = true
  try {
    const catalog = await getGroupModels(signal, groupIds)
    if (signal.aborted || request !== lineRequest) return
    const scope = new Set(groupIds)
    supportedModelCounts.value = Object.fromEntries(catalog
      .filter(group => scope.has(group.group_id))
      .map(group => [group.group_id, new Set(group.supported_models).size]))
  } catch {
    // A failed/missing catalogue is unavailable, never a claim of zero models.
    // Keep line statistics and the model-performance action usable.
  } finally {
    if (!signal.aborted && request === lineRequest) modelCountsLoading.value = false
  }
}
async function loadLines() {
  lineController?.abort(); const controller = new AbortController(); lineController = controller; const request = ++lineRequest
  const scope = filter(); rows.value = []; coverage.value = undefined; error.value = false
  supportedModelCounts.value = {}; modelCountsLoading.value = false
  if (!scope.groupIds.length) { loading.value = false; return }
  void loadSupportedModelCounts(scope.groupIds, controller.signal, request)
  loading.value = true
  try {
    const [snapshot, matrix] = await Promise.all([getSnapshot(scope, false, controller.signal), getMatrix(scope, 'platform_group', false, controller.signal, { recentSamples: true })])
    if (controller.signal.aborted || request !== lineRequest) return
    const ids = new Set(scope.groupIds)
    rows.value = matrix.items.filter(row => row.group_id && ids.has(row.group_id))
    coverage.value = matrix.coverage; loadedRange = scope.range
    platformOrder.value = snapshot.config?.platforms?.filter(platform => platform.enabled).map(platform => platform.platform) || []
    timelineMode.value = snapshot.config?.card_timeline_mode || 'samples'
    refreshSeconds = snapshot.config?.refresh_interval_seconds || 300
    countdown.value = refreshSeconds
  } catch { if (!controller.signal.aborted && request === lineRequest) { error.value = true; emit('load-error') } }
  finally { if (request === lineRequest) { loading.value = false; if (!error.value && rows.value.length) await restoreAssociationEntry() } }
}
async function loadModels() {
  modelController?.abort(); const controller = new AbortController(); modelController = controller; const request = ++modelRequest
  const id = selectedGroup.value?.group_id
  modelRows.value = []; modelsError.value = false
  if (!id || !toolGroups.value.some(group => group.id === id)) { modelsLoading.value = false; return }
  modelsLoading.value = true
  try {
    const result = await getModels(filter([id]), false, controller.signal)
    if (!controller.signal.aborted && request === modelRequest && selectedGroup.value?.group_id === id) modelRows.value = result.items.filter(row => hasMonitorSamples(row.metrics))
  } catch { if (!controller.signal.aborted && request === modelRequest) { modelsError.value = true; emit('load-error') } }
  finally { if (request === modelRequest) modelsLoading.value = false }
}
function openModels(row: MonitorMatrixRow) {
  savedScroll = scrollContainer()?.scrollTop || 0; returnGroupId = row.group_id; returnPlatform = row.platform
  selectedGroup.value = row; emit('title', `${row.group_name || '#' + row.group_id} · ${t('channelMonitorV2.tool.models')}`)
  void nextTick(() => { const scroller = scrollContainer(); if (scroller) scroller.scrollTop = 0; surface.value?.querySelector<HTMLButtonElement>('[data-testid="tool-monitor-back"]')?.focus({ preventScroll: true }) })
  void loadModels()
}
async function backToLines() {
  modelController?.abort(); ++modelRequest; selectedGroup.value = null; emit('title', `${props.toolName} ${t('channelMonitorV2.tool.lines')}`)
  modelsLoading.value = false; modelsError.value = false; modelRows.value = []
  if (loadedRange !== range.value) await loadLines()
  await nextTick(() => {
    const scroller = scrollContainer(); if (scroller) scroller.scrollTop = savedScroll
    const entries = surface.value?.querySelectorAll<HTMLButtonElement>('[data-model-group-id]')
    Array.from(entries || []).find(entry => entry.dataset.modelGroupId === String(returnGroupId) && entry.dataset.modelPlatform === returnPlatform)?.focus({ preventScroll: true })
  })
}
function setRange(value: MonitorRange) { if (range.value === value) return; range.value = value; if (selectedGroup.value) void loadModels(); else void loadLines() }
watch([() => props.toolId, () => props.groups, () => props.resumeAssociation], () => {
  lineController?.abort(); modelController?.abort(); ++modelRequest
  modelsLoading.value = false; modelsError.value = false
  selectedGroup.value = null; modelRows.value = []; range.value = props.resumeAssociation?.range || '24h'; restoreAssociation = !!props.resumeAssociation
  emit('title', `${props.toolName} ${t('channelMonitorV2.tool.lines')}`)
  void loadLines()
}, { immediate: true })
timer = setInterval(() => { now.value = Date.now(); if (loading.value || modelsLoading.value) return; countdown.value -= 1; if (countdown.value <= 0) { countdown.value = refreshSeconds; if (selectedGroup.value) void loadModels(); else void loadLines() } }, 1000)
onBeforeUnmount(() => { clearInterval(timer); lineController?.abort(); modelController?.abort(); ++lineRequest; ++modelRequest })
</script>
<style scoped>
.tool-monitor-toolbar{display:flex;flex-wrap:wrap;align-items:center;gap:10px;margin-bottom:18px}
.tool-monitor-toolbar .tabs{padding:2px;gap:2px;border:0;border-radius:7px;background:transparent;box-shadow:none}
.tool-monitor-toolbar .tab{min-height:32px;padding:5px 10px;border:0;border-radius:5px;background:transparent;color:var(--xq-secondary);font-size:11px;font-weight:400;line-height:20px;box-shadow:none}
.tool-monitor-toolbar .tab-active{background:var(--xq-raised);color:var(--xq-text);font-weight:500}
.tool-monitor-toolbar .tab:hover{color:var(--xq-text);background:var(--xq-raised)}
.tool-monitor-toolbar .tool-monitor-refresh{margin-left:auto;min-height:32px;padding:5px 8px;border:1px solid transparent;border-radius:6px;background:transparent;color:var(--xq-secondary);font-size:11px;font-weight:400;line-height:20px;box-shadow:none}
.tool-monitor-toolbar .tool-monitor-refresh:hover:not(:disabled){background:var(--xq-raised);color:var(--xq-text)}
.tool-monitor-toolbar .tool-monitor-refresh:disabled{opacity:.5;cursor:wait}
.tool-monitor-toolbar .tab:focus-visible,.tool-monitor-toolbar .tool-monitor-refresh:focus-visible{outline:2px solid var(--xq-accent);outline-offset:2px}
.tool-monitor-description{color:var(--xq-secondary);font-size:12px;line-height:1.6;margin:0 0 20px}
.tool-monitor-message{display:flex;min-height:160px;align-items:center;justify-content:center;gap:12px;color:var(--xq-secondary);font-size:14px}
.tool-monitor-details :deep(article[data-testid="monitor-group-card"]){border-radius:12px;border:1px solid var(--xq-border);background:var(--xq-surface);box-shadow:none;padding:16px}
.tool-monitor-details :deep(article.monitor-card--priority){padding-top:32px}
.tool-monitor-details :deep(article[data-testid="monitor-group-card"] dl>div){border-radius:8px;background:var(--xq-depth);border-color:var(--xq-border)}
.tool-monitor-details :deep(.monitor-card-grid){grid-template-columns:repeat(3,minmax(0,1fr));gap:20px}
@media(min-width:701px) and (max-width:1099px){.tool-monitor-details :deep(.monitor-card-grid){grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:700px){.tool-monitor-toolbar{gap:8px}.tool-monitor-toolbar .tool-monitor-refresh{margin-left:auto}.tool-monitor-details :deep(.monitor-card-grid){grid-template-columns:minmax(0,1fr);gap:16px}.tool-monitor-message{flex-direction:column;text-align:center}}
</style>
