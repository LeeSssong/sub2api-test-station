<template>
  <AppLayout>
    <div class="min-h-full bg-[#f4f7f9] px-5 py-8 dark:bg-slate-950 max-sm:px-3 max-sm:pt-[22px] sm:py-9">
      <div class="mx-auto flex w-full max-w-[1240px] flex-col gap-[18px]" data-test="account-monitor-page">
      <header class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <h1 class="text-[27px] font-semibold leading-[1.25] text-gray-900 max-[430px]:text-[23px] dark:text-white">
            {{ t('admin.accountMonitor.title') }}
          </h1>
          <p class="mt-[7px] text-sm text-gray-500 max-[760px]:max-w-[272px] dark:text-gray-400">
            {{ t('admin.accountMonitor.description') }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-primary shrink-0"
          data-test="run-all"
          :disabled="runningAll || loading"
          :title="t('admin.accountMonitor.actions.refreshAll')"
          :aria-label="t('admin.accountMonitor.actions.refreshAll')"
          @click="handleRunAll"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': runningAll }" />
          <span class="hidden sm:inline">
            {{ runningAll ? t('admin.accountMonitor.actions.running') : t('admin.accountMonitor.actions.refreshAll') }}
          </span>
        </button>
      </header>

      <nav class="flex gap-6 overflow-x-auto border-b border-gray-200 dark:border-dark-700" role="tablist" aria-label="账号分组">
        <button
          type="button"
          role="tab"
          class="relative shrink-0 px-0.5 pb-3 text-sm font-semibold transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
          :class="tabClass(activeGroupId === null)"
          :aria-selected="activeGroupId === null"
          data-test="all-site-tab-button"
          @click="selectGroup(null, $event)"
        >
          全站
          <span class="ml-1.5 rounded-full bg-gray-100 px-2 py-0.5 font-mono text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-300">
            {{ allAccounts.length }}
          </span>
        </button>
        <button
          v-for="group in sortedGroups"
          :key="group.id"
          type="button"
          role="tab"
          class="relative shrink-0 px-0.5 pb-3 text-sm font-semibold transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
          :class="tabClass(activeGroupId === group.id)"
          :aria-selected="activeGroupId === group.id"
          :data-test="`group-tab-${group.id}`"
          @click="selectGroup(group.id, $event)"
        >
          {{ group.name }}
          <span class="ml-1.5 rounded-full bg-gray-100 px-2 py-0.5 font-mono text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-300">
            {{ groupAccountCount(group) }}
          </span>
        </button>
      </nav>

      <section class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center" aria-label="账号筛选">
        <AccountMonitorFilters
          :search="search"
          :status="status"
          @update:search="search = $event"
          @update:status="status = $event"
        />
        <div class="grid grid-cols-3 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-800" role="group" aria-label="统计时间范围">
          <button
            v-for="range in ranges"
            :key="range.value"
            type="button"
            class="min-h-8 rounded-md px-3 text-sm font-semibold transition-colors disabled:cursor-wait"
            :class="rangeClass(range.value === activeRange)"
            :data-test="`range-${range.value}`"
            :aria-pressed="range.value === activeRange"
            :disabled="loading && pendingRange === range.value"
            @click="selectRange(range.value)"
          >
            {{ range.label }}
          </button>
        </div>
      </section>

      <div
        v-if="rangeError"
        class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300"
        data-test="range-error"
        role="alert"
      >
        <span>{{ rangeError }}</span>
        <button type="button" class="btn btn-secondary shrink-0 px-3 py-1.5 text-xs" @click="load(activeRange)">
          {{ t('common.refresh') }}
        </button>
      </div>

      <section
        v-if="activeGroup"
        class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white sm:grid-cols-4 xl:grid-cols-7 dark:border-dark-700 dark:bg-dark-800"
        :aria-label="`${activeGroup.name} 原生分组汇总`"
        data-test="group-summary"
      >
        <div
          v-for="field in groupSummaryFields"
          :key="field.key"
          class="min-h-[82px] min-w-0 border-b border-r border-gray-100 px-3 py-3 last:border-r-0 xl:border-b-0 dark:border-dark-700"
          data-test="group-summary-field"
          :data-field="field.key"
        >
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ field.label }}</div>
          <div class="mt-1.5 break-words font-mono text-sm font-semibold text-gray-900 dark:text-white" :class="{ 'text-emerald-700 dark:text-emerald-300': field.key === 'status' && activeGroup.status === 'active' }">
            {{ field.value }}
          </div>
        </div>
      </section>

      <div v-if="loading && !projection" class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div v-for="item in 4" :key="item" class="card h-[310px] animate-pulse bg-gray-100 dark:bg-dark-800" />
      </div>

      <div
        v-else-if="projection && !filteredAccounts.length"
        class="rounded-lg border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        data-test="account-empty"
      >
        {{ scopedAccounts.length ? t('admin.accountMonitor.empty.filtered') : t('admin.accountMonitor.empty.pool') }}
      </div>

      <section v-else class="grid grid-cols-1 gap-4 lg:grid-cols-2" data-test="account-card-grid" aria-label="账号排名卡片">
        <AccountMonitorCard
          v-for="account in filteredAccounts"
          :key="account.account_id"
          :account="account"
          :concurrency="concurrencyByID[account.account_id] ?? null"
          :ranked-account-count="rankedAccountCount"
          :ranking-scope="activeGroup ? 'group' : 'global'"
          :running="runningAll || runningAccountIDs.includes(account.account_id)"
          :statistics-cutoff="projection?.observed_at ?? null"
          :selected-range="activeRange"
          @refresh="handleRunOne"
          @update-priority="updatePriority"
          @update-procurement-cost="updateProcurementCost"
        />
      </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  AccountMonitorAccount,
  AccountMonitorConcurrencyItem,
  AccountMonitorGroup,
  AccountMonitorProjection,
  AccountMonitorRange,
} from '@/api/admin/accountMonitor'
import AccountMonitorCard from '@/components/admin/account-monitor/AccountMonitorCard.vue'
import AccountMonitorFilters from '@/components/admin/account-monitor/AccountMonitorFilters.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

