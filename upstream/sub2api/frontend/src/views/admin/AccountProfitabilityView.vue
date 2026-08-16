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
        <button class="btn btn-secondary shrink-0" data-test="financial-refresh" @click="load">
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

      <div v-if="loading" data-test="financial-loading" class="text-sm text-gray-500">
        {{ t('admin.accountProfitability.loading') }}
      </div>
      <div v-if="refreshing" data-test="financial-refreshing" class="text-xs text-gray-500">
        {{ t('admin.accountProfitability.refreshing') }}
      </div>
      <div
        v-if="loadError"
        class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700"
        data-test="financial-load-error"
        role="alert"
      >
        <span>{{ loadError }}</span>
        <button class="btn btn-secondary ml-3" data-test="financial-retry" @click="load">
          {{ t('admin.accountProfitability.retry') }}
        </button>
      </div>

      <template v-if="hasLoaded">
        <div class="text-xs text-gray-500" data-test="financial-generated-at">{{ generatedAt }}</div>

        <section class="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-8">
          <article
            v-for="card in summaryCards"
            :key="card.key"
            class="card min-w-0 p-4"
            :data-test="`summary-${card.key}`"
          >
            <div class="text-xs text-gray-500">{{ card.label }}</div>
            <div class="mt-2 break-words text-xl font-semibold">{{ card.value }}</div>
            <div v-if="card.note" class="mt-1 text-xs text-gray-500">{{ card.note }}</div>
            <button
              v-if="card.key === 'probe-cost' && report.probe_data_error"
              class="mt-2 text-xs font-medium text-primary-600 hover:underline"
              data-test="financial-probe-retry"
              @click="load"
            >
              {{ t('admin.accountProfitability.probe.retry') }}
            </button>
          </article>
        </section>

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
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
            <article
              v-for="card in scopedCards"
              :key="card.key"
              class="min-w-0 border-t border-gray-200 pt-3 dark:border-gray-700"
            >
              <div class="text-xs text-gray-500">{{ card.label }}</div>
              <div class="mt-1 break-words text-base font-semibold">{{ card.value }}</div>
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

          <div
            v-else
            class="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2"
            data-test="account-card-grid"
          >
            <article
              v-for="account in sortedAccounts"
              :key="`${activePairPrefix}:${account.id}`"
              class="card min-w-0 overflow-hidden p-4"
              :data-test="`account-card-${account.id}`"
              :data-account-id="account.id"
              :data-pair="`${activePairPrefix}:${account.id}`"
            >
              <header class="flex min-w-0 items-start justify-between gap-3 border-b border-gray-200 pb-3 dark:border-gray-700">
                <div class="min-w-0">
                  <h3 class="break-words text-base font-semibold">{{ account.name }}</h3>
                  <p class="mt-1 break-words text-xs text-gray-500">
                    {{ t('admin.accountProfitability.account.meta', { platform: account.platform, type: account.type }) }}
                  </p>
                </div>
                <span class="shrink-0 text-xs text-gray-400">#{{ account.id }}</span>
              </header>

              <dl class="mt-3 grid min-w-0 grid-cols-2 gap-x-4 gap-y-3">
                <div
                  v-for="metric in accountMetrics(account.amounts)"
                  :key="metric.key"
                  class="min-w-0"
                  :data-metric="metric.key"
                >
                  <dt class="text-xs text-gray-500">{{ metric.label }}</dt>
                  <dd class="mt-1 break-words text-sm font-semibold">{{ metric.value }}</dd>
                  <dd v-if="metric.note" class="mt-1 text-xs text-gray-500">{{ metric.note }}</dd>
                </div>
              </dl>
            </article>
          </div>
        </section>
      </template>
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

type FinancialScope = { kind: 'all' } | { kind: 'group'; id: number; unassigned: boolean }
type FinancialSortKey = 'requests' | 'tokens' | 'cost' | 'user_cost' | 'profit' | 'margin'
type FinancialSort = { key: FinancialSortKey; direction: 'asc' | 'desc' }
type DisplayMetric = { key: string; label: string; value: string; note?: string }

