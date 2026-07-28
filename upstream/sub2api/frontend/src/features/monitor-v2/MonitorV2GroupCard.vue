<template>
  <article
    class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900/70 sm:p-6"
    :aria-labelledby="`monitor-v2-group-${group.id}`"
  >
    <header class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2
            :id="`monitor-v2-group-${group.id}`"
            class="truncate text-base font-semibold text-gray-950 dark:text-white"
          >
            {{ group.name }}
          </h2>
          <span class="rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600 dark:bg-dark-800 dark:text-gray-300">
            {{ group.platform || 'API' }}
          </span>
        </div>
        <p
          v-if="group.peak_rate_enabled"
          class="mt-1 text-xs text-gray-600 dark:text-gray-300"
        >
          {{ peakRateCopy }}
        </p>
      </div>
      <div class="flex flex-shrink-0 items-center gap-2">
        <span class="font-mono text-sm font-semibold tabular-nums text-primary-700 dark:text-primary-300">
          {{ formatRate(group.rate_multiplier) }}×
          <span class="sr-only">{{ t('monitorV2.baseRate') }}</span>
        </span>
        <span
          class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
          :class="statusClass(group.status)"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
          {{ t(`monitorV2.status.${group.status}`) }}
        </span>
      </div>
    </header>

    <dl class="mt-5 grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-4">
      <div v-for="metric in metrics" :key="metric.key" class="min-w-0">
        <dt class="text-xs text-gray-600 dark:text-gray-300">{{ metric.label }}</dt>
        <dd class="mt-1 font-mono text-base font-semibold tabular-nums text-gray-950 dark:text-white">
          {{ metric.value }}
        </dd>
        <dd class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
          {{ metric.samples }}
        </dd>
      </div>
    </dl>

    <section class="mt-5 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <div>
          <p class="text-xs text-gray-600 dark:text-gray-300">
            {{ t('monitorV2.availability') }}
          </p>
          <p class="mt-1 text-sm font-medium text-gray-950 dark:text-white">
            {{ callEvidence }}
          </p>
        </div>
        <p class="font-mono text-lg font-semibold tabular-nums text-gray-950 dark:text-white">
          {{ availabilityValue }}
        </p>
      </div>
      <MonitorV2Timeline :points="group.timeline" />
    </section>

    <details class="group/models mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
      <summary class="flex cursor-pointer list-none items-center justify-between rounded-lg text-sm font-medium text-gray-700 outline-none transition-colors hover:text-primary-700 focus-visible:ring-2 focus-visible:ring-primary-500/50 dark:text-gray-200 dark:hover:text-primary-300">
        <span>{{ t('monitorV2.models', { count: group.models.length }) }}</span>
        <span class="text-xs text-primary-700 group-open/models:hidden dark:text-primary-300">
          {{ t('monitorV2.viewModels') }}
        </span>
        <span class="hidden text-xs text-primary-700 group-open/models:inline dark:text-primary-300">
          {{ t('monitorV2.hideModels') }}
        </span>
      </summary>
      <div class="mt-3 space-y-4">
        <section>
          <h3 class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            {{ t('monitorV2.details.metrics') }}
          </h3>
          <dl class="mt-2 grid grid-cols-2 gap-3">
            <div
              v-for="metric in detailMetrics"
              :key="metric.key"
              class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800/70"
            >
              <dt class="text-xs text-gray-600 dark:text-gray-300">{{ metric.label }}</dt>
              <dd class="mt-1 font-mono text-sm font-semibold tabular-nums text-gray-950 dark:text-white">
                {{ metric.value }}
              </dd>
              <dd class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                {{ metric.samples }}
              </dd>
            </div>
          </dl>
          <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('monitorV2.details.definition') }}
          </p>
        </section>
        <p
          v-if="group.models.length === 0"
          class="text-sm text-gray-600 dark:text-gray-300"
        >
          {{ t('monitorV2.noModels') }}
        </p>
        <ul v-else class="flex flex-wrap gap-2">
          <li
            v-for="model in group.models"
            :key="model.name"
            class="inline-flex items-center gap-1.5 rounded-lg bg-gray-100 px-2.5 py-1.5 font-mono text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200"
          >
            <span
              class="h-1.5 w-1.5 rounded-full"
              :class="modelStatusClass(model.status)"
              aria-hidden="true"
            />
            <span>{{ model.name }}</span>
            <span class="text-gray-500 dark:text-gray-400">
              {{ t('monitorV2.modelStatus', { status: t(`monitorV2.status.${model.status || 'insufficient_data'}`) }) }}
            </span>
          </li>
        </ul>
      </div>
    </details>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import MonitorV2Timeline from './MonitorV2Timeline.vue'
