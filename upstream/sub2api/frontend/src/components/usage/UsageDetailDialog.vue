<template>
  <BaseDialog
    :show="show"
    :title="t('usage.detail.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="min-h-40 pr-1">
      <div
        v-if="loading"
        class="flex min-h-40 items-center justify-center gap-3 text-sm text-gray-500 dark:text-dark-400"
        role="status"
      >
        <span
          class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-primary-600 dark:border-dark-600 dark:border-t-primary-400"
          aria-hidden="true"
        />
        {{ t('usage.detail.loading') }}
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-40 flex-col items-center justify-center gap-3 text-center"
        role="alert"
      >
        <p class="text-sm font-medium text-gray-700 dark:text-dark-200">
          {{ t('usage.detail.loadFailed') }}
        </p>
        <button
          type="button"
          data-testid="usage-detail-retry"
          class="inline-flex items-center gap-2 rounded-md border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-800"
          @click="loadDetail"
        >
          <Icon name="refresh" size="sm" />
          {{ t('usage.detail.retry') }}
        </button>
      </div>

      <div
        v-else-if="!detail"
        class="flex min-h-40 items-center justify-center text-sm text-gray-500 dark:text-dark-400"
      >
        {{ t('usage.noRecords') }}
      </div>

      <div v-else class="space-y-6">
        <dl
          v-if="adminDetail"
          class="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-3"
        >
          <DetailItem
            :label="t('admin.usageCostDetail.siteActualCost')"
            :value="formatCost(adminDetail.actual_cost)"
            numeric
            emphasized
          />
          <DetailItem
            :label="t('admin.usageCostDetail.includedCost')"
            :value="includedCostValue"
            numeric
          />
          <DetailItem
            :label="grossMarginSummaryLabel"
            :value="grossMarginValue"
            numeric
            emphasized
          />
        </dl>
        <div v-else class="flex items-center justify-between gap-4">
          <span class="text-xs font-semibold text-gray-500 dark:text-dark-400">
            {{ t('usage.detail.consumption') }}
          </span>
          <span class="font-mono text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatCost(detail.actual_cost) }}
          </span>
        </div>

        <section aria-labelledby="usage-detail-request-heading">
          <h4
            id="usage-detail-request-heading"
            class="mb-3 text-sm font-semibold text-gray-900 dark:text-white"
          >
            {{ t('usage.detail.requestInfo') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <div class="min-w-0 sm:col-span-2">
              <dt :class="labelClass">{{ t('usage.detail.requestId') }}</dt>
              <dd class="mt-1 flex min-w-0 items-start gap-2">
                <span class="min-w-0 break-all font-mono text-sm text-gray-900 dark:text-white">
                  {{ displayValue(detail.request_id) }}
                </span>
                <button
                  v-if="detail.request_id"
                  type="button"
                  data-testid="usage-detail-copy-request-id"
                  class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-200"
                  :title="t('usage.detail.copyRequestId')"
                  :aria-label="t('usage.detail.copyRequestId')"
                  @click="copyRequestId"
                >
                  <Icon name="copy" size="sm" />
                </button>
              </dd>
            </div>
            <DetailItem :label="t('usage.detail.requestTime')" :value="formatRequestTime(detail.created_at)" />
            <DetailItem :label="t('usage.inboundEndpoint')" :value="displayValue(detail.inbound_endpoint)" mono />
            <DetailItem :label="t('usage.detail.apiKey')" :value="displayValue(detail.api_key?.name)" />
            <DetailItem :label="t('usage.detail.group')" :value="displayValue(detail.group?.name)" />
            <DetailItem :label="t('usage.detail.model')" :value="displayValue(detail.model)" mono />
            <DetailItem :label="t('usage.detail.requestType')" :value="requestTypeLabel(detail)" />
            <DetailItem :label="t('usage.serviceTier')" :value="getUsageServiceTierLabel(detail.service_tier, t)" />
            <DetailItem :label="t('usage.reasoningEffort')" :value="formatReasoningEffort(detail.reasoning_effort)" />
            <DetailItem :label="t('usage.detail.firstTokenTime')" :value="formatDuration(detail.first_token_ms)" />
            <DetailItem :label="t('usage.detail.responseTime')" :value="formatDuration(detail.duration_ms)" />
            <DetailItem
              v-if="detail.ip_address"
              :label="t('admin.usage.ipAddress')"
              :value="detail.ip_address"
              mono
            />
            <DetailItem
              v-if="detail.user_agent"
              :label="t('usage.userAgent')"
              :value="detail.user_agent"
            />
          </dl>
        </section>

        <section
          class="border-t border-gray-200 pt-5 dark:border-dark-700"
          aria-labelledby="usage-detail-token-heading"
        >
          <h4
            id="usage-detail-token-heading"
            class="mb-3 text-sm font-semibold text-gray-900 dark:text-white"
          >
            {{ t('usage.detail.tokenSection') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <DetailItem :label="t('usage.detail.inputTokens')" :value="formatTokens(detail.input_tokens)" numeric />
            <DetailItem :label="t('usage.detail.outputTokens')" :value="formatTokens(detail.output_tokens)" numeric />
            <DetailItem
              v-if="detail.cache_creation_tokens > 0"
              :label="t('usage.detail.cacheCreationTokens')"
              :value="formatTokens(detail.cache_creation_tokens)"
              numeric
            />
            <DetailItem
              v-if="detail.cache_read_tokens > 0"
              :label="t('usage.detail.cacheReadTokens')"
              :value="formatTokens(detail.cache_read_tokens)"
              numeric
            />
            <DetailItem
              v-if="detail.cache_creation_5m_tokens > 0"
              :label="`${t('admin.usage.cacheCreation5mTokens')} (5m)`"
              :value="formatTokens(detail.cache_creation_5m_tokens)"
              numeric
            />
            <DetailItem
              v-if="detail.cache_creation_1h_tokens > 0"
              :label="`${t('admin.usage.cacheCreation1hTokens')} (1h)`"
              :value="formatTokens(detail.cache_creation_1h_tokens)"
              numeric
            />
            <template v-if="showImageDetails">
              <DetailItem :label="t('usage.imageCount')" :value="formatTokens(detail.image_count)" numeric />
              <DetailItem :label="t('usage.imageBillingSize')" :value="formatImageBillingSize(detail, t)" />
              <DetailItem :label="t('usage.imageInputSize')" :value="formatImageInputSize(detail, t)" />
              <DetailItem :label="t('usage.imageOutputSize')" :value="formatImageOutputSize(detail, t)" />
              <DetailItem :label="t('usage.imageSizeSource')" :value="formatImageSizeSource(detail, t)" />
              <DetailItem
                v-if="imageSizeBreakdown"
                :label="t('usage.imageSizeBreakdown')"
                :value="imageSizeBreakdown"
              />
              <DetailItem
                v-if="detail.image_input_tokens > 0"
                :label="t('usage.imageInputTokens')"
                :value="formatTokens(detail.image_input_tokens)"
                numeric
              />
              <DetailItem
                v-if="detail.image_output_tokens > 0"
                :label="t('usage.imageOutputTokens')"
                :value="formatTokens(detail.image_output_tokens)"
                numeric
              />
            </template>
          </dl>
        </section>

        <section
          class="border-t border-gray-200 pt-5 dark:border-dark-700"
          aria-labelledby="usage-detail-billing-heading"
        >
          <h4
            id="usage-detail-billing-heading"
            class="mb-3 text-sm font-semibold text-gray-900 dark:text-white"
          >
            {{ t('usage.detail.billingSection') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <DetailItem :label="t('usage.detail.billingMode')" :value="billingModeLabel(detail)" />
            <DetailItem :label="t('usage.detail.billingType')" :value="billingTypeLabel(detail.billing_type)" />
            <DetailItem :label="t('usage.detail.inputCost')" :value="formatCost(detail.input_cost)" numeric />
            <DetailItem :label="t('usage.detail.outputCost')" :value="formatCost(detail.output_cost)" numeric />
            <DetailItem
              v-if="detail.cache_creation_cost > 0"
              :label="t('usage.detail.cacheCreationCost')"
              :value="formatCost(detail.cache_creation_cost)"
              numeric
            />
            <DetailItem
              v-if="detail.cache_read_cost > 0"
              :label="t('usage.detail.cacheReadCost')"
              :value="formatCost(detail.cache_read_cost)"
              numeric
            />
            <DetailItem
              v-if="detail.image_input_cost > 0"
              :label="t('usage.imageInputCost')"
              :value="formatCost(detail.image_input_cost)"
              numeric
            />
            <DetailItem
              v-if="detail.image_output_cost > 0"
              :label="t('usage.imageOutputCost')"
              :value="formatCost(detail.image_output_cost)"
              numeric
            />
            <DetailItem :label="t('usage.detail.standardCost')" :value="formatCost(detail.total_cost)" numeric />
            <DetailItem :label="t('usage.detail.actualCost')" :value="formatCost(detail.actual_cost)" numeric emphasized />
            <DetailItem
              :label="t('usage.detail.groupMultiplier')"
              :value="`${formatMultiplier(detail.rate_multiplier ?? 1)}x`"
              numeric
            />
            <DetailItem
              v-if="detail.long_context_billing_applied"
              :label="t('admin.accounts.form.longContextBilling')"
              :value="t('common.yes')"
            />
            <DetailItem
              v-if="effectiveInputPrice != null"
              :label="t('usage.detail.effectiveInputPrice')"
              :value="`${formatCost(effectiveInputPrice)} ${t('usage.perMillionTokens')}`"
              numeric
            />
            <DetailItem
              v-if="effectiveOutputPrice != null"
              :label="t('usage.detail.effectiveOutputPrice')"
              :value="`${formatCost(effectiveOutputPrice)} ${t('usage.perMillionTokens')}`"
              numeric
            />
          </dl>
        </section>

        <section
          v-if="adminDetail"
          class="border-t border-gray-200 pt-5 dark:border-dark-700"
          aria-labelledby="usage-detail-admin-heading"
        >
          <h4
            id="usage-detail-admin-heading"
            class="mb-3 text-sm font-semibold text-gray-900 dark:text-white"
          >
            {{ t('usage.detail.adminSection') }}
          </h4>
          <dl class="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <DetailItem
              :label="t('admin.usageCostDetail.siteRequestId')"
              :value="displayValue(adminDetail.request_id)"
              mono
            />
            <DetailItem
              :label="t('admin.usageCostDetail.upstreamRequestId')"
              :value="displayValue(adminUpstreamRequestId)"
              mono
            />
            <DetailItem
              :label="t('admin.usageCostDetail.siteStandardCost')"
              :value="formatCost(adminDetail.total_cost)"
              numeric
            />
            <DetailItem
              :label="t('admin.usageCostDetail.siteActualCost')"
              :value="formatCost(adminDetail.actual_cost)"
              numeric
              emphasized
            />
            <DetailItem
              :label="t('admin.usageCostDetail.upstreamActualCost')"
              :value="upstreamActualCostValue"
              numeric
            />
            <div class="min-w-0">
              <dt :class="labelClass">{{ t('admin.usageCostDetail.costSource') }}</dt>
              <dd :class="[valueClass, 'flex flex-wrap items-center gap-2']">
                <span>{{ costSourceLabel }}</span>
                <span
                  class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="costEvidenceBadgeClass"
                >
                  {{ costEvidenceLabel }}
                </span>
              </dd>
            </div>
            <DetailItem
              :label="t('admin.usageCostDetail.siteGroupMultiplier')"
              :value="`${formatMultiplier(adminDetail.rate_multiplier ?? 1)}x`"
              numeric
            />
            <DetailItem
              :label="t('admin.usageCostDetail.upstreamMultiplier')"
              :value="`${formatMultiplier(adminDetail.account_rate_multiplier ?? 1)}x`"
              numeric
            />
            <DetailItem
              :label="t('admin.usageCostDetail.includedCost')"
              :value="includedCostValue"
              numeric
              emphasized
            />
            <DetailItem
              :label="t('admin.usageCostDetail.grossMarginStatus')"
              :value="grossMarginStatusValue"
              numeric
            />
            <DetailItem
              v-if="adminDetail.account"
              :label="t('usage.detail.account')"
              :value="`${adminDetail.account.name} (#${adminDetail.account.id})`"
            />
            <DetailItem
              v-if="adminDetail.channel_id != null"
              :label="t('usage.detail.channelId')"
              :value="String(adminDetail.channel_id)"
              numeric
            />
            <DetailItem
              v-if="adminDetail.upstream_endpoint"
              :label="t('usage.detail.upstreamEndpoint')"
              :value="adminDetail.upstream_endpoint"
              mono
            />
            <DetailItem
              v-if="adminDetail.upstream_model"
              :label="t('usage.detail.upstreamModel')"
              :value="adminDetail.upstream_model"
              mono
            />
            <DetailItem
              v-if="adminDetail.model_mapping_chain"
              :label="t('usage.detail.modelMappingChain')"
              :value="adminDetail.model_mapping_chain"
              mono
            />
            <DetailItem
              v-if="adminDetail.billing_tier"
              :label="t('usage.detail.billingTier')"
              :value="adminDetail.billing_tier"
            />
          </dl>
        </section>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminUsageAPI } from '@/api/admin/usage'
import { usageAPI } from '@/api/usage'
import { useClipboard } from '@/composables/useClipboard'
import type { AdminUsageCostDetail, AdminUsageLog, UserUsageDetail } from '@/types'
import { formatDateTime, formatReasoningEffort } from '@/utils/format'
import { formatMultiplier } from '@/utils/formatters'
import { getBillingModeLabel, getDisplayBillingMode } from '@/utils/billingMode'
import {
  formatImageBillingSize,
  formatImageInputSize,
  formatImageOutputSize,
  formatImageSizeBreakdown,
  formatImageSizeSource,
  textInputTokens,
  textOutputTokens,
} from '@/utils/imageUsage'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import {
  confirmedUpstreamActualCost,
  effectivePerMillion,
  grossMargin,
  hasAdminUsageFields,
  includedUpstreamCost,
  usageCostEvidenceState,
  type UsageDetailScope,
} from './usageDetail'

const labelClass = 'text-xs font-medium text-gray-500 dark:text-dark-400'
const valueClass = 'mt-1 min-w-0 break-words text-sm text-gray-900 dark:text-white'

const DetailItem = defineComponent({
  props: {
    emphasized: Boolean,
    label: {
      type: String,
      required: true,
    },
    mono: Boolean,
    numeric: Boolean,
    value: {
      type: String,
      required: true,
    },
  },
  setup(itemProps) {
    return () => h('div', { class: 'min-w-0' }, [
      h('dt', { class: labelClass }, itemProps.label),
      h('dd', {
        class: [
          valueClass,
          itemProps.mono ? 'break-all font-mono' : '',
          itemProps.numeric ? 'font-mono tabular-nums' : '',
          itemProps.emphasized ? 'font-semibold text-primary-700 dark:text-primary-300' : '',
        ],
      }, itemProps.value),
    ])
  },
})

const props = defineProps<{
  show: boolean
  usageId: number | null
  scope: UsageDetailScope
}>()

const emit = defineEmits<{
  'update:show': [show: boolean]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
type UsageDetailRecord = UserUsageDetail | AdminUsageLog

const detail = ref<UsageDetailRecord | null>(null)
const adminCostDetail = ref<AdminUsageCostDetail | null>(null)
const loading = ref(false)
const loadError = ref(false)
let requestSequence = 0

const adminDetail = computed<AdminUsageLog | null>(() => {
  if (props.scope !== 'admin' || !detail.value || !hasAdminUsageFields(detail.value)) {
    return null
  }
  return detail.value
})

const costEvidenceState = computed(() => usageCostEvidenceState(adminCostDetail.value))
const confirmedActualCost = computed(() => confirmedUpstreamActualCost(adminCostDetail.value))
const includedCost = computed(() => includedUpstreamCost(adminCostDetail.value))
const margin = computed(() => grossMargin(adminDetail.value?.actual_cost, adminCostDetail.value))
const adminUpstreamRequestId = computed(() => (
  adminCostDetail.value?.upstream_request_id || adminDetail.value?.upstream_request_id || null
))
const includedCostValue = computed(() => (
  includedCost.value == null
    ? t('admin.usageCostDetail.pendingReconciliation')
    : formatCost(includedCost.value)
))
const upstreamActualCostValue = computed(() => (
  confirmedActualCost.value == null ? '-' : formatCost(confirmedActualCost.value)
))
const grossMarginSummaryLabel = computed(() => (
  costEvidenceState.value === 'estimated'
    ? t('admin.usageCostDetail.estimatedGrossMargin')
    : t('admin.usageCostDetail.grossMargin')
))
const grossMarginValue = computed(() => (
  margin.value == null
    ? t('admin.usageCostDetail.pendingReconciliation')
    : formatCost(margin.value)
))
const costEvidenceLabel = computed(() => t(`admin.usageCostDetail.${costEvidenceState.value}`))
const costEvidenceBadgeClass = computed(() => {
  if (costEvidenceState.value === 'confirmed') {
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  if (costEvidenceState.value === 'estimated') {
    return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
})
const costSourceLabel = computed(() => {
  const source = adminCostDetail.value?.cost_source?.trim()
  const sourceKeys: Record<string, string> = {
    '上游逐笔账单': 'nativeLedger',
    '上游价格表推算': 'upstreamPriceTable',
    '自购账号成本分摊': 'ownedAllocation',
    '待对账': 'pending',
  }
  return source && sourceKeys[source]
    ? t(`admin.usageCostDetail.costSources.${sourceKeys[source]}`)
    : source || t('admin.usageCostDetail.costSources.pending')
})
const grossMarginStatusValue = computed(() => {
  if (margin.value == null) return t('admin.usageCostDetail.pending')
  const stateLabel = costEvidenceState.value === 'confirmed'
    ? t('admin.usageCostDetail.confirmed')
    : t('admin.usageCostDetail.estimated')
  return `${stateLabel} · ${formatCost(margin.value)}`
})

const showImageDetails = computed(() => {
  const row = detail.value
  if (!row) return false
  return row.image_count > 0
    || row.image_input_tokens > 0
    || row.image_output_tokens > 0
    || Boolean(row.image_size || row.image_input_size || row.image_output_size)
    || Boolean(row.image_size_breakdown && Object.keys(row.image_size_breakdown).length > 0)
})

const imageSizeBreakdown = computed(() => formatImageSizeBreakdown(detail.value))
const effectiveInputPrice = computed(() => (
  detail.value
    ? effectivePerMillion(detail.value.input_cost, textInputTokens(detail.value))
    : null
))
const effectiveOutputPrice = computed(() => (
  detail.value
    ? effectivePerMillion(detail.value.output_cost, textOutputTokens(detail.value))
    : null
))

function clearState() {
  loading.value = false
  loadError.value = false
  detail.value = null
  adminCostDetail.value = null
}

async function loadAdminCost(row: AdminUsageLog, sequence: number) {
  try {
    const result = await adminUsageAPI.getRequestCost({ local_request_id: row.request_id })
    if (sequence === requestSequence && props.show && props.scope === 'admin') {
      adminCostDetail.value = result ?? null
    }
  } catch {
    if (sequence === requestSequence && props.show && props.scope === 'admin') {
      adminCostDetail.value = null
    }
  }
}

async function loadDetail() {
  const id = props.usageId
  if (!props.show || id == null || id <= 0) {
    requestSequence += 1
    clearState()
    return
  }

  const sequence = ++requestSequence
  loading.value = true
  loadError.value = false
  detail.value = null
  adminCostDetail.value = null

  try {
    const result = props.scope === 'admin'
      ? await adminUsageAPI.getById(id)
      : await usageAPI.getById(id)
    if (sequence === requestSequence && props.show) {
      detail.value = result
      if (props.scope === 'admin') {
        void loadAdminCost(result as AdminUsageLog, sequence)
      }
    }
  } catch {
    if (sequence === requestSequence && props.show) {
      loadError.value = true
    }
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
    }
  }
}

function handleClose() {
  requestSequence += 1
  clearState()
  emit('update:show', false)
}

function copyRequestId() {
  if (!detail.value?.request_id) return
  void copyToClipboard(detail.value.request_id, t('usage.detail.copied'))
}

function displayValue(value: string | null | undefined): string {
  return value?.trim() || '-'
}

function formatRequestTime(value: string): string {
  return formatDateTime(value) || '-'
}

function formatDuration(milliseconds: number | null | undefined): string {
  if (milliseconds == null || !Number.isFinite(milliseconds)) return '-'
  if (milliseconds < 1000) return `${milliseconds}ms`
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(2)}s`
  const totalSeconds = Math.round(milliseconds / 1000)
  if (totalSeconds < 3600) return `${Math.floor(totalSeconds / 60)}m ${totalSeconds % 60}s`
  return `${Math.floor(totalSeconds / 3600)}h ${Math.floor((totalSeconds % 3600) / 60)}m`
}

function formatTokens(value: number | null | undefined): string {
  return Number.isFinite(value) ? (value as number).toLocaleString() : '-'
}

function formatCost(value: number | null | undefined): string {
  return `$${(Number.isFinite(value) ? value as number : 0).toFixed(6)}`
}

function requestTypeLabel(row: Pick<UsageDetailRecord, 'request_type' | 'stream' | 'openai_ws_mode'>): string {
  const type = resolveUsageRequestType(row)
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

function billingModeLabel(row: Pick<UsageDetailRecord, 'billing_mode' | 'image_count'>): string {
  return getBillingModeLabel(getDisplayBillingMode(row), t)
}

function billingTypeLabel(type: number): string {
  return type === 1
    ? t('admin.usage.billingTypeSubscription')
    : t('admin.usage.billingTypeBalance')
}

watch(
  () => [props.show, props.usageId, props.scope] as const,
  ([show]) => {
    if (!show) {
      requestSequence += 1
      clearState()
      return
    }
    void loadDetail()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  requestSequence += 1
})
</script>