type CardConcurrency = AccountMonitorConcurrencyItem & { delayed?: boolean }
type SaveCompletion = {
  resolve: () => void
  reject: (reason?: unknown) => void
}

const ranges: { value: AccountMonitorRange; label: string }[] = [
  { value: '24h', label: '24 小时' },
  { value: '7d', label: '7 天' },
  { value: '30d', label: '30 天' },
]
const groupSummaryLabels = {
  status: '分组状态',
  platform: '平台',
  rate_multiplier: '分组倍率',
  rpm_limit: 'RPM 上限',
  account_count: '原生账号数',
  active_account_count: '原生活跃账号数',
  rate_limited_account_count: '原生限流账号数',
} as const

const { t } = useI18n()
const appStore = useAppStore()
const projection = ref<AccountMonitorProjection | null>(null)
const activeRange = ref<AccountMonitorRange>('24h')
const pendingRange = ref<AccountMonitorRange | null>(null)
const activeGroupId = ref<number | null>(null)
const search = ref('')
const status = ref('')
const loading = ref(false)
const runningAll = ref(false)
const runningAccountIDs = ref<number[]>([])
const rangeError = ref<string | null>(null)
const concurrencyByID = ref<Record<number, CardConcurrency>>({})

let abortController: AbortController | null = null
let loadGeneration = 0
let pollTimer: number | null = null
let pollInFlight = false
let pollQueued = false

function uniqueAccounts(source: AccountMonitorAccount[]): AccountMonitorAccount[] {
  const seen = new Set<number>()
  return source.filter((account) => {
    if (seen.has(account.account_id)) return false
    seen.add(account.account_id)
    return true
  })
}

