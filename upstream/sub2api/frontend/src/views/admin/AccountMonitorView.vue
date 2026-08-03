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
              当前展示 {{ filteredAccounts.length }} 个账号
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

      <section class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex gap-2 overflow-x-auto pb-1" role="tablist" aria-label="账号分组">
          <div data-test="all-site-tab">
            <button
              type="button"
              role="tab"
              :aria-selected="activeGroupId === null"
              class="shrink-0 rounded-lg border px-3 py-2 text-left text-sm transition-colors"
              :class="activeGroupId === null ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-gray-300'"
              data-test="all-site-tab-button"
              @click="selectAllSite"
            >
              <span class="block font-medium">全站</span>
              <span class="mt-0.5 block text-[11px] opacity-75">全部账号</span>
            </button>
          </div>
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
          :accounts="accounts"
          @update:search="search = $event"
          @update:platform="platform = $event"
          @update:status="status = $event"
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

      <div v-else class="space-y-5">
        <div v-if="!activeGroup" class="grid gap-6 xl:grid-cols-2">
          <section data-test="global-operating-summary">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div><h2 class="text-base font-semibold text-gray-900 dark:text-white">全站经营数据</h2><p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">默认今日 · 成本仅采用已对账的真实上游账单</p></div>
              <div class="flex items-center gap-2"><button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" data-test="open-ledger-history" @click="openLedgerHistory()">历史按日</button><button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" data-test="open-exceptions" @click="openExceptions">异常明细</button></div>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3" data-test="operations-overview">
              <LedgerMetric label="用户实际计费" :value="formatMoney(globalLedger?.user_charge, globalLedger?.currency)" /><LedgerMetric label="上游真实扣费" :value="formatMoney(globalLedger?.upstream_cost, globalLedger?.currency)" /><LedgerMetric label="账号利润" :value="formatMoney(globalLedger?.paper_profit, globalLedger?.currency)" /><LedgerMetric label="利润率" :value="formatProfitMargin(globalLedger)" /><LedgerMetric label="成本覆盖率" :value="formatCoverage(globalLedger)" :value-class="coverageClass(globalLedger)" /><LedgerMetric label="待对账" :value="String(globalLedger?.pending_attempts ?? 0)" />
            </div>
            <div v-if="globalLifetimeLedger" class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700"><div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400" data-test="global-lifetime-ledger"><span class="font-medium text-gray-700 dark:text-gray-200">历史累计</span><span>营收 {{ formatMoney(globalLifetimeLedger.user_charge, globalLifetimeLedger.currency) }}</span><span>成本 {{ formatMoney(globalLifetimeLedger.upstream_cost, globalLifetimeLedger.currency) }}</span><span>账号利润 {{ formatMoney(globalLifetimeLedger.paper_profit, globalLifetimeLedger.currency) }}</span><span>利润率 {{ formatProfitMargin(globalLifetimeLedger) }}</span></div><p v-if="hasUnattributedLedger(globalLifetimeLedger)" class="mt-2 text-xs text-gray-400 dark:text-gray-500" data-test="global-lifetime-unattributed-group-ledger">历史累计未归属分组：{{ globalLifetimeLedger.unattributed_attempts }} 笔请求 · 营收 {{ formatMoney(globalLifetimeLedger.unattributed_user_charge, globalLifetimeLedger.currency) }} · 成本 {{ formatMoney(globalLifetimeLedger.unattributed_upstream_cost, globalLifetimeLedger.currency) }}（已计入全站，不计入任何分组）</p></div>
            <p v-if="hasUnattributedLedger(globalLedger)" class="mt-2 text-xs text-gray-400 dark:text-gray-500" data-test="unattributed-group-ledger">今日未归属分组：{{ globalLedger?.unattributed_attempts }} 笔请求 · 营收 {{ formatMoney(globalLedger?.unattributed_user_charge, globalLedger?.currency) }} · 成本 {{ formatMoney(globalLedger?.unattributed_upstream_cost, globalLedger?.currency) }}（已计入全站，不计入任何分组）</p>
          </section>
          <section data-test="global-service-summary"><div class="mb-3"><h2 class="text-base font-semibold text-gray-900 dark:text-white">全站账号数据</h2><p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">账号存量、监控覆盖与服务质量</p></div><div class="grid grid-cols-2 gap-3 sm:grid-cols-3"><LedgerMetric label="账号总数" :value="String(globalHealth.total_accounts)" /><LedgerMetric label="监控中" :value="String(globalHealth.monitoring_accounts)" /><LedgerMetric label="可用" :value="String(globalHealth.available_accounts)" value-class="text-emerald-600" /><LedgerMetric label="不可用" :value="String(globalHealth.unavailable_accounts)" value-class="text-red-600" /><LedgerMetric label="成本不合格" :value="String(scopeAccountSections.cost_ineligible.length)" value-class="text-amber-600" /><LedgerMetric label="待确认" :value="String(globalHealth.pending_accounts)" value-class="text-amber-600" /><LedgerMetric label="暂停" :value="String(globalHealth.paused_accounts)" /><LedgerMetric label="成功率" :value="formatHealthPercent(globalHealth.success_rate)" /><LedgerMetric label="TTFT P50" :value="formatMs(globalHealth.ttft_p50_ms)" /><LedgerMetric label="延迟 P95" :value="formatMs(globalHealth.latency_p95_ms)" /></div></section>
        </div>

        <template v-else>
          <div class="grid gap-6 xl:grid-cols-2">
            <section data-test="group-operating-summary"><div class="mb-3 flex flex-wrap items-start justify-between gap-2"><div><div class="flex items-center gap-2"><h2 class="text-base font-semibold text-gray-900 dark:text-white">分组经营数据 · {{ activeGroup.name }}</h2><span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="activeGroup.operational_state === 'closed' ? 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300' : activeGroup.operational_state === 'operational' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'">{{ groupOperationalLabel(activeGroup) }}</span></div><p v-if="activeGroup.operational_state === 'closed'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前分组未向用户开放，空账号不会作为服务故障持续告警。</p></div><button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" @click="openLedgerHistory({ group_id: activeGroup.id })">本组历史</button></div><div class="grid grid-cols-2 gap-3 sm:grid-cols-3" data-test="group-operations-overview"><LedgerMetric label="用户实际计费" :value="formatMoney(groupLedger?.user_charge, groupLedger?.currency)" /><LedgerMetric label="上游真实扣费" :value="formatMoney(groupLedger?.upstream_cost, groupLedger?.currency)" /><LedgerMetric label="账号利润" :value="formatMoney(groupLedger?.paper_profit, groupLedger?.currency)" /><LedgerMetric label="利润率" :value="formatProfitMargin(groupLedger)" /><LedgerMetric label="成本覆盖率" :value="formatCoverage(groupLedger)" :value-class="coverageClass(groupLedger)" /><LedgerMetric label="待对账" :value="String(groupLedger?.pending_attempts ?? 0)" /></div><div v-if="groupLifetimeLedger" class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400" data-test="group-lifetime-ledger"><span class="font-medium text-gray-700 dark:text-gray-200">历史累计</span><span>营收 {{ formatMoney(groupLifetimeLedger.user_charge, groupLifetimeLedger.currency) }}</span><span>成本 {{ formatMoney(groupLifetimeLedger.upstream_cost, groupLifetimeLedger.currency) }}</span><span>账号利润 {{ formatMoney(groupLifetimeLedger.paper_profit, groupLifetimeLedger.currency) }}</span><span>利润率 {{ formatProfitMargin(groupLifetimeLedger) }}</span></div></section>
            <section data-test="group-service-summary"><div class="mb-3"><h2 class="text-base font-semibold text-gray-900 dark:text-white">分组服务数据</h2><p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">当前分组内账号的监控覆盖与服务质量</p></div><div class="grid grid-cols-2 gap-3 sm:grid-cols-3"><LedgerMetric label="账号总数" :value="String(groupHealth.total_accounts)" /><LedgerMetric label="监控中" :value="String(groupHealth.monitoring_accounts)" /><LedgerMetric label="可用" :value="String(groupHealth.available_accounts)" value-class="text-emerald-600" /><LedgerMetric label="不可用" :value="String(groupHealth.unavailable_accounts)" value-class="text-red-600" /><LedgerMetric label="成本不合格" :value="String(scopeAccountSections.cost_ineligible.length)" value-class="text-amber-600" /><LedgerMetric label="待确认" :value="String(groupHealth.pending_accounts)" value-class="text-amber-600" /><LedgerMetric label="暂停" :value="String(groupHealth.paused_accounts)" /><LedgerMetric label="成功率" :value="formatHealthPercent(groupHealth.success_rate)" /><LedgerMetric label="TTFT P50" :value="formatMs(groupHealth.ttft_p50_ms)" /><LedgerMetric label="延迟 P95" :value="formatMs(groupHealth.latency_p95_ms)" /></div></section>
          </div>
          <section class="flex flex-wrap items-center gap-x-4 gap-y-3 border-y border-gray-200 py-3 dark:border-dark-700" data-test="group-scope-action-row"><div class="flex flex-wrap items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><span v-for="bucket in monitorBuckets" :key="bucket.key" class="rounded border border-gray-200 px-2 py-1 dark:border-dark-600"><span>{{ bucket.label }}</span><span class="ml-1 font-mono font-semibold text-gray-900 dark:text-white">{{ scopeAccountSections[bucket.key].length }}</span></span></div><div class="ml-auto flex flex-wrap items-center gap-3"><span class="text-xs text-gray-500 dark:text-gray-400">成本 {{ activeGroup.score_weights.cost }} · 成功 {{ activeGroup.score_weights.success }} · TTFT {{ activeGroup.score_weights.ttft }} · 延迟 {{ activeGroup.score_weights.latency }}</span><button type="button" class="btn btn-primary px-3 py-1.5 text-xs" data-test="edit-group-score-weights" @click="showScoreDialog = true">评分权重</button></div></section>
        </template>

        <div v-if="!filteredAccounts.length" class="rounded-lg border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400" data-test="account-empty">{{ accounts.length ? t('admin.accountMonitor.empty.filtered') : t('admin.accountMonitor.empty.pool') }}</div>
          <div v-else class="space-y-6" data-test="monitor-group"><template v-for="bucket in monitorBuckets" :key="bucket.key"><section v-if="accountSections[bucket.key].length" :data-test="`account-section-${bucket.key}`"><div class="mb-3 flex items-center gap-2"><h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ bucket.label }}</h2><span class="font-mono text-sm text-gray-500 dark:text-gray-400">{{ accountSections[bucket.key].length }}</span></div><div class="grid grid-cols-1 gap-4 lg:grid-cols-2" data-test="account-card-grid"><div v-for="account in accountSections[bucket.key]" :key="account.account_id" class="min-w-0"><AccountMonitorCard :account="account" :scope="activeGroup ? 'group' : 'all'" :operations="accountLedgers[account.account_id] ?? null" :group-operational-state="activeGroup?.operational_state" :running="runningAccounts.has(account.account_id)" :saving-weight="savingWeights.has(account.account_id)" @refresh="handleRunOne" @update-priority="updateWeight" @settings="showSettings = true" @history="openHistory" /></div></div></section></template></div>
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
      <div v-else-if="historyError" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300" data-test="account-history-error">
        {{ historyError }}
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
              <td class="px-3 py-2">{{ historyStatusLabel(item.status) }}</td>
              <td class="px-3 py-2 font-mono">{{ formatMs(item.ttft_ms) }}</td>
              <td class="px-3 py-2 font-mono">{{ formatMs(item.latency_ms) }}</td>
              <td class="px-3 py-2 text-red-600 dark:text-red-400">{{ monitorFailureLabel(item.error_code) || '-' }}</td>
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
      <div v-else-if="exceptionsError" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300" data-test="exceptions-error">{{ exceptionsError }}</div>
      <div v-else-if="exceptions.length" class="space-y-3">
        <div v-for="item in exceptions" :key="item.id" class="rounded-lg border border-amber-200 p-3 dark:border-amber-900/50">
          <div class="grid gap-1 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
            <span>本地请求：<code>{{ item.attempt.local_request_id }}</code></span>
            <span>上游请求：<code>{{ item.attempt.upstream_request_id || '-' }}</code></span>
            <span>模型：{{ item.attempt.model }}</span>
            <span>原因：{{ reconciliationReasonLabel(item.reason_code) }}</span>
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
  AccountMonitorBucket,
  AccountMonitorGroup,
  AccountMonitorHealthSummary,
  AccountMonitorHistoryItem,
  AccountMonitorProjection,
  AccountMonitorScoreWeights,
} from '@/api/admin/accountMonitor'
import type { OperationsScopeParams, ReconciliationException, ReconciliationSummary } from '@/api/admin/reconciliation'
import type { AdminGroup } from '@/types'
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
const adminGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const search = ref('')
const platform = ref('')
const status = ref('')
const runningAll = ref(false)
const runningAccounts = ref(new Set<number>())
const savingWeights = ref(new Set<number>())
const showSettings = ref(false)
const savingSettings = ref(false)
const settingsError = ref<string | null>(null)
const historyAccount = ref<number | null>(null)
const historyLoading = ref(false)
const historyItems = ref<AccountMonitorHistoryItem[]>([])
const historyError = ref<string | null>(null)
const globalLedger = ref<ReconciliationSummary | null>(null)
const groupLedger = ref<ReconciliationSummary | null>(null)
const globalLifetimeLedger = ref<ReconciliationSummary | null>(null)
const groupLifetimeLedger = ref<ReconciliationSummary | null>(null)
const accountLedgers = ref<Record<number, ReconciliationSummary | null>>({})
const activeGroupId = ref<number | null>(null)
const ledgerHistoryScope = ref<OperationsScopeParams | null>(null)
const showScoreDialog = ref(false)
const savingScoreWeights = ref(false)
const scoreWeightsError = ref<string | null>(null)
const exceptions = ref<ReconciliationException[]>([])
const exceptionsOpen = ref(false)
const exceptionsLoading = ref(false)
const exceptionsError = ref<string | null>(null)
const adjustmentAmounts = ref<Record<number, string>>({})
const adjustingID = ref<number | null>(null)

