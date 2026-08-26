<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4 backdrop-blur-[2px] sm:p-6" data-test="detection-history-panel" @click.self="emit('close')">
    <section class="flex max-h-[calc(100dvh-2rem)] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-slate-600 bg-slate-800 text-slate-100 shadow-xl sm:max-h-[calc(100dvh-3rem)]" role="dialog" aria-modal="true" aria-labelledby="detection-history-title">
      <header class="flex min-h-[54px] items-center justify-between gap-4 border-b border-slate-600 px-5">
        <h2 id="detection-history-title" class="text-sm font-semibold text-white">{{ t('admin.accounts.modelDetection.historyTitle') }}</h2>
        <button class="icon-button shrink-0 text-slate-400 hover:text-white" type="button" :aria-label="t('admin.accounts.modelDetection.close')" :title="t('admin.accounts.modelDetection.close')" data-test="detection-history-close" @click="emit('close')"><Icon name="x" size="sm" /></button>
      </header>

      <div class="flex min-h-0 flex-1 flex-col p-4 sm:p-5">
        <div class="mb-4 flex flex-wrap items-start justify-between gap-x-5 gap-y-2"><div class="min-w-0"><p class="text-sm font-semibold text-white">{{ account?.name }}</p><p class="mt-1 text-[11px] text-slate-400">#{{ account?.account_id }}</p></div><p class="pt-0.5 text-[11px] text-slate-400">{{ items.length }}{{ t('admin.accounts.modelDetection.historyCountSuffix') }}</p></div>

        <div class="grid grid-cols-1 gap-2 rounded-lg border border-slate-600 bg-slate-800/80 p-3 sm:grid-cols-3" data-test="detection-history-filters">
          <label class="grid gap-1 text-[10px] text-slate-300" for="detection-history-juice-filter">{{ t('admin.accounts.modelDetection.juiceFilter') }}
            <select id="detection-history-juice-filter" v-model="juiceFilter" class="h-8 rounded-md border border-slate-600 bg-slate-700 px-2.5 text-xs text-slate-100 outline-none transition focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400/30" data-test="detection-history-juice-filter"><option value="">{{ t('common.all') }}</option><option value="pass">{{ juiceLabel('pass') }}</option><option value="mismatch">{{ juiceLabel('mismatch') }}</option><option value="insufficient">{{ juiceLabel('insufficient') }}</option><option value="non_gpt">{{ juiceLabel('non_gpt') }}</option></select>
          </label>
          <label class="grid gap-1 text-[10px] text-slate-300" for="detection-history-fingerprint-filter">{{ t('admin.accounts.modelDetection.fingerprintFilter') }}
            <select id="detection-history-fingerprint-filter" v-model="fingerprintFilter" class="h-8 rounded-md border border-slate-600 bg-slate-700 px-2.5 text-xs text-slate-100 outline-none transition focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400/30" data-test="detection-history-fingerprint-filter"><option value="">{{ t('common.all') }}</option><option value="strong_match">{{ fingerprintLabel('strong_match') }}</option><option value="unclear">{{ fingerprintLabel('unclear') }}</option><option value="unavailable">{{ fingerprintLabel('unavailable') }}</option></select>
          </label>
          <label class="grid gap-1 text-[10px] text-slate-300" for="detection-history-conclusion-filter">{{ t('admin.accounts.modelDetection.conclusionFilter') }}
            <select id="detection-history-conclusion-filter" v-model="statusFilter" class="h-8 rounded-md border border-slate-600 bg-slate-700 px-2.5 text-xs text-slate-100 outline-none transition focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400/30" data-test="detection-history-conclusion-filter"><option value="">{{ t('common.all') }}</option><option value="normal">{{ conclusionLabel('normal') }}</option><option value="abnormal">{{ conclusionLabel('abnormal') }}</option><option value="insufficient">{{ conclusionLabel('insufficient') }}</option><option value="failed">{{ conclusionLabel('failed') }}</option></select>
          </label>
        </div>

        <div v-if="loading && !items.length" class="py-10 text-center text-xs text-slate-400" role="status">{{ t('admin.accounts.modelDetection.loading') }}</div>
        <div v-else-if="loadError" class="py-10 text-center text-xs text-rose-300" data-test="detection-history-error" role="alert"><p>{{ t('admin.accounts.modelDetection.historyLoadError') }}</p><button class="btn btn-secondary mt-3" type="button" @click="load()">{{ t('common.refresh') }}</button></div>
        <div v-else-if="!items.length" class="py-10 text-center text-xs text-slate-400" data-test="detection-history-empty">{{ t('admin.accounts.modelDetection.historyEmpty') }}</div>
        <div v-else class="mt-4 min-h-0 flex-1 overflow-y-auto">
          <div class="hidden overflow-x-auto rounded-lg border border-slate-600 md:block" data-test="detection-history-table">
            <table class="min-w-[780px] w-full table-fixed text-left text-[11px]"><thead class="bg-slate-700 text-slate-300"><tr><th class="w-[116px] px-3 py-3 font-medium">{{ t('admin.accounts.modelDetection.time') }}</th><th class="w-[110px] px-3 py-3 font-medium">{{ t('admin.accounts.modelDetection.profileAndSamples') }}</th><th class="w-[170px] px-3 py-3 font-medium">{{ t('admin.accounts.modelDetection.juiceFilter') }}</th><th class="px-3 py-3 font-medium">{{ t('admin.accounts.modelDetection.fingerprint') }}</th><th class="w-[120px] px-3 py-3 font-medium">{{ t('admin.accounts.modelDetection.conclusionFilter') }}</th><th class="w-10 px-2 py-3" aria-label=""></th></tr></thead>
              <tbody><template v-for="item in items" :key="item.run_id"><tr class="border-t border-slate-600 transition-colors hover:bg-slate-700/70" data-test="detection-history-row" @click="toggle(item.run_id)"><td class="whitespace-nowrap px-3 py-3 text-slate-200">{{ formatTime(item.finished_at || item.queued_at) }}</td><td class="px-3 py-3"><div class="text-slate-100">{{ profileLabel(item) }}</div><div class="mt-1 font-mono text-[10px] text-slate-400">{{ samplesLabel(item) }}</div></td><td class="px-3 py-3"><EvidenceBadge kind="juice" :label="juiceLabelFor(item)" :tone="juiceClass(item)" /></td><td class="px-3 py-3"><EvidenceBadge kind="fingerprint" :label="fingerprintLabelFor(item)" :tone="fingerprintClass(item)" /></td><td class="px-3 py-3"><span class="font-semibold" :class="statusClass(item.status)">{{ conclusionLabelFor(item) }}</span></td><td class="px-2 py-3 text-right"><button class="icon-button h-7 w-7 text-slate-400 hover:text-white" type="button" :aria-label="t('admin.accounts.modelDetection.details')" :aria-expanded="selected === item.run_id" @click.stop="toggle(item.run_id)"><Icon name="chevronDown" size="xs" class="transition-transform motion-reduce:transition-none" :class="{ 'rotate-180': selected === item.run_id }" /></button></td></tr><tr v-if="selected === item.run_id" class="border-t border-slate-600 bg-slate-900/35"><td colspan="6" class="p-3"><Detail :item="item" /></td></tr></template></tbody>
            </table>
          </div>
          <div class="space-y-2 md:hidden" data-test="detection-history-timeline"><article v-for="item in items" :key="item.run_id" class="overflow-hidden rounded-lg border border-slate-600 bg-slate-800"><button class="w-full p-3 text-left" type="button" data-test="detection-history-timeline-row" :aria-expanded="selected === item.run_id" @click="toggle(item.run_id)"><div class="flex items-start justify-between gap-3"><div><p class="text-[11px] text-slate-300">{{ formatTime(item.finished_at || item.queued_at) }}</p><p class="mt-1 text-xs font-semibold text-white">{{ profileLabel(item) }} <span class="font-mono text-[10px] font-normal text-slate-400">{{ samplesLabel(item) }}</span></p></div><span class="text-xs font-semibold" :class="statusClass(item.status)">{{ conclusionLabelFor(item) }}</span></div><div class="mt-3 grid grid-cols-2 gap-2"><EvidenceBadge kind="juice" :label="juiceLabelFor(item)" :tone="juiceClass(item)" :compact="false" /><EvidenceBadge kind="fingerprint" :label="fingerprintLabelFor(item)" :tone="fingerprintClass(item)" :compact="false" /></div></button><div v-if="selected === item.run_id" class="border-t border-slate-600 p-3"><Detail :item="item" /></div></article></div>
          <button v-if="nextCursor" class="btn btn-secondary mt-4 w-full" type="button" data-test="detection-history-load-more" :disabled="loading" @click="loadMore">{{ loading ? t('admin.accounts.modelDetection.loading') : t('admin.accounts.modelDetection.loadMore') }}</button>
        </div>
        <p v-if="items.length" class="mt-3 text-[10px] text-slate-400">{{ t('admin.accounts.modelDetection.historyDetailsHint') }}</p>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { modelDetectionHistory, type AccountModelDetectionHistoryParams, type AccountModelDetectionSummary } from '@/api/admin/accountMonitor'

