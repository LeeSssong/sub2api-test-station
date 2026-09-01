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

      <section
        v-if="!activeGroup"
        class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800"
        aria-label="当前调度排名规则"
      >
        <div class="min-w-0">
          <div class="text-sm font-medium text-gray-900 dark:text-white">当前调度排名规则</div>
          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">成功率优先，其次 TTFT、有效成本 U 和账号 ID；旧评分权重不参与当前调度。</div>
        </div>
      </section>

      <div
        v-if="rangeError && projection"
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
        class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white sm:grid-cols-4 xl:grid-cols-8 dark:border-dark-700 dark:bg-dark-800"
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

      <div v-if="loading && !projection" class="grid grid-cols-1 gap-4">
        <div v-for="item in 4" :key="item" class="card h-[310px] animate-pulse bg-gray-100 dark:bg-dark-800" />
      </div>

      <div
        v-else-if="rangeError && !projection"
        class="flex flex-col items-center gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300"
        data-test="account-monitor-error-empty"
        role="alert"
      >
        <span>{{ rangeError }}</span>
        <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" @click="load(activeRange)">
          {{ t('common.refresh') }}
        </button>
      </div>

      <div
        v-else-if="projection && !filteredAccounts.length"
        class="rounded-lg border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        data-test="account-empty"
      >
        {{ scopedAccounts.length ? t('admin.accountMonitor.empty.filtered') : t('admin.accountMonitor.empty.pool') }}
      </div>

      <div v-if="nativeAccountLoading" class="text-xs text-gray-500 dark:text-gray-400" data-test="account-native-loading" role="status">
        正在加载账号详情…
      </div>
      <div v-if="nativeAccountError" class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300" data-test="account-native-error" role="alert">
        <span>{{ nativeAccountError }}</span>
        <button type="button" class="btn btn-secondary shrink-0 px-3 py-1.5 text-xs" @click="nativeAccountError = null">关闭</button>
      </div>

      <section v-if="projection && filteredAccounts.length" class="grid grid-cols-1 gap-4" data-test="account-card-grid" aria-label="账号排名卡片">
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
          :model-detection-models="modelDetectionModelsByID[account.account_id] ?? null"
          :saving-model-detection="savingModelDetectionIDs.includes(account.account_id)"
          :detecting-model-detection="detectingModelDetectionIDs.includes(account.account_id)"
          :use-history-panel="true"
          @refresh="handleRunOne"
          @edit-connection-probe-model="loadModelDetectionModels"
          @save-model-detection-models="saveModelDetectionModels"
          @detect-model-detection="enqueueModelDetection"
          @open-model-detection-history="openModelDetectionHistory"
          @update-priority="updatePriority"
          @edit-cost="openCostDialog"
          @account-info="openAccountInfo"
          @account-edit="openAccountEdit"
          @account-delete="openAccountDelete"
          @account-more="openAccountMore"
        />
      </section>
      </div>
      <AccountMonitorCostDialog
        v-if="showCostDialog && selectedCostAccount"
        :show="showCostDialog"
        :account="selectedCostAccount"
        :saving="savingCost"
        :error="costDialogError"
        @close="closeCostDialog"
        @save-procurement="saveProcurementCost"
        @save-multiplier="saveAccountMultiplier"
        @restore-auto="restoreAccountMultiplier"
        @clear="clearProcurementCost"
      />
      <AccountMonitorAccountInfoDialog
        :show="showAccountInfoDialog"
        :account="selectedNativeAccount"
        @close="closeAccountInfo"
      />
      <EditAccountModal
        :show="showEditAccountDialog"
        :account="selectedNativeAccount"
        :proxies="editProxies"
        :groups="editGroups"
        @close="closeEditAccount"
        @updated="handleNativeAccountUpdated"
      />
      <ConfirmDialog
        :show="showDeleteAccountDialog"
        title="删除账号"
        :message="`确定要删除账号“${selectedNativeAccount?.name ?? ''}”吗？此操作不可撤销。`"
        confirm-text="删除"
        cancel-text="取消"
        :danger="true"
        @confirm="confirmDeleteAccount"
        @cancel="closeDeleteAccount"
      />
      <ConfirmDialog
        :show="showSparkShadowDialog"
        title="创建 Spark 影子账号"
        :message="`确定要为账号“${creatingSparkShadow?.name ?? ''}”创建 Spark 影子账号吗？`"
        confirm-text="创建"
        cancel-text="取消"
        @confirm="confirmCreateSparkShadow"
        @cancel="showSparkShadowDialog = false"
      />
      <AccountActionMenu
        :show="accountActionMenu.show"
        :account="selectedNativeAccount"
        :position="accountActionMenu.position"
        @close="closeAccountMore"
        @test="handleTest"
        @stats="handleViewStats"
        @schedule="handleSchedule"
        @duplicate="handleDuplicateAccount"
        @reauth="handleReAuth"
        @refresh-token="handleRefresh"
        @recover-state="handleRecoverState"
        @reset-quota="handleResetQuota"
        @set-privacy="handleSetPrivacy"
        @create-spark-shadow="handleCreateSparkShadow"
      />
      <AccountTestModal :show="showTestDialog" :account="testingAccount" @close="closeTestDialog" />
      <AccountStatsModal :show="showStatsDialog" :account="statsAccount" @close="closeStatsDialog" />
      <ScheduledTestsPanel :show="showScheduleDialog" :account-id="scheduleAccount?.id ?? null" :model-options="scheduleModelOptions" @close="closeScheduleDialog" />
      <ReAuthAccountModal :show="showReAuthDialog" :account="reAuthAccount" @close="closeReAuthDialog" @reauthorized="handleNativeAccountUpdated" />
      <AccountModelDetectionHistoryPanel :show="showDetectionHistoryPanel" :account="selectedDetectionHistoryAccount" @close="closeModelDetectionHistory" />
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
  AccountModelDetectionModelsResponse,
} from '@/api/admin/accountMonitor'
import AccountMonitorCard from '@/components/admin/account-monitor/AccountMonitorCard.vue'
import AccountMonitorCostDialog from '@/components/admin/account-monitor/AccountMonitorCostDialog.vue'
import AccountMonitorAccountInfoDialog from '@/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue'
import AccountMonitorFilters from '@/components/admin/account-monitor/AccountMonitorFilters.vue'
import AccountModelDetectionHistoryPanel from '@/components/admin/account-monitor/AccountModelDetectionHistoryPanel.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { EditAccountModal } from '@/components/account'
import ReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'
import ScheduledTestsPanel from '@/components/admin/account/ScheduledTestsPanel.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getFloatingPanelPosition } from '@/utils/floatingPanel'
import type { Account, AdminGroup, Proxy as AccountProxy, ClaudeModel } from '@/types'

