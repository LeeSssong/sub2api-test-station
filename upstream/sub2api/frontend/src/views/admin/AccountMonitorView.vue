<template>
  <AppLayout>
    <div class="flex min-h-full flex-col gap-4 p-4 sm:p-6">
      <section class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accountMonitor.title') }}
            </h1>
            <span class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
              {{ t('admin.accountMonitor.monitoredCount', { count: filteredAccounts.length }) }}
            </span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.accountMonitor.description') }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm dark:border-dark-700 dark:bg-dark-800">
            <Icon name="clock" size="sm" class="text-gray-400" />
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accountMonitor.interval') }}</span>
            <span class="font-mono font-semibold text-gray-900 dark:text-white">{{ intervalLabel }}</span>
            <button
              type="button"
              class="icon-button -mr-2"
              :title="t('admin.accountMonitor.actions.settings')"
              :aria-label="t('admin.accountMonitor.actions.settings')"
              data-test="open-settings"
              @click="showSettings = true"
            >
              <Icon name="cog" size="sm" />
            </button>
          </div>
          <button
            type="button"
            class="btn btn-primary"
            data-test="run-all"
            :disabled="runningAll || loading"
            @click="handleRunAll"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': runningAll }" />
            {{ runningAll ? t('admin.accountMonitor.actions.running') : t('admin.accountMonitor.actions.refreshAll') }}
          </button>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800" data-test="operations-overview">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">全站经营总览</h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">默认今日 · 成本仅采用已对账的真实上游账单</p>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" data-test="open-ledger-history" @click="openLedgerHistory()">历史按日</button>
            <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" data-test="open-exceptions" @click="openExceptions">异常明细</button>
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
          <LedgerMetric label="用户实际计费" :value="formatMoney(globalLedger?.user_charge, globalLedger?.currency)" />
          <LedgerMetric label="上游真实扣费" :value="formatMoney(globalLedger?.upstream_cost, globalLedger?.currency)" />
          <LedgerMetric label="纸面利润" :value="formatMoney(globalLedger?.paper_profit, globalLedger?.currency)" />
          <LedgerMetric label="利润率" :value="formatProfitMargin(globalLedger)" />
          <LedgerMetric label="成本覆盖率" :value="formatCoverage(globalLedger)" :value-class="coverageClass(globalLedger)" />
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/70">
            <p class="text-xs text-gray-500 dark:text-gray-400">服务健康</p>
            <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white"><span class="text-green-600">{{ availableCount }}</span><span class="mx-1 text-gray-400">/</span>{{ accounts.length }}</p>
            <p class="mt-0.5 text-xs text-gray-500">{{ unavailableCount }} 个不可用</p>
          </div>
        </div>
      </section>

      <section v-if="sortedGroups.length" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex gap-2 overflow-x-auto pb-1" role="tablist" aria-label="账号分组">
          <div v-for="group in sortedGroups" :key="group.id" data-test="group-tab">
            <button
              type="button"
              role="tab"
              :aria-selected="activeGroupId === group.id"
              class="shrink-0 rounded-lg border px-3 py-2 text-left text-sm transition-colors"
              :class="activeGroupId === group.id ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-gray-300'"
              :data-test="`group-tab-${group.id}`"
              @click="selectGroup(group.id)"
            >
              <span class="block font-medium">{{ group.name }}</span>
              <span class="mt-0.5 block text-[11px] opacity-75">{{ formatMultiplier(group.rate_multiplier) }} · {{ groupOperationalLabel(group) }}</span>
            </button>
          </div>
        </div>
      </section>

      <section class="flex flex-col gap-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800 sm:flex-row sm:items-center">
        <AccountMonitorFilters
          :search="search"
          :platform="platform"
          :status="status"
          :group-id="groupId"
          :accounts="accounts"
          @update:search="search = $event"
          @update:platform="platform = $event"
          @update:status="status = $event"
          @update:group-id="selectFilterGroup"
        />
        <div class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accountMonitor.lastObserved', { time: formatDate(projection?.observed_at) }) }}
        </div>
      </section>

      <div v-if="loading && !accounts.length" class="grid gap-4 xl:grid-cols-2">
        <div v-for="item in 4" :key="item" class="card h-[330px] animate-pulse bg-gray-100 dark:bg-dark-800" />
      </div>

      <div
        v-else-if="error"
        class="rounded-lg border border-red-200 bg-red-50 p-5 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300"
      >
        {{ error }}
        <button type="button" class="btn btn-secondary ml-3 px-3 py-1.5 text-xs" @click="load">
          {{ t('common.refresh') }}
        </button>
      </div>

      <div
        v-else-if="!filteredAccounts.length"
        class="rounded-lg border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      >
        {{ accounts.length ? t('admin.accountMonitor.empty.filtered') : t('admin.accountMonitor.empty.pool') }}
      </div>

      <div v-else class="space-y-4">
        <section v-if="activeGroup" class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800" data-test="group-operations-overview">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ activeGroup.name }}</h2>
                <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="activeGroup.operational_state === 'closed' ? 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300' : activeGroup.operational_state === 'operational' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'">{{ groupOperationalLabel(activeGroup) }}</span>
              </div>
              <p v-if="activeGroup.operational_state === 'closed'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前分组未向用户开放，空账号不会作为服务故障持续告警。</p>
              <p v-else class="mt-1 text-xs text-gray-500 dark:text-gray-400">按质量评分从高到低，仅影响监控展示；全局调度优先级仍由账号卡片直接修改。</p>
            </div>
            <div class="flex items-center gap-2">
              <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" @click="openLedgerHistory({ group_id: activeGroup.id })">本组历史</button>
              <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" data-test="edit-group-score-weights" @click="showScoreDialog = true">评分权重</button>
            </div>
          </div>
          <div class="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <LedgerMetric label="用户实际计费" :value="formatMoney(groupLedger?.user_charge, groupLedger?.currency)" />
            <LedgerMetric label="上游真实扣费" :value="formatMoney(groupLedger?.upstream_cost, groupLedger?.currency)" />
            <LedgerMetric label="纸面利润" :value="formatMoney(groupLedger?.paper_profit, groupLedger?.currency)" />
            <LedgerMetric label="利润率" :value="formatProfitMargin(groupLedger)" />
            <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/70"><p class="text-xs text-gray-500">服务健康</p><p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ selectedAvailableCount }} / {{ scopedAccounts.length }}</p><p class="mt-0.5 text-xs text-gray-500">{{ groupLedger?.pending_attempts ?? 0 }} 笔待对账</p></div>
          </div>
          <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">评分组成：成本优势 {{ activeGroup.score_weights.cost }}% · 成功率 {{ activeGroup.score_weights.success }}% · TTFT {{ activeGroup.score_weights.ttft }}% · 总耗时 {{ activeGroup.score_weights.latency }}%</p>
        </section>

        <section data-test="monitor-group">
          <div class="grid gap-4 xl:grid-cols-2">
            <div v-for="account in scopedAccounts" :key="account.account_id" class="relative">
              <div v-if="account.quality_score != null" class="absolute right-4 top-3 z-10 rounded-full bg-primary-600 px-2 py-1 text-xs font-semibold text-white">{{ account.quality_score }} 分</div>
              <AccountMonitorCard
                :account="account"
                :operations="accountLedgers[account.account_id] ?? null"
                :group-operational-state="activeGroup?.operational_state"
                :running="runningAccounts.has(account.account_id)"
                :saving-weight="savingWeights.has(account.account_id)"
                @refresh="handleRunOne"
                @update-priority="updateWeight"
                @update-weight="updateWeight"
                @settings="showSettings = true"
                @history="openHistory"
              />
            </div>
          </div>
        </section>
      </div>
    </div>

    <AccountMonitorSettingsDialog
      :show="showSettings"
      :interval-seconds="intervalSeconds"
      :saving="savingSettings"
      :error="settingsError"
      @close="showSettings = false"
      @save="saveSettings"
    />

    <AccountMonitorGroupScoreDialog
      v-if="activeGroup"
      :show="showScoreDialog"
      :group-id="activeGroup.id"
      :group-name="activeGroup.name"
      :weights="activeGroup.score_weights"
      :saving="savingScoreWeights"
      :error="scoreWeightsError"
      @close="showScoreDialog = false"
      @save="saveGroupScoreWeights"
      @reset="resetGroupScoreWeights"
    />

    <AccountMonitorLedgerHistoryDrawer
      :show="ledgerHistoryScope !== null"
      :scope="ledgerHistoryScope ?? undefined"
      @close="ledgerHistoryScope = null"
    />

    <BaseDialog
      :show="historyAccount !== null"
      :title="t('admin.accountMonitor.history.title')"
      width="wide"
      @close="historyAccount = null"
    >
      <div v-if="historyLoading" class="py-8 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="historyItems.length" class="overflow-x-auto">
        <table class="min-w-full text-left text-sm">
          <thead class="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-dark-700 dark:text-gray-400">
            <tr>
              <th class="px-3 py-2">{{ t('admin.accountMonitor.history.checkedAt') }}</th>
              <th class="px-3 py-2">{{ t('admin.accountMonitor.history.status') }}</th>
              <th class="px-3 py-2">{{ t('admin.accountMonitor.history.ttft') }}</th>
              <th class="px-3 py-2">{{ t('admin.accountMonitor.history.latency') }}</th>
              <th class="px-3 py-2">{{ t('admin.accountMonitor.history.error') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in historyItems" :key="`${item.checked_at}-${item.model_id}`" class="border-b border-gray-100 dark:border-dark-800">
              <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">{{ formatDate(item.checked_at) }}</td>
              <td class="px-3 py-2">{{ item.status }}</td>
              <td class="px-3 py-2 font-mono">{{ formatMs(item.ttft_ms) }}</td>
              <td class="px-3 py-2 font-mono">{{ formatMs(item.latency_ms) }}</td>
              <td class="px-3 py-2 text-red-600 dark:text-red-400">{{ item.error_code || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.accountMonitor.history.empty') }}
      </div>
    </BaseDialog>

    <BaseDialog :show="exceptionsOpen" title="对账异常明细" width="wide" @close="exceptionsOpen = false">
      <div v-if="exceptionsLoading" class="py-8 text-center text-sm text-gray-500">加载中...</div>
      <div v-else-if="exceptions.length" class="space-y-3">
        <div v-for="item in exceptions" :key="item.id" class="rounded-lg border border-amber-200 p-3 dark:border-amber-900/50">
          <div class="grid gap-1 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
            <span>本地请求：<code>{{ item.attempt.local_request_id }}</code></span>
            <span>上游请求：<code>{{ item.attempt.upstream_request_id || '-' }}</code></span>
            <span>模型：{{ item.attempt.model }}</span>
            <span>原因：{{ item.reason_code }}</span>
          </div>
          <form class="mt-3 flex flex-wrap items-end gap-2" @submit.prevent="submitAdjustment(item)">
            <label class="text-xs text-gray-500">上游实际扣费
              <input v-model="adjustmentAmounts[item.attempt.id]" class="input mt-1 w-32" inputmode="decimal" required placeholder="0.000000" />
            </label>
            <button type="submit" class="btn btn-primary" :disabled="adjustingID === item.attempt.id">补登记</button>
          </form>
        </div>
      </div>
      <div v-else class="py-8 text-center text-sm text-gray-500">当前没有未解决异常</div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  AccountMonitorAccount,
  AccountMonitorGroup,
  AccountMonitorHistoryItem,
  AccountMonitorProjection,
  AccountMonitorScoreWeights,
} from '@/api/admin/accountMonitor'
import type { OperationsScopeParams, ReconciliationException, ReconciliationSummary } from '@/api/admin/reconciliation'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import AccountMonitorCard from '@/components/admin/account-monitor/AccountMonitorCard.vue'
import AccountMonitorFilters from '@/components/admin/account-monitor/AccountMonitorFilters.vue'
import AccountMonitorGroupScoreDialog from '@/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue'
import AccountMonitorLedgerHistoryDrawer from '@/components/admin/account-monitor/AccountMonitorLedgerHistoryDrawer.vue'
import AccountMonitorSettingsDialog from '@/components/admin/account-monitor/AccountMonitorSettingsDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const projection = ref<AccountMonitorProjection | null>(null)
const accounts = ref<AccountMonitorAccount[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const search = ref('')
const platform = ref('')
const status = ref('')
const groupId = ref('')
const runningAll = ref(false)
const runningAccounts = ref(new Set<number>())
const savingWeights = ref(new Set<number>())
const showSettings = ref(false)
const savingSettings = ref(false)
const settingsError = ref<string | null>(null)
const historyAccount = ref<number | null>(null)
const historyLoading = ref(false)
const historyItems = ref<AccountMonitorHistoryItem[]>([])
const globalLedger = ref<ReconciliationSummary | null>(null)
const groupLedger = ref<ReconciliationSummary | null>(null)
const accountLedgers = ref<Record<number, ReconciliationSummary | null>>({})
const activeGroupId = ref<number | null>(null)
const ledgerHistoryScope = ref<OperationsScopeParams | null>(null)
const showScoreDialog = ref(false)
const savingScoreWeights = ref(false)
const scoreWeightsError = ref<string | null>(null)
const exceptions = ref<ReconciliationException[]>([])
const exceptionsOpen = ref(false)
const exceptionsLoading = ref(false)
const adjustmentAmounts = ref<Record<number, string>>({})
const adjustingID = ref<number | null>(null)

let abortController: AbortController | null = null
let operationsGeneration = 0

const intervalSeconds = computed(() => projection.value?.settings.interval_seconds ?? 300)
const intervalLabel = computed(() => {
  const seconds = intervalSeconds.value
  if (seconds % 60 === 0) return t('admin.accountMonitor.intervalMinutes', { count: seconds / 60 })
  return t('admin.accountMonitor.intervalSeconds', { count: seconds })
})

const sortedGroups = computed(() => [...(projection.value?.groups ?? [])].sort((left, right) => {
  if (right.rate_multiplier !== left.rate_multiplier) return right.rate_multiplier - left.rate_multiplier
  if (left.native_order !== right.native_order) return left.native_order - right.native_order
  return left.id - right.id
}))
const activeGroup = computed<AccountMonitorGroup | null>(() => sortedGroups.value.find((group) => group.id === activeGroupId.value) ?? null)
const activeGroupAccountSource = computed(() => {
  const group = activeGroup.value
  if (group?.accounts?.length) return group.accounts
  if (group) return accounts.value.filter((account) => account.group_ids.includes(group.id))
  return accounts.value
})

const filteredAccounts = computed(() => {
  const query = search.value.trim().toLowerCase()
  return activeGroupAccountSource.value.filter((account) => {
    if (platform.value && account.platform !== platform.value) return false
    if (status.value && displayStatus(account) !== status.value) return false
    if (groupId.value && !account.group_ids.includes(Number(groupId.value))) return false
    if (!query) return true
    return [
      account.name,
      String(account.account_id),
      account.platform,
      account.model_id,
      ...account.group_names,
    ].some((value) => value.toLowerCase().includes(query))
  })
})
const scopedAccounts = computed(() => [...filteredAccounts.value].sort((left, right) => {
  const leftScore = left.quality_score ?? -1
  const rightScore = right.quality_score ?? -1
  if (rightScore !== leftScore) return rightScore - leftScore
  const leftRank = left.group_rank ?? Number.MAX_SAFE_INTEGER
  const rightRank = right.group_rank ?? Number.MAX_SAFE_INTEGER
  if (leftRank !== rightRank) return leftRank - rightRank
  return left.account_id - right.account_id
}))
const availableCount = computed(() => accounts.value.filter((account) => account.latest_status === 'success').length)
const unavailableCount = computed(() => Math.max(0, accounts.value.length - availableCount.value))
const selectedAvailableCount = computed(() => scopedAccounts.value.filter((account) => account.latest_status === 'success' && !account.stale && account.eligible !== false).length)

function displayStatus(account: AccountMonitorAccount): string {
  if (account.stale) return 'stale'
  return account.latest_status || 'unavailable'
}

async function load() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  error.value = null
  try {
    const result = await adminAPI.accountMonitor.list({ signal: controller.signal })
    if (controller.signal.aborted || abortController !== controller) return
    projection.value = result
    accounts.value = result.accounts.filter((account) => account.status === 'active' && account.schedulable)
    ensureActiveGroup()
    await loadOperations()
  } catch (err: unknown) {
    if (controller.signal.aborted) return
    error.value = extractApiErrorMessage(err, t('admin.accountMonitor.loadError'))
    appStore.showError(error.value)
  } finally {
    if (abortController === controller) {
      abortController = null
      loading.value = false
    }
  }
}

function ensureActiveGroup() {
  if (activeGroupId.value !== null && sortedGroups.value.some((group) => group.id === activeGroupId.value)) return
  activeGroupId.value = sortedGroups.value[0]?.id ?? null
  groupId.value = activeGroupId.value === null ? '' : String(activeGroupId.value)
}

async function loadOperations() {
  const groupID = activeGroupId.value
  const generation = ++operationsGeneration
  const isCurrent = () => generation === operationsGeneration && activeGroupId.value === groupID
  const visibleAccounts = scopedAccounts.value
  const calls: Promise<void>[] = [
    adminAPI.reconciliation.operations({})
      .then((summary) => {
        if (isCurrent()) globalLedger.value = summary
      })
      .catch(() => {
        if (isCurrent()) globalLedger.value = null
      }),
  ]
  if (groupID !== null) {
    calls.push(adminAPI.reconciliation.operations({ group_id: groupID })
      .then((summary) => {
        if (isCurrent()) groupLedger.value = summary
      })
      .catch(() => {
        if (isCurrent()) groupLedger.value = null
      }))
  } else {
    if (isCurrent()) groupLedger.value = null
  }
  const nextLedgers: Record<number, ReconciliationSummary | null> = {}
  for (const account of visibleAccounts) {
    calls.push(adminAPI.reconciliation.operations({ group_id: groupID ?? undefined, account_id: account.account_id })
      .then((summary) => { nextLedgers[account.account_id] = summary })
      .catch(() => { nextLedgers[account.account_id] = null }))
  }
  await Promise.all(calls)
  if (isCurrent()) accountLedgers.value = nextLedgers
}

async function selectGroup(groupID: number) {
  activeGroupId.value = groupID
  groupId.value = String(groupID)
  await loadOperations()
}

function selectFilterGroup(value: string) {
  groupId.value = value
  const parsed = Number(value)
  if (Number.isInteger(parsed) && parsed > 0) {
    void selectGroup(parsed)
    return
  }
  activeGroupId.value = sortedGroups.value[0]?.id ?? null
  void loadOperations()
}

async function handleRunAll() {
  if (runningAll.value) return
  runningAll.value = true
  try {
    await adminAPI.accountMonitor.runAll()
    await adminAPI.reconciliation.refresh()
    await load()
    appStore.showSuccess(t('admin.accountMonitor.messages.refreshAllSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    runningAll.value = false
  }
}

async function openExceptions() {
  exceptionsOpen.value = true
  exceptionsLoading.value = true
  try {
    exceptions.value = (await adminAPI.reconciliation.exceptions({ limit: 100 })).items
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '对账异常加载失败'))
  } finally {
    exceptionsLoading.value = false
  }
}

async function submitAdjustment(item: ReconciliationException) {
  const amount = adjustmentAmounts.value[item.attempt.id]?.trim()
  if (!amount) return
  adjustingID.value = item.attempt.id
  try {
    await adminAPI.reconciliation.adjust(item.attempt.id, amount)
    exceptions.value = exceptions.value.filter((current) => current.id !== item.id)
    await loadOperations()
    appStore.showSuccess('补登记成功')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '补登记失败'))
  } finally {
    adjustingID.value = null
  }
}

async function handleRunOne(accountID: number) {
  if (runningAccounts.value.has(accountID)) return
  runningAccounts.value = new Set(runningAccounts.value).add(accountID)
  try {
    await adminAPI.accountMonitor.runOne(accountID)
    await load()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    const next = new Set(runningAccounts.value)
    next.delete(accountID)
    runningAccounts.value = next
  }
}

async function updateWeight(accountID: number, priority: number) {
  if (savingWeights.value.has(accountID)) return
  savingWeights.value = new Set(savingWeights.value).add(accountID)
  try {
    const updated = await adminAPI.accounts.update(accountID, { priority })
    const account = accounts.value.find((item) => item.account_id === accountID)
    if (account) account.priority = updated.priority
    await load()
    appStore.showSuccess('全局调度优先级已更新')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '账号权重更新失败'))
  } finally {
    const next = new Set(savingWeights.value)
    next.delete(accountID)
    savingWeights.value = next
  }
}

