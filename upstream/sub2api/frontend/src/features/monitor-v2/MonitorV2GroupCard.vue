<template>
  <article
    class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900/70 sm:p-6"
    :aria-labelledby="`monitor-v2-group-${group.id}`"
  >
    <header class="flex flex-col items-stretch justify-between gap-5 sm:flex-row sm:items-center">
      <div class="flex min-w-0 items-center gap-3">
        <span
          class="inline-flex flex-shrink-0 items-center gap-1.5 rounded-full px-3 py-1 text-sm font-semibold"
          :class="group.status === 'operational'
            ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-200'
            : 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-200'"
        >
          <span class="h-2 w-2 rounded-full bg-current" aria-hidden="true" />
          {{ t(`monitorV2.status.${group.status}`) }}
        </span>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h2
              :id="`monitor-v2-group-${group.id}`"
              class="truncate text-lg font-semibold text-gray-950 dark:text-white"
            >
              {{ group.name }}
            </h2>
            <span v-if="group.is_flagship" data-test="monitor-flagship" class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-800 dark:bg-amber-500/20 dark:text-amber-200">
              {{ t('monitorV2.flagship') }}
            </span>
            <span class="text-lg font-semibold tabular-nums text-gray-700 dark:text-gray-200">
              {{ primaryDuration }}
            </span>
          </div>
          <div class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('monitorV2.metric.ttft') }} {{ metricValue(group.ttft, formatDuration) }}</span>
            <span>{{ t('monitorV2.metric.tps') }} {{ metricValue(group.tps, formatTPS) }}</span>
            <span data-test="monitor-rate-multiplier">{{ t('monitorV2.baseRate') }} {{ formatRate(group.rate_multiplier) }}×</span>
          </div>
        </div>
      </div>
      <MonitorV2Timeline :points="group.timeline" />
    </header>

  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import MonitorV2Timeline from './MonitorV2Timeline.vue'
import type { MonitorV2Group } from './types'

const props = defineProps<{
  group: MonitorV2Group
}>()

const { t } = useI18n()
const primaryDuration = computed(() => {
  const metric = props.group.latency
  return metric.state === 'available' && metric.value !== null ? formatDuration(metric.value) : '—'
})

function formatDuration(value: number): string {
  if (value < 1000) return `${Math.round(value)} ms`
  return `${Number((value / 1000).toFixed(2))} s`
}

function formatTPS(value: number): string {
  return `${Number(value.toFixed(1))} tok/s`
}

function formatRate(value: number): string {
  return Number(value.toFixed(4)).toString()
}

function metricValue(metric: MonitorV2Group['ttft'], formatter: (value: number) => string): string {
  return metric.state === 'available' && metric.value !== null ? formatter(metric.value) : '—'
}

</script>
