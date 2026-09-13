<template>
  <AppLayout>
    <section data-test="hybrid-performance-view" class="mx-auto w-full max-w-[1500px] space-y-6 rounded-2xl text-slate-900 dark:bg-[#07101f] dark:px-3 dark:py-3 dark:text-slate-100 sm:dark:px-4 sm:dark:py-4">
      <HybridPerformancePanel />
      <CodexRadarRecommendations />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import CodexRadarRecommendations from '@/features/monitor-v2/CodexRadarRecommendations.vue'
import HybridPerformanceGroupCard from './HybridPerformanceGroupCard.vue'
import { getHybridPerformanceSnapshot } from './api'
import type { MonitorV4Snapshot, MonitorV4Window } from './types'

const { t } = useI18n(); const currentWindow = ref<MonitorV4Window>('24h'); const pendingWindow = ref<MonitorV4Window | null>(null); const loading = ref(true); const loadError = ref(''); const snapshot = ref<MonitorV4Snapshot>({ contract_version:'2', window:'24h', refresh_interval_seconds:60, generated_at:new Date().toISOString(), groups:[] }); let controller: AbortController | null = null; let timer: number | null = null
const windowOptions = computed(() => [{ value:'24h' as const, label:t('monitorV2.window.24h') }, { value:'7d' as const, label:t('monitorV2.window.7d') }]); const updatedAt = computed(() => new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(snapshot.value.generated_at)))
function schedule() { if (timer !== null) window.clearTimeout(timer); if (snapshot.value.refresh_interval_seconds > 0) timer = window.setTimeout(() => reload(currentWindow.value), snapshot.value.refresh_interval_seconds * 1000) }
async function reload(windowValue: MonitorV4Window) { controller?.abort(); const nextController = new AbortController(); controller = nextController; pendingWindow.value = windowValue; loading.value = true; loadError.value = ''; try { const next = await getHybridPerformanceSnapshot(windowValue, nextController.signal); if (nextController.signal.aborted || controller !== nextController) return; snapshot.value = next; currentWindow.value = next.window; schedule() } catch (error: unknown) { const candidate = error as { name?: string }; if (candidate.name !== 'AbortError') { currentWindow.value = snapshot.value.window; loadError.value = t('channelMonitorV2.hybrid.loadError'); schedule() } } finally { if (controller === nextController) { controller = null; pendingWindow.value = null; loading.value = false } } }
function selectWindow(value: MonitorV4Window) { if (value !== currentWindow.value || loading.value) { currentWindow.value = value; void reload(value) } }
onMounted(() => void reload(currentWindow.value)); onBeforeUnmount(() => { controller?.abort(); if (timer !== null) window.clearTimeout(timer) })
</script>
