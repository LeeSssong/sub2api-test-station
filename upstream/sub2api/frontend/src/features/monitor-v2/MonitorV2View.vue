<template>
  <AppLayout>
    <section data-test="monitor-v2-page" class="mx-auto w-full max-w-[1500px] rounded-2xl text-slate-900 dark:bg-[#07101f] dark:px-3 dark:py-3 dark:text-slate-100 sm:dark:px-4 sm:dark:py-4" aria-labelledby="monitor-v2-title">
      <header class="flex flex-col gap-3 border-b border-slate-200/80 pb-4 dark:border-slate-800/80 md:flex-row md:items-center md:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1
              id="monitor-v2-title"
              class="text-2xl font-bold tracking-tight text-slate-950 dark:text-white sm:text-[1.7rem]"
            >
              {{ t('monitorV2.title') }}
            </h1>
            <span
              class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.08em]"
              :class="overallClass"
              role="status"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
              {{ t(`monitorV2.overall.${overallStatus}`) }}
            </span>
          </div>
          <p class="mt-1 text-[11px] text-slate-500 dark:text-slate-400">
            {{ t('monitorV2.updatedAt', { time: updatedAt }) }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <div
            class="inline-flex rounded-lg border border-slate-200/80 bg-slate-100/80 p-0.5 dark:border-slate-800 dark:bg-[#101827]"
            role="tablist"
            :aria-label="t('monitorV2.title')"
          >
            <button
              v-for="option in windowOptions"
              :key="option.value"
              type="button"
              role="tab"
              :data-test="`monitor-window-${option.value}`"
              :aria-selected="currentWindow === option.value"
              class="rounded-md px-3 py-1.5 text-xs font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-emerald-400/60 sm:px-3.5"
              :class="
                currentWindow === option.value
                  ? 'bg-white text-slate-950 shadow-sm dark:bg-slate-700 dark:text-white'
                  : 'text-slate-500 hover:text-slate-950 dark:text-slate-400 dark:hover:text-white'
              "
              :disabled="loading"
              @click="selectWindow(option.value)"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
      </header>

      <div
        v-if="snapshot.groups.length > 0"
        class="mt-4 grid min-w-0 grid-cols-1 gap-2 rounded-xl border border-slate-200/80 bg-slate-100/65 p-1.5 shadow-sm dark:border-slate-800 dark:bg-[#080f1b] sm:p-2"
        :aria-busy="loading"
      >
        <MonitorV2GroupCard
          v-for="group in snapshot.groups"
          :key="group.id"
          :group="group"
        />
      </div>

      <section
        v-else
        class="mt-4 rounded-lg border border-dashed border-slate-300 bg-white px-6 py-14 text-center dark:border-slate-700 dark:bg-[#0b1220]"
      >
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">
          {{ t('monitorV2.empty.title') }}
        </h2>
        <p class="mx-auto mt-2 max-w-lg text-sm text-gray-600 dark:text-gray-300">
          {{ t('monitorV2.empty.description') }}
        </p>
      </section>

      <CodexRadarRecommendations />

    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { getMonitorV2Snapshot } from './api'
import CodexRadarRecommendations from './CodexRadarRecommendations.vue'
import MonitorV2GroupCard from './MonitorV2GroupCard.vue'
import type { MonitorV2Snapshot, MonitorV2Window } from './types'

const props = defineProps<{
  initialSnapshot: MonitorV2Snapshot
}>()

const { t } = useI18n()
const snapshot = ref<MonitorV2Snapshot>(props.initialSnapshot)
const currentWindow = ref<MonitorV2Window>(props.initialSnapshot.window)
const loading = ref(false)
let abortController: AbortController | null = null
let refreshTimer: number | null = null
const REFRESH_RETRY_DELAY_MS = 5_000

const windowOptions = computed(() => [
  { value: '24h' as const, label: t('monitorV2.window.24h') },
  { value: '7d' as const, label: t('monitorV2.window.7d') },
  { value: '30d' as const, label: t('monitorV2.window.30d') },
])

const updatedAt = computed(() =>
  new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(snapshot.value.generated_at))
)

const overallStatus = computed(() => {
  return snapshot.value.groups.some((group) => group.status === 'operational')
    ? 'operational'
    : 'unavailable'
})

const overallClass = computed(() => {
  switch (overallStatus.value) {
    case 'operational':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    case 'unavailable':
      return 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'
  }
})

async function selectWindow(window: MonitorV2Window) {
  if (window === currentWindow.value) return
  await reload(window)
}

function clearRefreshTimer() {
  if (refreshTimer === null) return
  window.clearTimeout(refreshTimer)
  refreshTimer = null
}

function scheduleRefresh(intervalSeconds: MonitorV2Snapshot['refresh_interval_seconds'], delayMs?: number) {
  clearRefreshTimer()
  if ((intervalSeconds === 0 && delayMs === undefined) || document.visibilityState === 'hidden') return
  refreshTimer = window.setTimeout(async () => {
    refreshTimer = null
    await reload(currentWindow.value)
  }, delayMs ?? intervalSeconds * 1_000)
}

async function reload(window: MonitorV2Window) {
  clearRefreshTimer()
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    const next = await getMonitorV2Snapshot(window, controller.signal)
    if (controller.signal.aborted || abortController !== controller) return
    snapshot.value = next
    currentWindow.value = next.window
    scheduleRefresh(next.refresh_interval_seconds)
  } catch (error: unknown) {
    const candidate = error as { name?: string; code?: string }
    if (candidate?.name === 'AbortError' || candidate?.code === 'ERR_CANCELED') return
    scheduleRefresh(snapshot.value.refresh_interval_seconds, REFRESH_RETRY_DELAY_MS)
  } finally {
    if (abortController === controller) {
      abortController = null
      loading.value = false
    }
  }
}

function handleVisibilityChange() {
  if (document.visibilityState === 'hidden') {
    clearRefreshTimer()
    return
  }
  void reload(currentWindow.value)
}

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  scheduleRefresh(snapshot.value.refresh_interval_seconds)
})

onBeforeUnmount(() => {
  abortController?.abort()
  clearRefreshTimer()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
