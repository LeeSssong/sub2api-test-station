<template>
  <article
    class="card flex min-h-[330px] flex-col overflow-hidden border-l-4 bg-white p-4 shadow-sm transition-colors dark:border-slate-700 dark:bg-slate-950/95 dark:shadow-black/20"
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
      <Metric :label="t('admin.accountMonitor.metrics.successRate')" :value="formatPercent(account.success_rate)" />
      <Metric :label="t('admin.accountMonitor.metrics.ttft')" :value="formatMs(account.ttft_p50_ms)" />
      <Metric :label="t('admin.accountMonitor.metrics.latency')" :value="formatMs(account.latency_p95_ms)" />
      <Metric data-test="multiplier-metric" :label="t('admin.accountMonitor.metrics.multiplier')" :value="multiplierValue" :hint="multiplierHint" />
    </div>

    <section class="mt-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-slate-800 dark:bg-slate-900/80" data-test="quality-summary">
      <div class="flex items-center justify-between gap-2">
        <span class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-slate-400">质量评分</span>
        <span v-if="account.quality_score != null" class="font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ formatScore(account.quality_score) }}</span>
        <span v-else class="text-sm text-gray-400 dark:text-slate-500">—</span>
      </div>
      <div class="mt-1 flex items-center justify-between text-xs text-gray-500 dark:text-slate-400">
        <span v-if="account.group_rank != null">组内第 {{ account.group_rank }}</span>
        <span v-else>暂无组内排名</span>
        <span v-if="account.evidence?.source === 'global_fallback'">全局样本回退</span>
        <span v-else-if="account.evidence?.source === 'stale'">证据过期</span>
        <span v-else-if="account.evidence?.sample_count != null">{{ account.evidence.sample_count }} 个样本</span>
      </div>
    </section>

    <section v-if="costGuard" class="mt-3 space-y-2" data-test="cost-guard-summary">
      <div
        v-if="showCostAlert"
        class="rounded-lg border p-3"
        :class="costAlertClass"
        data-test="cost-inversion-alert"
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="text-sm font-semibold">{{ costAlertTitle }}</p>
            <p v-if="costAlertDescription" class="mt-1 text-xs leading-5">{{ costAlertDescription }}</p>
          </div>
          <span class="shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="costStatusBadgeClass">
            {{ costStatusLabel }}
          </span>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 p-3 dark:border-slate-800">
        <div class="mb-2 flex items-center justify-between gap-2">
          <span class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-slate-400">{{ t('admin.accountMonitor.costGuard.title') }}</span>
          <span v-if="!showCostAlert" class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="costStatusBadgeClass" data-test="cost-status-badge">
            {{ costStatusLabel }}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
          <CostMetric :label="t('admin.accountMonitor.costGuard.upstreamMultiplier')" :value="formatMultiplier(costGuard.upstream_multiplier)" />
          <CostMetric :label="t('admin.accountMonitor.costGuard.multiplierSource')" :value="upstreamMultiplierSourceLabel" />
          <CostMetric :label="t('admin.accountMonitor.costGuard.equivalentSiteMultiplier')" :value="formatMultiplier(costGuard.equivalent_site_multiplier)" />
          <CostMetric :label="t('admin.accountMonitor.costGuard.costSource')" :value="costSourceLabel" />
          <CostMetric :label="t('admin.accountMonitor.costGuard.groupMultiplier')" :value="formatMultiplier(costGuard.group_multiplier)" />
          <CostMetric :label="t('admin.accountMonitor.costGuard.status')" :value="costStatusLabel" />
        </div>
        <div class="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-gray-500 dark:text-slate-400" data-test="cost-evidence-meta">
          <span v-if="costGuard.model">{{ t('admin.accountMonitor.costGuard.model') }}：<span class="font-mono">{{ costGuard.model }}</span></span>
          <span v-if="costGuard.sample_count != null">{{ t('admin.accountMonitor.costGuard.samples') }}：{{ costGuard.sample_count }}/{{ requiredSampleCount }}</span>
          <span v-if="costGuard.observed_at">{{ t('admin.accountMonitor.costGuard.observedAt') }}：{{ formatDate(costGuard.observed_at) }}</span>
        </div>
      </div>
    </section>

    <section v-if="operations" class="mt-3 rounded-lg border border-gray-200 p-3 dark:border-slate-800" data-test="economics-summary">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-slate-400">投入产出</span>
        <span v-if="!operations.coverage_known || Number(operations.pending_attempts ?? 0) > 0" class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">待对账</span>
      </div>
      <div class="grid grid-cols-2 gap-2 text-xs">
        <LedgerMetric label="上游真实扣费" :value="formatMoney(operations.upstream_cost, operations.currency)" />
        <LedgerMetric label="用户实际计费" :value="formatMoney(operations.user_charge, operations.currency)" />
        <LedgerMetric label="纸面利润" :value="formatMoney(operations.paper_profit, operations.currency)" />
        <LedgerMetric label="利润率" :value="profitMarginLabel" />
      </div>
    </section>

    <div class="mt-3 flex items-center gap-2 text-xs text-gray-500 dark:text-slate-400">
      <label :for="`account-priority-${account.account_id}`">全局调度优先级</label>
      <input :id="`account-priority-${account.account_id}`" v-model.number="draftPriority" type="number" min="0" step="1" class="input h-8 w-20 px-2 py-1 font-mono" :disabled="savingWeight" @change="savePriority" />
      <span v-if="savingWeight">保存中...</span>
    </div>

    <div class="mt-4 grid grid-cols-2 gap-3 border-y border-gray-100 py-3 dark:border-slate-800">
      <div>
        <div class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-slate-500">{{ t('admin.accountMonitor.today.title') }}</div>
        <AccountTodayStatsCell class="mt-1" :stats="account.today_stats ?? null" />
      </div>
      <div>
        <div class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-slate-500">{{ t('admin.accountMonitor.card.usageWindows') }}</div>
        <AccountUsageCell class="mt-1" :account="usageAccount" :today-stats="account.today_stats ?? null" />
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
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import type { AccountMonitorAccount, AccountMonitorCostGuard } from '@/api/admin/accountMonitor'
import type { ReconciliationSummary } from '@/api/admin/reconciliation'
import type { Account } from '@/types'

