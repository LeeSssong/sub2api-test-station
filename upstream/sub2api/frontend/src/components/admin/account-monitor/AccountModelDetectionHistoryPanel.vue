<template>
  <div v-if="show" class="fixed inset-0 z-[80] flex bg-slate-950/70 backdrop-blur-[2px]" data-test="detection-history-panel" @click.self="emit('close')">
    <section class="ml-auto flex h-full w-full max-w-[640px] flex-col border-l border-slate-700/80 bg-slate-950 text-slate-100 shadow-[-12px_0_32px_rgba(2,6,23,0.28)] dark:bg-slate-950 max-md:max-w-none max-md:border-l-0" role="dialog" aria-modal="true" aria-labelledby="detection-history-title">
      <header class="flex items-start justify-between gap-4 border-b border-slate-800 px-6 py-5 max-sm:px-4">
        <div class="min-w-0"><p class="text-[11px] font-semibold uppercase tracking-[0.16em] text-cyan-300/80">{{ t('admin.accounts.modelDetection.historyEyebrow') }}</p><h2 id="detection-history-title" class="mt-1 text-lg font-semibold text-white">{{ t('admin.accounts.modelDetection.historyTitle') }}</h2><p class="mt-1 truncate text-xs text-slate-400">{{ account?.name }} #{{ account?.account_id }}</p></div>
        <button class="icon-button shrink-0" type="button" :aria-label="t('admin.accounts.modelDetection.close')" :title="t('admin.accounts.modelDetection.close')" data-test="detection-history-close" @click="emit('close')">×</button>
      </header>

      <div class="border-b border-slate-800 px-6 py-4 max-sm:px-4">
        <div class="flex items-center justify-between gap-3"><div><p class="text-sm font-medium text-slate-100">{{ t('admin.accounts.modelDetection.historySummaryTitle') }}</p><p class="mt-1 text-xs text-slate-400">{{ t('admin.accounts.modelDetection.historySummaryHint') }}</p></div><span class="rounded-full border border-cyan-400/30 bg-cyan-400/10 px-2.5 py-1 text-[11px] font-medium text-cyan-200">{{ items.length }}{{ t('admin.accounts.modelDetection.historyCountSuffix') }}</span></div>
        <div class="mt-4 grid grid-cols-2 gap-3">
          <label class="text-[11px] font-medium text-slate-400">{{ t('admin.accounts.modelDetection.statusFilter') }}<select v-model="statusFilter" class="mt-1 h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-3 text-xs text-slate-100 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/20" data-test="detection-history-status-filter"><option value="">{{ t('common.all') }}</option><option value="normal">{{ t('admin.accounts.modelDetection.status.normal') }}</option><option value="abnormal">{{ t('admin.accounts.modelDetection.status.abnormal') }}</option><option value="insufficient">{{ t('admin.accounts.modelDetection.status.insufficient') }}</option></select></label>
          <label class="text-[11px] font-medium text-slate-400">{{ t('admin.accounts.modelDetection.profileFilter') }}<select v-model="profileFilter" class="mt-1 h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-3 text-xs text-slate-100 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/20" data-test="detection-history-profile-filter"><option value="">{{ t('common.all') }}</option><option value="low">{{ t('admin.accounts.modelDetection.profileNames.low') }}</option><option value="medium">{{ t('admin.accounts.modelDetection.profileNames.medium') }}</option><option value="high">{{ t('admin.accounts.modelDetection.profileNames.high') }}</option><option value="unknown">{{ t('admin.accounts.modelDetection.profileNames.unknown') }}</option></select></label>
        </div>
      </div>

      <div v-if="loading && !items.length" class="px-5 py-8 text-center text-sm text-gray-500" role="status">{{ t('admin.accounts.modelDetection.loading') }}</div>
      <div v-else-if="loadError" class="px-5 py-8 text-center text-sm text-red-600 dark:text-red-300" data-test="detection-history-error" role="alert"><p>{{ t('admin.accounts.modelDetection.historyLoadError') }}</p><button class="btn btn-secondary mt-3" type="button" @click="load()">{{ t('common.refresh') }}</button></div>
      <div v-else-if="!items.length" class="px-5 py-8 text-center text-sm text-gray-500" data-test="detection-history-empty">{{ t('admin.accounts.modelDetection.historyEmpty') }}</div>
      <div v-else class="min-h-0 flex-1 overflow-y-auto px-6 py-5 max-sm:px-4">
        <div class="hidden overflow-hidden rounded-lg border border-slate-800 md:block" data-test="detection-history-table">
          <table class="w-full table-fixed text-left text-xs"><thead class="bg-slate-900/90 text-slate-400"><tr><th class="w-[138px] px-4 py-3">{{ t('admin.accounts.modelDetection.time') }}</th><th class="w-[110px] px-3 py-3">{{ t('admin.accounts.modelDetection.profile') }}</th><th class="w-[116px] px-3 py-3">{{ t('admin.accounts.modelDetection.mode') }}</th><th class="px-3 py-3">{{ t('admin.accounts.modelDetection.reason') }}</th><th class="w-[106px] px-3 py-3">{{ t('admin.accounts.modelDetection.samples') }}</th><th class="w-[100px] px-3 py-3">{{ t('admin.accounts.modelDetection.statusLabel') }}</th></tr></thead><tbody><template v-for="item in items" :key="item.run_id"><tr class="border-t border-slate-800 transition-colors hover:bg-slate-900/70"><td class="px-4 py-3 text-slate-300">{{ formatTime(item.finished_at || item.queued_at) }}</td><td class="px-3 py-3"><span class="rounded-full border px-2 py-1 font-mono text-[11px]" :class="profileClass(item)">{{ profileLabel(item) }}</span></td><td class="px-3 py-3 text-slate-300">{{ modeLabel(item) }}</td><td class="break-words px-3 py-3 text-slate-300">{{ reasonLabel(item) }}</td><td class="px-3 py-3 font-mono text-slate-200">{{ samplesLabel(item) }}</td><td class="px-3 py-3"><button class="rounded-md px-2 py-1 font-semibold transition hover:bg-slate-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/60" :class="statusClass(item.status)" type="button" data-test="detection-history-row" @click="toggle(item.run_id)">{{ statusLabel(item.status) }}</button></td></tr><tr v-if="selected === item.run_id" class="border-t border-slate-800 bg-slate-900/50"><td colspan="6" class="px-4 py-4"><Detail :item="item" /></td></tr></template></tbody></table>
        </div>
        <div class="space-y-3 md:hidden" data-test="detection-history-timeline"><article v-for="item in items" :key="item.run_id" class="rounded-lg border border-slate-800 bg-slate-900/45"><button class="w-full p-4 text-left" type="button" data-test="detection-history-timeline-row" @click="toggle(item.run_id)"><div class="flex items-start justify-between gap-3"><div><strong class="text-sm text-white">{{ profileLabel(item) }}</strong><p class="mt-1 text-[11px] text-slate-400">{{ formatTime(item.finished_at || item.queued_at) }} · {{ modeLabel(item) }}</p></div><span :class="statusClass(item.status)" class="rounded-full border px-2 py-1 text-[11px] font-semibold">{{ statusLabel(item.status) }}</span></div><div class="mt-3 flex items-center justify-between text-[11px] text-slate-400"><span>{{ reasonLabel(item) }}</span><span class="font-mono text-slate-200">{{ samplesLabel(item) }}</span></div></button><div v-if="selected === item.run_id" class="border-t border-slate-800 px-4 py-4"><Detail :item="item" /></div></article></div>
        <button v-if="nextCursor" class="btn btn-secondary mt-4 w-full" type="button" data-test="detection-history-load-more" :disabled="loading" @click="loadMore">{{ loading ? t('admin.accounts.modelDetection.loading') : t('admin.accounts.modelDetection.loadMore') }}</button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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
