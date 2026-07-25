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

      <div v-else class="grid gap-4 xl:grid-cols-2">
        <AccountMonitorCard
          v-for="account in filteredAccounts"
          :key="account.account_id"
          :account="account"
          :running="runningAccounts.has(account.account_id)"
          @refresh="handleRunOne"
          @settings="showSettings = true"
          @history="openHistory"
        />
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
const showSettings = ref(false)
const savingSettings = ref(false)
const settingsError = ref<string | null>(null)
const historyAccount = ref<number | null>(null)
const historyLoading = ref(false)
const historyItems = ref<AccountMonitorHistoryItem[]>([])

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

async function handleRunAll() {
  if (runningAll.value) return
  runningAll.value = true
  try {
    await adminAPI.accountMonitor.runAll()
    await load()
    appStore.showSuccess(t('admin.accountMonitor.messages.refreshAllSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accountMonitor.messages.refreshFailed')))
  } finally {
    runningAll.value = false
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

onMounted(load)
onUnmounted(() => abortController?.abort())
</script>