const props = withDefaults(defineProps<{
  account: AccountMonitorAccount
  operations?: ReconciliationSummary | null
  groupOperationalState?: string
  running?: boolean
  savingWeight?: boolean
}>(), { operations: null, groupOperationalState: 'operational', running: false, savingWeight: false })

const emit = defineEmits<{
  (event: 'refresh', accountID: number): void
  (event: 'settings'): void
  (event: 'history', accountID: number): void
  (event: 'updatePriority', accountID: number, priority: number): void
}>()

const { t } = useI18n()
const draftPriority = ref(props.account.priority ?? 0)
watch(() => props.account.priority, (value) => { draftPriority.value = value ?? 0 })
function savePriority() {
  const value = Math.max(0, Math.round(Number(draftPriority.value) || 0))
  draftPriority.value = value
  if (value !== props.account.priority) {
    emit('updatePriority', props.account.account_id, value)
  }
}

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
  if (props.groupOperationalState === 'closed') return 'closed'
  if (!props.account.checked_at && !props.account.latest) return 'unavailable'
  if (props.account.error_code === 'balance_exhausted') return 'balance_exhausted'
  if (props.account.stale || props.account.evidence?.source === 'stale') return 'stale'
  if (props.account.eligible === false || props.account.latest_status !== 'success') return 'failed'
  return props.account.latest_status || 'unavailable'
})