const profileFilter = ref('')

const Detail = defineComponent({
  props: { item: { type: Object, required: true } },
  setup(detailProps) {
    return () => {
      const item = detailProps.item as AccountModelDetectionSummary
      const safeJuice = safeSummary(item.juice_summary)
      const fingerprint = item.fingerprint_candidate ? `${item.fingerprint_candidate}${item.fingerprint_status ? ` · ${item.fingerprint_status}` : ''}` : '--'
      return h('div', { class: 'space-y-2 text-[11px] text-slate-300', 'data-test': 'detection-history-detail' }, [
        h('p', `${t('admin.accounts.modelDetection.mode')}：${modeLabel(item)} · ${t('admin.accounts.modelDetection.reason')}：${reasonLabel(item)}`),
        h('p', `${t('admin.accounts.modelDetection.juice')}：${item.juice_status || t('admin.accounts.modelDetection.evidenceUnavailable')}${safeJuice ? ` · ${safeJuice}` : ''}`),
        h('p', `${t('admin.accounts.modelDetection.fingerprint')}：${fingerprint}`),
        h('p', `${t('admin.accounts.modelDetection.declaredModel')}：${item.claimed_model || '--'} · detector ${item.detector_version || '--'}`),
        item.source === 'historical' || item.source === 'historical_final' ? h('p', { class: 'text-amber-300' }, t('admin.accounts.modelDetection.historical')) : null,
      ])
    }
  },
})

