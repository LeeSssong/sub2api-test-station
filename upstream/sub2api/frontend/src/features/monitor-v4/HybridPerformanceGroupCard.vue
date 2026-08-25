<template>
  <article class="hybrid-card" :class="`hybrid-card--${tone}`" data-test="hybrid-group-card">
    <header class="hybrid-card__header">
      <div>
        <h2 class="hybrid-card__title">{{ group.name }}</h2>
        <p class="hybrid-card__meta">{{ group.platform }}</p>
      </div>
      <span class="hybrid-card__status" data-test="monitoring-status">
        <span class="status-dot" aria-hidden="true" />{{ t('channelMonitorV2.hybrid.monitoring') }}
      </span>
    </header>

    <div class="hybrid-card__ring-wrap">
      <div class="hybrid-ring" :class="`hybrid-ring--${tone}`" data-test="ring" role="img" :aria-label="`${group.availability}%`">
        <div class="hybrid-ring__center">
          <strong data-test="availability">{{ formatAvailability(group.availability) }}%</strong>
          <span>{{ t('channelMonitorV2.hybrid.availability') }}</span>
        </div>
      </div>
    </div>

    <div class="hybrid-card__metrics">
      <div class="hybrid-metric">
        <span>{{ t('channelMonitorV2.hybrid.ttftP95') }}</span>
        <strong data-test="ttft-p95">{{ formatMs(group.ttft_p95_ms) }}</strong>
      </div>
      <div class="hybrid-metric">
        <span>{{ t('channelMonitorV2.hybrid.latencyP95') }}</span>
        <strong data-test="latency-p95">{{ formatMs(group.latency_p95_ms) }}</strong>
      </div>
    </div>

    <footer class="hybrid-card__footer">
      <span data-test="sample-count">{{ t('channelMonitorV2.hybrid.sampleCount', { count: group.sample_count }) }}</span>
      <span data-test="multiplier">{{ t('channelMonitorV2.hybrid.multiplier', { value: group.rate_multiplier }) }}</span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorV4Group } from './types'

const props = defineProps<{ group: MonitorV4Group }>()
const { t } = useI18n()
const tone = computed(() => props.group.availability >= 85 ? 'green' : props.group.availability >= 50 ? 'amber' : 'red')
const formatAvailability = (value: number) => Number.isInteger(value) ? String(value) : value.toFixed(1)
const formatMs = (value: number) => `${Math.round(value)} ms`
</script>

<style scoped>
.hybrid-card { --ring: #16a34a; display:flex; min-width:0; flex-direction:column; border:1px solid #dbe4ee; border-radius:12px; background:#fff; padding:20px; color:#172033; box-shadow:0 8px 24px rgba(15,23,42,.06); }
.hybrid-card--amber { --ring:#d89b18; } .hybrid-card--red { --ring:#dc3f4d; }
.hybrid-card__header { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
.hybrid-card__title { margin:0; font-size:1rem; font-weight:700; } .hybrid-card__meta { margin:3px 0 0; color:#718096; font-size:.72rem; }
.hybrid-card__status { display:inline-flex; align-items:center; gap:6px; color:#64748b; font-size:.7rem; white-space:nowrap; } .status-dot { width:6px; height:6px; border-radius:50%; background:var(--ring); }
.hybrid-card__ring-wrap { display:grid; place-items:center; padding:18px 0 20px; }
.hybrid-ring { position:relative; display:grid; width:min(50vw, 220px); aspect-ratio:1; place-items:center; border:10px solid color-mix(in srgb, var(--ring) 18%, transparent); border-top-color:var(--ring); border-right-color:color-mix(in srgb, var(--ring) 70%, transparent); border-radius:50%; box-shadow:0 0 0 1px color-mix(in srgb, var(--ring) 8%, transparent), 0 0 24px color-mix(in srgb, var(--ring) 16%, transparent); animation:hybrid-breathe 3.2s ease-in-out infinite; }
.hybrid-ring__center { display:flex; flex-direction:column; align-items:center; justify-content:center; } .hybrid-ring__center strong { color:var(--ring); font-size:clamp(2.2rem, 5vw, 3.25rem); line-height:1; } .hybrid-ring__center span { margin-top:8px; color:#7b8798; font-size:.75rem; }
.hybrid-card__metrics { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); border-top:1px solid #edf1f5; border-bottom:1px solid #edf1f5; padding:14px 0; } .hybrid-metric { min-width:0; text-align:center; } .hybrid-metric + .hybrid-metric { border-left:1px solid #edf1f5; } .hybrid-metric span { display:block; color:#7b8798; font-size:.7rem; } .hybrid-metric strong { display:block; margin-top:5px; color:#233047; font-size:1.05rem; }
.hybrid-card__footer { display:flex; justify-content:space-between; gap:10px; padding-top:14px; color:#7b8798; font-size:.7rem; } .hybrid-card__footer span:last-child { color:#526075; font-weight:600; }
@keyframes hybrid-breathe { 0%,100% { opacity:.88; box-shadow:0 0 0 1px color-mix(in srgb, var(--ring) 8%, transparent), 0 0 20px color-mix(in srgb, var(--ring) 12%, transparent); } 50% { opacity:1; box-shadow:0 0 0 1px color-mix(in srgb, var(--ring) 16%, transparent), 0 0 34px color-mix(in srgb, var(--ring) 28%, transparent); } }
@media (prefers-reduced-motion: reduce) { .hybrid-ring { animation:none; } }
@media (max-width:640px) { .hybrid-card { padding:16px; } .hybrid-ring { width:min(56vw, 190px); } }
</style>
