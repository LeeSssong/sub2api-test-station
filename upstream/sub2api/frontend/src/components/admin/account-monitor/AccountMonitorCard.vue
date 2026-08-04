<template>
  <article class="overflow-hidden rounded-lg border border-l-4 border-gray-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-950" :class="statusBorderClass" data-test="monitor-card">
    <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-[18px] py-4 dark:border-slate-800 max-[430px]:px-[14px] max-[430px]:py-[14px]" :class="statusHeaderClass" data-test="monitor-card-header">
      <div class="min-w-0">
        <h2 class="break-words text-base font-semibold text-gray-900 dark:text-white [overflow-wrap:anywhere]" data-test="account-identity">
          {{ account.name }} <span class="font-mono text-xs font-normal text-gray-500 dark:text-slate-400">#{{ account.account_id }}</span>
        </h2>
      </div>
      <span class="shrink-0 rounded-full px-2 py-1 text-xs font-semibold" :class="statusBadgeClass" data-test="status-badge">
        {{ statusLabel }}
      </span>
    </header>

    <div class="px-[18px] pb-0 pt-4 max-[430px]:px-[14px]">
      <section class="grid grid-cols-3 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 divide-x divide-gray-200 dark:divide-slate-800 dark:border-slate-800 dark:bg-slate-900/50" aria-label="评分排名与优先级">
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="score-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ scoreTitle }}</div>
          <div class="mt-1 flex items-baseline gap-1.5"><strong class="font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ scoreLabel }}</strong><span class="text-xs font-semibold text-gray-500 dark:text-slate-400">/ 100</span></div>
          <p class="mt-2 text-[10px] text-gray-500 dark:text-slate-400">基于 {{ account.request_count }} 次真实请求</p>
        </div>
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="rank-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ rankTitle }}</div>
          <div class="mt-1 flex min-h-8 items-baseline gap-1.5"><strong class="truncate font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ rankLabel }}</strong><span v-if="account.group_rank != null" class="shrink-0 text-xs font-semibold text-gray-500 dark:text-slate-400">/ {{ rankedAccountCount }}</span></div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">正常可用账号继续参与排名</p>
        </div>
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="priority-control">
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

      <section class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-5" aria-label="账号服务指标">
        <MetricCell data-test="success-rate-metric" tone="success" label="成功率" :value="formatPercent(account.success_rate)" :detail="`${formatNumber(account.request_count)} 次真实请求，${formatNumber(account.error_count)} 次失败`" />
        <MetricCell data-test="ttft-metric" tone="ttft" label="TTFT P50" :value="formatMs(account.ttft_p50_ms)" :detail="sampleDetail(account.ttft_sample_count)" />
        <MetricCell data-test="latency-metric" tone="latency" label="总耗时 P95" :value="formatMs(account.latency_p95_ms)" :detail="sampleDetail(account.latency_sample_count)" />
        <div class="min-h-[116px] min-w-0 rounded-lg border border-violet-200 bg-violet-50 p-3 service-metric dark:border-violet-900/50 dark:bg-violet-950/20" data-test="cost-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">账号成本</div>
          <template v-if="editingMultiplier">
            <div class="mt-1 flex h-8 items-center gap-1">
              <label class="sr-only" :for="`account-multiplier-${account.account_id}`">账号倍率</label>
              <input
                :id="`account-multiplier-${account.account_id}`"
                ref="multiplierInput"
                v-model="draftMultiplier"
                class="input h-8 min-w-0 flex-1 px-2 py-1 font-mono text-sm"
                data-test="multiplier-input"
                inputmode="decimal"
                min="0"
                step="0.01"
                type="number"
                :disabled="savingMultiplier"
                @keyup.enter="saveMultiplier"
                @keyup.esc="cancelMultiplierEdit"
              >
              <button class="icon-button h-8 w-8 shrink-0" data-test="save-multiplier" type="button" title="保存账号倍率" aria-label="保存账号倍率" :disabled="savingMultiplier" @click="saveMultiplier"><Icon name="check" size="xs" /></button>
              <button class="icon-button h-8 w-8 shrink-0" data-test="cancel-multiplier" type="button" title="取消编辑账号倍率" aria-label="取消编辑账号倍率" :disabled="savingMultiplier" @click="cancelMultiplierEdit"><Icon name="x" size="xs" /></button>
            </div>
            <p v-if="multiplierError" class="mt-1 text-[11px] text-red-600 dark:text-red-400" data-test="multiplier-error" role="alert">{{ multiplierError }}</p>
          </template>
          <template v-else-if="editingCost">
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
            <p class="mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500" data-test="cost-detail">{{ costDetail }}</p>
            <div class="mt-2 flex items-center gap-1" data-test="cost-actions">
              <button class="icon-button h-8 w-8" data-test="edit-cost" type="button" :title="displayedProcurementCost == null ? '录入采购成本' : '编辑采购成本'" :aria-label="displayedProcurementCost == null ? '录入采购成本' : '编辑采购成本'" @click="beginCostEdit"><Icon name="edit" size="xs" /></button>
              <button v-if="displayedProcurementCost == null && manualMultiplierEditable" class="icon-button h-8 w-8" data-test="edit-multiplier" type="button" :title="manualMultiplierAvailable ? '编辑账号倍率' : '录入账号倍率'" :aria-label="manualMultiplierAvailable ? '编辑账号倍率' : '录入账号倍率'" @click="beginMultiplierEdit"><Icon name="edit" size="xs" /></button>
              <button v-if="displayedProcurementCost != null" class="icon-button h-8 w-8" data-test="clear-cost" type="button" title="清空采购成本" aria-label="清空采购成本" @click="confirmClearCost"><Icon name="trash" size="xs" /></button>
            </div>
          </template>
        </div>
        <div class="min-h-[116px] min-w-0 rounded-lg border border-gray-200 bg-gray-50 p-3 service-metric dark:border-slate-800 dark:bg-slate-900/50" data-test="concurrency-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">当前并发</div>
          <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ concurrencyValue }}</div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">{{ concurrency?.delayed ? '数据延迟' : '近实时运维快照' }}</p>
        </div>
      </section>

      <section class="mt-4 border-t border-gray-100 py-4 dark:border-slate-800" aria-label="近期探测" data-test="probe-section">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-800 dark:text-slate-100">近期探测</h3>
          <span class="text-[11px] text-gray-500 dark:text-slate-400" data-test="probe-summary">{{ probeSummary }}</span>
        </div>
        <div class="mt-3 grid h-9 grid-cols-[repeat(24,minmax(3px,1fr))] items-end gap-1" role="img" :aria-label="timelineAriaLabel">
          <span
            v-for="(bar, index) in probeBars"
            :key="`${account.account_id}-${index}`"
            class="min-w-0 rounded-sm transition-[height,background-color] duration-200 motion-reduce:transition-none"
            :class="bar.colorClass"
            :style="{ height: `${bar.height}%` }"
            :title="bar.title"
            data-test="probe-bar"
            aria-hidden="true"
          />
        </div>
        <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-slate-500"><span>较早</span><span>最近</span></div>
      </section>

      <section class="border-t border-gray-100 dark:border-slate-800" data-test="calls-disclosure">
        <button
          class="flex min-h-12 w-full items-center gap-2 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
          data-test="calls-toggle"
          type="button"
          :aria-controls="callsPanelID"
          :aria-expanded="callsExpanded"
          @click="callsExpanded = !callsExpanded"
        >
          <span class="font-semibold text-gray-800 dark:text-slate-100">{{ callsTitle }}</span>
          <span class="text-[11px] text-gray-500 dark:text-slate-400">{{ callsSummary }}</span>
          <Icon name="chevronDown" size="xs" class="ml-auto transition-transform motion-reduce:transition-none" :class="{ 'rotate-180': callsExpanded }" />
        </button>
        <div v-if="callsExpanded" :id="callsPanelID" class="grid grid-cols-2 gap-2 border-t border-gray-100 pb-3 pt-3 dark:border-slate-800">
          <div class="rounded-md bg-gray-50 p-2.5 dark:bg-slate-900/50"><div class="text-[10px] text-gray-500 dark:text-slate-400">成功请求</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ successfulRequestCount }}</div></div>
          <div class="rounded-md bg-gray-50 p-2.5 dark:bg-slate-900/50"><div class="text-[10px] text-gray-500 dark:text-slate-400">失败请求</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ account.error_count }}</div></div>
        </div>
      </section>

      <footer class="flex min-h-[52px] items-center justify-between gap-3 border-t border-gray-100 text-[11px] text-gray-500 dark:border-slate-800 dark:text-slate-400 max-[430px]:flex-col max-[430px]:items-start max-[430px]:gap-[3px] max-[430px]:py-[9px]" data-test="card-footer">
        <span>检查于 {{ checkedAtLabel }} · 统计截止 {{ statisticsCutoffLabel }}</span>
        <button class="icon-button shrink-0 max-[430px]:self-end" data-test="refresh-account" type="button" title="刷新当前账号" aria-label="刷新当前账号" :disabled="running" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" /></button>
      </footer>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { AccountMonitorAccount, AccountMonitorConcurrencyItem, AccountMonitorRange } from '@/api/admin/accountMonitor'

