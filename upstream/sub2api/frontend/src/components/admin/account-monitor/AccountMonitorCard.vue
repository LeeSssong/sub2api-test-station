<template>
  <article
    class="card flex min-h-[360px] flex-col overflow-hidden border-l-4 bg-white p-4 shadow-sm transition-colors dark:border-slate-700 dark:bg-slate-950/95 dark:shadow-black/20"
    :class="statusBorderClass"
    data-test="monitor-card"
  >
    <header class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <a v-if="account.homepage_url" :href="account.homepage_url" target="_blank" rel="noopener noreferrer" class="truncate text-base font-semibold text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300">
            {{ account.name }}
          </a>
          <h2 v-else class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ account.name }}</h2>
          <span class="font-mono text-xs text-gray-500 dark:text-slate-400">#{{ account.account_id }}</span>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-slate-800 dark:text-slate-300">{{ account.platform }}</span>
        </div>
        <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-slate-400">
          <span v-if="account.group_names.length">{{ account.group_names.join(', ') }}</span>
          <span v-else>{{ t('admin.accountMonitor.card.noGroups') }}</span>
          <span>{{ account.model_id }}</span>
        </div>
      </div>
      <span class="rounded-full px-2 py-1 text-xs font-semibold" :class="statusBadgeClass">
        {{ statusLabel }}
      </span>
    </header>

    <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
      <Metric data-test="success-rate-metric" :label="t('admin.accountMonitor.metrics.successRate')" :value="formatPercent(metricEvidence.successRate)" :evidence="probeSampleLabel(metricEvidence.successSamples)" />
      <Metric data-test="ttft-metric" :label="t('admin.accountMonitor.metrics.ttft')" :value="formatMs(metricEvidence.ttft)" :evidence="probeSampleLabel(metricEvidence.ttftSamples)" />
      <Metric data-test="latency-metric" :label="t('admin.accountMonitor.metrics.latency')" :value="formatMs(metricEvidence.latency)" :evidence="probeSampleLabel(metricEvidence.latencySamples)" />
      <Metric data-test="multiplier-metric" :label="t('admin.accountMonitor.metrics.multiplier')" :value="multiplierValue" :hint="multiplierHint" :evidence="callSampleLabel(account.multiplier.sample_count)" />
    </div>

    <section class="mt-4 border-y border-gray-100 py-3 dark:border-slate-800" aria-label="近期探测结果">
      <div class="flex items-center justify-between gap-3 text-xs">
        <span class="font-medium text-gray-700 dark:text-slate-200">近期探测</span>
        <span class="text-gray-400 dark:text-slate-500">{{ account.timeline?.length ?? 0 }} 次结果</span>
      </div>
      <div class="mt-2 flex h-8 items-end gap-1" role="img" :aria-label="timelineAriaLabel">
        <span
          v-for="(bar, index) in probeBars"
          :key="`${account.account_id}-${index}`"
          data-test="probe-bar"
          class="min-w-1 flex-1 rounded-sm transition-[height,background-color] duration-200 motion-reduce:transition-none"
          :class="bar.colorClass"
          :style="{ height: `${bar.height}%` }"
          :title="bar.title"
          aria-hidden="true"
        />
      </div>
      <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-slate-500"><span>较早</span><span>最近</span></div>
    </section>

    <section v-if="scope === 'group'" class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-slate-400" data-test="quality-summary">
      <span v-if="account.quality_score != null" class="font-medium text-gray-700 dark:text-slate-200">组内评分 {{ formatScore(account.quality_score) }}</span>
      <span v-else>暂无组内评分</span>
      <span v-if="account.group_rank != null">组内第 {{ account.group_rank }}</span>
      <span v-else>暂无组内排名</span>
      <span v-if="account.evidence?.source === 'global_fallback'">本组数据不足，参考近期探测</span>
      <span v-else-if="account.evidence?.source === 'stale'">近期探测数据已过期</span>
    </section>

    <section v-if="operations" class="mt-3 rounded-lg border border-gray-200 p-3 dark:border-slate-800" data-test="economics-summary">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-slate-400">投入产出</span>
        <span v-if="!operations.coverage_known || Number(operations.pending_attempts ?? 0) > 0" class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">待对账</span>
      </div>
      <div class="grid grid-cols-2 gap-2 text-xs">
        <LedgerMetric label="上游真实扣费" :value="formatMoney(operations.upstream_cost, operations.currency)" />
        <LedgerMetric label="用户实际计费" :value="formatMoney(operations.user_charge, operations.currency)" />
        <LedgerMetric label="账号利润" :value="formatMoney(operations.paper_profit, operations.currency)" />
        <LedgerMetric label="利润率" :value="profitMarginLabel" />
      </div>
    </section>

    <div class="mt-2 flex min-h-7 items-center gap-1 text-xs text-gray-400 dark:text-slate-500">
      <template v-if="editingPriority">
        <label class="sr-only" :for="`account-priority-${account.account_id}`">调度优先级</label>
        <input ref="priorityInput" :id="`account-priority-${account.account_id}`" v-model.number="draftPriority" data-test="priority-input" type="number" min="0" step="1" class="input h-7 w-20 px-2 py-1 font-mono text-xs" :disabled="savingWeight" @keyup.enter="savePriority" @keyup.esc="cancelPriorityEdit" />
        <button type="button" class="rounded p-1 text-emerald-600 hover:bg-emerald-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 dark:text-emerald-400 dark:hover:bg-emerald-950/30" data-test="save-priority" title="保存调度优先级" aria-label="保存调度优先级" :disabled="savingWeight" @click="savePriority"><Icon name="check" size="xs" /></button>
        <button type="button" class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-400 dark:hover:bg-slate-800 dark:hover:text-slate-200" data-test="cancel-priority" title="取消编辑" aria-label="取消编辑" :disabled="savingWeight" @click="cancelPriorityEdit"><Icon name="x" size="xs" /></button>
      </template>
      <template v-else>
        <span>调度优先级 {{ account.priority ?? 0 }}</span>
        <button type="button" class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-slate-800 dark:hover:text-slate-200" data-test="edit-priority" title="编辑调度优先级" aria-label="编辑调度优先级" :disabled="savingWeight" @click="beginPriorityEdit"><Icon name="edit" size="xs" /></button>
      </template>
      <span v-if="savingWeight">保存中...</span>
    </div>

    <div class="mt-2 border-t border-gray-100 dark:border-slate-800">
      <button type="button" class="flex w-full items-center gap-2 py-2 text-left text-xs text-gray-500 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:text-slate-400 dark:hover:text-slate-100" data-test="today-toggle" :aria-expanded="todayExpanded" :aria-controls="todayPanelID" @click="todayExpanded = !todayExpanded">
        <span class="font-medium text-gray-700 dark:text-slate-200">{{ t('admin.accountMonitor.today.title') }}</span>
        <span class="min-w-0 flex-1 truncate">{{ todaySummary }}</span>
        <Icon name="chevronDown" size="xs" class="transition-transform motion-reduce:transition-none" :class="{ 'rotate-180': todayExpanded }" />
      </button>
      <div v-if="todayExpanded" :id="todayPanelID" class="grid grid-cols-1 gap-3 border-t border-gray-100 py-3 dark:border-slate-800 sm:grid-cols-2">
        <div>
          <div class="text-[11px] text-gray-400 dark:text-slate-500">{{ t('admin.accountMonitor.today.title') }}</div>
          <AccountTodayStatsCell class="mt-1" :stats="account.today_stats ?? null" />
        </div>
        <div>
          <div class="text-[11px] text-gray-400 dark:text-slate-500">{{ t('admin.accountMonitor.card.usageWindows') }}</div>
          <AccountUsageCell class="mt-1" :account="usageAccount" :today-stats="account.today_stats ?? null" />
        </div>
      </div>
    </div>

    <div class="mt-auto flex items-center justify-between gap-3 pt-3">
      <div class="min-w-0 text-xs text-gray-500 dark:text-slate-400">
        <span v-if="account.checked_at">{{ t('admin.accountMonitor.card.checkedAt', { time: formatDate(account.checked_at) }) }}</span>
        <span v-else>{{ t('admin.accountMonitor.status.noHistory') }}</span>
        <span v-if="account.error_code" class="ml-2 text-red-600 dark:text-red-400">{{ account.error_code }}</span>
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <button type="button" class="icon-button" :title="t('admin.accountMonitor.actions.settings')" :aria-label="t('admin.accountMonitor.actions.settings')" @click="emit('settings')"><Icon name="cog" size="sm" /></button>
        <button type="button" class="icon-button" :disabled="running" :title="t('admin.accountMonitor.actions.refreshOne')" :aria-label="t('admin.accountMonitor.actions.refreshOne')" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" /></button>
        <button type="button" class="icon-button" :title="t('admin.accountMonitor.actions.history')" :aria-label="t('admin.accountMonitor.actions.history')" @click="emit('history', account.account_id)"><Icon name="clock" size="sm" /></button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import type { AccountMonitorAccount } from '@/api/admin/accountMonitor'