const props = defineProps<{ show: boolean; account: { account_id: number; name?: string } | null }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const items = ref<AccountModelDetectionSummary[]>([])
const nextCursor = ref('')
const loading = ref(false)
const loadError = ref(false)
const selected = ref<string | undefined>()
const statusFilter = ref('')
const juiceFilter = ref('')
const fingerprintFilter = ref('')

const EvidenceBadge = defineComponent({
  props: { kind: { type: String, required: true }, label: { type: String, required: true }, tone: { type: String, required: true }, compact: { type: Boolean, default: true } },
  setup(evidenceProps) {
    return () => h('div', { class: ['flex min-w-0 items-start gap-1.5', evidenceProps.tone] }, [h('span', { class: 'shrink-0 rounded bg-slate-600 px-1.5 py-1 text-[10px] font-medium text-slate-100' }, evidenceProps.kind === 'juice' ? 'Juice' : t('admin.accounts.modelDetection.fingerprintShort')), h('strong', { class: [evidenceProps.compact ? 'truncate' : 'break-words leading-4', 'min-w-0 text-[11px] font-semibold'] }, evidenceProps.label)])
  },
})

function detailFact(label: string, value: string) { return h('div', [h('p', { class: 'mb-1 text-slate-500' }, label), h('p', { class: 'text-slate-200' }, value)]) }
const Detail = defineComponent({
  props: { item: { type: Object, required: true } },
  setup(detailProps) {
    return () => {
      const item = detailProps.item as AccountModelDetectionSummary
      return h('div', { class: 'grid gap-3 sm:grid-cols-2', 'data-test': 'detection-history-detail' }, [
        h('section', { class: 'rounded-md border border-slate-600 bg-slate-800 p-3' }, [h('p', { class: 'text-[10px] text-slate-400' }, t('admin.accounts.modelDetection.juiceFilter')), h('strong', { class: ['mt-1 block text-sm', juiceClass(item)] }, juiceLabelFor(item)), h('p', { class: 'mt-1.5 text-[11px] leading-5 text-slate-300' }, juiceDetail(item))]),
        h('section', { class: 'rounded-md border border-slate-600 bg-slate-800 p-3' }, [h('p', { class: 'text-[10px] text-slate-400' }, t('admin.accounts.modelDetection.fingerprint')), h('strong', { class: ['mt-1 block text-sm', fingerprintClass(item)] }, fingerprintLabelFor(item)), h('p', { class: 'mt-1.5 text-[11px] leading-5 text-slate-300' }, fingerprintDetail(item))]),
        h('div', { class: 'flex flex-wrap gap-x-7 gap-y-3 px-1 text-[10px] text-slate-400 sm:col-span-2' }, [detailFact(t('admin.accounts.modelDetection.reason'), reasonLabel(item)), detailFact(t('admin.accounts.modelDetection.samples'), samplesLabel(item)), detailFact(t('admin.accounts.modelDetection.declaredModel'), item.claimed_model || t('admin.accounts.modelDetection.evidenceUnavailable')), detailFact(t('admin.accounts.modelDetection.detectorVersion'), item.detector_version || t('admin.accounts.modelDetection.evidenceUnavailable'))]),
      ])
    }
  },
})