watch(() => [props.show, props.account?.account_id, statusFilter.value, profileFilter.value], () => {
  if (!props.show || !props.account) return
  items.value = []
  nextCursor.value = ''
  selected.value = undefined
  loadError.value = false
  void load()
}, { immediate: true })

async function load(cursor = '') {
  if (!props.account || loading.value) return
  loading.value = true
  loadError.value = false
  const params: AccountModelDetectionHistoryParams = { limit: 25 }
  if (cursor) params.cursor = cursor
  if (statusFilter.value) params.status = statusFilter.value
  if (profileFilter.value) params.profile = profileFilter.value
  try {
    const page = await modelDetectionHistory(props.account.account_id, params)
    items.value = cursor ? [...items.value, ...page.items] : page.items
    nextCursor.value = page.next_cursor || ''
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}
function loadMore() { if (nextCursor.value) void load(nextCursor.value) }
function toggle(id?: string) { selected.value = selected.value === id ? undefined : id }
function statusLabel(status?: string) { return t(`admin.accounts.modelDetection.status.${status || 'untested'}`) }
function statusClass(status?: string) { return status === 'normal' ? 'text-emerald-300' : status === 'abnormal' ? 'text-amber-300' : status === 'insufficient' ? 'text-orange-300' : status === 'failed' ? 'text-rose-300' : 'text-slate-400' }
function profileLabel(item: AccountModelDetectionSummary) { return item.profile === 'low' ? t('admin.accounts.modelDetection.profileNames.low') : item.profile === 'medium' ? t('admin.accounts.modelDetection.profileNames.medium') : item.profile === 'high' ? t('admin.accounts.modelDetection.profileNames.high') : t('admin.accounts.modelDetection.profileNames.unknown') }
function profileClass(item: AccountModelDetectionSummary) { return item.profile === 'high' ? 'border-fuchsia-400/30 bg-fuchsia-400/10 text-fuchsia-200' : item.profile === 'medium' ? 'border-cyan-400/30 bg-cyan-400/10 text-cyan-200' : item.profile === 'low' ? 'border-slate-500 bg-slate-800 text-slate-200' : 'border-slate-700 bg-slate-900 text-slate-400' }
function modeLabel(item: AccountModelDetectionSummary) { return item.mode === 'monitor' ? t('admin.accounts.modelDetection.modeNames.monitor') : item.mode === 'manual' ? t('admin.accounts.modelDetection.modeNames.manual') : item.mode === 'escalation' ? t('admin.accounts.modelDetection.modeNames.escalation') : t('admin.accounts.modelDetection.modeNames.historical') }
function reasonLabel(item: AccountModelDetectionSummary) { return item.trigger_reason ? t(`admin.accounts.modelDetection.reasonValue.${item.trigger_reason}`) : t('admin.accounts.modelDetection.historical') }
function samplesLabel(item: AccountModelDetectionSummary) {
  if (item.source === 'historical' || item.source === 'historical_final') return t('admin.accounts.modelDetection.samplesUnavailable')
  if (item.evidence_state !== 'complete' && (item.valid_samples == null || item.valid_samples === 0)) return t('admin.accounts.modelDetection.evidenceUnavailable')
  return `${item.valid_samples ?? 0}/${item.planned_requests ?? 0}`
}
function formatTime(value?: string) { if (!value) return '--'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date) }
function safeSummary(value?: Record<string, unknown>) {
  if (!value) return ''
  const allowed = Object.entries(value).filter(([key, raw]) => ['score', 'evidence_version', 'sample_count', 'status'].includes(key) && (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean')).slice(0, 6)
  return allowed.map(([key, raw]) => `${key}=${String(raw).slice(0, 80)}`).join(' · ')
}
</script>