import type { ReconciliationSummary } from '@/api/admin/reconciliation'
import type { Account } from '@/types'

const props = withDefaults(defineProps<{
  account: AccountMonitorAccount
  operations?: ReconciliationSummary | null
  scope?: 'all' | 'group'
  groupOperationalState?: string
  running?: boolean
  savingWeight?: boolean
}>(), { operations: null, scope: 'group', groupOperationalState: 'operational', running: false, savingWeight: false })

const emit = defineEmits<{
  (event: 'refresh', accountID: number): void
  (event: 'settings'): void
  (event: 'history', accountID: number): void
  (event: 'updatePriority', accountID: number, priority: number): void
}>()

const { t } = useI18n()
const draftPriority = ref(props.account.priority ?? 0)
const editingPriority = ref(false)
const priorityInput = ref<HTMLInputElement | null>(null)
const todayExpanded = ref(false)
const todayPanelID = computed(() => `account-today-${props.account.account_id}`)
watch(() => props.account.priority, (value) => { draftPriority.value = value ?? 0 })

async function beginPriorityEdit() {
  draftPriority.value = props.account.priority ?? 0
  editingPriority.value = true
  await nextTick()
  priorityInput.value?.focus()
  priorityInput.value?.select()
}
function cancelPriorityEdit() {
  draftPriority.value = props.account.priority ?? 0
  editingPriority.value = false
}
function savePriority() {
  const value = Math.max(0, Math.round(Number(draftPriority.value) || 0))
  draftPriority.value = value
  editingPriority.value = false
  if (value !== props.account.priority) {
    emit('updatePriority', props.account.account_id, value)
  }
}

