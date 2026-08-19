<template>
  <div
    class="mt-4"
    role="img"
    :aria-label="ariaLabel"
  >
    <div class="flex h-8 min-w-0 items-end gap-1 overflow-hidden" aria-hidden="true">
      <span
        v-for="(point, index) in points"
        :key="`${point.bucket_start}-${index}`"
        class="min-w-0 basis-0 flex-1 rounded-sm bg-teal-500 dark:bg-teal-400"
        style="height: 75%"
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

function pointTitle(point: MonitorV2TimelinePoint): string {
  const time = new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
  }).format(new Date(point.bucket_start))
  if (point.state !== 'available' || point.value === null) return `${time} · ${t('monitorV2.timeline.noData')}`
  const outcome = point.success_count > 0 ? t('monitorV2.timeline.success') : t('monitorV2.timeline.failed')
  const latency = point.latency_ms === null ? '' : ` · ${point.latency_ms} ms`
  return `${time} · ${outcome}${latency}`
}
</script>
