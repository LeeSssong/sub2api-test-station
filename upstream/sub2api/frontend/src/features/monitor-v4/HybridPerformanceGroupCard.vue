<template>
  <article class="hybrid-card" :class="`hybrid-card--${tone}`" data-test="hybrid-group-card">
    <header class="hybrid-card__header">
      <div>
        <h2 class="hybrid-card__title">{{ group.name }}</h2>
        <p class="hybrid-card__meta">{{ group.platform }}</p>
        <span class="hybrid-card__status" data-test="monitoring-status">
          <span class="status-dot" aria-hidden="true" />{{ t('channelMonitorV2.hybrid.monitoring') }}
        </span>
      </div>
      <span class="hybrid-card__multiplier" data-test="multiplier">
        {{ t('channelMonitorV2.hybrid.multiplier', { value: group.rate_multiplier }) }}
      </span>
    </header>

    <div class="hybrid-card__ring-wrap">
      <div class="hybrid-ring" :class="`hybrid-ring--${tone}`" data-test="ring" role="img" :aria-label="successRateLabel">
        <div class="hybrid-ring__center">
          <strong data-test="success-rate">{{ successRateLabel }}</strong>
          <span>{{ t('channelMonitorV2.hybrid.successRate') }}</span>
        </div>
      </div>
    </div>

    <div class="hybrid-card__metrics">
      <div class="hybrid-metric">
        <span>{{ t('channelMonitorV2.hybrid.ttftP95') }}</span>
        <strong data-test="ttft-p95">{{ formatSeconds(group.ttft_p95_ms) }}</strong>
      </div>
      <div class="hybrid-metric">
        <span>{{ t('channelMonitorV2.hybrid.latencyP95') }}</span>
        <strong data-test="latency-p95">{{ formatSeconds(group.latency_p95_ms) }}</strong>
      </div>
      <div class="hybrid-metric">
        <span>{{ t('channelMonitorV2.hybrid.cacheHitRate') }}</span>
        <strong data-test="cache-hit-rate">{{ formatCacheHitRate(group.cache_hit_rate) }}</strong>
      </div>
    </div>

    <footer class="hybrid-card__footer">
      <span data-test="sample-count">{{ t('channelMonitorV2.hybrid.sampleCount', { count: group.request_count }) }}</span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorV4Group } from './types'

const props = defineProps<{ group: MonitorV4Group }>()
const { t } = useI18n()
const tone = computed(() => props.group.success_rate === null ? 'amber' : props.group.success_rate >= 85 ? 'green' : props.group.success_rate >= 50 ? 'amber' : 'red')
const successRateLabel = computed(() => props.group.success_rate === null ? '--' : `${Number.isInteger(props.group.success_rate) ? props.group.success_rate : props.group.success_rate.toFixed(1)}%`)
const formatSeconds = (value: number | null) => value === null ? '--' : `${(value / 1000).toFixed(2)} s`
const formatCacheHitRate = (value: number | null) => value === null ? '--' : `${(value * 100).toFixed(1)}%`
</script>

<style scoped>
.hybrid-card {
  --ring: #16a34a;
  --card-bg: #fff;
  --card-border: #dbe4ee;
  --card-ink: #172033;
  --card-muted: #718096;
  --divider: #edf1f5;
  --ring-surface: #fff;
  display: flex;
  min-width: 0;
  min-height: 31rem;
  flex-direction: column;
  border: 1px solid var(--card-border);
  border-radius: 12px;
  background: var(--card-bg);
  padding: 22px;
  color: var(--card-ink);
}
.hybrid-card--amber { --ring: #d89b18; }
.hybrid-card--red { --ring: #dc3f4d; }
.hybrid-card__header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.hybrid-card__title { margin: 0; font-size: 1.05rem; font-weight: 750; letter-spacing: 0; }
.hybrid-card__meta { margin: 4px 0 0; color: var(--card-muted); font-size: 0.75rem; }
.hybrid-card__status { display: inline-flex; align-items: center; gap: 7px; margin-top: 10px; color: #64748b; font-size: 0.72rem; white-space: nowrap; }
.status-dot { width: 7px; height: 7px; flex: none; border-radius: 50%; background: var(--ring); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ring) 14%, transparent); }
.hybrid-card__multiplier { flex: none; color: var(--ring); font-size: 0.76rem; font-weight: 700; white-space: nowrap; }
.hybrid-card__ring-wrap { display: grid; flex: 1; min-height: 17rem; place-items: center; padding: 20px 0 22px; }
.hybrid-ring { position: relative; display: grid; width: min(70%, 300px); aspect-ratio: 1; place-items: center; border: 12px solid var(--ring); border-radius: 50%; background: var(--ring-surface); box-shadow: 0 0 0 1px color-mix(in srgb, var(--ring) 18%, transparent), 0 0 22px color-mix(in srgb, var(--ring) 18%, transparent); animation: hybrid-breathe 2.8s ease-in-out infinite; }
.hybrid-ring__center { display: flex; flex-direction: column; align-items: center; justify-content: center; }
.hybrid-ring__center strong { color: var(--ring); font-size: clamp(2.65rem, 5vw, 3.7rem); line-height: 1; }
.hybrid-ring__center span { margin-top: 10px; color: #7b8798; font-size: 0.78rem; }
.hybrid-card__metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-top: 1px solid var(--divider); border-bottom: 1px solid var(--divider); padding: 16px 0; }
.hybrid-metric { min-width: 0; text-align: center; }
.hybrid-metric + .hybrid-metric { border-left: 1px solid var(--divider); }
.hybrid-metric span { display: block; color: #7b8798; font-size: 0.72rem; }
.hybrid-metric strong { display: block; margin-top: 6px; color: #233047; font-size: 1.12rem; }
.hybrid-card__footer { display: flex; flex-direction: column; align-items: center; gap: 5px; padding-top: 15px; color: #7b8798; font-size: 0.72rem; text-align: center; }
.hybrid-card__footer span:first-child { color: #526075; font-weight: 600; }
@keyframes hybrid-breathe {
  0%, 100% {
    opacity: 0.76;
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--ring) 14%, transparent), 0 0 20px color-mix(in srgb, var(--ring) 14%, transparent), inset 0 0 10px color-mix(in srgb, var(--ring) 8%, transparent);
  }
  50% {
    opacity: 1;
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--ring) 30%, transparent), 0 0 48px color-mix(in srgb, var(--ring) 34%, transparent), inset 0 0 24px color-mix(in srgb, var(--ring) 16%, transparent);
  }
}
@media (prefers-reduced-motion: reduce) { .hybrid-ring { animation: none; } }
@media (max-width: 640px) { .hybrid-card { min-height: 29rem; padding: 18px; } .hybrid-card__ring-wrap { min-height: 15rem; } .hybrid-ring { width: min(72vw, 250px); } }
:global(.dark) .hybrid-card {
  --card-bg: #0d182a;
  --card-border: #293d59;
  --card-ink: #edf5ff;
  --card-muted: #a8bad0;
  --divider: #263a55;
  --ring-surface: #122136;
}
:global(.dark) .hybrid-card__meta,
:global(.dark) .hybrid-card__status,
:global(.dark) .hybrid-ring__center span,
:global(.dark) .hybrid-metric span,
:global(.dark) .hybrid-card__footer { color: #a8bad0; }
:global(.dark) .hybrid-metric strong { color: #dce8f5; }
:global(.dark) .hybrid-card__footer span:first-child { color: #c4d4e6; }
</style>