const todaySummary = computed(() => {
  const requests = Number(props.account.today_stats?.requests ?? props.account.request_count ?? 0)
  const errors = Number(props.account.error_count ?? 0)
  return `${new Intl.NumberFormat().format(requests)} 次请求${errors > 0 ? ` · ${new Intl.NumberFormat().format(errors)} 次错误` : ''}`
})

const usageAccount = computed(() => ({
  id: props.account.account_id,
  name: props.account.name,
  platform: props.account.platform,
  type: props.account.account_type,
  status: props.account.status,
  schedulable: props.account.schedulable,
  credentials: {},
  credentials_status: {},
} as Account))

const status = computed(() => {
  if (props.account.management_state === 'paused') return 'paused'
  if (props.groupOperationalState === 'closed') return 'closed'
  if (props.account.service_state === 'available') return 'success'
  if (props.account.service_state === 'unavailable') return 'failed'
  if (props.account.service_state === 'pending') return 'pending'
  return 'unavailable'
})

const statusLabel = computed(() => status.value === 'closed' ? '已关闭' : t(`admin.accountMonitor.status.${status.value}`))
const metricEvidence = computed(() => {
  const evidence = props.scope === 'group' ? props.account.evidence : undefined
  return {
    successRate: evidence ? evidence.success_rate : props.account.success_rate,
    successSamples: evidence ? evidence.success_sample_count : props.account.success_sample_count,
    ttft: evidence ? evidence.ttft_p50_ms : props.account.ttft_p50_ms,
    ttftSamples: evidence ? evidence.ttft_sample_count : props.account.ttft_sample_count,
    latency: evidence ? evidence.latency_p95_ms : props.account.latency_p95_ms,
    latencySamples: evidence ? evidence.latency_sample_count : props.account.latency_sample_count,
  }
})
const multiplierValue = computed(() => {
  const multiplier = props.account.multiplier
  if (multiplier.status === 'ok' && typeof multiplier.value === 'number' && Number.isFinite(multiplier.value)) {
    return `${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(multiplier.value)}x`
  }
  const knownStatus = ['stale', 'unsupported', 'failed', 'unavailable'].includes(multiplier.status) ? multiplier.status : 'unavailable'
  return t(`admin.accountMonitor.multiplier.${knownStatus}`)
})
const multiplierHint = computed(() => multiplierStatusHint(props.account.multiplier.source, props.account.multiplier.status, t))
const statusBadgeClass = computed(() => ({
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300': status.value === 'success',
  'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300': status.value === 'failed',
  'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300': status.value === 'pending',
  'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300': status.value === 'unavailable' || status.value === 'closed' || status.value === 'paused',
}))
const statusBorderClass = computed(() => ({
  'border-emerald-500': status.value === 'success',
  'border-red-500': status.value === 'failed',
  'border-amber-500': status.value === 'pending',
  'border-slate-300 dark:border-slate-700': status.value === 'unavailable' || status.value === 'closed' || status.value === 'paused',
}))
const profitMarginLabel = computed(() => {
  if (!props.operations?.coverage_known || Number(props.operations?.pending_attempts ?? 0) > 0) return '待对账'
  const margin = Number(props.operations?.profit_margin)
  return Number.isFinite(margin) ? `${(margin * 100).toFixed(1)}%` : '—'
})