function compareAccounts(left: AccountMonitorAccount, right: AccountMonitorAccount): number {
  const leftRanked = left.group_rank != null
  const rightRanked = right.group_rank != null
  if (leftRanked && rightRanked) {
    const rankDifference = Number(left.group_rank) - Number(right.group_rank)
    if (rankDifference !== 0) return rankDifference
    return left.account_id - right.account_id
  }
  if (leftRanked) return -1
  if (rightRanked) return 1
  return left.account_id - right.account_id
}

const allAccounts = computed(() => uniqueAccounts(projection.value?.accounts ?? []))
const sortedGroups = computed(() => [...(projection.value?.groups ?? [])].sort((left, right) => {
  if (left.native_order !== right.native_order) return left.native_order - right.native_order
  return left.id - right.id
}))
const activeGroup = computed(() => sortedGroups.value.find((group) => group.id === activeGroupId.value) ?? null)
const scopedAccounts = computed(() => {
  const group = activeGroup.value
  const source = group?.accounts ?? (group
    ? allAccounts.value.filter((account) => account.group_ids.includes(group.id))
    : allAccounts.value)
  return uniqueAccounts(source).sort(compareAccounts)
})
const filteredAccounts = computed(() => {
  const query = search.value.trim().toLowerCase()
  return scopedAccounts.value.filter((account) => {
    if (status.value && account.monitor_bucket !== status.value) return false
    if (!query) return true
    return [account.name, String(account.account_id), account.platform, account.model_id, ...account.group_names]
      .some((value) => value.toLowerCase().includes(query))
  })
})
const rankedAccountCount = computed(() => scopedAccounts.value.filter((account) => account.group_rank != null).length)
const visibleAccountIDs = computed(() => filteredAccounts.value.map((account) => account.account_id))
const visibleAccountIDKey = computed(() => visibleAccountIDs.value.join(','))
const groupSummaryFields = computed(() => {
  const group = activeGroup.value
  if (!group) return []
  return [
    { key: 'status', label: groupSummaryLabels.status, value: formatGroupStatus(group.status) },
    { key: 'platform', label: groupSummaryLabels.platform, value: group.platform || '--' },
    { key: 'rate_multiplier', label: groupSummaryLabels.rate_multiplier, value: formatMultiplier(group.rate_multiplier) },
    { key: 'rpm_limit', label: groupSummaryLabels.rpm_limit, value: formatNativeNumber(group.rpm_limit) },
    { key: 'account_count', label: groupSummaryLabels.account_count, value: formatNativeNumber(group.account_count) },
    { key: 'active_account_count', label: groupSummaryLabels.active_account_count, value: formatNativeNumber(group.active_account_count) },
    { key: 'rate_limited_account_count', label: groupSummaryLabels.rate_limited_account_count, value: formatNativeNumber(group.rate_limited_account_count) },
  ]
})

function tabClass(selected: boolean): string {
  return selected
    ? 'text-primary-700 after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-primary-600 dark:text-primary-300'
    : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'
}
function rangeClass(selected: boolean): string {
  return selected
    ? 'bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
    : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'
}
function formatGroupStatus(value?: string): string {
  if (value === 'active') return '启用'
  if (value === 'inactive') return '停用'
  return value || '--'
}
function formatMultiplier(value?: number): string {
  return value == null || !Number.isFinite(value) ? '--' : `${value.toFixed(2)}×`
}
function formatNativeNumber(value?: number): string {
  return value == null || !Number.isFinite(value) ? '--' : String(value)
}
function groupAccountCount(group: AccountMonitorGroup): number {
  return group.account_count ?? uniqueAccounts(group.accounts ?? []).length
}

function selectGroup(groupID: number | null, event: MouseEvent): void {
  activeGroupId.value = groupID
  if (event.detail > 0) (event.currentTarget as HTMLButtonElement).blur()
}

