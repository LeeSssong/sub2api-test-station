<template>
  <Select
    class="xq-line-select"
    dropdown-class="xq-line-select-dropdown"
    :model-value="modelValue"
    :options="options"
    :placeholder="placeholder"
    :search-placeholder="searchPlaceholder"
    :empty-text="emptyText"
    :aria-label="ariaLabel"
    searchable
    brand
    @update:model-value="emit('update:modelValue', $event as number | null)"
  >
    <template #selected="{ option }">
      <span v-if="option" class="line-selected">
        <img :src="providerIcon(line(option).platform)" alt="" />
        <span>{{ line(option).label }}</span>
        <small>{{ line(option).rateLabel }}</small>
      </span>
      <span v-else>{{ placeholder }}</span>
    </template>
    <template #option="{ option }">
      <div class="line-option">
        <div class="line-option-head">
          <img :src="providerIcon(line(option).platform)" alt="" />
          <div class="line-option-name">
            <strong>{{ line(option).label }}</strong>
            <span v-if="isRecommended(line(option))" class="line-recommended">推荐最佳线路</span>
          </div>
          <span class="line-rate">{{ line(option).rateLabel }}</span>
          <span class="line-status" :data-status="line(option).healthKind">
            <i aria-hidden="true" />{{ line(option).statusLabel }}
          </span>
        </div>
        <p v-if="line(option).description" class="line-description">{{ line(option).description }}</p>
        <div class="line-option-meta">
          <span :title="line(option).availabilityTitle">{{ line(option).availabilityLabel }} <span class="line-success-rate" :data-tone="line(option).successTone">{{ line(option).successLabel }}</span></span>
          <span :title="line(option).ttftTitle">首字 <span :class="line(option).ttftSeverity ? LATENCY_TEXT_CLASSES[line(option).ttftSeverity!] : 'line-no-data'">{{ line(option).ttftLabel }}</span></span>
        </div>
        <small v-if="line(option).unavailableReason" class="line-unavailable">不可选：{{ line(option).unavailableReason }}</small>
      </div>
    </template>
  </Select>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useTimestamp } from '@vueuse/core'
import Select from '@/components/common/Select.vue'
import { providerIcon, toolIdsForGroup } from '@/features/ai-tools/model'
import type { Group } from '@/types'
import type { LineSelectionMetric } from '@/features/monitor-v4/lineSelection'
import { LATENCY_TEXT_CLASSES } from '@/utils/latencyHealth'
import { buildLineOptions, type LineOption } from './lineOptions'

const props = withDefaults(defineProps<{
  modelValue: number | null
  groups: Group[]
  rates: Record<number, number>
  metrics: Map<number, LineSelectionMetric>
  linkedCounts?: Map<number, number> | null
  metricsGeneratedAt?: string | null
  metricsError?: boolean
  recommendedGroupIds?: ReadonlyMap<string, number>
  toolId?: string
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
  ariaLabel?: string
}>(), {
  placeholder: '选择线路',
  searchPlaceholder: '搜索线路',
  emptyText: '暂无可配置线路',
  ariaLabel: '选择线路'
})
const emit = defineEmits<{ 'update:modelValue': [value: number | null] }>()
const clock = useTimestamp({ interval: 30000 })
const options = computed(() => buildLineOptions(props.groups, props.rates, props.metrics, props.linkedCounts ?? null, props.toolId, props.metricsGeneratedAt, Math.max(clock.value, Date.now()), props.metricsError))
const line = (value: unknown) => value as LineOption
const isRecommended = (option: LineOption) => {
  if (option.disabled || !props.recommendedGroupIds) return false
  if (props.toolId) return props.recommendedGroupIds.get(props.toolId) === option.value
  return toolIdsForGroup(option.group).some(toolId => props.recommendedGroupIds?.get(toolId) === option.value)
}
</script>