type SaveCompletion = {
  resolve: () => void
  reject: (reason?: unknown) => void
}

type CardConcurrency = AccountMonitorConcurrencyItem & { delayed?: boolean }
type ProbeBar = { colorClass: string, height: number, title: string }

function manualMultiplierFromAccount(account: AccountMonitorAccount): number | null {
  if (account.multiplier.source !== 'manual' || account.multiplier.value == null || !Number.isFinite(account.multiplier.value)) return null
  return account.multiplier.value
}

const props = withDefaults(defineProps<{
  account: AccountMonitorAccount
  concurrency?: CardConcurrency | null
  running?: boolean
  rankedAccountCount?: number
  rankingScope?: 'group' | 'global'
  statisticsCutoff?: string | null
  selectedRange?: AccountMonitorRange
}>(), { concurrency: null, running: false, rankedAccountCount: 0, rankingScope: 'group', statisticsCutoff: null, selectedRange: '24h' })

const emit = defineEmits<{
  (event: 'updatePriority', accountID: number, priority: number, completion: SaveCompletion): void
  (event: 'updateProcurementCost', accountID: number, cost: number | null, completion: SaveCompletion): void
  (event: 'updateMultiplier', accountID: number, multiplier: number, completion: SaveCompletion): void
  (event: 'refresh', accountID: number): void
}>()