watch(() => [props.show, props.account?.account_id, statusFilter.value, juiceFilter.value, fingerprintFilter.value], () => {
  if (!props.show || !props.account) return
  items.value = []; nextCursor.value = ''; selected.value = undefined; loadError.value = false
  void load()
}, { immediate: true })

async function load(cursor = '') {
  if (!props.account || loading.value) return
  loading.value = true; loadError.value = false
  const params: AccountModelDetectionHistoryParams = { limit: 25 }
  if (cursor) params.cursor = cursor
  if (statusFilter.value) params.status = statusFilter.value
  if (juiceFilter.value) params.juice_status = juiceFilter.value
  if (fingerprintFilter.value) params.fingerprint_status = fingerprintFilter.value
  try { const page = await modelDetectionHistory(props.account.account_id, params); items.value = cursor ? [...items.value, ...page.items] : page.items; nextCursor.value = page.next_cursor || '' } catch { loadError.value = true } finally { loading.value = false }
}
function loadMore() { if (nextCursor.value) void load(nextCursor.value) }
function toggle(id?: string) { selected.value = selected.value === id ? undefined : id }
function isHistorical(item: AccountModelDetectionSummary) { return item.source === 'historical' || item.source === 'historical_final' }
function conclusionLabel(status?: string) { return t(`admin.accounts.modelDetection.conclusion.${status || 'failed'}`) }
function conclusionLabelFor(item: AccountModelDetectionSummary) { return isHistorical(item) ? t('admin.accounts.modelDetection.historical') : conclusionLabel(item.status) }
function statusClass(status?: string) { return status === 'normal' ? 'text-emerald-300' : status === 'insufficient' ? 'text-amber-300' : status === 'abnormal' || status === 'failed' ? 'text-rose-300' : 'text-slate-400' }
function juiceLabel(status?: string) { return status ? t(`admin.accounts.modelDetection.juiceStatus.${status}`) : t('admin.accounts.modelDetection.evidenceUnavailable') }
function juiceLabelFor(item: AccountModelDetectionSummary) { return isHistorical(item) ? t('admin.accounts.modelDetection.historical') : juiceLabel(item.juice_status) }
function juiceClass(item: AccountModelDetectionSummary) { return item.juice_status === 'pass' ? 'text-emerald-300' : item.juice_status === 'mismatch' ? 'text-rose-300' : item.juice_status === 'insufficient' ? 'text-amber-300' : 'text-slate-400' }
function fingerprintLabel(status?: string) { return status ? t(`admin.accounts.modelDetection.fingerprintStatus.${status}`) : t('admin.accounts.modelDetection.evidenceUnavailable') }
function fingerprintLabelFor(item: AccountModelDetectionSummary) { if (isHistorical(item)) return t('admin.accounts.modelDetection.historical'); const base = fingerprintLabel(item.fingerprint_status); return item.fingerprint_candidate ? `${base} ${item.fingerprint_candidate}` : base }
function fingerprintClass(item: AccountModelDetectionSummary) { return item.fingerprint_status === 'strong_match' ? 'text-sky-300' : item.fingerprint_status === 'unclear' ? 'text-slate-300' : 'text-slate-400' }
function profileLabel(item: AccountModelDetectionSummary) { return item.profile === 'low' ? t('admin.accounts.modelDetection.profileNames.low') : item.profile === 'medium' ? t('admin.accounts.modelDetection.profileNames.medium') : item.profile === 'high' ? t('admin.accounts.modelDetection.profileNames.high') : t('admin.accounts.modelDetection.profileNames.unknown') }
function reasonLabel(item: AccountModelDetectionSummary) { return item.trigger_reason ? t(`admin.accounts.modelDetection.reasonValue.${item.trigger_reason}`) : t('admin.accounts.modelDetection.historical') }
function samplesLabel(item: AccountModelDetectionSummary) { if (isHistorical(item)) return t('admin.accounts.modelDetection.samplesUnavailable'); if (item.evidence_state !== 'complete' && (item.valid_samples == null || item.valid_samples === 0)) return t('admin.accounts.modelDetection.evidenceUnavailable'); return `${item.valid_samples ?? 0} / ${item.planned_requests ?? 0}` }
function formatTime(value?: string) { if (!value) return '--'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date) }
function safeSummary(value?: Record<string, unknown>) { if (!value) return ''; const allowed = Object.entries(value).filter(([key, raw]) => ['score', 'evidence_version', 'sample_count', 'status'].includes(key) && (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean')).slice(0, 6); return allowed.map(([key, raw]) => `${key}=${String(raw).slice(0, 80)}`).join(' · ') }
function juiceDetail(item: AccountModelDetectionSummary) { if (isHistorical(item)) return t('admin.accounts.modelDetection.historicalRecordHint'); const summary = safeSummary(item.juice_summary); return summary || t(`admin.accounts.modelDetection.juiceDetail.${item.juice_status || 'unavailable'}`) }
function fingerprintDetail(item: AccountModelDetectionSummary) { if (isHistorical(item)) return t('admin.accounts.modelDetection.historicalRecordHint'); return item.fingerprint_candidate ? t('admin.accounts.modelDetection.fingerprintCandidateDetail', { candidate: item.fingerprint_candidate }) : t(`admin.accounts.modelDetection.fingerprintDetail.${item.fingerprint_status || 'unavailable'}`) }
</script>
