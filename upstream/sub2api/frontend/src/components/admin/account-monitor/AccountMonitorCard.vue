<template>
  <article class="card overflow-hidden border border-gray-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-950" data-test="monitor-card">
    <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-slate-800">
      <div class="min-w-0">
        <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
          {{ account.name }} <span class="font-mono text-xs font-normal text-gray-500 dark:text-slate-400">#{{ account.account_id }}</span>
        </h2>
      </div>
      <span class="shrink-0 rounded-full px-2 py-1 text-xs font-semibold" :class="statusBadgeClass" data-test="status-badge">
        {{ statusLabel }}
      </span>
    </header>

    <div class="p-4">
      <section class="grid grid-cols-3 divide-x divide-gray-100 border border-gray-100 dark:divide-slate-800 dark:border-slate-800" aria-label="评分排名与优先级">
        <div class="min-w-0 p-3" data-test="score-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">账号分组评分</div>
          <div class="mt-1 font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ scoreLabel }}</div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">/ 100</p>
        </div>
        <div class="min-w-0 p-3" data-test="rank-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">组内排名</div>
          <div class="mt-1 truncate font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ rankLabel }}</div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">正常可用账号参与排名</p>
        </div>
        <div class="min-w-0 p-3" data-test="priority-control">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">全局优先级</div>
          <template v-if="editingPriority">
            <div class="mt-1 flex h-8 items-center gap-1">
              <label class="sr-only" :for="`account-priority-${account.account_id}`">全局优先级</label>
              <input
                :id="`account-priority-${account.account_id}`"
                ref="priorityInput"
                v-model="draftPriority"
                class="input h-8 min-w-0 flex-1 px-2 py-1 font-mono text-sm"
                data-test="priority-input"
                inputmode="numeric"
                min="1"
                step="1"
                type="number"
                :disabled="savingPriority"
                @keyup.enter="savePriority"
                @keyup.esc="cancelPriorityEdit"
              >
              <button class="icon-button h-8 w-8 shrink-0" data-test="save-priority" type="button" title="保存全局优先级" aria-label="保存全局优先级" :disabled="savingPriority" @click="savePriority"><Icon name="check" size="xs" /></button>
              <button class="icon-button h-8 w-8 shrink-0" data-test="cancel-priority" type="button" title="取消编辑全局优先级" aria-label="取消编辑全局优先级" :disabled="savingPriority" @click="cancelPriorityEdit"><Icon name="x" size="xs" /></button>
            </div>
            <p v-if="priorityError" class="mt-1 text-[11px] text-red-600 dark:text-red-400" data-test="priority-error" role="alert">{{ priorityError }}</p>
          </template>
          <template v-else>
            <div class="mt-1 flex h-8 items-center gap-1">
              <strong class="font-mono text-2xl text-gray-900 dark:text-white">{{ displayedPriority }}</strong>
              <button class="icon-button h-8 w-8 shrink-0" data-test="edit-priority" type="button" title="编辑全局优先级" aria-label="编辑全局优先级" @click="beginPriorityEdit"><Icon name="edit" size="xs" /></button>
            </div>
            <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">数值越小，调度优先级越高</p>
          </template>
        </div>
      </section>

      <section class="mt-4 grid grid-cols-1 gap-px overflow-hidden border border-gray-100 bg-gray-100 sm:grid-cols-2 xl:grid-cols-5 dark:border-slate-800 dark:bg-slate-800" aria-label="账号服务指标">
        <MetricCell data-test="success-rate-metric" label="成功率" :value="formatPercent(account.success_rate)" :detail="`${account.request_count} 次真实请求，${account.error_count} 次失败`" />
        <MetricCell data-test="ttft-metric" label="TTFT P50" :value="formatMs(account.ttft_p50_ms)" :detail="sampleDetail(account.ttft_sample_count)" />
        <MetricCell data-test="latency-metric" label="总耗时 P95" :value="formatMs(account.latency_p95_ms)" :detail="sampleDetail(account.latency_sample_count)" />
        <div class="min-w-0 bg-white p-3 dark:bg-slate-950" data-test="cost-metric">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[11px] text-gray-500 dark:text-slate-400">账号成本</span>
            <div v-if="!editingCost" class="flex shrink-0 items-center gap-1">
              <button class="icon-button h-8 w-8" data-test="edit-cost" type="button" :title="displayedProcurementCost == null ? '录入采购成本' : '编辑采购成本'" :aria-label="displayedProcurementCost == null ? '录入采购成本' : '编辑采购成本'" @click="beginCostEdit"><Icon name="edit" size="xs" /></button>
              <button v-if="displayedProcurementCost != null" class="icon-button h-8 w-8" data-test="clear-cost" type="button" title="清空采购成本" aria-label="清空采购成本" @click="confirmClearCost"><Icon name="trash" size="xs" /></button>
            </div>
          </div>
          <template v-if="editingCost">
            <div class="mt-1 flex h-8 items-center gap-1">
              <label class="sr-only" :for="`account-cost-${account.account_id}`">采购成本（人民币）</label>
              <input
                :id="`account-cost-${account.account_id}`"
                ref="costInput"
                v-model="draftCost"
                class="input h-8 min-w-0 flex-1 px-2 py-1 font-mono text-sm"
                data-test="cost-input"
                inputmode="decimal"
                min="0"
                step="0.01"
                type="number"
                :disabled="savingCost"
                @keyup.enter="saveCost"
                @keyup.esc="cancelCostEdit"
              >
              <button class="icon-button h-8 w-8 shrink-0" data-test="save-cost" type="button" title="保存采购成本" aria-label="保存采购成本" :disabled="savingCost" @click="saveCost"><Icon name="check" size="xs" /></button>
              <button class="icon-button h-8 w-8 shrink-0" data-test="cancel-cost" type="button" title="取消编辑采购成本" aria-label="取消编辑采购成本" :disabled="savingCost" @click="cancelCostEdit"><Icon name="x" size="xs" /></button>
            </div>
            <p v-if="costError" class="mt-1 text-[11px] text-red-600 dark:text-red-400" data-test="cost-error" role="alert">{{ costError }}</p>
          </template>
          <template v-else>
            <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ costValue }}</div>
            <p class="mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500">{{ costDetail }}</p>
          </template>
        </div>
        <div class="min-w-0 bg-white p-3 dark:bg-slate-950" data-test="concurrency-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">当前并发</div>
          <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ concurrencyValue }}</div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">{{ concurrency?.delayed ? '数据延迟' : '近实时运维快照' }}</p>
        </div>
      </section>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { AccountMonitorAccount, AccountMonitorConcurrencyItem } from '@/api/admin/accountMonitor'