const displayedPriority = ref(props.account.priority)
const displayedProcurementCost = ref<number | null>(props.account.procurement_cost_cny ?? null)
const displayedManualMultiplier = ref<number | null>(manualMultiplierFromAccount(props.account))
const editingPriority = ref(false)
const editingCost = ref(false)
const editingMultiplier = ref(false)
const savingPriority = ref(false)
const savingCost = ref(false)
const savingMultiplier = ref(false)
const draftPriority = ref(String(displayedPriority.value))
const draftCost = ref(displayedProcurementCost.value == null ? '' : String(displayedProcurementCost.value))
const draftMultiplier = ref(displayedManualMultiplier.value == null ? '' : String(displayedManualMultiplier.value))
const priorityError = ref('')
const costError = ref('')
const multiplierError = ref('')
const priorityInput = ref<HTMLInputElement | null>(null)
const costInput = ref<HTMLInputElement | null>(null)
const multiplierInput = ref<HTMLInputElement | null>(null)
const callsExpanded = ref(false)

watch(() => props.account.priority, (value) => {
  if (!editingPriority.value && !savingPriority.value) displayedPriority.value = value
})
watch(() => props.account.procurement_cost_cny, (value) => {
  if (!editingCost.value && !savingCost.value) displayedProcurementCost.value = value ?? null
})
watch(() => props.account.multiplier, (value) => {
  if (!editingMultiplier.value && !savingMultiplier.value) displayedManualMultiplier.value = value?.source === 'manual' ? value.value ?? null : null
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
const statusBorderClass = computed(() => ({
  'border-emerald-500': statusLabel.value === '正常',
  'border-amber-500': statusLabel.value === '待确认',
  'border-gray-300 dark:border-slate-700': statusLabel.value === '暂停',
  'border-red-500': statusLabel.value === '不可用',
}))
const statusHeaderClass = computed(() => ({
  'border-emerald-100 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20': statusLabel.value === '正常',
  'border-amber-100 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/20': statusLabel.value === '待确认',
  'bg-gray-50 dark:bg-slate-900/50': statusLabel.value === '暂停',
  'border-red-100 bg-red-50 dark:border-red-900/50 dark:bg-red-950/20': statusLabel.value === '不可用',
}))
const scoreLabel = computed(() => props.account.quality_score == null ? '--' : new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(props.account.quality_score))
const rankLabel = computed(() => props.account.group_rank == null ? '未排名' : `第 ${props.account.group_rank}`)
const scoreTitle = computed(() => props.rankingScope === 'global' ? '账号服务评分' : '账号分组评分')
const rankTitle = computed(() => props.rankingScope === 'global' ? '全站排名' : '组内排名')
const concurrencyValue = computed(() => props.concurrency ? `${props.concurrency.current} / ${props.concurrency.limit}` : '-- / --')
const callsPanelID = computed(() => `account-calls-${props.account.account_id}`)
const callsTitle = computed(() => ({ '24h': '24 小时调用', '7d': '7 天调用', '30d': '30 天调用' }[props.selectedRange]))
const callsSummary = computed(() => `${formatNumber(props.account.request_count)} 次请求 · ${formatNumber(props.account.error_count)} 次失败`)
const successfulRequestCount = computed(() => Math.max(0, Number(props.account.request_count) - Number(props.account.error_count)))
const checkedAtLabel = computed(() => formatDateTime(props.account.checked_at ?? props.account.latest?.checked_at ?? null))
const statisticsCutoffLabel = computed(() => formatShortTime(props.statisticsCutoff))
const timelinePoints = computed(() => (props.account.timeline ?? []).slice(-24))
const probeBars = computed<ProbeBar[]>(() => {
  const bars: ProbeBar[] = Array.from({ length: Math.max(0, 24 - timelinePoints.value.length) }, () => ({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: '暂无探测' }))
  for (const point of timelinePoints.value) {
    const timestamp = formatDateTime(point.checked_at)
    if (isCompletedProbe(point.status)) {
      const latency = point.latency_ms ?? point.ttft_ms
      bars.push({ colorClass: 'bg-emerald-500 dark:bg-emerald-400', height: point.status === 'unavailable' || point.status === 'model_unavailable' || point.status === 'degraded' ? 40 : latencyBarHeight(latency), title: `${timestamp} · ${latency == null ? '探测完成' : `成功 · ${formatMs(latency)}`}` })
    } else if (isFailedProbe(point.status)) {
      bars.push({ colorClass: 'bg-red-500 dark:bg-red-400', height: 28, title: `${timestamp} · 探测失败` })
    } else {
      bars.push({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: `${timestamp} · 暂无结果` })
    }
  }
  return bars
})
const probeSummary = computed(() => {
  const successes = timelinePoints.value.filter((point) => isCompletedProbe(point.status)).length
  const failures = timelinePoints.value.filter((point) => isFailedProbe(point.status)).length
  return `${timelinePoints.value.length} 次结果 · ${successes} 成功 · ${failures} 失败`
})
const timelineAriaLabel = computed(() => `近期 ${probeSummary.value}探测`)
const manualMultiplierAvailable = computed(() => displayedManualMultiplier.value != null && Number.isFinite(displayedManualMultiplier.value))
const nativeMultiplierAvailable = computed(() => {
  const multiplier = props.account.multiplier
  return multiplier.source !== 'manual' && multiplier.status === 'ok' && multiplier.value != null && Number.isFinite(multiplier.value)
})
const manualMultiplierEditable = computed(() => !nativeMultiplierAvailable.value)
const costValue = computed(() => {
  if (displayedProcurementCost.value != null) return `¥${displayedProcurementCost.value.toFixed(2)}`
  if (manualMultiplierAvailable.value) return formatMultiplier(displayedManualMultiplier.value)
  if (nativeMultiplierAvailable.value) return formatMultiplier(props.account.multiplier.value)
  return '--'
})
const costDetail = computed(() => {
  if (displayedProcurementCost.value == null) {
    if (manualMultiplierAvailable.value) return '手工录入倍率'
    if (nativeMultiplierAvailable.value) return '上游托管倍率'
    return '未录入账号倍率'
  }
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
	return `${Math.round(value)} ms`
}
function formatMultiplier(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return `${value.toFixed(2)}×`
}
function formatDate(value: string): string {
  return value.slice(0, 10)
}
function formatDateTime(value?: string | null): string {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}
function formatShortTime(value?: string | null): string {
	if (!value) return '--'
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return '--'
	return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}
function formatNumber(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(Math.max(0, Number(value) || 0))
}
function sampleDetail(count: number): string {
  return `基于 ${formatNumber(count)} 次有效响应`
}
function isCompletedProbe(status: string): boolean {
  return ['success', 'operational', 'ok', 'unavailable', 'model_unavailable', 'degraded'].includes(status)
}
function isFailedProbe(status: string): boolean {
  return ['failed', 'error'].includes(status)
}
function latencyBarHeight(value?: number | null): number {
  if (value == null || !Number.isFinite(value)) return 65
  const ratio = Math.log10(Math.max(100, value) / 100)
  return Math.round(Math.max(35, Math.min(100, 100 - ratio * 32.5)))
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
function waitForMultiplierSave(accountID: number, multiplier: number): Promise<void> {
  return new Promise((resolve, reject) => emit('updateMultiplier', accountID, multiplier, { resolve, reject }))
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
  let shouldRefocus = false
  try {
    await waitForPrioritySave(props.account.account_id, priority)
    displayedPriority.value = priority
    editingPriority.value = false
  } catch (reason) {
    priorityError.value = errorMessage(reason, '保存全局优先级失败')
    shouldRefocus = true
  } finally {
    savingPriority.value = false
    if (shouldRefocus) {
      await nextTick()
      priorityInput.value?.focus()
    }
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

async function beginMultiplierEdit() {
  draftMultiplier.value = manualMultiplierAvailable.value ? String(displayedManualMultiplier.value) : ''
  multiplierError.value = ''
  editingMultiplier.value = true
  await nextTick()
  multiplierInput.value?.focus()
  multiplierInput.value?.select()
}
function cancelMultiplierEdit() {
  if (savingMultiplier.value) return
  draftMultiplier.value = manualMultiplierAvailable.value ? String(displayedManualMultiplier.value) : ''
  multiplierError.value = ''
  editingMultiplier.value = false
}
async function saveMultiplier() {
  const multiplier = Number(draftMultiplier.value)
  if (!String(draftMultiplier.value).trim() || !Number.isFinite(multiplier) || multiplier < 0) {
    multiplierError.value = '请输入大于或等于 0 的账号倍率'
    await nextTick()
    multiplierInput.value?.focus()
    return
  }
  savingMultiplier.value = true
  multiplierError.value = ''
  let shouldRefocus = false
  try {
    await waitForMultiplierSave(props.account.account_id, multiplier)
    displayedManualMultiplier.value = multiplier
    editingMultiplier.value = false
  } catch (reason) {
    multiplierError.value = errorMessage(reason, '保存账号倍率失败')
    shouldRefocus = true
  } finally {
    savingMultiplier.value = false
    if (shouldRefocus) {
      await nextTick()
      multiplierInput.value?.focus()
    }
  }
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
  let shouldRefocus = false
  try {
    await waitForCostSave(props.account.account_id, cost)
    displayedProcurementCost.value = cost
    editingCost.value = false
  } catch (reason) {
    costError.value = errorMessage(reason, '保存采购成本失败')
    shouldRefocus = true
  } finally {
    savingCost.value = false
    if (shouldRefocus) {
      await nextTick()
      costInput.value?.focus()
    }
  }
}
async function confirmClearCost() {
  if (!window.confirm('确认清空采购成本并恢复倍率模式？')) return
  savingCost.value = true
  costError.value = ''
  let shouldRefocus = false
  try {
    await waitForCostSave(props.account.account_id, null)
    displayedProcurementCost.value = null
  } catch (reason) {
    costError.value = errorMessage(reason, '清空采购成本失败')
    editingCost.value = true
    shouldRefocus = true
  } finally {
    savingCost.value = false
    if (shouldRefocus) {
      await nextTick()
      costInput.value?.focus()
    }
  }
}

const MetricCell = defineComponent({
  name: 'AccountMonitorMetricCell',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    detail: { type: String, required: true },
    tone: { type: String, required: true },
  },
  setup(metricProps, { attrs }) {
    const toneClass: Record<string, string> = {
      success: 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20',
      ttft: 'border-blue-200 bg-blue-50 dark:border-blue-900/50 dark:bg-blue-950/20',
      latency: 'border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/20',
    }
    return () => h('div', { ...attrs, class: ['min-h-[116px] min-w-0 rounded-lg border p-3 service-metric', toneClass[metricProps.tone], attrs.class] }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 whitespace-nowrap font-mono text-lg font-semibold text-gray-900 dark:text-white', 'data-test': `${metricProps.tone}-metric-value` }, metricProps.value),
      h('p', { class: 'mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500' }, metricProps.detail),
    ])
  },
})
</script>