let abortController: AbortController | null = null
let operationsGeneration = 0
const accountOperationsConcurrency = 6

const intervalSeconds = computed(() => projection.value?.settings.interval_seconds ?? 300)
const intervalLabel = computed(() => {
  const seconds = intervalSeconds.value
  if (seconds % 60 === 0) return t('admin.accountMonitor.intervalMinutes', { count: seconds / 60 })
  return t('admin.accountMonitor.intervalSeconds', { count: seconds })
})

const monitorGroupByID = computed(() => new Map((projection.value?.groups ?? []).map((group) => [group.id, group])))
const emptyGroupScoreWeights: AccountMonitorScoreWeights = { cost: 15, success: 45, ttft: 20, latency: 20 }
const sortedGroups = computed<AccountMonitorGroup[]>(() => {
  const monitorGroups = monitorGroupByID.value
  const source = adminGroups.value.map((group) => {
    const monitor = monitorGroups.get(group.id)
    const groupAccounts = monitor?.accounts ?? accounts.value.filter((account) => account.group_ids.includes(group.id))
    return {
      id: group.id,
      name: group.name,
      rate_multiplier: group.rate_multiplier,
      customer_visible: group.status === 'active',
      native_order: group.sort_order,
      score_weights: monitor?.score_weights ?? emptyGroupScoreWeights,
      operational_state: monitor?.operational_state ?? (group.status === 'active' ? 'operational' : 'closed'),
      health: monitor?.health ?? emptyHealth,
      accounts: groupAccounts,
    }
  })
  return source.sort((left, right) => {
    if (left.native_order !== right.native_order) return left.native_order - right.native_order
    return left.id - right.id
  })
})
const activeGroup = computed<AccountMonitorGroup | null>(() => sortedGroups.value.find((group) => group.id === activeGroupId.value) ?? null)
function uniqueAccountsByID(source: AccountMonitorAccount[]): AccountMonitorAccount[] {
  const accountIDs = new Set<number>()
  return source.filter((account) => {
    if (accountIDs.has(account.account_id)) return false
    accountIDs.add(account.account_id)
    return true
  })
}
const activeGroupAccountSource = computed(() => {
  const group = activeGroup.value
  const source = group?.accounts ?? (group
    ? accounts.value.filter((account) => account.group_ids.includes(group.id))
    : accounts.value)
  return uniqueAccountsByID(source)
})

