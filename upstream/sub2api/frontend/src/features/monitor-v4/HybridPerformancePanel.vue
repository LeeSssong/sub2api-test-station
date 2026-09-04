<template>
  <section data-test="hybrid-performance-panel" class="w-full" aria-labelledby="hybrid-performance-title">
    <header class="flex flex-col gap-3 border-b border-slate-200/80 pb-4 dark:border-slate-800/80 md:flex-row md:items-center md:justify-between">
      <div>
        <h2 id="hybrid-performance-title" class="text-lg font-semibold text-slate-950 dark:text-white">
          {{ t('channelMonitorV2.hybrid.title') }}
        </h2>
        <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
          {{ t('channelMonitorV2.hybrid.updated', { time: updatedAt }) }}
        </p>
      </div>
      <div class="inline-flex self-start rounded-lg border border-slate-200/80 bg-slate-100/80 p-0.5 dark:border-slate-700 dark:bg-[#071426]" role="tablist" :aria-label="t('channelMonitorV2.hybrid.title')">
        <button
          v-for="option in windowOptions"
          :key="option.value"
          :data-test="`hybrid-window-${option.value}`"
          type="button"
          role="tab"
          :aria-selected="currentWindow === option.value"
          class="min-h-9 rounded-md px-3 text-xs font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-sky-400/70"
          :class="currentWindow === option.value ? 'bg-white text-slate-950 shadow-sm dark:bg-[#0a2440] dark:text-sky-300 dark:shadow-[inset_0_-2px_#ffca48]' : 'text-slate-500 hover:text-slate-950 dark:text-slate-400 dark:hover:text-white'"
          :disabled="loading && pendingWindow === option.value"
          @click="selectWindow(option.value)"
        >
          {{ option.label }}
        </button>
      </div>
    </header>

    <div v-if="loadError" data-test="hybrid-load-error" role="alert" class="mt-4 flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/80 dark:bg-red-950/30 dark:text-red-300">
      <span>{{ loadError }}</span>
      <button type="button" data-test="hybrid-retry" class="shrink-0 font-semibold underline underline-offset-2" @click="reload(currentWindow)">
        {{ t('channelMonitorV2.hybrid.retry') }}
      </button>
    </div>

    <div v-if="snapshot.groups.length" data-test="hybrid-group-status" class="mt-4 grid min-w-0 grid-cols-[repeat(auto-fit,minmax(300px,1fr))] items-stretch gap-4" :aria-busy="loading">
      <HybridPerformanceGroupCard v-for="group in snapshot.groups" :key="group.id" :group="group" />
    </div>
    <div v-else data-test="hybrid-group-status" class="mt-4 rounded-lg border border-dashed border-slate-300 bg-white px-6 py-14 text-center dark:border-[#173a5e] dark:bg-[#0a162c]">
      <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('channelMonitorV2.hybrid.empty') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import HybridPerformanceGroupCard from './HybridPerformanceGroupCard.vue'
import { getHybridPerformanceSnapshot } from './api'
import type { MonitorV4Snapshot, MonitorV4Window } from './types'

const { t } = useI18n()
const currentWindow = ref<MonitorV4Window>('24h')
const pendingWindow = ref<MonitorV4Window | null>(null)
const loading = ref(true)
const loadError = ref('')
const snapshot = ref<MonitorV4Snapshot>({ contract_version: '2', window: '24h', refresh_interval_seconds: 60, generated_at: new Date().toISOString(), groups: [] })
let controller: AbortController | null = null
let timer: number | null = null

const windowOptions = computed(() => [
  { value: '1h' as const, label: t('monitorV2.window.1h') },
  { value: '24h' as const, label: t('monitorV2.window.24h') },
  { value: '7d' as const, label: t('monitorV2.window.7d') },
])
const updatedAt = computed(() => new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(snapshot.value.generated_at)))

function schedule() {
  if (timer !== null) window.clearTimeout(timer)
  if (snapshot.value.refresh_interval_seconds > 0) timer = window.setTimeout(() => reload(currentWindow.value), snapshot.value.refresh_interval_seconds * 1000)
}

async function reload(windowValue: MonitorV4Window) {
  controller?.abort()
  const nextController = new AbortController()
  controller = nextController
  pendingWindow.value = windowValue
  loading.value = true
  loadError.value = ''
  try {
    const next = await getHybridPerformanceSnapshot(windowValue, nextController.signal)
    if (nextController.signal.aborted || controller !== nextController) return
    snapshot.value = next
    currentWindow.value = next.window
    schedule()
  } catch (error: unknown) {
    if ((error as { name?: string }).name !== 'AbortError') {
      currentWindow.value = snapshot.value.window
      loadError.value = t('channelMonitorV2.hybrid.loadError')
      schedule()
    }
  } finally {
    if (controller === nextController) {
      controller = null
      pendingWindow.value = null
      loading.value = false
    }
  }
}

function selectWindow(value: MonitorV4Window) {
  if (value !== currentWindow.value || loading.value) {
    currentWindow.value = value
    void reload(value)
  }
}

onMounted(() => void reload(currentWindow.value))
onBeforeUnmount(() => {
  controller?.abort()
  if (timer !== null) window.clearTimeout(timer)
})
</script>
