<template>
  <AppLayout>
    <section class="mx-auto w-full max-w-[1500px] px-3 py-4 sm:px-5" data-test="hybrid-performance-view">
      <header class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-4 dark:border-slate-800">
        <div><h1 class="text-2xl font-bold text-slate-950 dark:text-white">{{ t('monitorV2.hybrid.title') }}</h1><p class="mt-1 text-xs text-slate-500">{{ t('monitorV2.hybrid.updated', { time: updatedAt }) }}</p></div>
        <div class="inline-flex rounded-lg border border-slate-200 bg-slate-100 p-1 dark:border-slate-700 dark:bg-slate-900"><button v-for="option in windowOptions" :key="option.value" type="button" class="rounded-md px-3 py-1.5 text-xs font-semibold" :class="currentWindow === option.value ? 'bg-white text-slate-950 shadow-sm dark:bg-slate-700 dark:text-white' : 'text-slate-500'" :disabled="loading" @click="selectWindow(option.value)">{{ option.label }}</button></div>
      </header>
      <div v-if="snapshot.groups.length" class="mt-5 grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2" :aria-busy="loading"><HybridPerformanceGroupCard v-for="group in snapshot.groups" :key="group.id" :group="group" /></div>
      <div v-else class="mt-5 rounded-xl border border-dashed border-slate-300 px-6 py-14 text-center text-sm text-slate-500">{{ t('monitorV2.hybrid.empty') }}</div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import HybridPerformanceGroupCard from './HybridPerformanceGroupCard.vue'
import { getHybridPerformanceSnapshot } from './api'
import type { MonitorV4Snapshot, MonitorV4Window } from './types'

const { t } = useI18n(); const currentWindow = ref<MonitorV4Window>('7d'); const loading = ref(true); const snapshot = ref<MonitorV4Snapshot>({ contract_version:'1', window:'7d', refresh_interval_seconds:60, generated_at:new Date().toISOString(), groups:[] }); let controller: AbortController | null = null; let timer: number | null = null
const windowOptions = computed(() => [{ value:'24h' as const, label:t('monitorV2.window.24h') }, { value:'7d' as const, label:t('monitorV2.window.7d') }, { value:'30d' as const, label:t('monitorV2.window.30d') }]); const updatedAt = computed(() => new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(snapshot.value.generated_at)))
function schedule() { if (timer !== null) window.clearTimeout(timer); if (snapshot.value.refresh_interval_seconds > 0) timer = window.setTimeout(() => reload(currentWindow.value), snapshot.value.refresh_interval_seconds * 1000) }
async function reload(windowValue: MonitorV4Window) { controller?.abort(); const nextController = new AbortController(); controller = nextController; loading.value = true; try { const next = await getHybridPerformanceSnapshot(windowValue, nextController.signal); if (nextController.signal.aborted || controller !== nextController) return; snapshot.value = next; currentWindow.value = next.window; schedule() } catch (error: unknown) { const candidate = error as { name?: string }; if (candidate.name !== 'AbortError') schedule() } finally { if (controller === nextController) { controller = null; loading.value = false } } }
function selectWindow(value: MonitorV4Window) { if (value !== currentWindow.value) void reload(value) }
onMounted(() => void reload(currentWindow.value)); onBeforeUnmount(() => { controller?.abort(); if (timer !== null) window.clearTimeout(timer) })
</script>
