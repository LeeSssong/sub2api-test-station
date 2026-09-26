<template>
  <Select
    class="xq-line-select"
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
          <strong>{{ line(option).label }}</strong>
          <span>{{ line(option).rateLabel }}</span>
        </div>
        <small>{{ line(option).statusLabel }} · 关联密钥 {{ line(option).linkedCount == null ? '暂不可用' : `${line(option).linkedCount} 把` }}</small>
        <small>近 1 小时稳定性 <span class="line-success-rate" :data-tone="line(option).successTone">{{ line(option).successLabel }}</span> · 首字 {{ line(option).ttftLabel }}</small>
      </div>
    </template>
  </Select>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select from '@/components/common/Select.vue'
import { providerIcon } from '@/features/ai-tools/model'
import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import { buildLineOptions, type LineOption } from './lineOptions'

const props = withDefaults(defineProps<{
  modelValue: number | null
  groups: Group[]
  rates: Record<number, number>
  metrics: Map<number, MonitorV4Group>
  linkedCounts: Map<number, number> | null
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
const options = computed(() => buildLineOptions(props.groups, props.rates, props.metrics, props.linkedCounts, props.toolId))
const line = (value: unknown) => value as LineOption
</script>

<style scoped>
.line-selected,.line-option-head{display:flex;align-items:center;gap:8px;min-width:0}
.line-selected img,.line-option-head img{width:20px;height:20px;object-fit:contain;flex:none}
.line-selected>span,.line-option-head strong{min-width:0;overflow-wrap:anywhere}
.line-selected small{margin-left:auto;color:var(--xq-secondary)}
.line-option{min-width:0;width:100%;display:grid;gap:5px;color:var(--xq-text)}
.line-option-head strong{flex:1;font-size:14px;font-weight:600}
.line-option-head>span{flex:none;font-size:11px;color:var(--xq-accent)}
.line-option small{font-size:11px;color:var(--xq-secondary);white-space:normal}
.line-success-rate{color:var(--xq-muted)}
.line-success-rate[data-tone="green"]{color:var(--xq-success)}
.line-success-rate[data-tone="amber"]{color:var(--xq-warning)}
.line-success-rate[data-tone="red"]{color:var(--xq-danger)}
</style>
