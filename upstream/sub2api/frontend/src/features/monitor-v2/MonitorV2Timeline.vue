<template>
  <div
    class="mt-4"
    role="img"
    :aria-label="ariaLabel"
  >
    <div class="flex h-8 items-end gap-1" aria-hidden="true">
      <span
        v-for="(point, index) in points"
        :key="`${point.bucket_start}-${index}`"
        class="min-w-1 flex-1 rounded-sm transition-[height,background-color] duration-200 motion-reduce:transition-none"
        :class="toneClass(point)"
        :style="{ height: barHeight(point) }"
        :title="pointTitle(point)"
      />
      <span
        v-if="points.length === 0"
        class="h-1 w-full rounded-sm bg-gray-200 dark:bg-dark-700"
      />
    </div>
    <p
      v-if="points.length === 0"
      class="mt-1.5 text-xs text-gray-500 dark:text-gray-400"
    >
      {{ t('monitorV2.timeline.noData') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorV2TimelinePoint } from './types'

const props = defineProps<{
  points: MonitorV2TimelinePoint[]
}>()

const { t } = useI18n()

const ariaLabel = computed(() => {
  if (props.points.length === 0) return t('monitorV2.timeline.noData')
  return props.points
    .map((point) => pointTitle(point))
    .join('；')
})

function barHeight(point: MonitorV2TimelinePoint): string {
  if (point.state !== 'available' || point.value === null) return '20%'
  if (point.success_count === 0) return '32%'
  if (point.latency_ms === null) return '40%'
  return `${Math.max(28, Math.min(100, 100 - point.latency_ms / 200))}%`
}

function toneClass(point: MonitorV2TimelinePoint): string {
  if (point.state !== 'available' || point.value === null) {
    return 'bg-gray-200 dark:bg-dark-700'
  }
  if (point.success_count > 0) return 'bg-emerald-500 dark:bg-emerald-400'
  return 'bg-red-500 dark:bg-red-400'
}

function pointTitle(point: MonitorV2TimelinePoint): string {
  const time = new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
  }).format(new Date(point.bucket_start))
  if (point.state !== 'available' || point.value === null) {
    return `${time} · ${t('monitorV2.timeline.noData')}`
  }
  const outcome = point.success_count > 0 ? t('monitorV2.timeline.success') : t('monitorV2.timeline.failed')
  const latency = point.latency_ms === null ? '' : ` · ${point.latency_ms} ms`
  return `${time} · ${outcome}${latency}`
}
</script>
