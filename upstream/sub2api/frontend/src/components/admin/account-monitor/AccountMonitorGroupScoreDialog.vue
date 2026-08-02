<template>
  <BaseDialog :show="show" :title="`分组评分规则${groupName ? ` · ${groupName}` : ''}`" width="normal" @close="emit('close')">
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
      <div v-if="error" class="text-sm text-red-600 dark:text-red-400">{{ error }}</div>
    </form>
    <template #footer>
      <div class="flex items-center justify-between gap-2">
        <button type="button" class="btn btn-secondary" data-test="reset-score-weights" :disabled="saving" @click="emit('reset')">恢复默认</button>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" @click="emit('close')">取消</button>
          <button type="button" class="btn btn-primary" data-test="save-score-weights" :disabled="saving || total !== 100" @click="save">保存</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountMonitorScoreWeights } from '@/api/admin/accountMonitor'

type EditableWeights = Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency'>
const props = withDefaults(defineProps<{
  show: boolean
  groupId: number
  groupName?: string
  weights: EditableWeights
  saving?: boolean
  error?: string | null
}>(), { groupName: '', saving: false, error: null })
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', weights: EditableWeights): void
  (event: 'reset'): void
}>()

const fields: Array<{ key: keyof EditableWeights; label: string }> = [
  { key: 'cost', label: '成本优势' },
  { key: 'success', label: '成功率' },
  { key: 'ttft', label: 'TTFT' },
  { key: 'latency', label: '总耗时' },
]
const draft = reactive<EditableWeights>({ ...props.weights })
watch(() => props.weights, (weights) => Object.assign(draft, weights), { deep: true })
const total = computed(() => fields.reduce((sum, field) => sum + Math.max(0, Number(draft[field.key]) || 0), 0))
function save() {
  if (total.value !== 100 || props.saving) return
  emit('save', { cost: Number(draft.cost), success: Number(draft.success), ttft: Number(draft.ttft), latency: Number(draft.latency) })
}
</script>