type CardConcurrency = AccountMonitorConcurrencyItem & { delayed?: boolean }
type SaveCompletion = {
  resolve: () => void
  reject: (reason?: unknown) => void
}
type ProcurementMutationSession = {
  accountID: number
  payload: string
  idempotencyKey: string
}
type NativeAccountWithMonitorProjection = Account & {
  effective_schedulable?: boolean
  effective_schedulable_at?: string | null
  effective_unschedulable_reason?: string | null
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
const modelDetectionModelsByID = ref<Record<number, AccountModelDetectionModelsResponse>>({})
const savingModelDetectionIDs = ref<number[]>([])
const detectingModelDetectionIDs = ref<number[]>([])
const rangeError = ref<string | null>(null)
const concurrencyByID = ref<Record<number, CardConcurrency>>({})
const showCostDialog = ref(false)
const savingCost = ref(false)
const costDialogError = ref<string | null>(null)
const selectedCostAccount = ref<AccountMonitorAccount | null>(null)
let procurementMutationSession: ProcurementMutationSession | null = null
const selectedNativeAccount = ref<NativeAccountWithMonitorProjection | null>(null)
const showAccountInfoDialog = ref(false)
const showEditAccountDialog = ref(false)
const showDeleteAccountDialog = ref(false)
const editProxies = ref<AccountProxy[]>([])
const editGroups = ref<AdminGroup[]>([])
const accountActionMenu = ref<{ show: boolean; position: { top: number; left: number } | null }>({ show: false, position: null })
const showTestDialog = ref(false)
const testingAccount = ref<Account | null>(null)
const showStatsDialog = ref(false)
const statsAccount = ref<Account | null>(null)
const showScheduleDialog = ref(false)
const scheduleAccount = ref<Account | null>(null)
const scheduleModelOptions = ref<{ value: string; label: string }[]>([])
const showReAuthDialog = ref(false)
const reAuthAccount = ref<Account | null>(null)
const showDetectionHistoryPanel = ref(false)
const selectedDetectionHistoryAccount = ref<AccountMonitorAccount | null>(null)
const nativeAccountRequests = new Map<number, Promise<Account>>()
const nativeAccountError = ref<string | null>(null)
const nativeAccountLoading = ref(false)
let nativeEntryGeneration = 0

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
  const leftRank = left.scheduler_rank
  const rightRank = right.scheduler_rank
  const leftRanked = leftRank != null
  const rightRanked = rightRank != null
  if (leftRanked && rightRanked) {
    const rankDifference = Number(leftRank) - Number(rightRank)
    if (rankDifference !== 0) return rankDifference
    return left.account_id - right.account_id
  }
  if (leftRanked) return -1
  if (rightRanked) return 1
  return left.account_id - right.account_id
}