async function saveGroupScoreWeights(weights: Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency'>) {
  if (!activeGroup.value) return
  savingScoreWeights.value = true
  scoreWeightsError.value = null
  try {
    const updated = await adminAPI.accountMonitor.updateGroupScoreWeights(activeGroup.value.id, weights)
    projection.value = projection.value
      ? { ...projection.value, groups: (projection.value.groups ?? []).map((group) => group.id === activeGroup.value?.id ? { ...group, score_weights: updated } : group) }
      : projection.value
    showScoreDialog.value = false
    await load()
    appStore.showSuccess('分组评分权重已更新')
  } catch (err: unknown) {
    scoreWeightsError.value = extractApiErrorMessage(err, '分组评分权重更新失败')
  } finally {
    savingScoreWeights.value = false
  }
}

async function resetGroupScoreWeights() {
  if (!activeGroup.value) return
  savingScoreWeights.value = true
  scoreWeightsError.value = null
  try {
    const updated = await adminAPI.accountMonitor.resetGroupScoreWeights(activeGroup.value.id)
    projection.value = projection.value
      ? { ...projection.value, groups: (projection.value.groups ?? []).map((group) => group.id === activeGroup.value?.id ? { ...group, score_weights: updated } : group) }
      : projection.value
    await load()
    appStore.showSuccess('已恢复默认评分权重')
  } catch (err: unknown) {
    scoreWeightsError.value = extractApiErrorMessage(err, '恢复默认评分权重失败')
  } finally {
    savingScoreWeights.value = false
  }
}

