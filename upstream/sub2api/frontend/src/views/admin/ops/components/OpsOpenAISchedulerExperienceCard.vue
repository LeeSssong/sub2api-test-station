<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import {
  opsAPI,
  type OpsOpenAISchedulerExperienceParams,
  type OpsOpenAISchedulerExperienceResponse,
  type OpsOpenAISchedulerMetricStatus,
  type OpsOpenAISchedulerRateMetric,
  type OpsOpenAISchedulerExperienceTimeRange
} from '@/api/admin/ops'

interface Props {
  timeRange: OpsOpenAISchedulerExperienceTimeRange
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAISchedulerExperienceResponse | null>(null)

let requestController: AbortController | null = null
let requestSequence = 0

const rateMetrics = computed(() => {
  const metrics = response.value?.metrics
  return [
    { id: 'auto-recovery', label: 'admin.ops.openaiSchedulerExperience.metrics.autoRecoveryRate', metric: metrics?.auto_recovery_rate },
    { id: 'repeated-bad-account', label: 'admin.ops.openaiSchedulerExperience.metrics.repeatedBadAccountRate', metric: metrics?.repeated_bad_account_rate },
    { id: 'retry-budget-exhausted', label: 'admin.ops.openaiSchedulerExperience.metrics.retryBudgetExhaustedRate', metric: metrics?.retry_budget_exhausted_rate },
    { id: 'sticky-kept', label: 'admin.ops.openaiSchedulerExperience.metrics.stickyKeptRate', metric: metrics?.sticky_kept_rate },
    { id: 'sticky-escape', label: 'admin.ops.openaiSchedulerExperience.metrics.stickyEscapeRate', metric: metrics?.sticky_escape_rate },
    { id: 'top-k-filtered', label: 'admin.ops.openaiSchedulerExperience.metrics.topKFilteredRate', metric: metrics?.top_k_filtered_rate },
    { id: 'ttft-report-eligible', label: 'admin.ops.openaiSchedulerExperience.metrics.ttftReportEligibleRate', metric: metrics?.ttft_report_eligible_rate }
  ]
})

const hasNoData = computed(() => !loading.value && !errorMessage.value && response.value?.sample_size === 0)

function isCanceledRequest(error: unknown): boolean {
  return !!error && typeof error === 'object' && 'code' in error && (error as { code?: unknown }).code === 'ERR_CANCELED'
}

function abortRequest() {
  requestController?.abort()
  requestController = null
}

function buildParams(): OpsOpenAISchedulerExperienceParams {
  const params: OpsOpenAISchedulerExperienceParams = { time_range: props.timeRange }
  const platform = props.platformFilter.trim()
  if (platform) params.platform = platform
  if (typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0) params.group_id = props.groupIdFilter
  return params
}

function statusLabel(status?: OpsOpenAISchedulerMetricStatus): string {
  if (status === 'insufficient_data') return t('admin.ops.openaiSchedulerExperience.status.insufficientData')
  if (status === 'no_data') return t('admin.ops.openaiSchedulerExperience.status.noData')
  return ''
}

function formatRate(metric?: OpsOpenAISchedulerRateMetric): string {
  if (metric?.status !== 'ok' || typeof metric.value !== 'number' || !Number.isFinite(metric.value)) return statusLabel(metric?.status)
  return `${(metric.value * 100).toFixed(1)}%`
}

function formatAttempts(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value.toFixed(2)
}

function formatTime(value?: string | null): string {
  if (!value) return t('admin.ops.openaiSchedulerExperience.status.noData')
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

async function loadData() {
  abortRequest()
  requestSequence += 1
  const sequence = requestSequence
  requestController = new AbortController()
  loading.value = true
  errorMessage.value = ''

  try {
    response.value = await opsAPI.getOpenAISchedulerExperience(buildParams(), { signal: requestController.signal })
  } catch (error: any) {
    if (sequence !== requestSequence || isCanceledRequest(error)) return
    response.value = null
    errorMessage.value = error?.message || t('admin.ops.openaiSchedulerExperience.failedToLoad')
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
      requestController = null
    }
  }
}

watch(
  () => [props.timeRange, props.platformFilter, props.groupIdFilter, props.refreshToken] as const,
  () => {
    void loadData()
  },
  { immediate: true }
)

onUnmounted(abortRequest)
</script>

<template>
  <section data-test="scheduler-experience-card" class="card min-w-0 overflow-hidden p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h3 class="break-words text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.openaiSchedulerExperience.title') }}
        </h3>
        <p class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.openaiSchedulerExperience.sampleSize', { count: response?.sample_size ?? 0 }) }}
          <template v-if="response?.latest_event_at">
            · {{ t('admin.ops.openaiSchedulerExperience.latestEvent') }} {{ formatTime(response.latest_event_at) }}
          </template>
        </p>
      </div>
      <span v-if="loading" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.loadingText') }}</span>
    </div>

    <div v-if="errorMessage" class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      <div class="break-words">{{ errorMessage }}</div>
      <button data-test="retry" class="btn btn-secondary btn-sm mt-2" :disabled="loading" @click="loadData">
        {{ t('admin.ops.openaiSchedulerExperience.retry') }}
      </button>
    </div>

    <div v-else-if="loading && !response" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <EmptyState
      v-else-if="hasNoData"
      :title="t('common.noData')"
      :description="t('admin.ops.openaiSchedulerExperience.empty')"
    />

    <div v-else-if="response" data-test="scheduler-metrics-grid" class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <article
        data-metric="average-attempts"
        class="min-w-0 rounded-xl border border-gray-200 p-3 dark:border-dark-700"
      >
        <div class="break-words text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiSchedulerExperience.metrics.averageAttempts') }}</div>
        <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatAttempts(response.metrics.average_attempts.value) }}</div>
        <div class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.openaiSchedulerExperience.sampleSize', { count: response.metrics.average_attempts.sample_size }) }}
          <template v-if="response.metrics.average_attempts.p95 !== null && response.metrics.average_attempts.p95 !== undefined">
            · {{ t('admin.ops.openaiSchedulerExperience.p95', { value: response.metrics.average_attempts.p95 }) }}
          </template>
        </div>
      </article>

      <article
        v-for="item in rateMetrics"
        :key="item.id"
        :data-metric="item.id"
        class="min-w-0 rounded-xl border border-gray-200 p-3 dark:border-dark-700"
      >
        <div class="break-words text-xs text-gray-500 dark:text-gray-400">{{ t(item.label) }}</div>
        <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ formatRate(item.metric) }}</div>
        <div class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.openaiSchedulerExperience.ratio', { numerator: item.metric?.numerator ?? 0, denominator: item.metric?.denominator ?? 0 }) }}
        </div>
      </article>
    </div>
  </section>
</template>