const allAccounts = computed(() => uniqueAccounts(projection.value?.accounts ?? []))
const sortedGroups = computed(() => [...(projection.value?.groups ?? [])].sort((left, right) => {
  if (left.rate_multiplier !== right.rate_multiplier) return right.rate_multiplier - left.rate_multiplier
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
const rankedAccountCount = computed(() => scopedAccounts.value.filter((account) => account.scheduler_rank != null).length)
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

async function load(range: AccountMonitorRange, options: { notifyError?: boolean; commitIf?: () => boolean } = {}): Promise<boolean> {
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
    if (options.commitIf && !options.commitIf()) return false
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
    if (options.commitIf && !options.commitIf()) return false
    rangeError.value = extractApiErrorMessage(reason, t('admin.accountMonitor.loadError'))
    if (options.notifyError !== false) appStore.showError(rangeError.value)
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
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      appStore.showError('全部探测已完成，但最新卡片加载失败，请重试')
      return
    }
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
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      appStore.showError('探测已完成，但最新卡片加载失败，请重试')
      return
    }
    appStore.showSuccess('账号探测与监控卡片已刷新')
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    runningAccountIDs.value = runningAccountIDs.value.filter((id) => id !== accountID)
  }
}

async function loadModelDetectionModels(account: AccountMonitorAccount) {
  try {
    const models = await adminAPI.accountMonitor.getModelDetectionModels(account.account_id)
    modelDetectionModelsByID.value = { ...modelDetectionModelsByID.value, [account.account_id]: models }
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, '加载账号检测模型失败'))
  }
}

async function saveModelDetectionModels(accountID: number, payload: { connectionModel: string; detectionModel: string }) {
  if (savingModelDetectionIDs.value.includes(accountID)) return
  savingModelDetectionIDs.value = [...savingModelDetectionIDs.value, accountID]
  try {
    const models = await adminAPI.accountMonitor.saveModelDetectionModels(accountID, {
      connection_probe_model: payload.connectionModel,
      model_detection_model: payload.detectionModel,
    })
    modelDetectionModelsByID.value = { ...modelDetectionModelsByID.value, [accountID]: models }
    await load(activeRange.value, { notifyError: false })
    appStore.showSuccess('账号连接测试模型与检测模型已保存')
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, '保存账号检测模型失败'))
  } finally {
    savingModelDetectionIDs.value = savingModelDetectionIDs.value.filter((id) => id !== accountID)
  }
}

