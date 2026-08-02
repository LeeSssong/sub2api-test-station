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

    <section v-if="scope === 'group'" class="mt-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-slate-800 dark:bg-slate-900/80" data-test="quality-summary">
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
  if (props.account.management_state === 'paused') return 'paused'
  if (props.groupOperationalState === 'closed') return 'closed'
  if (props.account.service_state === 'available') return 'success'
  if (props.account.service_state === 'unavailable') return 'failed'
  if (props.account.service_state === 'pending') return 'pending'
  return 'unavailable'
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

function multiplierStatusHint(source: string | undefined, statusValue: string, translate: (key: string) => string): string {
  if (statusValue !== 'ok') return ''
  if (source === 'declared') return translate('admin.accountMonitor.multiplier.declared')
  if (source === 'measured') return translate('admin.accountMonitor.multiplier.measured')
  return ''
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
</script>
