<template>
  <div v-if="show" class="fixed inset-0 z-[70] flex items-center justify-center bg-black/40 p-4" data-test="model-detection-dialog" @click.self="emit('close')">
    <section class="w-full max-w-lg rounded-xl bg-white p-5 shadow-xl dark:bg-slate-900" role="dialog" aria-modal="true" aria-labelledby="model-detection-dialog-title">
      <header class="flex items-start justify-between gap-3">
        <div>
          <h2 id="model-detection-dialog-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.modelDetection.title') }}</h2>
          <p class="mt-1 text-xs text-gray-500 dark:text-slate-400">{{ account?.name }} #{{ account?.account_id }}</p>
        </div>
        <button type="button" class="icon-button" data-test="model-detection-close" :aria-label="t('admin.accounts.modelDetection.close')" @click="emit('close')">×</button>
      </header>

      <div class="mt-4 space-y-3">
        <label class="block text-xs font-medium text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.connectionProbeModel') }}
          <select v-model="connectionModel" class="input mt-1 w-full" data-test="connection-model-select">
            <option v-for="option in models?.connection_models ?? []" :key="option.id" :value="option.id">{{ option.id }}</option>
          </select>
        </label>
        <p v-if="detectorOffline" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200" data-test="detector-availability">{{ detectorAvailabilityLabel }}</p>
        <label class="block text-xs font-medium text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.detectionModel') }}
          <select v-model="detectionModel" class="input mt-1 w-full" data-test="detection-model-select" :disabled="detectorOffline">
            <option v-for="option in models?.detection_models ?? []" :key="option.id" :value="option.id" :disabled="detectorOffline || !option.supported">{{ option.id }}{{ option.supported ? '' : ` (${optionHint(option)})` }}</option>
          </select>
        </label>
      </div>

      <div class="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-slate-700 dark:bg-slate-950/50" data-test="model-detection-details">
        <div class="flex items-center justify-between gap-2"><span class="text-gray-500 dark:text-slate-400">{{ t('admin.accounts.modelDetection.recentStatus') }}</span><strong :class="statusClass">{{ statusLabel }}</strong></div>
        <p v-if="recent?.claimed_model" class="mt-2 text-xs text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.declaredModel') }}：{{ recent.claimed_model }}</p>
        <div v-if="evidence" class="mt-2 space-y-1 rounded-md border border-amber-200/70 bg-amber-50/50 p-2 dark:border-amber-900/50 dark:bg-amber-950/20" data-test="model-detection-evidence">
          <p v-if="evidence.verdict" class="text-xs font-semibold text-amber-800 dark:text-amber-200" data-test="model-detection-verdict">{{ t(`admin.accounts.modelDetection.verdict.${evidence.verdict}`) }}</p>
          <p v-if="evidence.requestedModel" class="text-xs text-gray-700 dark:text-slate-200" data-test="model-detection-requested-model">{{ t('admin.accounts.modelDetection.requestedModel') }}：{{ evidence.requestedModel }}</p>
          <p class="text-xs text-gray-700 dark:text-slate-200" data-test="model-detection-response-model">{{ t('admin.accounts.modelDetection.upstreamResponseModel') }}：{{ responseModelLabel }}</p>
          <p class="break-words text-xs text-gray-700 dark:text-slate-200" data-test="model-detection-catalog">{{ t('admin.accounts.modelDetection.catalogEvidence') }}：{{ catalogLabel }}</p>
          <p class="break-words text-xs text-gray-700 dark:text-slate-200" data-test="model-detection-fingerprint">{{ t('admin.accounts.modelDetection.fingerprintEvidence') }}：{{ fingerprintLabel }}</p>
        </div>
        <p v-if="recent?.juice_status" class="mt-1 text-xs text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.juice') }}：{{ recent.juice_status }}</p>
        <p v-if="recent?.juice_summary" class="mt-1 break-words text-xs text-gray-600 dark:text-slate-300" data-test="model-detection-juice-summary">{{ t('admin.accounts.modelDetection.juiceSummary') }}：{{ formatSummary(recent.juice_summary) }}</p>
        <p v-if="recent?.fingerprint_candidate" class="mt-1 text-xs text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.fingerprintCandidate') }}：{{ recent.fingerprint_candidate }}</p>
        <p v-if="recent?.fingerprint_similarity" class="mt-1 break-words text-xs text-gray-600 dark:text-slate-300" data-test="model-detection-fingerprint-similarity">{{ t('admin.accounts.modelDetection.fingerprintSimilarity') }}：{{ formatSummary(recent.fingerprint_similarity) }}</p>
        <p v-if="recent?.detector_version" class="mt-1 text-xs text-gray-600 dark:text-slate-300">{{ t('admin.accounts.modelDetection.detectorVersion') }}：{{ recent.detector_version }}</p>
        <p v-if="recentTime" class="mt-1 text-xs text-gray-600 dark:text-slate-300" data-test="model-detection-finished-at">{{ t('admin.accounts.modelDetection.detectionTime') }}：{{ recentTime }}</p>
        <p v-if="recent?.error_code || recent?.error_message" class="mt-1 break-words text-xs text-red-600 dark:text-red-300" data-test="model-detection-error">{{ t('admin.accounts.modelDetection.error') }}：{{ [recent.error_code, recent.error_message].filter(Boolean).join(' · ') }}</p>
        <p v-if="status === 'abnormal'" class="mt-2 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.modelDetection.abnormalDisclaimer') }}</p>
      </div>

      <footer class="mt-5 flex flex-wrap justify-end gap-2">
        <button type="button" class="btn btn-secondary" data-test="model-detection-save" :disabled="saving" @click="emit('save', { connectionModel, detectionModel })">{{ t('admin.accounts.modelDetection.saveModels') }}</button>
        <button type="button" class="btn btn-primary" data-test="model-detection-run" :disabled="detecting || detectorOffline || status === 'unsupported'" @click="emit('detect')">{{ detecting ? t('admin.accounts.modelDetection.detecting') : t('admin.accounts.modelDetection.detectNow') }}</button>
      </footer>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountModelDetectionModelOption, AccountModelDetectionModelsResponse, AccountModelDetectionStatus, AccountModelDetectionSummary, AccountMonitorAccount } from '@/api/admin/accountMonitor'