const { t } = useI18n()
const activeRange = ref<FinancialRange>('today')
const activeScope = ref<FinancialScope>({ kind: 'all' })
const sort = ref<FinancialSort>({ key: 'requests', direction: 'desc' })
const loading = ref(false)
const refreshing = ref(false)
const loadError = ref('')
const hasLoaded = ref(false)
let requestSequence = 0
let timer: ReturnType<typeof setInterval> | undefined

const emptyAmounts = (): FinancialAmounts => ({
  requests: 0,
  tokens: 0,
  cost: 0,
  user_cost: 0,
  profit: 0,
  margin: null,
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
const compact = (value: number) => new Intl.NumberFormat(undefined, {
  notation: 'compact',
  maximumFractionDigits: 2,
}).format(value)
const percent = (value: number | null) => value == null ? '—' : `${(value * 100).toFixed(2)}%`

const probeMetric = (amounts: FinancialAmounts): DisplayMetric => {
  if (report.value.probe_data_error) {
    return {
      key: 'probe-cost',
      label: t('admin.accountProfitability.summary.probeCost'),
      value: t('admin.accountProfitability.probe.dataError'),
    }
  }
  if (amounts.probe_cost_status === 'unavailable') {
    return {
      key: 'probe-cost',
      label: t('admin.accountProfitability.summary.probeCost'),
      value: usd(0),
      note: t('admin.accountProfitability.probe.noRecords'),
    }
  }
  if (amounts.probe_cost_status === 'incomplete') {
    return {
      key: 'probe-cost',
      label: t('admin.accountProfitability.summary.probeCost'),
      value: '—',
      note: t('admin.accountProfitability.probe.incomplete'),
    }
  }
  return {
    key: 'probe-cost',
    label: t('admin.accountProfitability.summary.probeCost'),
    value: usd(amounts.probe_cost),
  }
}

const financialMetrics = (amounts: FinancialAmounts): DisplayMetric[] => [
  { key: 'requests', label: t('admin.accountProfitability.summary.requests'), value: compact(amounts.requests) },
  { key: 'tokens', label: t('admin.accountProfitability.summary.tokens'), value: compact(amounts.tokens) },
  { key: 'cost', label: t('admin.accountProfitability.summary.accountCost'), value: usd(amounts.cost) },
  { key: 'user-cost', label: t('admin.accountProfitability.summary.userCost'), value: usd(amounts.user_cost) },
  { key: 'profit', label: t('admin.accountProfitability.summary.profit'), value: usd(amounts.profit) },
  { key: 'margin', label: t('admin.accountProfitability.summary.margin'), value: percent(amounts.margin) },
]

const selectedGroup = computed(() => {
  const scope = activeScope.value
  return scope.kind === 'group'
    ? report.value.groups.find((group) => group.id === scope.id && group.unassigned === scope.unassigned)
    : undefined
})
const selectedAccounts = computed(() => selectedGroup.value?.accounts ?? report.value.accounts)
const selectedAmounts = computed(() => selectedGroup.value?.amounts ?? report.value.summary)
const summaryCards = computed<DisplayMetric[]>(() => [
  ...financialMetrics(report.value.summary),
  probeMetric(report.value.summary),
  {
    key: 'unconsumed-balance',
    label: t('admin.accountProfitability.summary.unconsumedBalance'),
    value: usd(report.value.user_unconsumed_balance_cny),
  },
])
const scopedCards = computed<DisplayMetric[]>(() => [
  ...financialMetrics(selectedAmounts.value),
  probeMetric(selectedAmounts.value),
])
const sortOptions = computed(() => ([
  { key: 'requests' as const, label: t('admin.accountProfitability.summary.requests') },
  { key: 'tokens' as const, label: t('admin.accountProfitability.summary.tokens') },
  { key: 'cost' as const, label: t('admin.accountProfitability.summary.accountCost') },
  { key: 'user_cost' as const, label: t('admin.accountProfitability.summary.userCost') },
  { key: 'profit' as const, label: t('admin.accountProfitability.summary.profit') },
  { key: 'margin' as const, label: t('admin.accountProfitability.summary.margin') },
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

function accountMetrics(amounts: FinancialAmounts) {
  return [...financialMetrics(amounts), probeMetric(amounts)]
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

async function load() {
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
  void load()
}

onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 60_000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