async function enqueueModelDetection(accountID: number) {
  if (detectingModelDetectionIDs.value.includes(accountID)) return
  detectingModelDetectionIDs.value = [...detectingModelDetectionIDs.value, accountID]
  try {
    const result = await adminAPI.accountMonitor.enqueueModelDetection(accountID)
    await load(activeRange.value, { notifyError: false })
    appStore.showSuccess(result.reused ? '已复用该账号正在进行的检测' : '账号模型检测已排队')
  } catch (reason: unknown) {
    appStore.showError(extractApiErrorMessage(reason, '账号模型检测排队失败'))
  } finally {
    detectingModelDetectionIDs.value = detectingModelDetectionIDs.value.filter((id) => id !== accountID)
  }
}

function openModelDetectionHistory(accountID: number) {
  selectedDetectionHistoryAccount.value = allAccounts.value.find((account) => account.account_id === accountID) ?? null
  showDetectionHistoryPanel.value = selectedDetectionHistoryAccount.value != null
}

function closeModelDetectionHistory() {
  showDetectionHistoryPanel.value = false
  selectedDetectionHistoryAccount.value = null
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

function openCostDialog(account: AccountMonitorAccount): void {
  procurementMutationSession = null
  selectedCostAccount.value = account
  costDialogError.value = null
  showCostDialog.value = true
}

function closeCostDialog(): void {
  if (savingCost.value) return
  showCostDialog.value = false
  procurementMutationSession = null
  costDialogError.value = null
}

function procurementIdempotencyKey(accountID: number, cost: number | null, estimatedQuotaUSD: number | null): string {
  const payload = JSON.stringify([cost, estimatedQuotaUSD])
  if (procurementMutationSession?.accountID === accountID && procurementMutationSession.payload === payload) {
    return procurementMutationSession.idempotencyKey
  }
  const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  const idempotencyKey = `account-procurement-${accountID}-${requestID}`
  procurementMutationSession = { accountID, payload, idempotencyKey }
  return idempotencyKey
}

function reportCostReloadFailure(operation: string): void {
  const message = `${operation}成功，但最新监控卡片加载失败，请重试`
  costDialogError.value = message
  appStore.showError(message)
}

async function saveProcurementCost(cost: number, estimatedQuotaUSD: number) {
  const account = selectedCostAccount.value
  if (!account || savingCost.value) return
  savingCost.value = true
  costDialogError.value = null
  try {
    await adminAPI.accounts.updateProcurementCost(account.account_id, cost, estimatedQuotaUSD, procurementIdempotencyKey(account.account_id, cost, estimatedQuotaUSD))
    procurementMutationSession = null
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      reportCostReloadFailure('保存采购成本')
      return
    }
    showCostDialog.value = false
    procurementMutationSession = null
    appStore.showSuccess('采购成本与预计额度已更新')
  } catch (reason: unknown) {
    costDialogError.value = extractApiErrorMessage(reason, '保存采购成本失败')
    appStore.showError(costDialogError.value)
  } finally {
    savingCost.value = false
  }
}

async function saveAccountMultiplier(multiplier: number, model: 'direct_multiplier' | 'ratio_based_upstream' = 'direct_multiplier', actualCost?: number, obtainedQuota?: number) {
  const account = selectedCostAccount.value
  if (!account || savingCost.value) return
  savingCost.value = true
  costDialogError.value = null
  try {
    await adminAPI.accounts.update(account.account_id, {
      rate_multiplier: multiplier,
      effective_cost_model: model,
      upstream_actual_cost: model === 'ratio_based_upstream' ? actualCost : null,
      upstream_obtained_quota: model === 'ratio_based_upstream' ? obtainedQuota : null,
      upstream_billing_rate_sync_enabled: false,
    })
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      reportCostReloadFailure('保存账号倍率')
      return
    }
    showCostDialog.value = false
    appStore.showSuccess('账号倍率已更新')
  } catch (reason: unknown) {
    costDialogError.value = extractApiErrorMessage(reason, '保存账号倍率失败')
    appStore.showError(costDialogError.value)
  } finally {
    savingCost.value = false
  }
}