const props = withDefaults(defineProps<{
  show: boolean
  account: AccountMonitorAccount | null
  models?: AccountModelDetectionModelsResponse | null
  saving?: boolean
  detecting?: boolean
}>(), { models: null, saving: false, detecting: false })
const { t } = useI18n()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', payload: { connectionModel: string; detectionModel: string }): void
  (event: 'detect'): void
}>()

const connectionModel = ref('')
const detectionModel = ref('')
watch(() => props.models, (value) => {
  connectionModel.value = value?.connection_probe_model ?? props.account?.connection_probe_model ?? ''
  detectionModel.value = value?.model_detection_model ?? props.account?.model_detection?.settings.model_detection_model ?? ''
}, { immediate: true })

const recent = computed<AccountModelDetectionSummary | null>(() => props.account?.model_detection?.recent ?? null)
const status = computed<AccountModelDetectionStatus>(() => props.account?.model_detection?.status ?? 'untested')
const detectorOffline = computed(() => modelsState.value === 'unconfigured' || modelsState.value === 'unavailable')
const modelsState = computed(() => props.models?.detector_state ?? 'unconfigured')
const detectorAvailabilityLabel = computed(() => modelsState.value === 'unconfigured' ? t('admin.accounts.modelDetection.detectorUnconfigured') : t('admin.accounts.modelDetection.detectorUnavailable'))
const statusLabel = computed(() => t(`admin.accounts.modelDetection.status.${status.value}`))
const statusClass = computed(() => ({ 'text-emerald-600 dark:text-emerald-300': status.value === 'normal', 'text-amber-600 dark:text-amber-300': ['queued', 'running', 'abnormal', 'insufficient'].includes(status.value), 'text-red-600 dark:text-red-300': ['failed', 'service_unavailable'].includes(status.value), 'text-gray-500 dark:text-slate-400': ['untested', 'unsupported', 'service_unconfigured'].includes(status.value) }))
const recentTime = computed(() => formatDetectionTime(recent.value?.finished_at ?? recent.value?.started_at ?? recent.value?.queued_at))

