<template>
  <div class="model-performance">
    <table>
      <thead><tr><th>{{ t('channelMonitorV2.tool.model') }}</th><th>{{ t('channelMonitorV2.metrics.successRate') }}</th><th>{{ t('channelMonitorV2.metrics.ttftP50') }}</th><th>{{ t('channelMonitorV2.metrics.cacheRate') }}</th></tr></thead>
      <tbody><tr v-for="row in items" :key="`${row.platform}:${row.model}`">
        <td class="model-identity"><span class="model-platform">{{ providerLabel(row.platform) }}</span><strong>{{ row.model }}</strong></td>
        <td :data-label="t('channelMonitorV2.metrics.successRate')"><div><span class="model-main">{{ successLabel(row) }}</span><small>{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatMonitorPercent(row.metrics.error_rate) }) }}</small></div></td>
        <td :data-label="t('channelMonitorV2.metrics.ttftP50')"><div><span class="model-main">{{ formatMonitorMs(row.metrics.ttft.p50_ms) }}</span><small>{{ formatLatencyPrivacy(row.metrics.ttft.p50_ms, row.metrics.ttft.p90_ms, row.metrics.ttft.avg_ms) }}</small></div></td>
        <td :data-label="t('channelMonitorV2.metrics.cacheRate')"><span class="model-main">{{ formatMonitorPercent(row.metrics.cache_rate) }}</span></td>
      </tr></tbody>
    </table>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MonitorModelRow } from '@/api/channelMonitorV2'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { formatMonitorMs, formatMonitorPercent, formatLatencyPrivacy } from './monitorFormat'
defineProps<{ items: MonitorModelRow[] }>()
const { t } = useI18n()
const { providerLabel } = useChannelMonitorFormat()
function successLabel(row: MonitorModelRow) {
 const rate = row.metrics.success_rate ?? (row.metrics.request_count > 0 ? row.metrics.success_requests / row.metrics.request_count : null)
 return rate == null || !Number.isFinite(rate) ? '—' : formatMonitorPercent(rate)
}
</script>
<style scoped>
.model-performance{border:1px solid var(--xq-border);border-radius:12px;overflow:hidden}.model-performance table{width:100%;border-collapse:collapse;text-align:left;font-size:13px}.model-performance th{padding:13px 16px;background:var(--xq-depth);color:var(--xq-secondary);font-size:12px;font-weight:500}.model-performance td{padding:18px 16px;border-top:1px solid var(--xq-border);vertical-align:top}.model-identity{max-width:280px;overflow-wrap:anywhere}.model-platform{display:block;margin-bottom:5px;font-size:11px;color:var(--xq-secondary)}.model-performance strong{font-weight:600;color:var(--xq-text)}.model-main{display:block;font-variant-numeric:tabular-nums;color:var(--xq-text)}.model-performance small{display:block;margin-top:5px;color:var(--xq-secondary);font-size:11px;line-height:1.7;font-variant-numeric:tabular-nums}@media(max-width:700px){.model-performance thead{display:none}.model-performance table,.model-performance tbody,.model-performance tr,.model-performance td{display:block}.model-performance tr{padding:16px}.model-performance tr+tr{border-top:1px solid var(--xq-border)}.model-performance td{border:0;padding:9px 0;display:flex;align-items:flex-start;justify-content:space-between;gap:18px;text-align:right}.model-performance td:before{content:attr(data-label);text-align:left;color:var(--xq-secondary);flex-shrink:0}.model-performance .model-identity{display:block;max-width:none;text-align:left;padding:0 0 12px}.model-identity:before{display:none}.model-performance small{max-width:210px}}
</style>
