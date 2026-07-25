<template>
  <article
    class="card flex min-h-[330px] flex-col overflow-hidden border-l-4 p-4"
    :class="statusBorderClass"
    data-test="monitor-card"
  >
    <header class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
            {{ account.name }}
          </h2>
          <span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ account.account_id }}</span>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
            {{ account.platform }}
          </span>
        </div>
        <div class="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
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
      <Metric
        :label="t('admin.accountMonitor.metrics.multiplier')"
        :value="multiplierDisplay.value"
        :detail="multiplierDisplay.detail"
        test-id="multiplier-metric"
      />
    </div>

    <div class="mt-4 grid grid-cols-2 gap-3 border-y border-gray-100 py-3 dark:border-dark-700">
      <div>
        <div class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
          {{ t('admin.accountMonitor.today.title') }}
        </div>
        <AccountTodayStatsCell class="mt-1" :stats="account.today_stats ?? null" />
      </div>
      <div>
        <div class="text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
          {{ t('admin.accountMonitor.card.usageWindows') }}
        </div>
        <AccountUsageCell
          class="mt-1"
          :account="usageAccount"
          :today-stats="account.today_stats ?? null"
        />
      </div>
    </div>

    <div class="mt-auto flex items-center justify-between gap-3 pt-3">
      <div class="min-w-0 text-xs text-gray-500 dark:text-gray-400">
        <span v-if="account.checked_at">
          {{ t('admin.accountMonitor.card.checkedAt', { time: formatDate(account.checked_at) }) }}
        </span>
        <span v-else>{{ t('admin.accountMonitor.status.noHistory') }}</span>
        <span v-if="account.error_code" class="ml-2 text-red-600 dark:text-red-400">
          {{ account.error_code }}
        </span>
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <button
          type="button"
          class="icon-button"
          :title="t('admin.accountMonitor.actions.settings')"
          :aria-label="t('admin.accountMonitor.actions.settings')"
          @click="emit('settings')"
        >
          <Icon name="cog" size="sm" />
        </button>
        <button
          type="button"
          class="icon-button"
          :disabled="running"
          :title="t('admin.accountMonitor.actions.refreshOne')"
          :aria-label="t('admin.accountMonitor.actions.refreshOne')"
          @click="emit('refresh', account.account_id)"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" />
        </button>
        <button
          type="button"
          class="icon-button"
          :title="t('admin.accountMonitor.actions.history')"
          :aria-label="t('admin.accountMonitor.actions.history')"
          @click="emit('history', account.account_id)"
        >
          <Icon name="clock" size="sm" />
        </button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import type { AccountMonitorAccount } from '@/api/admin/accountMonitor'
import type { Account } from '@/types'

const props = defineProps<{
  account: AccountMonitorAccount
  running?: boolean
}>()

const emit = defineEmits<{
  (event: 'refresh', accountID: number): void
  (event: 'settings'): void
  (event: 'history', accountID: number): void
}>()

const { t } = useI18n()

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
	if (!props.account.checked_at && !props.account.latest) return 'unavailable'
  if (props.account.error_code === 'balance_exhausted') return 'balance_exhausted'
  if (props.account.stale) return 'stale'
  return props.account.latest_status || 'unavailable'
})

const statusLabel = computed(() => t(`admin.accountMonitor.status.${status.value}`))

const multiplierDisplay = computed(() => {
  const multiplier = props.account.multiplier
  if (
    multiplier.status === 'ok'
    && multiplier.value != null
    && Number.isFinite(multiplier.value)
    && multiplier.value >= 0
  ) {
    const source = multiplier.source === 'measured' ? 'measured' : 'declared'
    return {
      value: `${multiplier.value.toFixed(2)}x`,
      detail: t(`admin.accountMonitor.multiplier.${source}`),
    }
  }
  const statusKey = ['stale', 'unsupported', 'failed'].includes(multiplier.status)
    ? multiplier.status
    : 'unavailable'
  return {
    value: t(`admin.accountMonitor.multiplier.${statusKey}`),
    detail: '',
  }
})

const statusBadgeClass = computed(() => ({
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300': status.value === 'success',
  'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300': status.value === 'failed',
  'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300': status.value === 'balance_exhausted',
  'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300': status.value === 'stale',
  'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300': status.value === 'unavailable',
}))

const statusBorderClass = computed(() => ({
  'border-emerald-500': status.value === 'success',
  'border-red-500': status.value === 'failed',
  'border-orange-500': status.value === 'balance_exhausted',
  'border-amber-500': status.value === 'stale',
  'border-gray-300 dark:border-dark-600': status.value === 'unavailable',
}))

function formatPercent(value: number): string {
  return `${Math.round(value * 100)}%`
}

function formatMs(value?: number | null): string {
  return value == null ? '-' : `${Math.round(value)} ms`
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString()
}

const Metric = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    detail: { type: String, default: '' },
    testId: { type: String, default: '' },
  },
  setup(metricProps) {
    return () => h('div', {
      class: 'rounded bg-gray-50 p-2 dark:bg-dark-800/70',
      'data-test': metricProps.testId || undefined,
    }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-gray-400' }, metricProps.label),
      h('div', { class: 'mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white' }, metricProps.value),
      metricProps.detail
        ? h('div', { class: 'mt-0.5 text-[10px] text-gray-500 dark:text-gray-400' }, metricProps.detail)
        : null,
    ])
  },
})
</script>