interface ProbeBar {
  colorClass: string
  height: number
  title: string
}

const probeBars = computed<ProbeBar[]>(() => {
  const points = (props.account.timeline ?? []).slice(-24)
  const bars: ProbeBar[] = Array.from({ length: 24 - points.length }, () => ({
    colorClass: 'bg-gray-200 dark:bg-slate-700',
    height: 15,
    title: '暂无探测',
  }))
  for (const point of points) {
    const timestamp = formatDate(point.checked_at)
    if (point.status === 'success' || point.status === 'operational' || point.status === 'ok') {
      const latency = point.latency_ms ?? point.ttft_ms
      bars.push({
        colorClass: 'bg-emerald-500 dark:bg-emerald-400',
        height: latencyBarHeight(latency),
        title: `${timestamp} · 成功${latency == null ? '' : ` · ${formatMs(latency)}`}`,
      })
    } else if (point.status === 'unavailable' || point.status === 'model_unavailable' || point.status === 'degraded') {
      // A channel probe that cannot produce a model response still completed
      // its reachability check; keep it green but visibly shorter than a
      // latency-backed success.
      bars.push({
        colorClass: 'bg-emerald-500 dark:bg-emerald-400',
        height: 40,
        title: `${timestamp} · 探测完成（无可用模型）`,
      })
    } else if (point.status === 'failed' || point.status === 'error') {
      bars.push({
        colorClass: 'bg-red-500 dark:bg-red-400',
        height: 28,
        title: `${timestamp} · 失败${point.error_code ? ` · ${point.error_code}` : ''}`,
      })
    } else {
      bars.push({
        colorClass: 'bg-gray-200 dark:bg-slate-700',
        height: 15,
        title: `${timestamp} · 暂无结果`,
      })
    }
  }
  return bars
})

const timelineAriaLabel = computed(() => {
  const points = props.account.timeline ?? []
  if (!points.length) return '近期暂无探测结果'
  const successes = points.filter((point) => ['success', 'operational', 'ok', 'unavailable', 'model_unavailable', 'degraded'].includes(point.status)).length
  const failures = points.filter((point) => ['failed', 'error'].includes(point.status)).length
  return `近期 ${points.length} 次探测，成功 ${successes} 次，失败 ${failures} 次`
})

function latencyBarHeight(value?: number | null): number {
  if (value == null || !Number.isFinite(value)) return 65
  const ratio = Math.log10(Math.max(100, value) / 100)
  return Math.round(Math.max(35, Math.min(100, 100 - ratio * 32.5)))
}

function multiplierStatusHint(source: string | undefined, statusValue: string, translate: (key: string) => string): string {
  if (statusValue !== 'ok') return ''
  if (source === 'declared') return translate('admin.accountMonitor.multiplier.declared')
  if (source === 'measured') return translate('admin.accountMonitor.multiplier.measured')
  return ''
}
function probeSampleLabel(value?: number | null): string { return `基于 ${Math.max(0, Number(value) || 0)} 次探测` }
function callSampleLabel(value?: number | null): string { return `基于 ${Math.max(0, Number(value) || 0)} 次调用` }
function formatPercent(value: number): string { return `${Math.round(value * 100)}%` }
function formatScore(value: number): string { return String(Math.round(value)) }
function formatMs(value?: number | null): string { return value == null ? '-' : `${Math.round(value)} ms` }
function formatDate(value: string): string { return new Date(value).toLocaleString() }
function formatMoney(value: string | number | null | undefined, currency?: string | null): string {
  const amount = Number(value)
  if (!Number.isFinite(amount)) return '—'
  try {
    if (currency) {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
    }
    return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
  } catch {
    return amount.toFixed(2)
  }
}

const Metric = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true }, hint: { type: String, default: '' }, evidence: { type: String, default: '' } },
  setup(metricProps) {
    return () => h('div', { class: 'rounded bg-gray-50 p-2 dark:bg-slate-900/70' }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white' }, metricProps.value),
      metricProps.hint ? h('div', { class: 'mt-0.5 text-[10px] font-medium text-gray-400 dark:text-slate-500' }, metricProps.hint) : null,
      metricProps.evidence ? h('div', { class: 'mt-0.5 text-[10px] text-gray-400 dark:text-slate-500' }, metricProps.evidence) : null,
    ])
  },
})

const LedgerMetric = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(metricProps) {
    return () => h('div', { class: 'rounded bg-gray-50 p-2 dark:bg-slate-900/70' }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white' }, metricProps.value),
    ])
  },
})
</script>
