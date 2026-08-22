<template>
  <div
    ref="rootElement"
    data-timeline-root
    data-timeline-orientation="vertical-bars"
    class="relative flex w-full min-w-0 max-w-full flex-col justify-center"
    role="group"
    :aria-label="ariaLabel"
    @mouseleave="activeIndex = null"
  >
    <div
      data-timeline-scroll
      class="max-w-full overflow-x-auto overflow-y-hidden overscroll-x-contain [scrollbar-width:thin]"
      @scroll="repositionTooltip"
    >
      <div
        data-timeline-track
        class="items-center gap-[6px] px-1 py-1"
        :style="trackStyle"
      >
        <span
          v-for="(point, index) in points"
          :key="`${point.bucket_start}-${index}`"
          :data-timeline-point="index"
          :data-timeline-point-state="pointState(point)"
          role="img"
          class="h-8 w-1.5 w-auto min-w-1.5 shrink-0 cursor-default rounded-[2px] transition-[box-shadow,filter,background-color] duration-200 ease-out hover:shadow-[0_0_10px_currentColor] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-300/70"
          :class="pointClasses(point)"
          :aria-label="pointLabel(point)"
          tabindex="0"
          @mouseenter="activatePoint(index, $event)"
          @focus="activatePoint(index, $event)"
          @blur="activeIndex = null"
        />
        <span
          v-if="points.length === 0"
          class="h-1 w-full min-w-40 rounded-sm bg-gray-200 dark:bg-dark-700"
        />
      </div>
    </div>
    <div
      data-timeline-tooltip-row
      class="pointer-events-none absolute inset-x-0 top-full z-30"
      aria-live="polite"
    >
      <div
        v-if="activePoint"
        data-timeline-tooltip
        class="absolute top-0 z-20 min-w-[196px] -translate-x-1/2 rounded-xl border border-slate-600 bg-slate-950 px-4 py-3 text-center text-sm text-white shadow-2xl transition-opacity duration-200"
        :style="{ left: tooltipLeft }"
      >
        <span
          class="block text-base font-black tracking-wide"
          :class="headingClass(activePoint)"
        >
          {{ heading(activePoint) }}
        </span>
        <span class="mt-1 block whitespace-nowrap text-slate-200">{{ tooltipTimestamp(activePoint.bucket_start) }}</span>
        <span class="mt-1 block text-slate-400">
          {{ pointStatusLabel(activePoint) }}<template v-if="activePoint.latency_ms !== null"> · {{ activePoint.latency_ms }} ms</template>
        </span>
        <span data-timeline-tooltip-arrow class="absolute -top-1 left-1/2 h-2.5 w-2.5 -translate-x-1/2 rotate-45 border-l border-t border-slate-600 bg-slate-950" />
      </div>
    </div>
    <div v-if="points.length > 0" class="mt-0.5 flex justify-between px-1 text-[11px] text-gray-400 dark:text-gray-500" aria-hidden="true">
      <span>{{ compactTimestamp(points[0].bucket_start) }}</span>
      <span>{{ compactTimestamp(points[points.length - 1].bucket_start) }}</span>
    </div>
    <p
      v-else
      class="mt-1.5 text-xs text-gray-500 dark:text-gray-400"
    >
      {{ t('monitorV2.timeline.noData') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorV2TimelinePoint } from './types'

const props = defineProps<{
  points: MonitorV2TimelinePoint[]
}>()

const { t } = useI18n()
const activeIndex = ref<number | null>(null)
const rootElement = ref<HTMLElement | null>(null)
const activeElement = ref<HTMLElement | null>(null)
const tooltipLeftPixels = ref<number | null>(null)

const activePoint = computed(() => activeIndex.value === null ? null : props.points[activeIndex.value] ?? null)
const trackStyle = computed(() => ({
  '--timeline-count': String(Math.max(props.points.length, 1)),
} as Record<string, string>))
const tooltipLeft = computed(() => {
  if (tooltipLeftPixels.value !== null) return `${tooltipLeftPixels.value}px`
  if (activeIndex.value === null || props.points.length === 0) return '50%'
  const raw = ((activeIndex.value + 0.5) / props.points.length) * 100
  return `${Math.min(88, Math.max(12, raw))}%`
})

function activatePoint(index: number, event: Event) {
  activeIndex.value = index
  activeElement.value = event.currentTarget as HTMLElement
  void nextTick(repositionTooltip)
}

function repositionTooltip() {
  const root = rootElement.value
  const point = activeElement.value
  if (!root || !point || activeIndex.value === null) return
  const rootRect = root.getBoundingClientRect()
  const pointRect = point.getBoundingClientRect()
  if (rootRect.width <= 0) {
    tooltipLeftPixels.value = null
    return
  }
  const halfTooltip = 98
  const center = pointRect.left - rootRect.left + pointRect.width / 2
  tooltipLeftPixels.value = Math.min(rootRect.width - halfTooltip, Math.max(halfTooltip, center))
}

const ariaLabel = computed(() => {
  if (props.points.length === 0) return t('monitorV2.timeline.noData')
  return t('monitorV2.timeline.label', { count: props.points.length })
})

function pointLabel(point: MonitorV2TimelinePoint): string {
  const outcome = isNoDataPoint(point)
    ? t('monitorV2.timeline.noDataBucketLabel')
    : pointStatusLabel(point)
  const latency = point.latency_ms === null ? '' : ` · ${point.latency_ms} ms`
  return `${tooltipTimestamp(point.bucket_start)} · ${outcome}${latency}`
}

function isNoDataPoint(point: MonitorV2TimelinePoint): boolean {
  return !point.has_result
}

function pointState(point: MonitorV2TimelinePoint): 'operational' | 'unavailable' | 'no-data' {
  return isNoDataPoint(point) ? 'no-data' : point.status
}

function pointClasses(point: MonitorV2TimelinePoint): string {
  if (isNoDataPoint(point)) {
    return 'border border-dashed border-slate-300 bg-slate-400/70 text-slate-500 dark:border-slate-500 dark:bg-slate-600/70 dark:text-slate-300'
  }
  return point.status === 'operational'
    ? 'bg-emerald-400 text-emerald-400 dark:bg-emerald-400'
    : 'bg-red-400 text-red-400 dark:bg-red-400'
}

function heading(point: MonitorV2TimelinePoint): 'UP' | 'DOWN' | 'NO DATA' {
  if (isNoDataPoint(point)) return 'NO DATA'
  return point.status === 'operational' ? 'UP' : 'DOWN'
}

function headingClass(point: MonitorV2TimelinePoint): string {
  if (isNoDataPoint(point)) return 'text-amber-300'
  return point.status === 'operational' ? 'text-emerald-400' : 'text-red-400'
}

function pointStatusLabel(point: MonitorV2TimelinePoint): string {
  return isNoDataPoint(point)
    ? t('monitorV2.timeline.noDataBucket')
    : t(`monitorV2.status.${point.status}`)
}

function tooltipTimestamp(value: string): string {
  const date = new Date(value)
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function compactTimestamp(value: string): string {
  return new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}
</script>

<style scoped>
[data-timeline-track] {
  display: grid;
  grid-template-columns: repeat(var(--timeline-count), minmax(0, 1fr));
  min-width: 0;
}

[data-timeline-point] {
  width: auto;
  min-width: 4px;
}

@media (max-width: 639px) {
  [data-timeline-track] {
    display: flex;
    min-width: 620px;
  }

  [data-timeline-point] {
    width: 6px;
    flex: 0 0 6px;
  }
}
</style>
