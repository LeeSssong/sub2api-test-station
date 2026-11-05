<template>
  <component :is="embedded ? 'div' : AppLayout" ref="surface" class="native-monitor-content">
    <div v-if="loadError" role="alert" class="native-monitor-error">{{ loadError }}<button class="btn btn-secondary" @click="refresh()">{{ t('channelMonitorV2.tool.retry') }}</button></div>
    <template v-if="!embedded || !showDetail">
    <MonitorHero
      :overall-status="overallStatus"
      :interval-seconds="DEFAULT_INTERVAL_SECONDS"
      :window="currentWindow"
      :loading="loading"
      :auto-refresh="autoRefresh"
      @update:window="handleWindowChange"
      @refresh="refresh()"
    />

    <p v-if="embedded && items.length && items.every(item => !item.primary_status && !item.timeline?.length)" role="status" class="mb-4 text-xs text-gray-500">{{ t('channelMonitorV2.tool.noProbeSamples') }}</p>
    <MonitorCardGrid
      :hide-unobserved="embedded"
      :items="items"
      :window="currentWindow"
      :countdown-seconds="countdown"
      :loading="loading"
      :detail-cache="detailCache"
      @card-click="openDetail"
    />

    </template>
    <MonitorDetailDialog
      :inline="embedded"
      :tool-id="toolId"
      :show="showDetail"
      :monitor-id="detailTarget?.id ?? null"
      :title="detailTitle"
      @close="closeDetail"
      @load-error="emit('load-error')"
    />
  </component>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  list as listChannelMonitorViews,
  status as fetchChannelMonitorDetail,
  type UserMonitorView,
  type UserMonitorDetail,
} from '@/api/channelMonitor'
import AppLayout from '@/components/layout/AppLayout.vue'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '@/components/user/monitor/MonitorHero.vue'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '@/constants/channelMonitor'
import { useAutoRefresh } from '@/composables/useAutoRefresh'

const props = defineProps<{embedded?:boolean;toolId?:string}>()
const emit = defineEmits<{'load-error':[]}>()
const { t } = useI18n()
const appStore = useAppStore()

// ── State ──
const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const loadError = ref('')
let lifetime = new AbortController()
const surface=ref<HTMLElement>()
let returnId:number|undefined
let detailTrigger: HTMLElement | null = null
const currentWindow = ref<MonitorWindow>('7d')
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  defaultEnabled: true,
  onRefresh: () => refresh(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

// ── Computed ──
const overallStatus = computed<OverallStatus>(() => {
  if (props.embedded) {
    const observed = items.value.filter(item => item.primary_status)
    if (observed.some(item => item.primary_status !== STATUS_OPERATIONAL)) return 'degraded'
    if (!observed.length || observed.length !== items.value.length) return 'unknown'
    return 'operational'
  }
  if (items.value.length === 0) return 'operational'
  for (const it of items.value) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const detailTitle = computed(() => {
  return detailTarget.value?.name || t('channelStatus.detailTitle')
})

// ── Loaders ──
async function reload(silent = false) {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listChannelMonitorViews({ signal: ctrl.signal, toolId: props.toolId })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
    loadError.value = ''
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    if(ctrl.signal.aborted || abortController !== ctrl)return
    if(props.embedded){items.value=[];loadError.value=extractApiErrorMessage(err,t('channelStatus.loadError'));emit('load-error')}
    else appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      if (!silent) loading.value = false
      autoRefresh.resetCountdown()
      abortController = null
    }
  }
}

async function refresh(silent = false) {
  await reload(silent)
  // After base reload, refresh any cached detail records so non-7d availability
  // values stay in sync without forcing the user to switch tabs again.
  if (currentWindow.value !== '7d') {
    if(!lifetime.signal.aborted) await Promise.all(items.value.map(it => loadDetail(it.id, true)))
  }
}

async function loadDetail(id: number, force = false) {
  if (!force && detailCache[id]) return
  const signal = lifetime.signal
  const toolId = props.toolId
  try {
    const detail = await fetchChannelMonitorDetail(id,{signal,toolId})
    if(!signal.aborted && props.toolId===toolId) detailCache[id] = detail
  } catch (err: unknown) {
    if(signal.aborted)return
    if(props.embedded){loadError.value=extractApiErrorMessage(err,t('channelStatus.detailLoadError'));emit('load-error')}
    else appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  }
}

async function ensureDetailsForWindow() {
  if (currentWindow.value === '7d') return
  await Promise.all(items.value.map(it => loadDetail(it.id)))
}

// ── Handlers ──
async function handleWindowChange(value: MonitorWindow) {
  currentWindow.value = value
  await ensureDetailsForWindow()
}

function openDetail(row: UserMonitorView) {
  returnId=row.id
  detailTrigger = document.activeElement as HTMLElement
  detailTarget.value = row
  showDetail.value = true
  if(props.embedded)void nextTick(()=>surface.value?.querySelector<HTMLButtonElement>('[data-native-back]')?.focus({preventScroll:true}))
}

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
  void nextTick(()=>{if(props.embedded) surface.value?.querySelector<HTMLButtonElement>(`[data-monitor-id="${returnId}"]`)?.focus({preventScroll:true});else if(detailTrigger?.isConnected)detailTrigger.focus()})
}

watch(()=>props.toolId,()=>{
 lifetime.abort();lifetime=new AbortController();abortController?.abort();items.value=[];closeDetail();Object.keys(detailCache).forEach(id=>delete detailCache[Number(id)]);void reload()
})

watch(items, () => {
  void ensureDetailsForWindow()
})

watch(
  () => appStore.cachedPublicSettings?.channel_monitor_enabled,
  (enabled) => {
    if (enabled === false) autoRefresh.stop()
    else if (autoRefresh.enabled.value) autoRefresh.start()
  },
)

onMounted(() => {
  void reload(false)
  if (appStore.cachedPublicSettings?.channel_monitor_enabled !== false) {
    autoRefresh.setEnabled(autoRefresh.enabled.value)
  }
})

onBeforeUnmount(() => {
  lifetime.abort()
  if (abortController) abortController.abort()
})
</script>

<style scoped>
.native-monitor-content{min-width:0}
.native-monitor-error{display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin-bottom:16px;font-size:13px;color:var(--xq-text)}
</style>