async function load(range: AccountMonitorRange): Promise<boolean> {
  abortController?.abort()
  const controller = new AbortController()
  const generation = ++loadGeneration
  abortController = controller
  loading.value = true
  pendingRange.value = range
  rangeError.value = null
  try {
    const result = await adminAPI.accountMonitor.list(range, { signal: controller.signal })
    if (controller.signal.aborted || generation !== loadGeneration) return false
    if (result.range !== range) {
      throw new Error(`账号监控统计范围不匹配：请求 ${range}，返回 ${result.range ?? '缺失'}`)
    }
    projection.value = result
    activeRange.value = range
    if (activeGroupId.value !== null && !(result.groups ?? []).some((group) => group.id === activeGroupId.value)) {
      activeGroupId.value = null
    }
    return true
  } catch (reason: unknown) {
    if (controller.signal.aborted || generation !== loadGeneration) return false
    rangeError.value = extractApiErrorMessage(reason, t('admin.accountMonitor.loadError'))
    appStore.showError(rangeError.value)
    return false
  } finally {
    if (generation === loadGeneration) {
      abortController = null
      loading.value = false
      pendingRange.value = null
    }
  }
}

function selectRange(range: AccountMonitorRange) {
  if (range === activeRange.value && !rangeError.value) return
  void load(range)
}

async function pollConcurrency(): Promise<void> {
  if (document.hidden) return
  if (pollInFlight) {
    pollQueued = true
    return
  }
  const accountIDs = [...new Set(visibleAccountIDs.value)]
  if (!accountIDs.length) return
  pollInFlight = true
  try {
    const response = await adminAPI.accountMonitor.getConcurrency(accountIDs)
    const next = { ...concurrencyByID.value }
    for (const item of response.items) {
      if (accountIDs.includes(item.account_id)) next[item.account_id] = { ...item, delayed: false }
    }
    concurrencyByID.value = next
  } catch {
    const next = { ...concurrencyByID.value }
    for (const accountID of accountIDs) {
      const previous = next[accountID]
      if (previous) next[accountID] = { ...previous, delayed: true }
    }
    concurrencyByID.value = next
  } finally {
    pollInFlight = false
    if (pollQueued && !document.hidden) {
      pollQueued = false
      void pollConcurrency()
    }
  }
}

function handleVisibilityChange() {
  if (!document.hidden) {
    pollQueued = false
    void pollConcurrency()
  }
}

async function handleRunAll() {
  if (runningAll.value) return
  runningAll.value = true
  try {
    await adminAPI.accountMonitor.runAll()
    await load(activeRange.value)
    appStore.showSuccess(t('admin.accountMonitor.messages.refreshAllSuccess'))
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    runningAll.value = false
  }
}

async function handleRunOne(accountID: number) {
  if (runningAccountIDs.value.includes(accountID)) return
  runningAccountIDs.value = [...runningAccountIDs.value, accountID]
  try {
    await adminAPI.accountMonitor.runOne(accountID)
    await load(activeRange.value)
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    runningAccountIDs.value = runningAccountIDs.value.filter((id) => id !== accountID)
  }
}

async function updatePriority(accountID: number, priority: number, completion: SaveCompletion) {
  try {
    await adminAPI.accounts.update(accountID, { priority })
    await load(activeRange.value)
    completion.resolve()
    appStore.showSuccess('账号调度优先级已更新')
  } catch (reason: unknown) {
    completion.reject(reason)
    appStore.showError(extractApiErrorMessage(reason, '账号调度优先级更新失败'))
  }
}

async function updateProcurementCost(accountID: number, cost: number | null, completion: SaveCompletion) {
  try {
    await adminAPI.accounts.updateProcurementCost(accountID, cost)
    await load(activeRange.value)
    completion.resolve()
    appStore.showSuccess(cost == null ? '采购成本已清空' : '采购成本已更新')
  } catch (reason: unknown) {
    completion.reject(reason)
    appStore.showError(extractApiErrorMessage(reason, cost == null ? '清空采购成本失败' : '保存采购成本失败'))
  }
}

watch(visibleAccountIDKey, (next, previous) => {
  if (next && next !== previous) void pollConcurrency()
})

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  pollTimer = window.setInterval(() => {
    if (!document.hidden) void pollConcurrency()
  }, 5000)
  void load('24h')
})

onBeforeUnmount(() => {
  abortController?.abort()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  if (pollTimer !== null) window.clearInterval(pollTimer)
})
</script>