<style scoped>
.line-selected,.line-option-head{display:flex;align-items:center;gap:8px;min-width:0}
.line-selected img,.line-option-head img{width:20px;height:20px;object-fit:contain;flex:none}
.line-selected>span,.line-option-name strong{min-width:0;overflow-wrap:anywhere}
.line-selected small{margin-left:auto;color:var(--xq-secondary)}
.line-option{min-width:0;width:100%;display:grid;gap:7px;color:var(--xq-text)}
.line-option-name{display:flex;align-items:center;flex:1;flex-wrap:wrap;gap:4px 6px;min-width:0}
.line-option-name strong{font-size:14px;font-weight:600}
.line-recommended{flex:none;padding:2px 6px;border-radius:5px;background:rgba(97,201,217,.12);color:var(--xq-accent);font-size:11px;font-weight:600;line-height:1.35;white-space:nowrap}
.line-rate{flex:none;padding:3px 8px;border-radius:999px;background:rgba(97,201,217,.12);color:var(--xq-accent);font-size:11px;font-weight:600}
.line-status{display:inline-flex;align-items:center;gap:5px;flex:none;color:var(--xq-muted);font-size:11px}
.line-status i{width:6px;height:6px;border-radius:50%;background:var(--xq-muted)}
.line-status[data-status="success"] i{background:var(--xq-success)}
.line-status[data-status="warning"] i{background:var(--xq-warning)}
.line-status[data-status="danger"] i{background:var(--xq-danger)}
.line-status[data-status="success"]{color:var(--xq-success)}
.line-status[data-status="warning"]{color:var(--xq-warning)}
.line-status[data-status="danger"]{color:var(--xq-danger)}
.line-description{margin:0;font-size:12px;line-height:1.6;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--xq-secondary)}
.line-option-meta{display:flex;flex-wrap:wrap;gap:6px 16px;font-size:12px;line-height:1.5;color:var(--xq-secondary);font-variant-numeric:tabular-nums}
.line-no-data,.line-unavailable{color:var(--xq-muted)}
.line-success-rate{color:var(--xq-muted)}
.line-success-rate[data-tone="green"]{color:var(--xq-success)}
.line-success-rate[data-tone="amber"]{color:var(--xq-warning)}
.line-success-rate[data-tone="red"]{color:var(--xq-danger)}

:global(.xq-line-select-dropdown){
  width:480px;
  min-width:420px !important;
  max-width:min(480px, calc(100vw - 16px));
  display:flex;flex-direction:column;max-height:var(--select-available-height, 480px);
  padding:8px;
  border-radius:12px;
  border-color:var(--xq-border);
  background:var(--xq-depth);
  box-shadow:0 18px 45px rgba(0,0,0,.35);
}
:global(.xq-line-select-dropdown .select-search){
  flex:none;
  margin-bottom:8px;
  padding:10px 12px;
  border:1px solid var(--xq-border);
  border-radius:10px;
  background:var(--xq-surface);
}
:global(.xq-line-select-dropdown .select-options){max-height:480px;min-height:0;overflow-y:auto;padding:0;}
:global(.xq-line-select-dropdown .select-option){
  align-items:stretch;
  margin:4px 0;
  padding:12px 14px;
  border:1px solid transparent;
  border-radius:10px;
  background:var(--xq-surface);
}
:global(.xq-line-select-dropdown .select-option:hover),
:global(.xq-line-select-dropdown .select-option-focused){
  background:var(--xq-raised);
  border-color:var(--xq-accent);
}
:global(.xq-line-select-dropdown .select-option-selected){
  background:var(--xq-raised);
  border-color:var(--xq-accent);
}
:global(.xq-line-select-dropdown .select-option > svg){flex:none;margin-top:2px;color:var(--xq-accent)}
:global(.xq-line-select-dropdown .select-empty){padding:28px 16px;color:var(--xq-secondary)}
@media (max-width:700px){
  :global(.xq-line-select-dropdown){min-width:0 !important;width:calc(100vw - 32px);max-width:calc(100vw - 32px)}
  .line-option-head{gap:6px;flex-wrap:wrap}
  .line-option-name{flex-basis:calc(100% - 32px)}
  .line-status{margin-left:28px}
}
</style>
