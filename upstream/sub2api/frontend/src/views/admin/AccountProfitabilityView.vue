<template>
  <AppLayout>
    <div class="min-h-full bg-[#f4f7f9] px-5 py-8 dark:bg-slate-950 max-sm:px-3 max-sm:pt-5 sm:py-9">
      <div class="mx-auto flex w-full max-w-[1400px] flex-col gap-5" data-test="account-profitability-page">
        <header class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
              <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />
              {{ t('admin.accountProfitability.eyebrow') }}
            </div>
            <h1 class="text-[27px] font-semibold leading-tight text-gray-900 dark:text-white max-[430px]:text-[23px]">
              {{ t('admin.accountProfitability.title') }}
            </h1>
            <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-gray-400">
              {{ t('admin.accountProfitability.description') }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-secondary inline-flex shrink-0 items-center gap-2"
            data-test="export-csv"
            :disabled="loading || !filteredRows.length"
            @click="exportCsv"
          >
            <Icon name="download" size="sm" />
            <span>{{ t('admin.accountProfitability.export') }}</span>
          </button>
        </header>

        <section class="flex flex-col gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-5" aria-label="filters">
          <div class="flex flex-wrap items-center gap-2" role="group" :aria-label="t('admin.accountProfitability.dateRange')">
            <button
              v-for="range in ranges"
              :key="range.value"
              type="button"
              class="rounded-lg px-3 py-2 text-sm font-semibold transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
              :class="activeRange === range.value ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
              :data-test="`range-${range.value}`"
              :aria-pressed="activeRange === range.value"
              @click="selectRange(range.value)"
            >
              {{ t(`admin.accountProfitability.ranges.${range.value}`) }}
            </button>
            <span class="mx-1 hidden h-6 w-px bg-gray-200 dark:bg-dark-600 sm:block" />
            <label class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
              <span class="sr-only">{{ t('admin.accountProfitability.startDate') }}</span>
              <input v-model="startDate" type="date" class="input h-9 w-[138px] text-sm" data-test="start-date" @change="selectRange('custom')" />
              <span>—</span>
              <span class="sr-only">{{ t('admin.accountProfitability.endDate') }}</span>
              <input v-model="endDate" type="date" class="input h-9 w-[138px] text-sm" data-test="end-date" @change="selectRange('custom')" />
            </label>
          </div>
          <div class="grid gap-3 lg:grid-cols-[minmax(240px,1fr)_180px_180px_auto] lg:items-center">
            <label class="relative block">
              <span class="sr-only">{{ t('admin.accountProfitability.search') }}</span>
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="search" type="search" class="input h-10 w-full pl-9" :placeholder="t('admin.accountProfitability.searchPlaceholder')" data-test="search" />
            </label>
            <label class="block">
              <span class="sr-only">{{ t('admin.accountProfitability.source') }}</span>
              <select v-model="sourceFilter" class="input h-10 w-full" data-test="source-filter">
                <option value="all">{{ t('admin.accountProfitability.filters.allSources') }}</option>
                <option value="sub2api">{{ t('admin.accountProfitability.sources.sub2api') }}</option>
                <option value="newapi">{{ t('admin.accountProfitability.sources.newapi') }}</option>
                <option value="self_purchased">{{ t('admin.accountProfitability.sources.self_purchased') }}</option>
                <option value="pending">{{ t('admin.accountProfitability.sources.pending') }}</option>
              </select>
            </label>
            <label class="block">
              <span class="sr-only">{{ t('admin.accountProfitability.status') }}</span>
              <select v-model="statusFilter" class="input h-10 w-full" data-test="status-filter">
                <option value="all">{{ t('admin.accountProfitability.filters.allStatuses') }}</option>
                <option value="known">{{ t('admin.accountProfitability.statuses.known') }}</option>
                <option value="pending">{{ t('admin.accountProfitability.statuses.pending') }}</option>
              </select>
            </label>
            <div class="text-right text-xs text-gray-500 dark:text-gray-400 lg:whitespace-nowrap">
              {{ dateLabel }} · {{ t('admin.accountProfitability.accountCount', { count: filteredRows.length }) }}
            </div>
          </div>
        </section>

        <ReadModelStatus
          v-if="readMode !== 'legacy_only'"
          :generated-at="readModel.generatedAt.value"
          :completeness="readModel.completeness.value"
          :calculation-version="readModel.calculationVersion.value"
          :degraded="controlPlaneDegraded || readModel.degraded.value"
          :source-label="usingControlPlane ? '控制面' : '现有系统'"
          @retry="load"
        />

        <div v-if="error" class="flex items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300" role="alert" data-test="load-error">
          <span>{{ error }}</span>
          <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" @click="load">{{ t('common.refresh') }}</button>
        </div>

        <section class="grid grid-cols-2 gap-3 lg:grid-cols-5" aria-label="summary">
          <div v-for="card in summaryCards" :key="card.key" class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800" :data-test="`summary-${card.key}`">
            <div class="flex items-start justify-between gap-2">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</span>
              <span class="h-2 w-2 rounded-full" :class="card.dot" />
            </div>
            <div class="mt-2 font-mono text-xl font-semibold tracking-tight" :class="card.valueClass">{{ card.value }}</div>
          </div>
        </section>

        <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div v-if="loading" class="flex items-center justify-center px-4 py-16 text-sm text-gray-500 dark:text-gray-400" data-test="loading">{{ t('common.loading') }}</div>
          <div v-else-if="!filteredRows.length" class="px-4 py-16 text-center text-sm text-gray-500 dark:text-gray-400" data-test="empty">{{ t('admin.accountProfitability.empty') }}</div>
          <template v-else>
            <div class="hidden overflow-x-auto lg:block">
              <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
                <thead class="bg-gray-50/80 dark:bg-dark-800/80">
                  <tr>
                    <th v-for="column in columns" :key="column.key" class="px-5 py-3.5 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ column.label }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="row in filteredRows" :key="row.account_id" :data-test="`account-row-${row.account_id}`" class="transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/30">
                    <td class="whitespace-nowrap px-5 py-4">
                      <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
                      <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">#{{ row.account_id }} · {{ row.platform }}</div>
                    </td>
                    <td class="px-5 py-4"><SourceBadge :source="row.source" /></td>
                    <td class="px-5 py-4"><StatusBadge :status="row.expense_status" /></td>
                    <td class="px-5 py-4 text-right font-mono text-sm text-gray-700 dark:text-gray-200" :data-test="row.expense_status === 'pending' ? 'pending-expense' : undefined">{{ formatMoney(expenseValue(row), row.expense_status, row.expense_currency) }}</td>
                    <td class="px-5 py-4 text-right font-mono text-sm text-gray-700 dark:text-gray-200">{{ formatMoney(row.revenue) }}</td>
                    <td class="px-5 py-4 text-right font-mono text-sm" :class="numberClass(row.profit)">{{ formatProfit(row.profit, row.expense_currency) }}</td>
                    <td class="px-5 py-4 text-right font-mono text-sm" :class="numberClass(row.margin)">{{ formatProfit(row.margin, row.expense_currency, true) }}</td>
                    <td class="px-5 py-4 text-right font-mono text-xs text-gray-500 dark:text-gray-400">{{ formatNumber(row.request_count) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-700 lg:hidden">
              <article v-for="row in filteredRows" :key="row.account_id" :data-test="`mobile-account-row-${row.account_id}`" class="space-y-3 p-4">
                <div class="flex items-start justify-between gap-3">
                  <div><div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div><div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">#{{ row.account_id }} · {{ row.platform }}</div></div>
                  <SourceBadge :source="row.source" />
                </div>
                <div class="grid grid-cols-2 gap-3 text-sm">
                  <div><span class="block text-xs text-gray-500">{{ t('admin.accountProfitability.columns.expense') }}</span><span class="font-mono">{{ formatMoney(expenseValue(row), row.expense_status, row.expense_currency) }}</span></div>
                  <div><span class="block text-xs text-gray-500">{{ t('admin.accountProfitability.columns.revenue') }}</span><span class="font-mono">{{ formatMoney(row.revenue) }}</span></div>
                  <div><span class="block text-xs text-gray-500">{{ t('admin.accountProfitability.columns.profit') }}</span><span class="font-mono" :class="numberClass(row.profit)">{{ formatProfit(row.profit, row.expense_currency) }}</span></div>
                  <div><span class="block text-xs text-gray-500">{{ t('admin.accountProfitability.columns.margin') }}</span><span class="font-mono" :class="numberClass(row.margin)">{{ formatProfit(row.margin, row.expense_currency, true) }}</span></div>
                </div>
              </article>
            </div>
          </template>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { controlPlaneAPI, getControlPlaneReadMode, type ControlPlaneResponse } from '@/api/controlPlane'
import type { AccountProfitabilityResponse, AccountProfitabilitySource } from '@/api/admin/accountProfitability'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ReadModelStatus from '@/components/admin/ReadModelStatus.vue'
import { useReadModelFreshness } from '@/composables/useReadModelFreshness'
import { useAppStore } from '@/stores/app'

type Range = 'today' | '7d' | '30d' | 'month' | 'custom'
type Filter = 'all' | AccountProfitabilitySource

const { t } = useI18n()
const appStore = useAppStore()
const readMode = getControlPlaneReadMode('account_profitability')
const activeRange = ref<Range>('month')
const startDate = ref('')
const endDate = ref('')
const sourceFilter = ref<Filter>('all')
const statusFilter = ref<'all' | 'known' | 'pending'>('all')
const search = ref('')
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<AccountProfitabilityResponse | null>(null)
const controlPlaneResponse = ref<ControlPlaneResponse<unknown> | null>(null)
const controlPlaneDegraded = ref(false)
const usingControlPlane = ref(false)
const readModel = useReadModelFreshness(controlPlaneResponse)

const ranges: { value: Range }[] = [{ value: 'today' }, { value: '7d' }, { value: '30d' }, { value: 'month' }]
const columns = computed(() => [
  { key: 'account', label: t('admin.accountProfitability.columns.account') },
  { key: 'source', label: t('admin.accountProfitability.columns.source') },
  { key: 'status', label: t('admin.accountProfitability.columns.status') },
  { key: 'expense', label: t('admin.accountProfitability.columns.expense') },
  { key: 'revenue', label: t('admin.accountProfitability.columns.revenue') },
  { key: 'profit', label: t('admin.accountProfitability.columns.profit') },
  { key: 'margin', label: t('admin.accountProfitability.columns.margin') },
  { key: 'requests', label: t('admin.accountProfitability.columns.requests') },
])

function dateToString(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
function monthStart(date: Date): string {
  return dateToString(new Date(date.getFullYear(), date.getMonth(), 1))
}
function resolveRange(range: Range): { start: string; end: string } {
  const now = new Date()
  const end = dateToString(now)
  if (range === 'month') return { start: monthStart(now), end }
  if (range === 'today') return { start: end, end }
  const days = range === '7d' ? 6 : 29
  const startDateValue = new Date(now.getFullYear(), now.getMonth(), now.getDate() - days)
  return { start: dateToString(startDateValue), end }
}
function ensureDates() {
  if (!startDate.value || !endDate.value) {
    const range = resolveRange(activeRange.value)
    startDate.value = range.start
    endDate.value = range.end
  }
}
const dateLabel = computed(() => `${data.value?.start_date ?? startDate.value} — ${data.value?.end_date ?? endDate.value}`)

const filteredRows = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  return (data.value?.rows ?? []).filter((row) => {
    if (sourceFilter.value !== 'all' && row.source !== sourceFilter.value) return false
    if (statusFilter.value === 'known' && row.expense_status !== 'available' && row.expense_status !== 'known' && row.status !== 'known') return false
    if (statusFilter.value === 'pending' && row.expense_status !== 'pending' && row.status !== 'pending') return false
    if (!keyword) return true
    return [row.name, row.platform, row.account_type, row.source].some((value) => value.toLowerCase().includes(keyword))
  })
})

const summaryCards = computed(() => {
  const summary = data.value?.summary
  return [
    { key: 'revenue', label: t('admin.accountProfitability.summary.revenue'), value: formatMoney(summary?.revenue), dot: 'bg-blue-500', valueClass: 'text-blue-700 dark:text-blue-300' },
    { key: 'expense', label: t('admin.accountProfitability.summary.expense'), value: formatMoney(summary?.expense, summary?.pending_count ? 'pending' : undefined), dot: 'bg-amber-500', valueClass: 'text-amber-700 dark:text-amber-300' },
    { key: 'profit', label: t('admin.accountProfitability.summary.profit'), value: formatMoney(summary?.profit, summary?.pending_count ? 'pending' : undefined), dot: 'bg-emerald-500', valueClass: 'text-emerald-700 dark:text-emerald-300' },
    { key: 'margin', label: t('admin.accountProfitability.summary.margin'), value: formatMargin(summary?.margin, summary?.pending_count ? 'pending' : undefined), dot: 'bg-violet-500', valueClass: 'text-violet-700 dark:text-violet-300' },
    { key: 'pending', label: t('admin.accountProfitability.summary.pending'), value: formatNumber(summary?.pending_count ?? 0), dot: 'bg-rose-500', valueClass: 'text-rose-700 dark:text-rose-300' },
  ]
})

function numeric(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null
  const result = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(result) ? result : null
}
function formatNumber(value: number | string | null | undefined): string {
  const number = numeric(value)
  return number === null ? '—' : new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(number)
}
function normalizedCurrency(currency?: string): string {
  return String(currency ?? 'USD').trim().toUpperCase() || 'USD'
}
function expenseValue(row: { expense: number | string | null; expense_currency?: string; procurement_expense_cny?: number | string | null }): number | string | null {
  if (normalizedCurrency(row.expense_currency) === 'CNY' && row.procurement_expense_cny !== null && row.procurement_expense_cny !== undefined) {
    return row.procurement_expense_cny
  }
  return row.expense
}
function formatMoney(value: number | string | null | undefined, status?: string, currency?: string): string {
  if ((status === 'pending' || value === null || value === undefined) && status === 'pending') return t('admin.accountProfitability.pendingCost')
  const number = numeric(value)
  return number === null ? '—' : `${normalizedCurrency(currency) === 'CNY' ? '¥' : '$'}${number.toFixed(2)}`
}
function formatMargin(value: number | string | null | undefined, status?: string): string {
  if (status === 'pending' || value === null || value === undefined) return status === 'pending' ? t('admin.accountProfitability.pendingCost') : '—'
  const number = numeric(value)
  return number === null ? '—' : `${(number * 100).toFixed(1)}%`
}
function formatProfit(value: number | string | null | undefined, currency?: string, margin = false): string {
  if (normalizedCurrency(currency) === 'CNY') return t('admin.accountProfitability.pendingConversion')
  if (value === null || value === undefined) return '—'
  return margin ? formatMargin(value) : formatMoney(value, undefined, 'USD')
}
function numberClass(value: number | string | null | undefined): string {
  const number = numeric(value)
  if (number === null) return 'text-gray-400 dark:text-gray-500'
  return number >= 0 ? 'text-emerald-700 dark:text-emerald-300' : 'text-rose-700 dark:text-rose-300'
}
function sourceLabel(source: AccountProfitabilitySource): string {
  return t(`admin.accountProfitability.sources.${source}`, source)
}
function statusLabel(status: string): string {
  const key = status === 'available' ? 'known' : status
  return t(`admin.accountProfitability.statuses.${key}`, status)
}

const SourceBadge = (props: { source: AccountProfitabilitySource }) => h('span', { class: 'inline-flex rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-200' }, sourceLabel(props.source))
const StatusBadge = (props: { status: string }) => h('span', { class: props.status === 'pending' ? 'inline-flex rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'inline-flex rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' }, statusLabel(props.status))

function asCompatibleResponse(value: unknown): AccountProfitabilityResponse | null {
  if (!value || typeof value !== 'object') return null
  const candidate = value as Partial<AccountProfitabilityResponse>
  return Array.isArray(candidate.rows) && Boolean(candidate.summary) && typeof candidate.start_date === 'string' && typeof candidate.end_date === 'string'
    ? candidate as AccountProfitabilityResponse
    : null
}

async function loadControlPlane(params: { start_date: string; end_date: string; timezone: string }): Promise<AccountProfitabilityResponse | null> {
  controlPlaneDegraded.value = false
  usingControlPlane.value = false
  try {
    const response = await controlPlaneAPI.profitability(params)
    controlPlaneResponse.value = response
    const compatible = asCompatibleResponse(response.items)
    if (readMode === 'external_primary' && compatible) {
      usingControlPlane.value = true
      return compatible
    }
    if (readMode === 'external_primary' && !compatible) controlPlaneDegraded.value = true
  } catch {
    controlPlaneResponse.value = null
    controlPlaneDegraded.value = true
  }
  return null
}

async function load() {
  ensureDates()
  loading.value = true
  error.value = null
  try {
    const params = { start_date: startDate.value, end_date: endDate.value, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' }
    const legacyResult = await adminAPI.accountProfitability.get(params)
    const controlPlaneResult = readMode === 'legacy_only' ? null : await loadControlPlane(params)
    data.value = controlPlaneResult ?? legacyResult
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : t('admin.accountProfitability.loadError')
    error.value = message
    appStore.showError?.(message)
  } finally {
    loading.value = false
  }
}
function selectRange(range: Range) {
  activeRange.value = range
  if (range !== 'custom') {
    const resolved = resolveRange(range)
    startDate.value = resolved.start
    endDate.value = resolved.end
  }
  void load()
}
function exportCsv() {
  const header = [...columns.value.map((column) => column.label), t('admin.accountProfitability.columns.expenseCurrency')]
  const lines = filteredRows.value.map((row) => [row.name, sourceLabel(row.source), statusLabel(row.expense_status), formatMoney(expenseValue(row), row.expense_status, row.expense_currency), formatMoney(row.revenue), formatProfit(row.profit, row.expense_currency), formatProfit(row.margin, row.expense_currency, true), row.request_count, normalizedCurrency(row.expense_currency)].map(csvCell).join(','))
  const blob = new Blob([[header.map(csvCell).join(','), ...lines].join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `account-profitability-${startDate.value}-${endDate.value}.csv`
  link.click()
  URL.revokeObjectURL(url)
}
function csvCell(value: unknown): string {
  return `"${String(value ?? '').replace(/"/g, '""')}"`
}

onMounted(() => {
  ensureDates()
  void load()
})
</script>
