<template>
  <div class="tool-carousel" role="region" aria-label="AI 工具">
    <button type="button" class="tool-carousel-arrow" aria-label="查看前面的 AI 工具" :disabled="!canPrevious" @click="move(-1)"><Icon name="chevronLeft" /></button>
    <div ref="viewport" class="tool-grid" tabindex="0" aria-label="AI 工具卡片，可按整页左右滚动" @scroll.passive="updateEdges" @keydown="onKeydown" @focusin="revealFocusedCard" @wheel.passive="clearPending" @pointerdown="clearPending" @touchstart.passive="clearPending"><slot /><div v-for="n in emptySlots" :key="n" class="tool-slot-empty" aria-hidden="true"></div></div>
    <button type="button" class="tool-carousel-arrow" aria-label="查看后面的 AI 工具" :disabled="!canNext" @click="move(1)"><Icon name="chevronRight" /></button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ itemCount: number }>()
const viewport = ref<HTMLElement | null>(null)
const canPrevious = ref(false)
const canNext = ref(false)
const visibleSlots = ref(4)
const emptySlots = computed(() => (visibleSlots.value - props.itemCount % visibleSlots.value) % visibleSlots.value)
let observer: ResizeObserver | undefined
let pendingPage: number | null = null
let activePage = 0
function clearPending() { pendingPage = null }
function slotsFromCSS(el: HTMLElement) {
  return Math.max(1, parseInt(el.style.getPropertyValue('--tool-visible') || getComputedStyle(el).getPropertyValue('--tool-visible'), 10) || 4)
}
function pageStep(el: HTMLElement) {
  const card = el.querySelector<HTMLElement>('.tool-card')
  if (!card) return 0
  const gap = parseFloat(getComputedStyle(el).columnGap) || 16
  return (card.getBoundingClientRect().width + gap) * visibleSlots.value
}
async function resized() {
  const el = viewport.value
  if (!el) return
  const firstVisibleItem = activePage * visibleSlots.value
  clearPending()
  visibleSlots.value = slotsFromCSS(el)
  await nextTick()
  const targetPage = Math.min(Math.floor(firstVisibleItem / visibleSlots.value), Math.max(0, Math.ceil(props.itemCount / visibleSlots.value) - 1))
  activePage = targetPage
  const targetLeft = targetPage * pageStep(el)
  if (Math.abs(el.scrollLeft - targetLeft) > 1) el.scrollTo({ left: targetLeft, behavior: 'auto' })
  updateEdges()
}
function updateEdges() {
  const el = viewport.value
  if (!el) return
  const step = pageStep(el)
  if (pendingPage !== null && step && Math.abs(el.scrollLeft - pendingPage * step) < 1) clearPending()
  if (pendingPage === null && step) activePage = Math.round(el.scrollLeft / step)
  canPrevious.value = el.scrollLeft > 1
  canNext.value = el.scrollLeft < el.scrollWidth - el.clientWidth - 1
}
function move(direction: number) {
  const el = viewport.value
  if (!el) return
  const step = pageStep(el)
  if (!step) return
  const lastPage = Math.max(0, Math.ceil(props.itemCount / visibleSlots.value) - 1)
  pendingPage = Math.max(0, Math.min(lastPage, (pendingPage ?? Math.round(el.scrollLeft / step)) + direction))
  activePage = pendingPage
  el.scrollTo({ left: pendingPage * step, behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
}
function onKeydown(event: KeyboardEvent) {
  // Card controls keep their own key handling.
  if (event.target !== viewport.value || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return
  event.preventDefault()
  move(event.key === 'ArrowRight' ? 1 : -1)
}
function revealFocusedCard(event: FocusEvent) {
  const el = viewport.value
  const target = event.target
  if (!el || !(target instanceof HTMLElement) || target === el) return
  const card = target.closest<HTMLElement>('.tool-card')
  if (!card) return
  const cardIndex = Array.from(el.querySelectorAll('.tool-card')).indexOf(card)
  const targetPage = Math.floor(cardIndex / visibleSlots.value)
  if (cardIndex >= 0 && targetPage !== activePage) {
    clearPending()
    activePage = targetPage
    el.scrollTo({ left: targetPage * pageStep(el), behavior: 'auto' })
  }
}
onMounted(async () => {
  await nextTick()
  if (!viewport.value) return
  observer = new ResizeObserver(resized)
  observer.observe(viewport.value)
  const card = viewport.value.querySelector('.tool-card')
  if (card) observer.observe(card)
  window.addEventListener('resize', resized)
  await resized()
})
onBeforeUnmount(() => { observer?.disconnect(); window.removeEventListener('resize', resized) })
</script>
