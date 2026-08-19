<template>
  <article
    :data-test="`monitor-group-${group.id}`"
    class="group/monitor rounded-xl border border-gray-200 bg-white px-5 py-5 transition-all duration-300 ease-out hover:-translate-y-0.5 hover:border-emerald-400/60 hover:bg-emerald-500/10 hover:shadow-[0_16px_40px_-24px_rgba(16,185,129,0.65)] focus-within:border-emerald-400/60 focus-within:bg-emerald-500/10 dark:border-dark-700 dark:bg-dark-900/75 dark:hover:border-emerald-400/45 dark:hover:bg-emerald-400/10 sm:px-6 sm:py-6 lg:px-7"
    :aria-labelledby="`monitor-v2-group-${group.id}`"
  >
    <header class="grid min-w-0 grid-cols-1 items-center gap-6 lg:grid-cols-[minmax(370px,0.9fr)_minmax(0,1.25fr)] lg:gap-10">
      <div class="flex min-w-0 items-center gap-3.5">
        <span
          class="inline-flex flex-shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-extrabold tabular-nums shadow-sm"
          :class="availabilityClass"
        >
          <span class="h-2 w-2 rounded-full bg-current" aria-hidden="true" />
          {{ availabilityText }}
          <span class="sr-only">{{ t(`monitorV2.status.${group.status}`) }}</span>
        </span>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <h2
              :id="`monitor-v2-group-${group.id}`"
              class="truncate text-xl font-bold tracking-tight text-gray-950 dark:text-white"
            >
              {{ group.name }}
            </h2>
            <span
              data-test="monitor-rate-multiplier"
              class="inline-flex items-center rounded-lg border border-emerald-400/25 bg-emerald-500/15 px-2.5 py-1 text-base font-black tabular-nums text-emerald-700 shadow-sm dark:text-emerald-300"
            >
              {{ formatRate(group.rate_multiplier) }}×
            </span>
          </div>
          <div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-gray-500 dark:text-gray-400">
            <span>{{ t('monitorV2.metric.availability') }}{{ metricValue(group.availability, formatAvailability) }}%</span>
            <span>{{ t('monitorV2.metric.ttft') }}{{ metricValue(group.ttft, formatDuration) }}</span>
            <span>{{ t('monitorV2.metric.averageLatency') }}{{ metricValue(group.average_latency, formatDuration) }}</span>
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
const availabilityText = computed(() => {
  const metric = props.group.availability
  if (metric.state !== 'available' || metric.value === null) return t('monitorV2.availabilityNoData')
  return t('monitorV2.availability', { value: formatAvailability(metric.value) })
})

const availabilityClass = computed(() => {
  return props.group.status === 'operational'
    ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-200'
    : 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-200'
})

function formatAvailability(value: number): string {
  if (Number.isInteger(value)) return String(value)
  return value.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

function formatDuration(value: number): string {
  if (value < 1000) return `${Math.round(value)} ms`
  return `${Number((value / 1000).toFixed(2))} s`
}

function formatRate(value: number): string {
  return Number(value.toFixed(4)).toString()
}

function metricValue(metric: MonitorV2Group['ttft'], formatter: (value: number) => string): string {
  return metric.state === 'available' && metric.value !== null ? formatter(metric.value) : '—'
}
</script>
