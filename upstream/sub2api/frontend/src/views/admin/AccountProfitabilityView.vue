<template>
  <AppLayout>
    <main
      class="mx-auto w-full max-w-[1400px] space-y-5 overflow-x-hidden p-4 sm:p-5"
      data-test="account-profitability-page"
    >
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h1 class="text-2xl font-semibold">{{ t('admin.accountProfitability.title') }}</h1>
          <p class="text-sm text-gray-500">{{ t('admin.accountProfitability.description') }}</p>
        </div>
        <button class="btn btn-secondary shrink-0" data-test="financial-refresh" @click="refreshCurrentView">
          {{ t('common.refresh') }}
        </button>
      </header>

      <nav class="flex max-w-full gap-2 overflow-x-auto pb-1" aria-label="range">
        <button
          v-for="item in ranges"
          :key="item"
          :data-test="`range-${item}`"
          class="btn btn-secondary shrink-0"
          :class="{ 'bg-primary-600 text-white': activeRange === item }"
          @click="selectRange(item)"
        >
          {{ t(`admin.accountProfitability.ranges.${item}`) }}
        </button>
      </nav>

      <div class="inline-flex max-w-full rounded border border-gray-200 p-1 dark:border-gray-700" role="group" aria-label="经营视图">
        <button
          class="min-h-9 px-3 py-1.5 text-sm"
          :class="activeView === 'usd' ? 'bg-primary-600 text-white' : 'text-gray-600 dark:text-gray-300'"
          data-test="view-usd"
          :aria-pressed="activeView === 'usd'"
          @click="selectView('usd')"
        >经营结果 · USD</button>
        <button
          class="min-h-9 px-3 py-1.5 text-sm"
          :class="activeView === 'cny' ? 'bg-primary-600 text-white' : 'text-gray-600 dark:text-gray-300'"
          data-test="view-cny"
          :aria-pressed="activeView === 'cny'"
          @click="selectView('cny')"
        >自购专题 · CNY</button>
      </div>

      <div v-if="activeView === 'usd' && loading" data-test="financial-loading" class="text-sm text-gray-500">
        {{ t('admin.accountProfitability.loading') }}
      </div>
      <div v-if="activeView === 'usd' && refreshing" data-test="financial-refreshing" class="text-xs text-gray-500">
        {{ t('admin.accountProfitability.refreshing') }}
      </div>
      <div
        v-if="activeView === 'usd' && loadError"
        class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700"
        data-test="financial-load-error"
        role="alert"
      >
        <span>{{ loadError }}</span>
        <button class="btn btn-secondary ml-3" data-test="financial-retry" @click="loadUSD">
          {{ t('admin.accountProfitability.retry') }}
        </button>
      </div>

      <template v-if="activeView === 'usd' && hasLoaded">
        <div class="text-xs text-gray-500" data-test="financial-generated-at">{{ generatedAt }}</div>

        <section class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5" data-test="summary-grid">
          <article
            v-for="card in summaryCards"
            :key="card.key"
            class="card min-w-0 p-4"
            :data-test="`summary-${card.key}`"
          >
            <div class="text-xs text-gray-500">{{ card.label }}</div>
            <div class="mt-2 break-words text-xl font-semibold" :class="card.tone">{{ card.value }}</div>
            <div v-if="card.note" class="mt-1 text-xs text-gray-500">{{ card.note }}</div>
          </article>
        </section>
        <p class="text-xs text-gray-500" data-test="usd-cny-exclusion-note">
          USD 经营结果未含自购账号 CNY 采购成本；自购实际采购利润请查看自购专题。
        </p>
        <p class="text-xs text-gray-500" data-test="financial-role-history-note">
          {{ t('admin.accountProfitability.roleHistoryNote') }}
        </p>

        <nav
          class="flex max-w-full gap-2 overflow-x-auto pb-1"
          :aria-label="t('admin.accountProfitability.scope.label')"
        >
          <button
            class="btn btn-secondary shrink-0"
            data-test="scope-all"
            :class="{ 'bg-primary-600 text-white': activeScope.kind === 'all' }"
            @click="activeScope = { kind: 'all' }"
          >
            {{ t('admin.accountProfitability.scope.all') }}
          </button>
          <button
            v-for="group in report.groups"
            :key="`${group.unassigned ? 'unassigned' : 'group'}-${group.id}`"
            class="btn btn-secondary shrink-0"
            :data-test="`scope-group-${group.id}`"
            :class="{ 'bg-primary-600 text-white': isSelectedGroup(group.id, group.unassigned) }"
            @click="activeScope = { kind: 'group', id: group.id, unassigned: group.unassigned }"
          >
            {{ group.unassigned ? t('admin.accountProfitability.scope.unassigned') : group.name }}
          </button>
        </nav>

        <section
          v-if="selectedGroup"
          class="min-w-0 space-y-3"
          :data-test="`group-summary-${selectedGroup.id}`"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h2 class="text-base font-semibold">
              <span class="text-gray-500">{{ t('admin.accountProfitability.scope.groupSummary') }}:</span>
              {{ selectedGroup.unassigned ? t('admin.accountProfitability.scope.unassigned') : selectedGroup.name }}
            </h2>
            <span class="text-xs text-gray-500">
              {{ t('admin.accountProfitability.scope.accountCount', { count: sortedAccounts.length }) }}
            </span>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            <article
              v-for="card in scopedCards"
              :key="card.key"
              class="min-w-0 border-t border-gray-200 pt-3 dark:border-gray-700"
            >
              <div class="text-xs text-gray-500">{{ card.label }}</div>
              <div class="mt-1 break-words text-base font-semibold" :class="card.tone">{{ card.value }}</div>
              <div v-if="card.note" class="mt-1 text-xs text-gray-500">{{ card.note }}</div>
            </article>
          </div>
        </section>

        <section class="space-y-3">
          <div class="flex flex-wrap items-center gap-2" :aria-label="t('admin.accountProfitability.sort.label')">
            <span class="mr-1 text-xs font-medium text-gray-500">
              {{ t('admin.accountProfitability.sort.label') }}
            </span>
            <button
              v-for="option in sortOptions"
              :key="option.key"
              class="btn btn-secondary min-h-9 px-3 py-1.5 text-xs"
              :class="{ 'border-primary-500 text-primary-600': sort.key === option.key }"
              :data-test="`sort-${option.key}`"
              :aria-sort="ariaSort(option.key)"
              @click="setSort(option.key)"
            >
              <span>{{ option.label }}</span>
              <span v-if="sort.key === option.key" class="ml-1" aria-hidden="true">
                {{ sort.direction === 'asc' ? '↑' : '↓' }}
              </span>
            </button>
          </div>

          <div v-if="isEmpty" data-test="financial-empty" class="text-sm text-gray-500">
            {{ t('admin.accountProfitability.empty') }}
          </div>

          <div v-else class="min-w-0 overflow-x-auto" data-test="account-financial-table-wrap">
            <table class="min-w-[760px] w-full border-collapse text-left" data-test="account-financial-table">
              <thead>
                <tr class="border-b border-gray-200 text-xs text-gray-500 dark:border-gray-700">
                  <th class="min-w-[220px] px-3 py-3 font-medium">{{ t('admin.accountProfitability.table.account') }}</th>
                  <th class="min-w-[120px] px-3 py-3 text-right font-medium">{{ t('admin.accountProfitability.summary.operationalCost') }}</th>
                  <th class="min-w-[120px] px-3 py-3 text-right font-medium">{{ t('admin.accountProfitability.summary.businessCost') }}</th>
                  <th class="min-w-[120px] px-3 py-3 text-right font-medium">{{ t('admin.accountProfitability.summary.businessRevenue') }}</th>
                  <th class="min-w-[120px] px-3 py-3 text-right font-medium">{{ t('admin.accountProfitability.summary.totalCost') }}</th>
                  <th class="min-w-[120px] px-3 py-3 text-right font-medium">{{ t('admin.accountProfitability.summary.netProfit') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
              v-for="account in sortedAccounts"
              :key="`${activePairPrefix}:${account.id}`"
              class="border-b border-gray-100 align-top dark:border-gray-800"
              :data-test="`account-row-${account.id}`"
              :data-account-id="account.id"
              :data-pair="`${activePairPrefix}:${account.id}`"
                >
                  <th scope="row" class="px-3 py-3 font-normal">
                    <div class="min-w-0">
                      <div class="break-words text-sm font-semibold">{{ account.name }}</div>
                      <div class="mt-1 break-words text-xs text-gray-500">
                        {{ t('admin.accountProfitability.account.meta', { platform: account.platform, type: account.type }) }} · #{{ account.id }}
                      </div>
                    </div>
                  </th>
                  <td class="px-3 py-3 text-right text-sm font-semibold" data-metric="operational-cost" :class="toneFor('operational_cost')">{{ usd(account.amounts.operational_cost) }}</td>
                  <td class="px-3 py-3 text-right text-sm font-semibold" data-metric="business-cost" :class="toneFor('business_cost')">{{ usd(account.amounts.business_cost) }}</td>
                  <td class="px-3 py-3 text-right text-sm font-semibold" data-metric="business-revenue" :class="toneFor('business_revenue')">{{ usd(account.amounts.business_revenue) }}</td>
                  <td class="px-3 py-3 text-right text-sm font-semibold" data-metric="total-cost" :class="toneFor('total_cost')">{{ usd(account.amounts.total_cost) }}</td>
                  <td class="px-3 py-3 text-right text-sm font-semibold" data-metric="net-profit" :class="toneFor('net_profit', account.amounts.net_profit)">{{ usd(account.amounts.net_profit) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>

      <section v-if="activeView === 'cny'" class="card min-w-0 space-y-4" data-test="self-purchased-panel">
        <div>
          <h2 class="text-lg font-semibold">自购账号 · 人民币</h2>
          <p class="text-xs text-gray-500">独立于渠道 USD 汇总；按标准额度消耗确认采购成本。</p>
        </div>
        <div v-if="selfPurchasedLoading" data-test="self-purchased-loading" class="text-sm text-gray-500">加载中</div>
        <div v-if="selfPurchasedRefreshing" data-test="self-purchased-refreshing" class="text-xs text-gray-500">刷新中</div>
        <div v-if="selfPurchasedError" class="text-sm text-red-600" data-test="self-purchased-error" role="alert">
          {{ selfPurchasedError }}
          <button class="btn btn-secondary ml-3" data-test="self-purchased-retry" @click="loadSelfPurchased">重试</button>
        </div>
        <template v-if="selfPurchasedLoaded">
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7" data-test="self-purchased-summary-grid">
            <div class="min-w-0" data-test="self-summary-account-count"><div class="text-xs text-gray-500">自购账号数</div><div class="font-semibold">{{ selfPurchased.summary.account_count }}</div></div>
            <div class="min-w-0" data-test="self-summary-revenue"><div class="text-xs text-gray-500">人民币营收</div><div class="font-semibold">{{ cny(selfPurchased.summary.revenue_cny) }}</div></div>
            <div class="min-w-0" data-test="self-summary-confirmed-cost"><div class="text-xs text-gray-500">已确认采购成本</div><div class="font-semibold">{{ cny(selfPurchased.summary.confirmed_cost_cny) }}</div></div>
            <div class="min-w-0" data-test="self-summary-pending-cost"><div class="text-xs text-gray-500">待摊成本</div><div class="font-semibold">{{ cny(selfPurchased.summary.pending_cost_cny) }}</div></div>
            <div class="min-w-0" data-test="self-summary-loss"><div class="text-xs text-gray-500">采购损失</div><div class="font-semibold">{{ cny(selfPurchased.summary.procurement_loss_cny) }}</div></div>
            <div class="min-w-0" data-test="self-summary-net-profit"><div class="text-xs text-gray-500">人民币净利润</div><div class="font-semibold">{{ cny(selfPurchased.summary.net_profit_cny) }}</div></div>
            <div class="min-w-0" data-test="self-summary-margin"><div class="text-xs text-gray-500">利润率</div><div class="font-semibold">{{ percent(selfPurchased.summary.margin) }}</div></div>
          </div>
          <div v-if="selfPurchased.rows.length" class="min-w-0 overflow-x-auto" data-test="self-purchased-table-wrap">
            <table class="min-w-[1180px] w-full text-left text-sm" data-test="self-purchased-table"><thead><tr class="border-b text-xs text-gray-500"><th class="px-2 py-2">账号</th><th class="px-2 py-2 text-right">采购成本</th><th class="px-2 py-2 text-right">预计额度</th><th class="px-2 py-2 text-right">标准消耗</th><th class="px-2 py-2 text-right">利用率</th><th class="px-2 py-2 text-right">确认成本</th><th class="px-2 py-2 text-right">待摊</th><th class="px-2 py-2 text-right">采购损失</th><th class="px-2 py-2 text-right">营收</th><th class="px-2 py-2 text-right">净利润</th><th class="px-2 py-2 text-right">利润率</th><th class="px-2 py-2">状态</th></tr></thead><tbody><tr v-for="row in selfPurchased.rows" :key="row.account_id" class="border-b"><th class="px-2 py-2 font-medium">{{ row.name }} <span class="text-xs text-gray-500">#{{ row.account_id }}</span></th><td class="px-2 py-2 text-right">{{ row.procurement_cost_cny == null ? '成本待录入' : cny(row.procurement_cost_cny) }}</td><td class="px-2 py-2 text-right">{{ row.estimated_quota_usd == null ? '—' : `${row.estimated_quota_usd.toFixed(2)} USD` }}</td><td class="px-2 py-2 text-right">{{ row.standard_consumed_usd.toFixed(2) }} USD</td><td class="px-2 py-2 text-right">{{ percent(row.utilization) }}</td><td class="px-2 py-2 text-right">{{ cny(row.confirmed_cost_cny) }}</td><td class="px-2 py-2 text-right">{{ cny(row.pending_cost_cny) }}</td><td class="px-2 py-2 text-right">{{ cny(row.procurement_loss_cny) }}</td><td class="px-2 py-2 text-right">{{ cny(row.revenue_cny) }}</td><td class="px-2 py-2 text-right">{{ cny(row.net_profit_cny) }}</td><td class="px-2 py-2 text-right">{{ percent(row.margin) }}</td><td class="px-2 py-2">{{ row.cost_status === 'cost_pending' ? '成本待录入' : row.cost_status }}<button type="button" class="btn btn-secondary ml-2 px-2 py-1 text-xs" :data-test="`edit-procurement-${row.account_id}`" @click="openProcurementDialog(row)">{{ row.procurement_cost_cny == null ? '录入成本' : '编辑成本' }}</button><button v-if="row.cost_status !== 'settled' && row.cost_status !== 'cost_pending' && (row.status === 'disabled' || row.status === 'error' || row.status === 'expired')" class="btn btn-secondary ml-2 px-2 py-1 text-xs" :data-test="`settle-${row.account_id}`" @click="settleSelfPurchased(row.account_id)">确认失效</button></td></tr></tbody></table>
          </div>
        </template>
      </section>
      <AccountMonitorCostDialog
        v-if="selectedProcurementAccount"
        :show="true"
        :account="selectedProcurementAccount"
        :saving="procurementSaving"
        :error="procurementDialogError"
        @close="closeProcurementDialog"
        @save-procurement="saveProcurement"
        @clear="clearProcurement"
      />
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  AccountFinancialReport,
  FinancialAccount,
  FinancialAmounts,
  FinancialRange,
} from '@/api/admin/accountFinancial'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { SelfPurchasedReport, SelfPurchasedRow } from '@/api/admin/selfPurchasedProfitability'
import AccountMonitorCostDialog from '@/components/admin/account-monitor/AccountMonitorCostDialog.vue'

type FinancialScope = { kind: 'all' } | { kind: 'group'; id: number; unassigned: boolean }
type FinancialView = 'usd' | 'cny'
type FinancialSortKey = 'operational_cost' | 'business_cost' | 'business_revenue' | 'total_cost' | 'net_profit' | 'external_margin'
type FinancialSort = { key: FinancialSortKey; direction: 'asc' | 'desc' }
type DisplayMetric = { key: string; label: string; value: string; note?: string; tone?: string }

const { t } = useI18n()
const activeView = ref<FinancialView>('usd')
const activeRange = ref<FinancialRange>('today')
const activeScope = ref<FinancialScope>({ kind: 'all' })
const sort = ref<FinancialSort>({ key: 'net_profit', direction: 'desc' })
const loading = ref(false)
const refreshing = ref(false)
const loadError = ref('')
const hasLoaded = ref(false)
const selfPurchased = ref<SelfPurchasedReport>({ start_date:'', end_date:'', generated_at:'', currency:'CNY', summary:{procurement_cost_cny:0,standard_consumed_usd:0,confirmed_cost_cny:0,pending_cost_cny:0,procurement_loss_cny:0,revenue_cny:0,net_profit_cny:null,margin:null,account_count:0}, rows:[] })
const selfPurchasedLoaded = ref(false)
const selfPurchasedLoadedRange = ref<FinancialRange | null>(null)
const selfPurchasedLoading = ref(false)
const selfPurchasedRefreshing = ref(false)
const selfPurchasedError = ref('')
const selectedProcurementAccount = ref<{ account_id:number; name:string; platform:string; account_type:string; procurement_cost_cny:number|null; estimated_usable_quota_usd:number|null } | null>(null)
const procurementSaving = ref(false)
const procurementDialogError = ref('')
let procurementMutationKey: { payload: string; key: string } | null = null
let requestSequence = 0
let selfPurchasedRequestSequence = 0
let timer: ReturnType<typeof setInterval> | undefined

const emptyAmounts = (): FinancialAmounts => ({
  requests: 0,
  tokens: 0,
  cost: 0,
  user_cost: 0,
  profit: 0,
  margin: null,
  operational_cost: 0,
  business_cost: 0,
  business_revenue: 0,
  total_cost: 0,
  net_profit: 0,
  external_margin: null,
  probe_requests: 0,
  probe_tokens: 0,
  probe_cost: 0,
  probe_cost_status: 'unavailable',
})
const emptyReport = (): AccountFinancialReport => ({
  generated_at: '',
  range: 'today',
  currency: 'USD',
  probe_data_error: false,
  probe_error_code: null,
  summary: emptyAmounts(),
  accounts: [],
  groups: [],
  user_unconsumed_balance_cny: 0,
})

const report = ref<AccountFinancialReport>(emptyReport())
const ranges: FinancialRange[] = ['today', '24h', '7d', '31d']
const generatedAt = computed(() => report.value.generated_at
  ? new Date(report.value.generated_at).toLocaleString()
  : '—')

const usd = (value: number | null) => value == null || !Number.isFinite(value)
  ? '—'
  : new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value)
const percent = (value: number | null) => value == null ? '—' : `${(value * 100).toFixed(2)}%`
const cny = (value: number | null) => value == null || !Number.isFinite(value) ? '—' : new Intl.NumberFormat(undefined,{style:'currency',currency:'CNY',minimumFractionDigits:2,maximumFractionDigits:2}).format(value)

const toneFor = (key: FinancialSortKey, value?: number) => {
  if (key === 'business_revenue') return 'text-blue-700 dark:text-blue-300'
  if (key === 'operational_cost') return 'text-purple-700 dark:text-purple-300'
  if (key === 'business_cost' || key === 'total_cost') return 'text-amber-700 dark:text-amber-300'
  if (key === 'net_profit' || key === 'external_margin') {
    return value != null && value < 0 ? 'text-red-700 dark:text-red-300' : 'text-emerald-700 dark:text-emerald-300'
  }
  return ''
}

const resultMetrics = (amounts: FinancialAmounts): DisplayMetric[] => [
  { key: 'business-revenue', label: t('admin.accountProfitability.summary.businessRevenue'), value: usd(amounts.business_revenue), tone: toneFor('business_revenue') },
  { key: 'total-cost', label: t('admin.accountProfitability.summary.totalCost'), value: usd(amounts.total_cost), tone: toneFor('total_cost') },
  { key: 'net-profit', label: t('admin.accountProfitability.summary.netProfit'), value: usd(amounts.net_profit), tone: toneFor('net_profit', amounts.net_profit) },
  { key: 'external-margin', label: t('admin.accountProfitability.summary.externalMargin'), value: percent(amounts.external_margin), tone: toneFor('external_margin', amounts.external_margin ?? undefined) },
  { key: 'operational-cost', label: t('admin.accountProfitability.summary.operationalCost'), value: usd(amounts.operational_cost), note: t('admin.accountProfitability.summary.includedInTotal'), tone: toneFor('operational_cost') },
]

const selectedGroup = computed(() => {
  const scope = activeScope.value
  return scope.kind === 'group'
    ? report.value.groups.find((group) => group.id === scope.id && group.unassigned === scope.unassigned)
    : undefined
})
const selectedAccounts = computed(() => selectedGroup.value?.accounts ?? report.value.accounts)
const selectedAmounts = computed(() => selectedGroup.value?.amounts ?? report.value.summary)
const summaryCards = computed<DisplayMetric[]>(() => resultMetrics(report.value.summary))
const scopedCards = computed<DisplayMetric[]>(() => resultMetrics(selectedAmounts.value))
const sortOptions = computed(() => ([
  { key: 'operational_cost' as const, label: t('admin.accountProfitability.summary.operationalCost') },
  { key: 'business_cost' as const, label: t('admin.accountProfitability.summary.businessCost') },
  { key: 'business_revenue' as const, label: t('admin.accountProfitability.summary.businessRevenue') },
  { key: 'total_cost' as const, label: t('admin.accountProfitability.summary.totalCost') },
  { key: 'net_profit' as const, label: t('admin.accountProfitability.summary.netProfit') },
  { key: 'external_margin' as const, label: t('admin.accountProfitability.summary.externalMargin') },
]))
const sortedAccounts = computed(() => [...selectedAccounts.value].sort(compareAccounts))
const activePairPrefix = computed(() => activeScope.value.kind === 'group' ? activeScope.value.id : 'all')
const isEmpty = computed(() => hasLoaded.value && sortedAccounts.value.length === 0)

function compareAccounts(a: FinancialAccount, b: FinancialAccount) {
  const key = sort.value.key
  const left = a.amounts[key]
  const right = b.amounts[key]
  if (left == null && right == null) return a.id - b.id
  if (left == null) return 1
  if (right == null) return -1
  const difference = left - right
  if (difference === 0) return a.id - b.id
  return sort.value.direction === 'asc' ? difference : -difference
}

function setSort(key: FinancialSortKey) {
  if (sort.value.key === key) {
    sort.value = { key, direction: sort.value.direction === 'desc' ? 'asc' : 'desc' }
  } else {
    sort.value = { key, direction: 'desc' }
  }
}

function ariaSort(key: FinancialSortKey) {
  if (sort.value.key !== key) return 'none'
  return sort.value.direction === 'asc' ? 'ascending' : 'descending'
}

function isSelectedGroup(id: number, unassigned: boolean) {
  return activeScope.value.kind === 'group'
    && activeScope.value.id === id
    && activeScope.value.unassigned === unassigned
}

function openProcurementDialog(row: SelfPurchasedRow) {
  procurementMutationKey = null
  procurementDialogError.value = ''
  selectedProcurementAccount.value = {
    account_id: row.account_id, name: row.name, platform: row.platform, account_type: row.account_type,
    procurement_cost_cny: row.procurement_cost_cny, estimated_usable_quota_usd: row.estimated_quota_usd,
  }
}
function closeProcurementDialog() { if (!procurementSaving.value) selectedProcurementAccount.value = null }
function procurementKey(accountId: number, cost: number | null, quota: number | null) {
  const payload = JSON.stringify([accountId, cost, quota])
  if (procurementMutationKey?.payload === payload) return procurementMutationKey.key
  const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  const key = `account-procurement-${accountId}-${requestId}`
  procurementMutationKey = { payload, key }
  return key
}
async function mutateProcurement(cost: number | null, quota: number | null) {
  const account = selectedProcurementAccount.value
  if (!account || procurementSaving.value) return
  procurementSaving.value = true
  procurementDialogError.value = ''
  try {
    const updated = await adminAPI.accounts.updateProcurementCost(account.account_id, cost, quota, procurementKey(account.account_id, cost, quota))
    account.procurement_cost_cny = updated.procurement_cost_cny ?? null
    account.estimated_usable_quota_usd = updated.estimated_usable_quota_usd ?? null
    const partial = updated.procurement_readback_status === 'failed'
    procurementMutationKey = null
    const refreshed = await loadSelfPurchased()
    if (partial) procurementDialogError.value = updated.procurement_message || '采购成本已保存，但账号刷新失败，请刷新页面确认'
    else if (refreshed) selectedProcurementAccount.value = null
    else procurementDialogError.value = '采购成本已保存，但最新 CNY 数据刷新失败，请重试刷新'
    window.dispatchEvent(new CustomEvent('account-procurement-updated', { detail: { accountId: account.account_id } }))
  } catch (reason: any) {
    procurementDialogError.value = reason?.message || reason?.response?.data?.message || '采购成本保存失败，请稍后重试'
  } finally { procurementSaving.value = false }
}
function saveProcurement(cost: number, quota: number) { return mutateProcurement(cost, quota) }
function clearProcurement() { return mutateProcurement(null, null) }

async function settleSelfPurchased(accountId: number) {
  if (!window.confirm('确认账号失效并结算剩余采购成本？')) return
  try { await adminAPI.selfPurchasedProfitability.settle(accountId, { request_id: `ui-${Date.now()}-${accountId}`, reason: 'administrator_confirmed_expired' }); await loadSelfPurchased() } catch { selfPurchasedError.value = '结算失败' }
}

async function loadSelfPurchased() {
	const sequence = ++selfPurchasedRequestSequence
	if (selfPurchasedLoaded.value) selfPurchasedRefreshing.value = true
	else selfPurchasedLoading.value = true
  selfPurchasedError.value = ''
  try {
    const next = await adminAPI.selfPurchasedProfitability.get({ range: activeRange.value })
    if (sequence !== selfPurchasedRequestSequence) return false
    selfPurchased.value = next
    selfPurchasedLoaded.value = true
    selfPurchasedLoadedRange.value = activeRange.value
    return true
  } catch {
    if (sequence === selfPurchasedRequestSequence) selfPurchasedError.value = '自购账号数据加载失败'
    return false
  } finally {
    if (sequence === selfPurchasedRequestSequence) {
      selfPurchasedLoading.value = false
      selfPurchasedRefreshing.value = false
    }
  }
}

async function loadUSD() {
  const sequence = ++requestSequence
  if (hasLoaded.value) refreshing.value = true
  else loading.value = true
  loadError.value = ''
  try {
    const next = await adminAPI.accountFinancial.getReport({ range: activeRange.value })
    if (sequence !== requestSequence) return
    report.value = next
    hasLoaded.value = true
    if (activeScope.value.kind === 'group' && !selectedGroup.value) {
      activeScope.value = { kind: 'all' }
    }
  } catch {
    if (sequence === requestSequence) loadError.value = t('admin.accountProfitability.loadError')
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
      refreshing.value = false
    }
  }
}

function selectRange(range: FinancialRange) {
  activeRange.value = range
  void refreshCurrentView()
}

function selectView(view: FinancialView) {
  activeView.value = view
  if (view === 'usd' && !hasLoaded.value) void loadUSD()
  if (view === 'cny' && selfPurchasedLoadedRange.value !== activeRange.value) void loadSelfPurchased()
}

function refreshCurrentView() {
  if (activeView.value === 'cny') return loadSelfPurchased()
  return loadUSD()
}

onMounted(() => {
  void loadUSD()
  timer = setInterval(() => void refreshCurrentView(), 60_000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
