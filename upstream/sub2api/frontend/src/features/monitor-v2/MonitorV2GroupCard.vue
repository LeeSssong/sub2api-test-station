<template>
  <article
    :data-test="`monitor-group-${group.id}`"
    data-monitor-layout="service-line"
    class="group/monitor rounded-lg border border-slate-200/75 bg-white px-4 py-3.5 transition-[background-color,border-color,box-shadow] duration-200 ease-out hover:border-emerald-400/60 hover:bg-emerald-500/5 hover:shadow-[0_12px_28px_-22px_rgba(16,185,129,0.65)] focus-within:border-emerald-400/60 focus-within:bg-emerald-500/5 dark:border-slate-800 dark:bg-[#0b1220] dark:hover:border-emerald-400/45 dark:hover:bg-emerald-400/5 sm:px-5 sm:py-4"
    :aria-labelledby="`monitor-v2-group-${group.id}`"
  >
    <header class="grid min-w-0 grid-cols-1 items-center gap-3.5 lg:grid-cols-[minmax(360px,0.95fr)_minmax(0,1.35fr)] lg:gap-5">
      <div class="flex min-w-0 items-center gap-2.5">
        <div class="flex flex-shrink-0 items-baseline gap-1.5">
          <span
            data-test="monitor-availability-badge"
            class="inline-flex min-w-[62px] items-center justify-center rounded-md border px-2.5 py-1.5 text-base font-black tabular-nums shadow-sm"
            :class="availabilityBadgeClass"
          >
            {{ availabilityLabel }}
          </span>
          <span data-test="monitor-availability-label" class="text-[10px] font-semibold text-slate-400 dark:text-slate-500">
            {{ t('monitorV2.metric.availabilityLabel') }}
          </span>
        </div>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <h2
              :id="`monitor-v2-group-${group.id}`"
              class="truncate text-lg font-bold tracking-tight text-slate-950 dark:text-white"
            >
              {{ group.name }}
            </h2>
            <span
              data-test="monitor-rate-multiplier"
              class="inline-flex items-center rounded-md border border-emerald-400/25 bg-emerald-500/12 px-2 py-0.5 text-[11px] font-black tabular-nums text-emerald-700 shadow-sm dark:text-emerald-300"
            >
              {{ formatRate(group.rate_multiplier) }}×
            </span>
          </div>
          <div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-medium text-slate-500 dark:text-slate-400">
            <span>{{ t('monitorV2.metric.ttft') }}{{ metricValue(group.ttft, formatDuration) }}</span>
            <span>{{ t('monitorV2.metric.averageLatency') }}{{ metricValue(group.average_latency, formatDuration) }}</span>
          </div>
          <p data-test="monitor-freshness" class="mt-1.5 text-[11px] text-slate-400 dark:text-slate-500">
            {{ freshnessLabel }}
          </p>
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
const availabilityBadgeClass = computed(() => {
  return props.group.status === 'operational'
    ? 'border-emerald-300/60 bg-emerald-50 text-emerald-700 dark:border-emerald-400/35 dark:bg-emerald-500/15 dark:text-emerald-200'
    : 'border-red-300/60 bg-red-50 text-red-700 dark:border-red-400/35 dark:bg-red-500/15 dark:text-red-200'
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

const availabilityLabel = computed(() => {
  if (props.group.availability.state !== 'available' || props.group.availability.value === null) return '—'
  return `${formatAvailability(props.group.availability.value)}%`
})

const freshnessLabel = computed(() => {
  if (!props.group.source_updated_at) return t('monitorV2.freshness.noProbe')
  const time = new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(props.group.source_updated_at))
  return t('monitorV2.freshness.latestProbe', { time })
})
</script>