type EvidenceStatus = 'match' | 'mismatch' | 'missing' | 'unavailable'
type EvidenceVerdict = 'verified' | 'suspected_mapping' | 'suspected_replacement' | 'high_risk_inconsistent' | 'insufficient'
type DetectionEvidence = {
  requestedModel: string
  verdict: EvidenceVerdict | ''
  catalog: { status: EvidenceStatus; returnedCount: number; returnedModels: string[] }
  activeResponse: { status: EvidenceStatus; returnedModel: string }
  fingerprint: { status: EvidenceStatus; candidate: string; similarity: number | null }
}

const evidence = computed<DetectionEvidence | null>(() => parseEvidence(recent.value?.juice_summary))
const responseModelLabel = computed(() => {
  if (!evidence.value) return ''
  if (evidence.value.activeResponse.status === 'missing') return t('admin.accounts.modelDetection.upstreamModelMissing')
  if (evidence.value.activeResponse.status === 'unavailable') return t('admin.accounts.modelDetection.activeResponseUnavailable')
  return evidence.value.activeResponse.returnedModel || t('admin.accounts.modelDetection.upstreamModelMissing')
})
const catalogLabel = computed(() => {
  if (!evidence.value) return ''
  const catalog = evidence.value.catalog
  if (catalog.status === 'unavailable') return t('admin.accounts.modelDetection.catalogUnavailable')
  const matched = catalog.status === 'match'
    ? t('admin.accounts.modelDetection.catalogMatch')
    : t('admin.accounts.modelDetection.catalogMissing')
  const models = catalog.returnedModels.join('、') || '--'
  return `${matched}；${t('admin.accounts.modelDetection.catalogReturned', { count: catalog.returnedCount, models })}`
})
const fingerprintLabel = computed(() => {
  if (!evidence.value) return ''
  const fingerprint = evidence.value.fingerprint
  if (fingerprint.status === 'unavailable') return t('admin.accounts.modelDetection.fingerprintUnavailable')
  if (fingerprint.status === 'match') return t('admin.accounts.modelDetection.fingerprintMatch')
  const similarity = fingerprint.similarity == null ? '--' : `${Math.round(fingerprint.similarity * 100)}%`
  return t('admin.accounts.modelDetection.fingerprintMismatch', { candidate: fingerprint.candidate || '--', similarity })
})

function formatSummary(value: Record<string, unknown>): string {
  try {
    return JSON.stringify(value)
  } catch {
    return '--'
  }
}

function parseEvidence(value?: Record<string, unknown>): DetectionEvidence | null {
  if (!value || value.evidence_version !== 'model-detection-evidence-v1') return null
  const catalog = asRecord(value.catalog)
  const activeResponse = asRecord(value.active_response)
  const fingerprint = asRecord(value.fingerprint)
  const verdict = asVerdict(value.verdict)
  if (!catalog || !activeResponse || !fingerprint || !verdict) return null
  return {
    requestedModel: asString(value.requested_model),
    verdict,
    catalog: {
      status: asEvidenceStatus(catalog.status),
      returnedCount: asNumber(catalog.returned_count),
      returnedModels: asStringArray(catalog.returned_models),
    },
    activeResponse: {
      status: asEvidenceStatus(activeResponse.status),
      returnedModel: asString(activeResponse.returned_model),
    },
    fingerprint: {
      status: asEvidenceStatus(fingerprint.status),
      candidate: asString(fingerprint.candidate),
      similarity: asNullableNumber(fingerprint.similarity),
    },
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function asNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function asNullableNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(Boolean).slice(0, 10) : []
}

function asEvidenceStatus(value: unknown): EvidenceStatus {
  return value === 'match' || value === 'mismatch' || value === 'missing' || value === 'unavailable' ? value : 'unavailable'
}

function asVerdict(value: unknown): EvidenceVerdict | '' {
  return value === 'verified' || value === 'suspected_mapping' || value === 'suspected_replacement' || value === 'high_risk_inconsistent' || value === 'insufficient' ? value : ''
}

function optionHint(option: AccountModelDetectionModelOption): string {
  if (modelsState.value === 'unconfigured') return t('admin.accounts.modelDetection.detectorUnconfigured')
  if (modelsState.value === 'unavailable') return t('admin.accounts.modelDetection.detectorUnavailable')
  return option.reason === 'detector_unsupported' ? t('admin.accounts.modelDetection.detectorUnsupported') : t('admin.accounts.modelDetection.detectorUnsupported')
}

function formatDetectionTime(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}
</script>
