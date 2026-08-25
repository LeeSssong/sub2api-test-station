<template>
  <div v-if="show" class="fixed inset-0 z-[80] flex bg-black/40" data-test="detection-history-panel" @click.self="emit('close')">
    <section class="ml-auto flex h-full w-full max-w-[640px] flex-col bg-white shadow-2xl dark:bg-slate-950 max-md:max-w-none" role="dialog" aria-modal="true" aria-labelledby="detection-history-title">
      <header class="flex items-start justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-slate-800">
        <div class="min-w-0"><h2 id="detection-history-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.modelDetection.historyTitle') }}</h2><p class="mt-1 truncate text-xs text-gray-500 dark:text-slate-400">{{ account?.name }} #{{ account?.account_id }}</p></div>
        <button class="icon-button shrink-0" type="button" :aria-label="t('admin.accounts.modelDetection.close')" :title="t('admin.accounts.modelDetection.close')" data-test="detection-history-close" @click="emit('close')">×</button>
      </header>

      <div class="flex flex-wrap gap-2 border-b border-gray-100 px-5 py-3 dark:border-slate-800">
        <select v-model="statusFilter" class="input h-8 text-xs" data-test="detection-history-status-filter"><option value="">{{ t('common.all') }}</option><option value="normal">{{ t('admin.accounts.modelDetection.status.normal') }}</option><option value="abnormal">{{ t('admin.accounts.modelDetection.status.abnormal') }}</option><option value="insufficient">{{ t('admin.accounts.modelDetection.status.insufficient') }}</option></select>
        <select v-model="profileFilter" class="input h-8 text-xs" data-test="detection-history-profile-filter"><option value="">{{ t('admin.accounts.modelDetection.profile') }}</option><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select>
      </div>

      <div v-if="loading && !items.length" class="px-5 py-8 text-center text-sm text-gray-500" role="status">{{ t('admin.accounts.modelDetection.loading') }}</div>
      <div v-else-if="loadError" class="px-5 py-8 text-center text-sm text-red-600 dark:text-red-300" data-test="detection-history-error" role="alert"><p>{{ t('admin.accounts.modelDetection.historyLoadError') }}</p><button class="btn btn-secondary mt-3" type="button" @click="load()">{{ t('common.refresh') }}</button></div>
      <div v-else-if="!items.length" class="px-5 py-8 text-center text-sm text-gray-500" data-test="detection-history-empty">{{ t('admin.accounts.modelDetection.historyEmpty') }}</div>
      <div v-else class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <div class="hidden overflow-hidden rounded-lg border border-gray-200 md:block dark:border-slate-800" data-test="detection-history-table">
          <table class="w-full table-fixed text-left text-xs"><thead class="bg-gray-50 text-gray-500 dark:bg-slate-900 dark:text-slate-400"><tr><th class="w-[132px] px-3 py-2">时间</th><th class="w-[68px] px-2 py-2">{{ t('admin.accounts.modelDetection.profile') }}</th><th class="px-2 py-2">{{ t('admin.accounts.modelDetection.reason') }}</th><th class="w-[100px] px-2 py-2">{{ t('admin.accounts.modelDetection.samples') }}</th><th class="w-[72px] px-2 py-2">状态</th></tr></thead><tbody><template v-for="item in items" :key="item.run_id"><tr class="border-t border-gray-100 dark:border-slate-800"><td class="px-3 py-3 text-gray-600 dark:text-slate-300">{{ formatTime(item.finished_at || item.queued_at) }}</td><td class="px-2 py-3 font-mono">{{ item.profile || 'unknown' }}</td><td class="break-words px-2 py-3 text-gray-600 dark:text-slate-300">{{ item.trigger_reason || '--' }}</td><td class="px-2 py-3 font-mono">{{ item.valid_samples ?? 0 }}/{{ item.planned_requests ?? 0 }}</td><td class="px-2 py-3"><button class="font-semibold" :class="statusClass(item.status)" type="button" data-test="detection-history-row" @click="toggle(item.run_id)">{{ statusLabel(item.status) }}</button></td></tr><tr v-if="selected === item.run_id" class="border-t border-dashed border-gray-200 bg-gray-50 dark:border-slate-800 dark:bg-slate-900/50"><td colspan="5" class="px-3 py-3"><Detail :item="item" /></td></tr></template></tbody></table>
        </div>
        <div class="space-y-3 md:hidden" data-test="detection-history-timeline"><article v-for="item in items" :key="item.run_id" class="border-l-2 border-gray-300 pl-3 dark:border-slate-700"><button class="w-full text-left" type="button" data-test="detection-history-timeline-row" @click="toggle(item.run_id)"><div class="flex items-center justify-between gap-2"><strong class="font-mono text-xs">{{ t('admin.accounts.modelDetection.profile') }} {{ item.profile || 'unknown' }}</strong><span :class="statusClass(item.status)" class="font-semibold text-xs">{{ statusLabel(item.status) }}</span></div><p class="mt-1 text-[11px] text-gray-500 dark:text-slate-400">{{ formatTime(item.finished_at || item.queued_at) }} · {{ item.valid_samples ?? 0 }}/{{ item.planned_requests ?? 0 }} {{ item.trigger_reason || '' }}</p></button><div v-if="selected === item.run_id" class="mt-2 rounded-md bg-gray-50 p-3 dark:bg-slate-900/60"><Detail :item="item" /></div></article></div>
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
      return h('div', { class: 'space-y-1 text-[11px] text-gray-600 dark:text-slate-300', 'data-test': 'detection-history-detail' }, [
        h('p', `${t('admin.accounts.modelDetection.mode')}：${item.mode || 'historical'} · ${t('admin.accounts.modelDetection.reason')}：${item.trigger_reason || '--'}`),
        h('p', `${t('admin.accounts.modelDetection.juice')}：${item.juice_status || '--'}${safeJuice ? ` · ${safeJuice}` : ''}`),
        h('p', `${t('admin.accounts.modelDetection.fingerprint')}：${fingerprint}`),
        h('p', `模型：${item.claimed_model || '--'} · detector ${item.detector_version || '--'}`),
        item.source === 'historical_final' ? h('p', { class: 'text-amber-700 dark:text-amber-300' }, t('admin.accounts.modelDetection.historical')) : null,
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
function statusClass(status?: string) { return status === 'normal' ? 'text-emerald-600 dark:text-emerald-300' : status === 'abnormal' ? 'text-amber-600 dark:text-amber-300' : status === 'insufficient' ? 'text-orange-600 dark:text-orange-300' : 'text-gray-500' }
function formatTime(value?: string) { if (!value) return '--'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date) }
function safeSummary(value?: Record<string, unknown>) {
  if (!value) return ''
  const allowed = Object.entries(value).filter(([key, raw]) => ['score', 'evidence_version', 'sample_count', 'status'].includes(key) && (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean')).slice(0, 6)
  return allowed.map(([key, raw]) => `${key}=${String(raw).slice(0, 80)}`).join(' · ')
}
</script>