type SaveCompletion = {
  resolve: () => void
  reject: (reason?: unknown) => void
}

type CardConcurrency = AccountMonitorConcurrencyItem & { delayed?: boolean }

const props = withDefaults(defineProps<{
  account: AccountMonitorAccount
  concurrency?: CardConcurrency | null
  operations?: unknown
  scope?: 'all' | 'group'
  groupOperationalState?: string
  running?: boolean
  savingWeight?: boolean
}>(), { concurrency: null })

const emit = defineEmits<{
  (event: 'updatePriority', accountID: number, priority: number, completion: SaveCompletion): void
  (event: 'updateProcurementCost', accountID: number, cost: number | null, completion: SaveCompletion): void
  (event: 'refresh', accountID: number): void
  (event: 'settings'): void
  (event: 'history', accountID: number): void
}>()

const displayedPriority = ref(props.account.priority)
const displayedProcurementCost = ref<number | null>(props.account.procurement_cost_cny ?? null)
const editingPriority = ref(false)
const editingCost = ref(false)
const savingPriority = ref(false)
const savingCost = ref(false)
const draftPriority = ref(String(displayedPriority.value))
const draftCost = ref(displayedProcurementCost.value == null ? '' : String(displayedProcurementCost.value))
const priorityError = ref('')
const costError = ref('')
const priorityInput = ref<HTMLInputElement | null>(null)
const costInput = ref<HTMLInputElement | null>(null)

watch(() => props.account.priority, (value) => {
  if (!editingPriority.value && !savingPriority.value) displayedPriority.value = value
})
watch(() => props.account.procurement_cost_cny, (value) => {
  if (!editingCost.value && !savingCost.value) displayedProcurementCost.value = value ?? null
})

const statusLabel = computed(() => {
  if (props.account.management_state === 'paused') return '暂停'
  if (props.account.service_state === 'available') return '正常'
  if (props.account.service_state === 'pending') return '待确认'
  return '不可用'
})
const statusBadgeClass = computed(() => ({
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300': statusLabel.value === '正常',
  'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300': statusLabel.value === '待确认',
  'bg-gray-100 text-gray-600 dark:bg-slate-800 dark:text-slate-300': statusLabel.value === '暂停',
  'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300': statusLabel.value === '不可用',
}))
const scoreLabel = computed(() => props.account.quality_score == null ? '--' : new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(props.account.quality_score))
const rankLabel = computed(() => props.account.group_rank == null ? '未排名' : `第 ${props.account.group_rank}`)
const concurrencyValue = computed(() => props.concurrency ? `${props.concurrency.current} / ${props.concurrency.limit}` : '-- / --')
const costValue = computed(() => displayedProcurementCost.value == null
  ? formatMultiplier(props.account.multiplier.value)
  : `¥${displayedProcurementCost.value.toFixed(2)}`)
