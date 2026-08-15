<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef, nextTick, watch } from 'vue'

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click' | 'hover-click'
  widthClass?: string
  resetKey?: string | number | null
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const clickPinned = ref(false)
const suppressTriggerFocusOpen = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const tooltipStyle = ref({ top: '0px', left: '0px' })

watch(() => props.trigger, (trigger) => {
  if (trigger !== 'hover-click') {
    clickPinned.value = false
  }
})

watch(() => props.resetKey, () => {
  if (props.trigger === 'hover-click') {
    closeTooltip({ clearPin: true })
  }
})

watch(show, (visible) => {
  if (visible) {
    updatePosition()
  }
}, { flush: 'post' })

function focusTrigger() {
  const target = triggerRef.value?.querySelector<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')
    ?? triggerRef.value
  if (!target || typeof target.focus !== 'function') return
  suppressTriggerFocusOpen.value = true
  target.focus()
  nextTick(() => {
    suppressTriggerFocusOpen.value = false
  })
}

function openTooltip() {
  show.value = true
}

function closeTooltip(options?: { clearPin?: boolean; restoreFocus?: boolean }) {
  show.value = false
  if (options?.clearPin) {
    clickPinned.value = false
  }
  if (options?.restoreFocus) {
    focusTrigger()
  }
}

function onEnter() {
  if (props.trigger !== 'hover' && props.trigger !== 'hover-click') return
  openTooltip()
}

function onLeave() {
  if (props.trigger !== 'hover' && props.trigger !== 'hover-click') return
  if (props.trigger === 'hover-click' && clickPinned.value) return
  closeTooltip()
}

function onFocusIn() {
  if (props.trigger !== 'hover' && props.trigger !== 'hover-click') return
  if (suppressTriggerFocusOpen.value) return
  openTooltip()
}

function onFocusOut(event: FocusEvent) {
  if (props.trigger !== 'hover' && props.trigger !== 'hover-click') return
  const nextTarget = event.relatedTarget as Node | null
  if (nextTarget && (triggerRef.value?.contains(nextTarget) || tooltipRef.value?.contains(nextTarget))) return
  if (props.trigger === 'hover-click' && clickPinned.value) return
  closeTooltip()
}

function onTooltipFocusOut(event: FocusEvent) {
  if (props.trigger !== 'click' && props.trigger !== 'hover-click') return
  const nextTarget = event.relatedTarget as Node | null
  if (nextTarget && (triggerRef.value?.contains(nextTarget) || tooltipRef.value?.contains(nextTarget))) return
  closeTooltip({ clearPin: true })
}

function onClick(event: MouseEvent) {
  if (props.trigger !== 'click' && props.trigger !== 'hover-click') return
  event.stopPropagation()
  if (props.trigger === 'hover-click') {
    if (clickPinned.value) {
      clickPinned.value = false
      closeTooltip()
    } else {
      clickPinned.value = true
      openTooltip()
    }
    return
  }
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip()
}

function onDocumentClick(event: MouseEvent) {
  if ((props.trigger !== 'click' && props.trigger !== 'hover-click') || !show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip({ clearPin: true })
}

function onDocumentKeydown(event: KeyboardEvent) {
  if ((props.trigger !== 'click' && props.trigger !== 'hover-click') || !show.value) return
  if (event.key === 'Escape') {
    closeTooltip({ clearPin: true, restoreFocus: true })
  }
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const tooltipWidth = tooltipRef.value?.offsetWidth ?? 0
  const centerLeft = rect.left + rect.width / 2
  const edgePadding = 12
  const minLeft = tooltipWidth ? (tooltipWidth / 2) + edgePadding : centerLeft
  const maxLeft = tooltipWidth ? window.innerWidth - (tooltipWidth / 2) - edgePadding : centerLeft
  const clampedLeft = tooltipWidth && minLeft <= maxLeft ? Math.min(Math.max(centerLeft, minLeft), maxLeft) : centerLeft
  tooltipStyle.value = {
    top: `${rect.top}px`,
    left: `${clampedLeft}px`,
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<template>
  <div
    ref="trigger"
    class="group relative ml-1 inline-flex items-center align-middle"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @click="onClick"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    </slot>

    <!-- Teleport to body to escape modal overflow clipping -->
    <Teleport to="body">
      <div
        ref="tooltip"
        v-show="show"
        role="tooltip"
        @focusout="onTooltipFocusOut"
        :class="[
          'fixed z-[99999] -translate-x-1/2 -translate-y-full rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 dark:bg-gray-800',
          props.widthClass,
        ]"
        :style="{ top: `calc(${tooltipStyle.top} - 8px)`, left: tooltipStyle.left }"
      >
        <button
          v-if="props.trigger === 'click' || props.trigger === 'hover-click'"
          type="button"
          class="absolute right-1.5 top-1.5 rounded p-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white"
          aria-label="Close"
          @click.stop="closeTooltip({ clearPin: true, restoreFocus: true })"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <slot>{{ content }}</slot>
        <div class="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
      </div>
    </Teleport>
  </div>
</template>