const statusLabel = computed(() => status.value === 'closed' ? '已关闭' : t(`admin.accountMonitor.status.${status.value}`))
const multiplierValue = computed(() => {
  const multiplier = props.account.multiplier
  if (multiplier.status === 'ok' && typeof multiplier.value === 'number' && Number.isFinite(multiplier.value)) {
    return `${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(multiplier.value)}x`
  }
  const knownStatus = ['stale', 'unsupported', 'failed', 'unavailable'].includes(multiplier.status) ? multiplier.status : 'unavailable'
  return t(`admin.accountMonitor.multiplier.${knownStatus}`)
})
const multiplierHint = computed(() => multiplierStatusHint(props.account.multiplier.source, props.account.multiplier.status, t))
const costGuard = computed<AccountMonitorCostGuard | null>(() => props.account.cost_guard ?? null)
const requiredSampleCount = computed(() => {
  const value = Number(costGuard.value?.required_sample_count)
  return Number.isInteger(value) && value > 0 ? value : 6
})
const costStatusLabel = computed(() => {
  const statusValue = costGuard.value?.status ?? 'unknown'
  if (statusValue === 'loss_observing') {
    return t('admin.accountMonitor.costGuard.statuses.lossObserving', {
      count: costGuard.value?.sample_count ?? 0,
      required: requiredSampleCount.value,
    })
  }
  return t(`admin.accountMonitor.costGuard.statuses.${costStatusKey(statusValue)}`)
})
const upstreamMultiplierSourceLabel = computed(() => t(`admin.accountMonitor.costGuard.multiplierSources.${multiplierSourceKey(costGuard.value?.upstream_multiplier_source)}`))
const costSourceLabel = computed(() => t(`admin.accountMonitor.costGuard.costSources.${costSourceKey(costGuard.value?.cost_source)}`))
const showCostAlert = computed(() => ['loss_confirmed', 'loss_observing', 'pricing_risk', 'loss_risk', 'zero_margin'].includes(costGuard.value?.status ?? ''))
const costAlertTitle = computed(() => {
  const modelSuffix = costGuard.value?.model ? ` · ${costGuard.value.model}` : ''
  const statusValue = costGuard.value?.status
  if (statusValue === 'loss_confirmed') return `${t('admin.accountMonitor.costGuard.alerts.inversion')}${modelSuffix}`
  if (statusValue === 'loss_observing') return `${costStatusLabel.value}${modelSuffix}`
  if (statusValue === 'pricing_risk' || statusValue === 'loss_risk') return `${t('admin.accountMonitor.costGuard.statuses.pricingRisk')}${modelSuffix}`
  return `${t('admin.accountMonitor.costGuard.statuses.zeroMargin')}${modelSuffix}`
})
const costAlertDescription = computed(() => {
  const equivalent = finiteNumber(costGuard.value?.equivalent_site_multiplier)
  const group = finiteNumber(costGuard.value?.group_multiplier)
  if (equivalent == null || group == null) return ''
  const comparison = equivalent > group ? '>' : Math.abs(equivalent - group) <= 1e-9 ? '=' : '<'
  const gap = finiteNumber(costGuard.value?.gap) ?? equivalent - group
  const base = `${t('admin.accountMonitor.costGuard.equivalentSiteMultiplier')} ${formatMultiplier(equivalent)} ${comparison} ${t('admin.accountMonitor.costGuard.groupMultiplier')} ${formatMultiplier(group)}`
  if (costGuard.value?.status === 'loss_confirmed') {
    return `${base}，${t('admin.accountMonitor.costGuard.alerts.aboveBy', { gap: formatMultiplier(Math.max(gap, 0)) })}`
  }
  if (costGuard.value?.status === 'loss_observing') {
    return `${base}；${t('admin.accountMonitor.costGuard.alerts.observingEvidence', { count: costGuard.value.sample_count ?? 0, required: requiredSampleCount.value })}`
  }
  if (costGuard.value?.status === 'pricing_risk' || costGuard.value?.status === 'loss_risk') {
    return `${base}；${t('admin.accountMonitor.costGuard.alerts.pricingEvidence')}`
  }
  return `${base}；${t('admin.accountMonitor.costGuard.alerts.zeroMargin')}`
})
const costAlertClass = computed(() => {
  if (costGuard.value?.status === 'loss_confirmed') return 'border-red-300 bg-red-50 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200'
  if (costGuard.value?.status === 'loss_observing') return 'border-orange-300 bg-orange-50 text-orange-800 dark:border-orange-900/60 dark:bg-orange-950/30 dark:text-orange-200'
  return 'border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200'
})
const costStatusBadgeClass = computed(() => {
  const statusValue = costGuard.value?.status
  if (statusValue === 'loss_confirmed') return 'bg-red-600 text-white dark:bg-red-500 dark:text-white'
  if (statusValue === 'loss_observing' || statusValue === 'pricing_risk' || statusValue === 'loss_risk') return 'bg-orange-100 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300'
  if (statusValue === 'zero_margin') return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
  if (statusValue === 'cost_covered') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
})
const statusBadgeClass = computed(() => ({
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300': status.value === 'success',
  'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300': status.value === 'failed',
  'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300': status.value === 'balance_exhausted',
  'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300': status.value === 'stale',
  'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300': status.value === 'unavailable' || status.value === 'closed',
}))
const statusBorderClass = computed(() => ({
  'border-emerald-500': status.value === 'success',
  'border-red-500': status.value === 'failed',
  'border-orange-500': status.value === 'balance_exhausted',
  'border-amber-500': status.value === 'stale',
  'border-slate-300 dark:border-slate-700': status.value === 'unavailable' || status.value === 'closed',
}))
const profitMarginLabel = computed(() => {
  if (!props.operations?.coverage_known || Number(props.operations?.pending_attempts ?? 0) > 0) return '待对账'
  const margin = Number(props.operations?.profit_margin)
  return Number.isFinite(margin) ? `${(margin * 100).toFixed(1)}%` : '—'
})

