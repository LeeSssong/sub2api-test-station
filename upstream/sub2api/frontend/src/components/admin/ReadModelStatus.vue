<template>
  <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400" data-test="read-model-status">
    <span :class="degraded ? 'text-amber-600 dark:text-amber-300' : 'text-emerald-600 dark:text-emerald-300'">
      {{ degraded ? '控制面暂时不可用' : `来源：${sourceLabel}` }}
    </span>
    <span v-if="generatedAt">更新于 {{ formatTime(generatedAt) }}</span>
    <span>完整性：{{ completeness }}</span>
    <span v-if="calculationVersion !== 'unknown'">计算版本：{{ calculationVersion }}</span>
    <button v-if="degraded" type="button" class="underline underline-offset-2" @click="$emit('retry')">重试</button>
  </div>
</template>

<script setup lang="ts">
defineProps<{ generatedAt?: string; completeness?: string; calculationVersion?: string; degraded?: boolean; sourceLabel?: string }>()
defineEmits<{ retry: [] }>()
const formatTime = (value: string) => {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}
</script>