async function saveSettings(interval: number) {
  savingSettings.value = true
  settingsError.value = null
  try {
    const settings = await adminAPI.accountMonitor.updateSettings(interval)
    if (projection.value) projection.value = { ...projection.value, settings }
    showSettings.value = false
    appStore.showSuccess(t('admin.accountMonitor.messages.settingsSaved'))
  } catch (err: unknown) {
    settingsError.value = extractApiErrorMessage(err, t('admin.accountMonitor.messages.settingsFailed'))
  } finally {
    savingSettings.value = false
  }
}

async function openHistory(accountID: number) {
  historyAccount.value = accountID
  historyLoading.value = true
  historyItems.value = []
  try {
    const response = await adminAPI.accountMonitor.history(accountID, 25)
    historyItems.value = response.items
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accountMonitor.history.loadError')))
    historyAccount.value = null
  } finally {
    historyLoading.value = false
  }
}

function formatMs(value?: number | null): string {
  return value == null ? '-' : `${Math.round(value)} ms`
}

function formatDate(value?: string | null): string {
  return value ? new Date(value).toLocaleString() : t('common.time.never')
}

function openLedgerHistory(scope: OperationsScopeParams = {}) {
  ledgerHistoryScope.value = scope
}

function coverageClass(summary?: { coverage_known: boolean; coverage_ratio: number } | null): string {
  if (!summary?.coverage_known) return 'text-gray-500'
  return Number(summary.coverage_ratio) >= 1 ? 'text-green-600' : 'text-amber-600'
}

