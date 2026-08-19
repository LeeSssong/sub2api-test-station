<template>
  <div
    ref="rootElement"
    data-timeline-root
    class="relative w-full min-w-0 max-w-full sm:w-[620px] sm:flex-shrink-0"
    role="group"
    :aria-label="ariaLabel"
    @mouseleave="activeIndex = null"
  >
    <div
      v-if="activePoint"
      data-timeline-tooltip
      class="pointer-events-none absolute top-0 z-20 min-w-[168px] -translate-x-1/2 rounded-lg border border-slate-600 bg-slate-950 px-3 py-2 text-center text-xs text-white shadow-2xl transition-all duration-200"
      :style="{ left: tooltipLeft }"
      aria-live="polite"
    >
      <span
        class="block text-sm font-black tracking-wide"
        :class="activePoint.status === 'operational' ? 'text-emerald-400' : 'text-red-400'"
      >
        {{ activePoint.status === 'operational' ? 'UP' : 'DOWN' }}
      </span>
      <span class="mt-0.5 block whitespace-nowrap text-slate-200">{{ tooltipTimestamp(activePoint.bucket_start) }}</span>
      <span class="mt-0.5 block text-slate-400">
        {{ t(`monitorV2.status.${activePoint.status}`) }}<template v-if="activePoint.latency_ms !== null"> · {{ activePoint.latency_ms }} ms</template>
      </span>
      <span class="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 border-b border-r border-slate-600 bg-slate-950" />
    </div>

    <div
      data-timeline-scroll
      class="max-w-full overflow-x-auto overflow-y-hidden overscroll-x-contain pt-12 [scrollbar-width:thin]"
      @scroll="repositionTooltip"
    >
      <div
        data-timeline-track
        class="flex min-w-max items-end gap-[4px] px-1 pb-1"
      >
        <span
          v-for="(point, index) in points"
          :key="`${point.bucket_start}-${index}`"
          :data-timeline-point="index"
          role="img"
          class="h-4 w-[5px] shrink-0 cursor-default rounded-full transition-all duration-200 ease-out hover:-translate-y-1 hover:scale-y-125 hover:shadow-[0_0_10px_currentColor] focus-visible:-translate-y-1 focus-visible:scale-y-125 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-300/70"
          :class="point.status === 'operational' ? 'bg-emerald-400 text-emerald-400 dark:bg-emerald-400' : 'bg-red-400 text-red-400 dark:bg-red-400'"
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
    <div v-if="points.length > 0" class="mt-1 flex justify-between px-1 text-[10px] text-gray-400 dark:text-gray-500" aria-hidden="true">
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
  const halfTooltip = 84
  const center = pointRect.left - rootRect.left + pointRect.width / 2
  tooltipLeftPixels.value = Math.min(rootRect.width - halfTooltip, Math.max(halfTooltip, center))
}

const ariaLabel = computed(() => {
  if (props.points.length === 0) return t('monitorV2.timeline.noData')
  return t('monitorV2.timeline.label', { count: props.points.length })
})

function pointLabel(point: MonitorV2TimelinePoint): string {
  const outcome = t(`monitorV2.status.${point.status}`)
  const latency = point.latency_ms === null ? '' : ` · ${point.latency_ms} ms`
  return `${tooltipTimestamp(point.bucket_start)} · ${outcome}${latency}`
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