async function restoreAccountMultiplier() {
  const account = selectedCostAccount.value
  if (!account || savingCost.value) return
  savingCost.value = true
  costDialogError.value = null
  try {
    await adminAPI.accounts.update(account.account_id, {
      upstream_billing_probe_enabled: true,
      upstream_billing_rate_sync_enabled: true,
    })
    await adminAPI.accountMonitor.runOne(account.account_id)
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      reportCostReloadFailure('恢复自动倍率')
      return
    }
    showCostDialog.value = false
    appStore.showSuccess('已恢复自动获取倍率')
  } catch (reason: unknown) {
    costDialogError.value = extractApiErrorMessage(reason, '恢复自动倍率失败')
    appStore.showError(costDialogError.value)
  } finally {
    savingCost.value = false
  }
}

async function clearProcurementCost() {
  const account = selectedCostAccount.value
  if (!account || savingCost.value) return
  savingCost.value = true
  costDialogError.value = null
  try {
    await adminAPI.accounts.updateProcurementCost(account.account_id, null, null, procurementIdempotencyKey(account.account_id, null, null))
    procurementMutationSession = null
    const reloaded = await load(activeRange.value, { notifyError: false })
    if (!reloaded) {
      reportCostReloadFailure('清空采购成本')
      return
    }
    showCostDialog.value = false
    procurementMutationSession = null
    appStore.showSuccess('采购成本已清空')
  } catch (reason: unknown) {
    costDialogError.value = extractApiErrorMessage(reason, '清空采购成本失败')
    appStore.showError(costDialogError.value)
  } finally {
    savingCost.value = false
  }
}


async function fetchNativeAccount(accountID: number): Promise<Account> {
  const pending = nativeAccountRequests.get(accountID)
  if (pending) return pending
  const request = adminAPI.accounts.getById(accountID)
    .then((account) => account)
    .finally(() => {
      nativeAccountRequests.delete(accountID)
    })
  nativeAccountRequests.set(accountID, request)
  return request
}

function beginNativeEntry(): number {
  const generation = ++nativeEntryGeneration
  nativeAccountError.value = null
  nativeAccountLoading.value = true
  return generation
}

async function loadNativeForEntry(accountID: number, generation: number): Promise<Account | null> {
  try {
    const native = await fetchNativeAccount(accountID)
    if (generation !== nativeEntryGeneration) return null
    return native
  } catch (reason: unknown) {
    if (generation === nativeEntryGeneration) {
      nativeAccountError.value = extractApiErrorMessage(reason, '账号信息加载失败')
      appStore.showError(nativeAccountError.value)
    }
    return null
  } finally {
    if (generation === nativeEntryGeneration) nativeAccountLoading.value = false
  }
}

async function loadNativeEditOptions(): Promise<void> {
  try {
    const [groups, proxies] = await Promise.all([
      adminAPI.groups.getAllIncludingInactive(),
      adminAPI.proxies.getAll(),
    ])
    editGroups.value = groups
    editProxies.value = proxies
  } catch {
    // Native editor remains usable for fields that do not need a proxy/group list.
  }
}

async function openAccountInfo(account: AccountMonitorAccount): Promise<void> {
  const generation = beginNativeEntry()
  const native = await loadNativeForEntry(account.account_id, generation)
  if (!native || generation !== nativeEntryGeneration) return
  selectedNativeAccount.value = {
    ...native,
    effective_schedulable: account.effective_schedulable,
    effective_schedulable_at: account.effective_schedulable_at,
    effective_unschedulable_reason: account.effective_unschedulable_reason,
  }
  showAccountInfoDialog.value = true
}

async function openAccountEdit(account: AccountMonitorAccount): Promise<void> {
  const generation = beginNativeEntry()
  const native = await loadNativeForEntry(account.account_id, generation)
  if (!native || generation !== nativeEntryGeneration) return
  selectedNativeAccount.value = native
  void loadNativeEditOptions()
  showEditAccountDialog.value = true
}