function multiplierStatusHint(source: string | undefined, statusValue: string, translate: (key: string) => string): string {
  if (statusValue !== 'ok') return ''
  if (source === 'declared') return translate('admin.accountMonitor.multiplier.declared')
  if (source === 'measured') return translate('admin.accountMonitor.multiplier.measured')
  return ''
}
function multiplierSourceKey(source?: string | null): string {
  if (source === 'upstream_declared' || source === 'declared') return 'upstreamDeclared'
  if (source === 'upstream_pricing') return 'upstreamPricing'
  if (source === 'manual' || source === 'manual_config') return 'manual'
  return 'unknown'
}
function costSourceKey(source?: string | null): string {
  if (source === 'reconciled_bill') return 'reconciledBill'
  if (source === 'upstream_pricing' || source === 'pricing_estimate') return 'upstreamPricing'
  if (source === 'quota_measurement' || source === 'measured') return 'quotaMeasurement'
  return 'unknown'
}
function costStatusKey(statusValue: string): string {
  if (statusValue === 'loss_confirmed') return 'lossConfirmed'
  if (statusValue === 'pricing_risk' || statusValue === 'loss_risk') return 'pricingRisk'
  if (statusValue === 'zero_margin') return 'zeroMargin'
  if (statusValue === 'cost_covered') return 'costCovered'
  if (statusValue === 'insufficient_samples') return 'insufficientSamples'
  return 'unknown'
}
function finiteNumber(value?: number | null): number | null {
  const numeric = Number(value)
  return value != null && Number.isFinite(numeric) ? numeric : null
}
function formatMultiplier(value?: number | null): string {
  const numeric = finiteNumber(value)
  if (numeric == null) return '—'
  return `${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(numeric)}x`
}
function formatPercent(value: number): string { return `${Math.round(value * 100)}%` }
function formatScore(value: number): string { return value.toFixed(1).replace(/\.0$/, '') }
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
  props: { label: { type: String, required: true }, value: { type: String, required: true }, hint: { type: String, default: '' } },
  setup(metricProps) {
    return () => h('div', { class: 'rounded bg-gray-50 p-2 dark:bg-slate-900/70' }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white' }, metricProps.value),
      metricProps.hint ? h('div', { class: 'mt-0.5 text-[10px] font-medium text-gray-400 dark:text-slate-500' }, metricProps.hint) : null,
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

const CostMetric = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(metricProps) {
    return () => h('div', { class: 'rounded bg-gray-50 p-2 dark:bg-slate-900/70' }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 break-words font-mono text-xs font-semibold text-gray-900 dark:text-white' }, metricProps.value),
    ])
  },
})
</script>