import type { MonitorV2Group, MonitorV2GroupStatus, MonitorV2Metric } from './types'

const props = defineProps<{
  group: MonitorV2Group
}>()

const { t } = useI18n()
const numberFormat = new Intl.NumberFormat()

const metrics = computed(() => [
  metricRow('ttft', t('monitorV2.metric.ttft'), props.group.ttft, formatDuration),
  metricRow('tps', t('monitorV2.metric.tps'), props.group.tps, formatTPS),
  metricRow('latency', t('monitorV2.metric.latency'), props.group.latency, formatDuration),
  metricRow('cache', t('monitorV2.metric.cache'), props.group.cache_hit, formatPercent),
])

const detailMetrics = computed(() => [
  metricRow('ttft-p95', t('monitorV2.metric.ttftP95'), props.group.ttft_p95, formatDuration),
  metricRow(
    'latency-p95',
    t('monitorV2.metric.latencyP95'),
    props.group.latency_p95,
    formatDuration
  ),
])

const callEvidence = computed(() => {
  if (props.group.availability.eligible_count === 0) {
    return t(`monitorV2.metric.${props.group.availability.state}`)
  }
  return t('monitorV2.callEvidence', {
    success: numberFormat.format(props.group.availability.success_count),
    eligible: numberFormat.format(props.group.availability.eligible_count),
  })
})

const availabilityValue = computed(() => {
  if (props.group.availability.state !== 'available' || props.group.availability.value === null) {
    return '—'
  }
  return formatPercent(props.group.availability.value)
})

const peakRateCopy = computed(() =>
  t('monitorV2.peakRate', {
    start: props.group.peak_start,
    end: props.group.peak_end,
    rate: formatRate(props.group.peak_rate_multiplier),
  })
)

function metricRow(
  key: string,
  label: string,
  metric: MonitorV2Metric,
  formatter: (value: number) => string
) {
  const available = metric.state === 'available' && metric.value !== null
  return {
    key,
    label,
    value: available ? formatter(metric.value as number) : t(`monitorV2.metric.${metric.state}`),
    samples:
      metric.sample_count > 0
        ? t('monitorV2.samples', { count: numberFormat.format(metric.sample_count) })
        : ' ',
  }
}

function formatRate(value: number): string {
  return Number(value.toFixed(4)).toString()
}

function formatDuration(value: number): string {
  if (value < 1000) return `${Math.round(value)} ms`
  return `${Number((value / 1000).toFixed(2))} s`
}

function formatTPS(value: number): string {
  return `${Number(value.toFixed(1))} tok/s`
}

function formatPercent(value: number): string {
  return `${Number(value.toFixed(2))}%`
}

function statusClass(status: MonitorV2GroupStatus): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    case 'degraded':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'unavailable':
      return 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'
  }
}

function modelStatusClass(status: string): string {
  switch (status) {
    case 'operational':
      return 'bg-emerald-500'
    case 'degraded':
      return 'bg-amber-500'
    case 'unavailable':
    case 'failed':
    case 'error':
      return 'bg-red-500'
    default:
      return 'bg-gray-400'
  }
}
</script>