async function openAccountDelete(account: AccountMonitorAccount): Promise<void> {
  const generation = beginNativeEntry()
  const native = await loadNativeForEntry(account.account_id, generation)
  if (!native || generation !== nativeEntryGeneration) return
  selectedNativeAccount.value = native
  showDeleteAccountDialog.value = true
}

async function openAccountMore(account: AccountMonitorAccount, triggerEvent?: MouseEvent): Promise<void> {
  const triggerRect = triggerEvent?.currentTarget instanceof HTMLElement
    ? triggerEvent.currentTarget.getBoundingClientRect()
    : null
  const generation = beginNativeEntry()
  const native = await loadNativeForEntry(account.account_id, generation)
  if (!native || generation !== nativeEntryGeneration) return
  selectedNativeAccount.value = native
  const viewportWidth = document.documentElement.clientWidth || window.innerWidth
  const viewportHeight = window.innerHeight
  const floatingPosition = triggerRect
    ? getFloatingPanelPosition(triggerRect, viewportWidth, viewportHeight, { maxWidth: 208, maxHeightRatio: 0.7 })
    : null
  const estimatedMenuHeight = Math.min(420, floatingPosition?.maxHeight || 420)
  accountActionMenu.value = {
    show: true,
    position: {
      top: floatingPosition?.top ?? Math.max(12, viewportHeight - (floatingPosition?.bottom ?? 12) - estimatedMenuHeight),
      left: floatingPosition?.left ?? Math.max(12, viewportWidth - 224),
    },
  }
}

function closeAccountInfo(): void {
  showAccountInfoDialog.value = false
}

function closeEditAccount(): void {
  showEditAccountDialog.value = false
}

function closeDeleteAccount(): void {
  if (!showDeleteAccountDialog.value) return
  showDeleteAccountDialog.value = false
}

async function handleNativeAccountUpdated(): Promise<void> {
  showEditAccountDialog.value = false
  showReAuthDialog.value = false
  reAuthAccount.value = null
  await load(activeRange.value, { notifyError: false })
}

async function confirmDeleteAccount(): Promise<void> {
  const account = selectedNativeAccount.value
  if (!account) return
  try {
    await adminAPI.accounts.delete(account.id)
    closeDeleteAccount()
    selectedNativeAccount.value = null
    await load(activeRange.value, { notifyError: false })
  } catch (reason: unknown) {
    console.error('Failed to delete account:', reason)
  }
}

function closeAccountMore(): void {
  accountActionMenu.value = { show: false, position: null }
}

function closeTestDialog(): void {
  showTestDialog.value = false
  testingAccount.value = null
}

function closeStatsDialog(): void {
  showStatsDialog.value = false
  statsAccount.value = null
}

function closeScheduleDialog(): void {
  showScheduleDialog.value = false
  scheduleAccount.value = null
  scheduleModelOptions.value = []
}

function closeReAuthDialog(): void {
  showReAuthDialog.value = false
  reAuthAccount.value = null
}

function handleTest(account: Account): void {
  closeAccountMore()
  testingAccount.value = account
  showTestDialog.value = true
}

function handleViewStats(account: Account): void {
  closeAccountMore()
  statsAccount.value = account
  showStatsDialog.value = true
}

async function handleSchedule(account: Account): Promise<void> {
  closeAccountMore()
  scheduleAccount.value = account
  scheduleModelOptions.value = []
  showScheduleDialog.value = true
  try {
    const models = await adminAPI.accounts.getAvailableModels(account.id)
    scheduleModelOptions.value = models.map((model: ClaudeModel) => ({ value: model.id, label: model.display_name || model.id }))
  } catch {
    scheduleModelOptions.value = []
  }
}

function handleReAuth(account: Account): void {
  closeAccountMore()
  reAuthAccount.value = account
  showReAuthDialog.value = true
}

const duplicatingNativeAccountIDs = new Set<number>()

