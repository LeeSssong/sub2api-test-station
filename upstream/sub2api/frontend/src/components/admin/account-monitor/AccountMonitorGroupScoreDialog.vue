<template>
  <BaseDialog :show="show" :title="dialogTitle" width="normal" @close="emit('close')">
    <form class="space-y-4" @submit.prevent="save">
      <p class="text-sm text-gray-500 dark:text-slate-400">四项权重只影响监控展示排序，不改变生产调度。合计必须为 100。</p>
      <div class="grid grid-cols-2 gap-3">
        <label v-for="field in fields" :key="field.key" class="text-xs text-gray-500 dark:text-slate-400">
          {{ field.label }}
          <input v-model.number="draft[field.key]" type="number" min="0" step="1" class="input mt-1 w-full font-mono" :disabled="saving" />
        </label>
      </div>
      <div class="flex items-center justify-between text-sm">
        <span class="text-gray-500 dark:text-slate-400">当前合计</span>
        <span data-test="score-total" :class="total === 100 ? 'text-emerald-600' : 'text-amber-600'">{{ total }}</span>
      </div>
      <div v-if="mode === 'group'" class="border-t border-gray-100 pt-4 dark:border-slate-800">
        <p class="mb-3 text-sm font-medium text-gray-700 dark:text-slate-200">服务指标评分范围</p>
        <div class="grid grid-cols-2 gap-3">
          <label v-for="field in thresholdFields" :key="field.key" class="text-xs text-gray-500 dark:text-slate-400">
            {{ field.label }}
            <input v-model.number="draft[field.key]" type="number" min="0" step="100" class="input mt-1 w-full font-mono" :disabled="saving" />
          </label>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-slate-400">达到目标值得满分，超过上限得 0 分，中间线性递减。</p>
      </div>
      <div v-if="mode === 'group' && !thresholdsValid" class="text-sm text-amber-600 dark:text-amber-400">每项上限必须大于目标值。</div>
      <div v-if="error" class="text-sm text-red-600 dark:text-red-400">{{ error }}</div>
    </form>
    <template #footer>
      <div class="flex items-center justify-between gap-2">
        <button type="button" class="btn btn-secondary" data-test="reset-score-weights" :disabled="saving" @click="emit('reset')">恢复默认</button>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
          <button type="button" class="btn btn-primary" data-test="save-score-weights" :disabled="saving || total !== 100 || !thresholdsValid" @click="save">保存</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountMonitorFourScoreWeights, AccountMonitorScoreWeights } from '@/api/admin/accountMonitor'

type EditableWeights = Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency' | 'ttft_target_ms' | 'ttft_limit_ms' | 'latency_target_ms' | 'latency_limit_ms'>
const props = withDefaults(defineProps<{
  show: boolean
  mode?: 'group' | 'global'
  groupId: number
  groupName?: string
  weights: EditableWeights
  saving?: boolean
  error?: string | null
}>(), { mode: 'group', groupName: '', saving: false, error: null })
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', weights: EditableWeights | AccountMonitorFourScoreWeights): void
  (event: 'reset'): void
}>()

const fields: Array<{ key: keyof EditableWeights; label: string }> = [
  { key: 'cost', label: '成本优势' },
  { key: 'success', label: '成功率' },
  { key: 'ttft', label: 'TTFT' },
  { key: 'latency', label: '总耗时' },
]
const thresholdFields: Array<{ key: keyof EditableWeights; label: string }> = [
  { key: 'ttft_target_ms', label: 'TTFT 目标（毫秒）' },
  { key: 'ttft_limit_ms', label: 'TTFT 上限（毫秒）' },
  { key: 'latency_target_ms', label: '总耗时目标（毫秒）' },
  { key: 'latency_limit_ms', label: '总耗时上限（毫秒）' },
]
const withThresholdDefaults = (weights: EditableWeights): EditableWeights => ({
  ...weights,
  ttft_target_ms: weights.ttft_target_ms ?? 1000,
  ttft_limit_ms: weights.ttft_limit_ms ?? 5000,
  latency_target_ms: weights.latency_target_ms ?? 10000,
  latency_limit_ms: weights.latency_limit_ms ?? 60000,
})
const draft = reactive<EditableWeights>(withThresholdDefaults(props.weights))
watch(() => props.weights, (weights) => Object.assign(draft, withThresholdDefaults(weights)), { deep: true })
const dialogTitle = computed(() => props.mode === 'global'
  ? '全局评分规则'
  : `分组评分规则${props.groupName ? ` · ${props.groupName}` : ''}`)
const total = computed(() => fields.reduce((sum, field) => sum + Math.max(0, Number(draft[field.key]) || 0), 0))
const thresholdsValid = computed(() => props.mode === 'global' || (
  Number(draft.ttft_target_ms) >= 0 && Number(draft.ttft_limit_ms) > Number(draft.ttft_target_ms) &&
  Number(draft.latency_target_ms) >= 0 && Number(draft.latency_limit_ms) > Number(draft.latency_target_ms)
))
function save() {
  if (total.value !== 100 || props.saving) return
  if (!thresholdsValid.value) return
  if (props.mode === 'global') {
    emit('save', {
      cost: Number(draft.cost),
      success: Number(draft.success),
      ttft: Number(draft.ttft),
      latency: Number(draft.latency),
    })
    return
  }
  emit('save', {
    cost: Number(draft.cost), success: Number(draft.success), ttft: Number(draft.ttft), latency: Number(draft.latency),
    ttft_target_ms: Number(draft.ttft_target_ms), ttft_limit_ms: Number(draft.ttft_limit_ms),
    latency_target_ms: Number(draft.latency_target_ms), latency_limit_ms: Number(draft.latency_limit_ms),
  })
}
</script>