const costDetail = computed(() => {
  if (displayedProcurementCost.value == null) return '账号倍率模式'
  if (!props.account.expires_at) return '有效期缺失，无法计算等效倍率'
  const effective = props.account.effective_multiplier
  if (effective == null || !Number.isFinite(effective)) return '缺少窗口基础成本，无法计算等效倍率'
  return `有效至 ${formatDate(props.account.expires_at)} · 当前窗口等效倍率 ${formatMultiplier(effective)}`
})

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '--'
  return new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 }).format(value)
}
function formatMs(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value)} ms`
}
function formatMultiplier(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return `${value.toFixed(2)}x`
}
function formatDate(value: string): string {
  return value.slice(0, 10)
}
function sampleDetail(count: number): string {
  return `基于 ${count} 次有效响应`
}
function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Error && reason.message ? reason.message : fallback
}
function waitForPrioritySave(accountID: number, priority: number): Promise<void> {
  return new Promise((resolve, reject) => emit('updatePriority', accountID, priority, { resolve, reject }))
}
function waitForCostSave(accountID: number, cost: number | null): Promise<void> {
  return new Promise((resolve, reject) => emit('updateProcurementCost', accountID, cost, { resolve, reject }))
}
async function beginPriorityEdit() {
  draftPriority.value = String(displayedPriority.value)
  priorityError.value = ''
  editingPriority.value = true
  await nextTick()
  priorityInput.value?.focus()
  priorityInput.value?.select()
}
function cancelPriorityEdit() {
  if (savingPriority.value) return
  draftPriority.value = String(displayedPriority.value)
  priorityError.value = ''
  editingPriority.value = false
}
async function savePriority() {
  const priority = Number(draftPriority.value)
  if (!Number.isInteger(priority) || priority < 1) {
    priorityError.value = '请输入大于或等于 1 的整数'
    await nextTick()
    priorityInput.value?.focus()
    return
  }
  if (priority === displayedPriority.value) {
    editingPriority.value = false
    return
  }
  savingPriority.value = true
  priorityError.value = ''
  try {
    await waitForPrioritySave(props.account.account_id, priority)
    displayedPriority.value = priority
    editingPriority.value = false
  } catch (reason) {
    priorityError.value = errorMessage(reason, '保存全局优先级失败')
    await nextTick()
    priorityInput.value?.focus()
  } finally {
    savingPriority.value = false
  }
}
async function beginCostEdit() {
  draftCost.value = displayedProcurementCost.value == null ? '' : String(displayedProcurementCost.value)
  costError.value = ''
  editingCost.value = true
  await nextTick()
  costInput.value?.focus()
  costInput.value?.select()
}
function cancelCostEdit() {
  if (savingCost.value) return
  draftCost.value = displayedProcurementCost.value == null ? '' : String(displayedProcurementCost.value)
  costError.value = ''
  editingCost.value = false
}
async function saveCost() {
  const cost = Number(draftCost.value)
  if (!String(draftCost.value).trim() || !Number.isFinite(cost) || cost < 0) {
    costError.value = '请输入大于或等于 0 的采购成本'
    await nextTick()
    costInput.value?.focus()
    return
  }
  savingCost.value = true
  costError.value = ''
  try {
    await waitForCostSave(props.account.account_id, cost)
    displayedProcurementCost.value = cost
    editingCost.value = false
  } catch (reason) {
    costError.value = errorMessage(reason, '保存采购成本失败')
    await nextTick()
    costInput.value?.focus()
  } finally {
    savingCost.value = false
  }
}
async function confirmClearCost() {
  if (!window.confirm('确认清空采购成本并恢复倍率模式？')) return
  savingCost.value = true
  costError.value = ''
  try {
    await waitForCostSave(props.account.account_id, null)
    displayedProcurementCost.value = null
  } catch (reason) {
    costError.value = errorMessage(reason, '清空采购成本失败')
    editingCost.value = true
    await nextTick()
    costInput.value?.focus()
  } finally {
    savingCost.value = false
  }
}

const MetricCell = defineComponent({
  name: 'AccountMonitorMetricCell',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    detail: { type: String, required: true },
  },
  setup(metricProps, { attrs }) {
    return () => h('div', { ...attrs, class: ['min-w-0 bg-white p-3 dark:bg-slate-950', attrs.class] }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white' }, metricProps.value),
      h('p', { class: 'mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500' }, metricProps.detail),
    ])
  },
})
</script>