async function handleDuplicateAccount(account: Account): Promise<void> {
  closeAccountMore()
  if (duplicatingNativeAccountIDs.has(account.id)) return
  duplicatingNativeAccountIDs.add(account.id)
  try {
    const duplicate = await adminAPI.accounts.duplicate(account.id)
    appStore.showSuccess(t('admin.accounts.duplicateSuccess', { name: duplicate.name }))
    await load(activeRange.value, { notifyError: false })
  } catch (error: any) {
    console.error('Failed to duplicate account:', error)
    appStore.showError(error?.message || t('admin.accounts.duplicateFailed'))
  } finally {
    duplicatingNativeAccountIDs.delete(account.id)
  }
}

async function handleRefresh(account: Account): Promise<void> {
  closeAccountMore()
  try {
    await adminAPI.accounts.refreshCredentials(account.id)
    await load(activeRange.value, { notifyError: false })
  } catch (error) {
    console.error('Failed to refresh credentials:', error)
  }
}

async function handleRecoverState(account: Account): Promise<void> {
  closeAccountMore()
  try {
    await adminAPI.accounts.recoverState(account.id)
    await load(activeRange.value, { notifyError: false })
    appStore.showSuccess(t('admin.accounts.recoverStateSuccess'))
  } catch (error: any) {
    console.error('Failed to recover account state:', error)
    appStore.showError(error?.message || t('admin.accounts.recoverStateFailed'))
  }
}

async function handleResetQuota(account: Account): Promise<void> {
  closeAccountMore()
  try {
    await adminAPI.accounts.resetAccountQuota(account.id)
    await load(activeRange.value, { notifyError: false })
    appStore.showSuccess(t('common.success'))
  } catch (error) {
    console.error('Failed to reset quota:', error)
  }
}

function privacyResultMessageKey(account: Account): { type: 'success' | 'error'; key: string } {
  const mode = typeof account.extra?.privacy_mode === 'string' ? account.extra.privacy_mode : ''
  if (account.platform === 'openai') {
    if (mode === 'training_off') return { type: 'success', key: 'admin.accounts.privacyTrainingOff' }
    if (mode === 'training_set_cf_blocked') return { type: 'error', key: 'admin.accounts.privacyCfBlocked' }
    return { type: 'error', key: 'admin.accounts.privacyFailed' }
  }
  if (account.platform === 'antigravity') {
    return mode === 'privacy_set'
      ? { type: 'success', key: 'admin.accounts.privacyAntigravitySet' }
      : { type: 'error', key: 'admin.accounts.privacyAntigravityFailed' }
  }
  return { type: 'error', key: 'admin.accounts.privacyFailed' }
}

async function handleSetPrivacy(account: Account): Promise<void> {
  closeAccountMore()
  try {
    const updated = await adminAPI.accounts.setPrivacy(account.id)
    await load(activeRange.value, { notifyError: false })
    const result = privacyResultMessageKey(updated)
    if (result.type === 'success') appStore.showSuccess(t(result.key))
    else appStore.showError(t(result.key))
  } catch (error: any) {
    console.error('Failed to set privacy:', error)
    appStore.showError(error?.response?.data?.message || t('admin.accounts.privacyFailed'))
  }
}

const creatingSparkShadow = ref<Account | null>(null)
const showSparkShadowDialog = ref(false)

function handleCreateSparkShadow(account: Account): void {
  closeAccountMore()
  creatingSparkShadow.value = account
  showSparkShadowDialog.value = true
}

async function confirmCreateSparkShadow(): Promise<void> {
  const account = creatingSparkShadow.value
  if (!account) return
  try {
    await adminAPI.accounts.createSparkShadow(account.id, { name: `${account.name} (Spark)` })
    showSparkShadowDialog.value = false
    creatingSparkShadow.value = null
    await load(activeRange.value, { notifyError: false })
    appStore.showSuccess(t('admin.accounts.createSparkShadowSuccess'))
  } catch (error: any) {
    console.error('Failed to create spark shadow:', error)
    appStore.showError(error?.response?.data?.message || t('admin.accounts.createSparkShadowFailed'))
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