const filteredAccounts = computed(() => {
  const query = search.value.trim().toLowerCase()
  return activeGroupAccountSource.value.filter((account) => {
    if (platform.value && account.platform !== platform.value) return false
    if (status.value && account.monitor_bucket !== status.value) return false
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
const monitorBuckets: { key: AccountMonitorBucket; label: string }[] = [
  { key: 'available', label: '可用' },
  { key: 'unavailable', label: '不可用' },
  { key: 'cost_ineligible', label: '成本不合格' },
  { key: 'pending', label: '待确认' },
  { key: 'paused', label: '暂停' },
]
function partitionAccounts(source: AccountMonitorAccount[]): Record<AccountMonitorBucket, AccountMonitorAccount[]> {
  const sections: Record<AccountMonitorBucket, AccountMonitorAccount[]> = {
    available: [],
    unavailable: [],
    cost_ineligible: [],
    pending: [],
    paused: [],
  }
  for (const account of source) sections[account.monitor_bucket].push(account)
  for (const bucket of monitorBuckets) {
    sections[bucket.key].sort((left, right) => {
      if (activeGroup.value && bucket.key === 'available') {
        const scoreDifference = (right.quality_score ?? -1) - (left.quality_score ?? -1)
        if (scoreDifference !== 0) return scoreDifference
        const rankDifference = (left.group_rank ?? Number.MAX_SAFE_INTEGER) - (right.group_rank ?? Number.MAX_SAFE_INTEGER)
        if (rankDifference !== 0) return rankDifference
      }
      return left.account_id - right.account_id
    })
  }
  return sections
}
const accountSections = computed(() => partitionAccounts(filteredAccounts.value))
const scopeAccountSections = computed(() => partitionAccounts(activeGroupAccountSource.value))
const emptyHealth: AccountMonitorHealthSummary = {
  total_accounts: 0,
  monitoring_accounts: 0,
  available_accounts: 0,
  unavailable_accounts: 0,
  pending_accounts: 0,
  paused_accounts: 0,
  success_rate: 0,
  success_sample_count: 0,
  ttft_sample_count: 0,
  latency_sample_count: 0,
}
const globalHealth = computed(() => projection.value?.health ?? emptyHealth)
const groupHealth = computed(() => activeGroup.value?.health ?? emptyHealth)
function hasUnattributedLedger(summary?: ReconciliationSummary | null): boolean {
  if (!summary) return false
  const hasAmount = (value: string | number | null | undefined) => {
    const amount = Number(value)
    return Number.isFinite(amount) && amount !== 0
  }
  return Number(summary.unattributed_attempts) > 0
    || hasAmount(summary.unattributed_user_charge)
    || hasAmount(summary.unattributed_upstream_cost)
}

function historyStatusLabel(status: string): string {
  if (status === 'success') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'unavailable') return '不可用'
  return '未知状态'
}

function monitorFailureLabel(code?: string | null): string {
  const labels: Record<string, string> = {
    timeout: '请求超时',
    balance_exhausted: '余额或额度不足',
    model_unavailable: '当前模型不可用',
    malformed_stream: '响应格式异常',
    http_error: '上游服务返回异常',
    account_test_error: '账号探测失败',
  }
  const normalized = code?.trim().toLowerCase()
  if (!normalized) return ''
  return labels[normalized] ?? '探测失败，请稍后重试'
}

function reconciliationReasonLabel(reasonCode?: string | null): string {
  const labels: Record<string, string> = {
    upstream_record_missing: '暂未获取到对应的上游账单',
    missing_upstream_record: '暂未获取到对应的上游账单',
    late_automatic_after_manual: '自动账单晚于人工补登记到达，需要核对',
  }
  return labels[reasonCode?.trim().toLowerCase() ?? ''] ?? '账务数据需要人工核对'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isStringOrNumber(value: unknown): value is string | number {
  return typeof value === 'string' || isFiniteNumber(value)
}

function isAccountMonitorHistoryItem(value: unknown): value is AccountMonitorHistoryItem {
  if (!isRecord(value)) return false
  return isFiniteNumber(value.account_id)
    && typeof value.model_id === 'string'
    && typeof value.status === 'string'
    && typeof value.checked_at === 'string'
    && (value.error_code === undefined || typeof value.error_code === 'string')
    && (value.http_status == null || isFiniteNumber(value.http_status))
    && (value.ttft_ms == null || isFiniteNumber(value.ttft_ms))
    && (value.latency_ms == null || isFiniteNumber(value.latency_ms))
}

function isReconciliationException(value: unknown): value is ReconciliationException {
  if (!isRecord(value) || !isRecord(value.attempt)) return false
  const attempt = value.attempt
  return isFiniteNumber(value.id)
    && typeof value.reason_code === 'string'
    && typeof value.details === 'string'
    && isFiniteNumber(value.retry_count)
    && typeof value.first_detected_at === 'string'
    && typeof value.last_checked_at === 'string'
    && isFiniteNumber(attempt.id)
    && typeof attempt.attempt_id === 'string'
    && typeof attempt.local_request_id === 'string'
    && (attempt.upstream_request_id === undefined || typeof attempt.upstream_request_id === 'string')
    && isFiniteNumber(attempt.account_id)
    && typeof attempt.model === 'string'
    && isStringOrNumber(attempt.user_charge)
    && typeof attempt.currency === 'string'
    && typeof attempt.completed_at === 'string'
    && typeof attempt.reconcile_status === 'string'
}

async function load() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  error.value = null
  adminGroups.value = []
  try {
    const [result, groups] = await Promise.all([
      adminAPI.accountMonitor.list({ signal: controller.signal }),
      adminAPI.groups.getAllIncludingInactive().then((items) => {
        if (!Array.isArray(items)) throw new Error('invalid group list')
        return items
      }).catch(() => {
        throw new Error('分组列表加载失败，请检查分组服务连接')
      }),
    ])
    if (controller.signal.aborted || abortController !== controller) return
    projection.value = result
    adminGroups.value = groups
    const allProjectedAccounts = [
      ...result.accounts,
      ...(result.groups ?? []).flatMap((group) => group.accounts ?? []),
    ]
    accounts.value = uniqueAccountsByID(allProjectedAccounts)
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
  activeGroupId.value = null
}

async function loadOperations() {
  const groupID = activeGroupId.value
  const generation = ++operationsGeneration
  const isCurrent = () => generation === operationsGeneration && activeGroupId.value === groupID
  const lifetimeScope = { start: '1970-01-01T00:00:00.000Z', end: new Date().toISOString() }
  const visibleAccounts = activeGroupAccountSource.value
  const calls: Promise<void>[] = [
    adminAPI.reconciliation.operations({})
      .then((summary) => {
        if (isCurrent()) globalLedger.value = summary
      })
      .catch(() => {
        if (isCurrent()) globalLedger.value = null
      }),
    adminAPI.reconciliation.operations(lifetimeScope)
      .then((summary) => {
        if (isCurrent()) globalLifetimeLedger.value = summary
      })
      .catch(() => {
        if (isCurrent()) globalLifetimeLedger.value = null
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
    calls.push(adminAPI.reconciliation.operations({ group_id: groupID, ...lifetimeScope })
      .then((summary) => {
        if (isCurrent()) groupLifetimeLedger.value = summary
      })
      .catch(() => {
        if (isCurrent()) groupLifetimeLedger.value = null
      }))
  } else {
    if (isCurrent()) {
      groupLedger.value = null
      groupLifetimeLedger.value = null
    }
  }
  const nextLedgers: Record<number, ReconciliationSummary | null> = {}
  let nextAccountIndex = 0
  const loadAccountWorker = async () => {
    while (nextAccountIndex < visibleAccounts.length) {
      const account = visibleAccounts[nextAccountIndex]
      nextAccountIndex += 1
      const accountScope = groupID === null
        ? { account_id: account.account_id }
        : { group_id: groupID, account_id: account.account_id }
      try {
        nextLedgers[account.account_id] = await adminAPI.reconciliation.operations(accountScope)
      } catch {
        nextLedgers[account.account_id] = null
      }
    }
  }
  const workerCount = Math.min(accountOperationsConcurrency, visibleAccounts.length)
  for (let index = 0; index < workerCount; index += 1) calls.push(loadAccountWorker())
  await Promise.all(calls)
  if (isCurrent()) accountLedgers.value = nextLedgers
}

async function selectGroup(groupID: number) {
  activeGroupId.value = groupID
  await loadOperations()
}

async function selectAllSite() {
  activeGroupId.value = null
  await loadOperations()
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
  exceptionsError.value = null
  exceptions.value = []
  try {
    const response = await adminAPI.reconciliation.exceptions({ limit: 100 })
    if (!response || !Array.isArray(response.items) || !response.items.every(isReconciliationException)) {
      throw new Error('异常明细返回了无效数据，请检查账务服务连接')
    }
    exceptions.value = response.items
  } catch (err: unknown) {
    exceptionsError.value = extractApiErrorMessage(err, '对账异常加载失败')
    appStore.showError(exceptionsError.value)
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
    appStore.showSuccess('账号调度优先级已更新')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '账号权重更新失败'))
  } finally {
    const next = new Set(savingWeights.value)
    next.delete(accountID)
    savingWeights.value = next
  }
}

async function saveGroupScoreWeights(weights: Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency' | 'ttft_target_ms' | 'ttft_limit_ms' | 'latency_target_ms' | 'latency_limit_ms'>) {
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
  historyError.value = null
  try {
    const response = await adminAPI.accountMonitor.history(accountID, 25)
    if (!response || !Array.isArray(response.items) || !response.items.every(isAccountMonitorHistoryItem)) {
      throw new Error('账号历史返回了无效数据，请检查监控服务连接')
    }
    historyItems.value = response.items
  } catch (err: unknown) {
    historyError.value = extractApiErrorMessage(err, t('admin.accountMonitor.history.loadError'))
    appStore.showError(historyError.value)
  } finally {
    historyLoading.value = false
  }
}

function formatMs(value?: number | null): string {
  return value == null ? '-' : `${Math.round(value)} ms`
}

function formatHealthPercent(value?: number | null): string {
  const numeric = Number(value)
  return Number.isFinite(numeric) ? `${(numeric * 100).toFixed(1)}%` : '—'
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
