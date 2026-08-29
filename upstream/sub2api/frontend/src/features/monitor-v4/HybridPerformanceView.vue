<template>
  <AppLayout>
    <section data-test="hybrid-performance-view" class="mx-auto w-full max-w-[1500px] rounded-2xl text-slate-900 dark:bg-[#07101f] dark:px-3 dark:py-3 dark:text-slate-100 sm:dark:px-4 sm:dark:py-4" aria-labelledby="hybrid-performance-title">
      <header class="flex flex-col gap-3 border-b border-slate-200/80 pb-4 dark:border-slate-800/80 md:flex-row md:items-center md:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 id="hybrid-performance-title" class="text-2xl font-bold tracking-tight text-slate-950 dark:text-white sm:text-[1.7rem]">{{ t('channelMonitorV2.hybrid.title') }}</h1>
          </div>
          <p class="mt-1 text-[11px] text-slate-500 dark:text-slate-400">{{ t('channelMonitorV2.hybrid.updated', { time: updatedAt }) }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="inline-flex rounded-lg border border-slate-200/80 bg-slate-100/80 p-0.5 dark:border-slate-800 dark:bg-[#101827]" role="tablist" :aria-label="t('channelMonitorV2.hybrid.title')">
            <button v-for="option in windowOptions" :key="option.value" :data-test="`hybrid-window-${option.value}`" type="button" role="tab" :aria-selected="currentWindow === option.value" class="rounded-md px-3 py-1.5 text-xs font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-emerald-400/60 sm:px-3.5" :class="currentWindow === option.value ? 'bg-white text-slate-950 shadow-sm dark:bg-slate-700 dark:text-white' : 'text-slate-500 hover:text-slate-950 dark:text-slate-400 dark:hover:text-white'" :disabled="loading" @click="selectWindow(option.value)">{{ option.label }}</button>
          </div>
        </div>
      </header>
      <div v-if="snapshot.groups.length" data-test="hybrid-group-status" class="mt-4 grid min-w-0 grid-cols-[repeat(auto-fit,minmax(300px,1fr))] items-stretch gap-3 rounded-xl border border-slate-200/80 bg-slate-100/65 p-1.5 shadow-sm dark:border-slate-800 dark:bg-[#080f1b] sm:gap-4 sm:p-2" :aria-busy="loading">
        <HybridPerformanceGroupCard v-for="group in snapshot.groups" :key="group.id" :group="group" />
      </div>
      <section v-else data-test="hybrid-group-status" class="mt-4 rounded-lg border border-dashed border-slate-300 bg-white px-6 py-14 text-center dark:border-slate-700 dark:bg-[#0b1220]">
        <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('channelMonitorV2.hybrid.empty') }}</p>
      </section>

      <CodexRadarRecommendations />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import CodexRadarRecommendations from '@/features/monitor-v2/CodexRadarRecommendations.vue'
import HybridPerformanceGroupCard from './HybridPerformanceGroupCard.vue'
import { getHybridPerformanceSnapshot } from './api'
import type { MonitorV4Snapshot, MonitorV4Window } from './types'

const { t } = useI18n(); const currentWindow = ref<MonitorV4Window>('24h'); const loading = ref(true); const snapshot = ref<MonitorV4Snapshot>({ contract_version:'2', window:'24h', refresh_interval_seconds:60, generated_at:new Date().toISOString(), groups:[] }); let controller: AbortController | null = null; let timer: number | null = null
const windowOptions = computed(() => [{ value:'24h' as const, label:t('monitorV2.window.24h') }, { value:'7d' as const, label:t('monitorV2.window.7d') }, { value:'30d' as const, label:t('monitorV2.window.30d') }]); const updatedAt = computed(() => new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(snapshot.value.generated_at)))
function schedule() { if (timer !== null) window.clearTimeout(timer); if (snapshot.value.refresh_interval_seconds > 0) timer = window.setTimeout(() => reload(currentWindow.value), snapshot.value.refresh_interval_seconds * 1000) }
async function reload(windowValue: MonitorV4Window) { controller?.abort(); const nextController = new AbortController(); controller = nextController; loading.value = true; try { const next = await getHybridPerformanceSnapshot(windowValue, nextController.signal); if (nextController.signal.aborted || controller !== nextController) return; snapshot.value = next; currentWindow.value = next.window; schedule() } catch (error: unknown) { const candidate = error as { name?: string }; if (candidate.name !== 'AbortError') schedule() } finally { if (controller === nextController) { controller = null; loading.value = false } } }
function selectWindow(value: MonitorV4Window) { if (value !== currentWindow.value) void reload(value) }
onMounted(() => void reload(currentWindow.value)); onBeforeUnmount(() => { controller?.abort(); if (timer !== null) window.clearTimeout(timer) })
</script>