function formatCoverage(summary?: { coverage_known: boolean; coverage_ratio: number } | null): string {
  return !summary?.coverage_known ? '-' : `${(summary.coverage_ratio * 100).toFixed(2)}%`
}

function formatProfitMargin(summary?: ReconciliationSummary | null): string {
  if (!summary?.coverage_known || Number(summary.pending_attempts) > 0) return '待对账'
  const margin = Number(summary.profit_margin)
  return Number.isFinite(margin) ? `${(margin * 100).toFixed(1)}%` : '—'
}

function formatMoney(value?: string | number | null, currency?: string | null): string {
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

function formatMultiplier(value: number): string {
  return `${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(value)}x`
}

function groupOperationalLabel(group: AccountMonitorGroup): string {
  if (group.operational_state === 'closed') return '已关闭'
  if (group.operational_state === 'operational') return '运行中'
  return '暂无可用账号'
}

const LedgerMetric = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    valueClass: { type: String, default: 'text-gray-900 dark:text-white' },
  },
  setup(metricProps) {
    return () => h('div', { class: 'rounded-lg bg-gray-50 p-3 dark:bg-dark-900/70' }, [
      h('p', { class: 'text-xs text-gray-500 dark:text-gray-400' }, metricProps.label),
      h('p', { class: ['mt-1 font-mono text-lg font-semibold', metricProps.valueClass] }, metricProps.value),
    ])
  },
})

onMounted(load)
onUnmounted(() => abortController?.abort())
</script>
