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

      <section class="grid gap-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-2 xl:grid-cols-5" data-test="reconciliation-summary">
        <div><p class="text-xs text-gray-500">可用账号</p><p class="text-2xl font-semibold text-green-600">{{ availableCount }}</p></div>
        <div><p class="text-xs text-gray-500">不可用账号</p><p class="text-2xl font-semibold text-red-600">{{ unavailableCount }}</p></div>
        <div><p class="text-xs text-gray-500">成本覆盖率</p><p class="text-2xl font-semibold" :class="coverageRatio >= 1 ? 'text-green-600' : 'text-amber-600'">{{ formatCoverage(ledger?.coverage_ratio) }}</p></div>
        <div><p class="text-xs text-gray-500">待对账笔数</p><p class="text-2xl font-semibold text-amber-600">{{ ledger?.pending_attempts ?? '-' }}</p></div>
        <button type="button" class="btn btn-secondary self-end" data-test="open-exceptions" @click="openExceptions">异常明细</button>
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
          @update:group-id="groupId = $event"
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

      <div v-else class="space-y-6">
        <section v-for="group in groupedAccounts" :key="group.key" data-test="monitor-group">
          <header class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ group.name }}</h2>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                可用 {{ group.available }} 个 · 不可用 {{ group.unavailable }} 个 · 按质量分降序
              </p>
            </div>
            <div v-if="group.key !== unavailableGroupKey" class="flex items-center gap-2 text-xs text-gray-500">
              <span>价格 {{ scoreWeights.cost }}%</span><span>成功率 {{ scoreWeights.success }}%</span>
              <span>TTFT {{ scoreWeights.ttft }}%</span><span>总耗时 {{ scoreWeights.latency }}%</span>
            </div>
          </header>
          <div class="grid gap-4 xl:grid-cols-2">
            <div v-for="item in group.accounts" :key="item.account.account_id" class="relative">
              <div v-if="item.score !== null" class="absolute right-4 top-3 z-10 rounded-full bg-primary-600 px-2 py-1 text-xs font-semibold text-white" :title="item.scoreHint">
                {{ item.score }} 分
              </div>
              <AccountMonitorCard
                :account="item.account"
                :running="runningAccounts.has(item.account.account_id)"
                :saving-weight="savingWeights.has(item.account.account_id)"
                @refresh="handleRunOne"
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
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  AccountMonitorAccount,
  AccountMonitorHistoryItem,
  AccountMonitorProjection,
} from '@/api/admin/accountMonitor'
import type { ReconciliationException, ReconciliationSummary } from '@/api/admin/reconciliation'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import AccountMonitorCard from '@/components/admin/account-monitor/AccountMonitorCard.vue'
import AccountMonitorFilters from '@/components/admin/account-monitor/AccountMonitorFilters.vue'
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
const ledger = ref<ReconciliationSummary | null>(null)
const exceptions = ref<ReconciliationException[]>([])
const exceptionsOpen = ref(false)
const exceptionsLoading = ref(false)
const adjustmentAmounts = ref<Record<number, string>>({})
const adjustingID = ref<number | null>(null)

let abortController: AbortController | null = null

const intervalSeconds = computed(() => projection.value?.settings.interval_seconds ?? 300)
const intervalLabel = computed(() => {
  const seconds = intervalSeconds.value
  if (seconds % 60 === 0) return t('admin.accountMonitor.intervalMinutes', { count: seconds / 60 })
  return t('admin.accountMonitor.intervalSeconds', { count: seconds })
})

const filteredAccounts = computed(() => {
  const query = search.value.trim().toLowerCase()
  return accounts.value.filter((account) => {
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
const availableCount = computed(() => accounts.value.filter((account) => account.latest_status === 'success').length)
const unavailableCount = computed(() => Math.max(0, accounts.value.length - availableCount.value))
const coverageRatio = computed(() => Number(ledger.value?.coverage_ratio ?? 0))
const unavailableGroupKey = '__unavailable__'
const scoreWeights = { cost: 30, success: 30, ttft: 20, latency: 20 }

function qualityScore(account: AccountMonitorAccount): number | null {
  if (account.latest_status !== 'success' || account.stale) return null
  const multiplier = account.multiplier.value
  const cost = multiplier == null ? 50 : Math.max(0, Math.min(100, 100 - multiplier * 100))
  const success = Math.max(0, Math.min(100, account.success_rate * 100))
  const ttft = account.ttft_p50_ms == null ? 50 : Math.max(0, Math.min(100, 100 - account.ttft_p50_ms / 50))
  const latency = account.latency_p95_ms == null ? 50 : Math.max(0, Math.min(100, 100 - account.latency_p95_ms / 100))
  return Math.round((cost * scoreWeights.cost + success * scoreWeights.success + ttft * scoreWeights.ttft + latency * scoreWeights.latency) / 100)
}

const groupedAccounts = computed(() => {
  const groups = new Map<string, { key: string; name: string; accounts: { account: AccountMonitorAccount; score: number | null; scoreHint: string }[]; available: number; unavailable: number }>()
  for (const account of filteredAccounts.value) {
    const unavailable = account.latest_status !== 'success' || account.stale
    const selectedGroupIndex = groupId.value ? account.group_ids.indexOf(Number(groupId.value)) : -1
    const names = unavailable
      ? ['不可用账号']
      : selectedGroupIndex >= 0
        ? [account.group_names[selectedGroupIndex] || '未分组']
        : (account.group_names.length ? account.group_names : ['未分组'])
    for (const name of names) {
      const key = unavailable ? unavailableGroupKey : name
      const group = groups.get(key) ?? { key, name, accounts: [], available: 0, unavailable: 0 }
      const score = qualityScore(account)
      group.accounts.push({ account, score, scoreHint: score === null ? '' : `价格 ${scoreWeights.cost}% · 成功率 ${scoreWeights.success}% · TTFT ${scoreWeights.ttft}% · 总耗时 ${scoreWeights.latency}%` })
      if (unavailable) group.unavailable += 1
      else group.available += 1
      groups.set(key, group)
    }
  }
  return [...groups.values()].map((group) => ({ ...group, accounts: group.accounts.sort((a, b) => (b.score ?? -1) - (a.score ?? -1)) }))
})

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
    await loadLedger()
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

async function loadLedger() {
  try {
    ledger.value = await adminAPI.reconciliation.summary()
  } catch {
    ledger.value = null
  }
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
    await loadLedger()
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
    appStore.showSuccess('账号权重已更新')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '账号权重更新失败'))
  } finally {
    const next = new Set(savingWeights.value)
    next.delete(accountID)
    savingWeights.value = next
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

function formatCoverage(value?: number | null): string {
  return value == null ? '-' : `${(value * 100).toFixed(2)}%`
}

onMounted(load)
onUnmounted(() => abortController?.abort())
</script>
