<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { SchedulerLogDetail, SchedulerLogRange, SchedulerLogSummary } from '@/api/admin/schedulerLogs'

const { t } = useI18n()
const range = ref<SchedulerLogRange>('1h')
const loading = ref(true)
const loadingDetail = ref(false)
const error = ref('')
const items = ref<SchedulerLogSummary[]>([])
const nextCursor = ref<string | null>(null)
const incomplete = ref(false)
const droppedCount = ref(0)
const selectedID = ref('')
const detail = ref<SchedulerLogDetail | null>(null)

const selected = computed(() => items.value.find((item) => item.logical_request_id === selectedID.value) ?? null)

function shortID(value: string): string {
  return value.length > 18 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value
}
function formatTime(value?: string): string {
  if (!value) return '-'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}
function outcomeClass(value?: string): string {
  return value === 'success' ? 'log-status-success' : value === 'failure' ? 'log-status-failure' : ''
}
function decisionValue(event: { decision?: Record<string, unknown> }, key: string): string {
  const value = event.decision?.[key]
  if (value == null || value === '') return '-'
  return String(value)
}

async function load(reset = true): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const response = await adminAPI.schedulerLogs.list({ time_range: range.value, limit: 50, cursor: reset ? undefined : nextCursor.value ?? undefined })
    items.value = reset ? response.items : [...items.value, ...response.items]
    nextCursor.value = response.next_cursor ?? null
    incomplete.value = Boolean(response.incomplete)
    droppedCount.value = response.dropped_count ?? 0
    if (reset) { selectedID.value = ''; detail.value = null }
  } catch {
    error.value = t('admin.schedulerLogs.loadFailed')
  } finally {
    loading.value = false
  }
}

async function select(item: SchedulerLogSummary): Promise<void> {
  selectedID.value = item.logical_request_id
  detail.value = null
  loadingDetail.value = true
  try { detail.value = await adminAPI.schedulerLogs.getDetail(item.logical_request_id) }
  catch { error.value = t('admin.schedulerLogs.detailFailed') }
  finally { loadingDetail.value = false }
}

async function selectRange(value: SchedulerLogRange): Promise<void> { range.value = value; await load(true) }
onMounted(() => void load(true))
</script>

<template>
  <AppLayout>
    <main class="scheduler-log-page">
      <header class="scheduler-log-header">
        <div><p class="scheduler-log-kicker">OpenAI / Codex</p><h1>{{ t('admin.schedulerLogs.title') }}</h1><p>{{ t('admin.schedulerLogs.description') }}</p></div>
        <div class="scheduler-log-range" role="group" :aria-label="t('admin.schedulerLogs.range')">
          <button v-for="value in (['1h','24h','7d'] as SchedulerLogRange[])" :key="value" type="button" :class="{ active: range === value }" @click="selectRange(value)">{{ value === '1h' ? t('admin.schedulerLogs.oneHour') : value === '24h' ? t('admin.schedulerLogs.day') : t('admin.schedulerLogs.week') }}</button>
        </div>
      </header>
      <p v-if="incomplete" class="scheduler-log-warning" role="status">{{ t('admin.schedulerLogs.incomplete', { count: droppedCount }) }}</p>
      <p v-if="error" class="scheduler-log-error" role="alert">{{ error }}</p>
      <div class="scheduler-log-workspace">
        <section class="scheduler-log-list" :aria-label="t('admin.schedulerLogs.requests')">
          <div class="scheduler-log-list-head"><span>{{ t('admin.schedulerLogs.request') }}</span><span>{{ t('admin.schedulerLogs.runtime') }}</span></div>
          <div v-if="loading && !items.length" class="scheduler-log-empty">{{ t('common.loading') }}</div>
          <div v-else-if="!items.length" class="scheduler-log-empty">{{ t('admin.schedulerLogs.empty') }}</div>
          <button v-for="item in items" :key="item.logical_request_id" type="button" data-testid="scheduler-log-row" class="scheduler-log-row" :class="{ selected: selectedID === item.logical_request_id }" @click="select(item)">
            <span class="scheduler-log-request"><strong>{{ shortID(item.logical_request_id) }}</strong><small>{{ formatTime(item.started_at) }} · {{ item.canonical_model || '-' }}</small><small>{{ t('admin.schedulerLogs.account') }} #{{ item.selected_account_id || '-' }}</small></span>
            <span class="scheduler-log-runtime"><strong>{{ item.algorithm_version || '-' }}</strong><small>{{ t('admin.schedulerLogs.budget') }} {{ item.runtime_retry_budget }} · {{ t('admin.schedulerLogs.switches') }} {{ item.switch_count }}</small><small :class="outcomeClass(item.final_outcome)">{{ item.final_outcome || '-' }}</small></span>
          </button>
          <button v-if="nextCursor" type="button" class="scheduler-log-more" :disabled="loading" @click="load(false)">{{ t('admin.schedulerLogs.loadMore') }}</button>
        </section>
        <section class="scheduler-log-detail" :aria-label="t('admin.schedulerLogs.detail')">
          <div v-if="!selected" class="scheduler-log-empty">{{ t('admin.schedulerLogs.selectRequest') }}</div>
          <div v-else-if="loadingDetail" class="scheduler-log-empty">{{ t('common.loading') }}</div>
          <template v-else-if="detail">
            <header class="scheduler-log-detail-head"><div><h2>{{ shortID(detail.logical_request_id) }}</h2><p>{{ detail.algorithm_version }}</p></div><span :class="outcomeClass(detail.final_outcome)">{{ detail.final_outcome }}</span></header>
            <div class="scheduler-log-facts"><div><small>{{ t('admin.schedulerLogs.algorithm') }}</small><strong>{{ detail.algorithm_version }}</strong></div><div><small>{{ t('admin.schedulerLogs.budget') }}</small><strong>{{ detail.runtime_retry_budget }}</strong></div><div><small>{{ t('admin.schedulerLogs.switches') }}</small><strong>{{ detail.switch_count }}</strong></div></div>
            <h3>{{ t('admin.schedulerLogs.attemptTimeline') }}</h3>
            <ol class="scheduler-log-timeline">
              <li v-for="event in detail.events" :key="event.id"><span class="scheduler-log-dot"/><div><strong>{{ t('admin.schedulerLogs.attempt') }} {{ event.attempt_number || 1 }} · {{ event.event_name }}</strong><p>{{ formatTime(event.event_at) }} · {{ t('admin.schedulerLogs.account') }} #{{ event.account_id || '-' }} · {{ event.selection_layer || '-' }}</p><p>{{ t('admin.schedulerLogs.statusCode') }} {{ decisionValue(event, 'status_code') }} · {{ t('admin.schedulerLogs.rank') }} {{ decisionValue(event, 'selected_rank') }} · {{ t('admin.schedulerLogs.score') }} {{ decisionValue(event, 'quality_score') }}</p><p>{{ t('admin.schedulerLogs.replay') }} {{ decisionValue(event, 'safe_to_replay') }} · {{ decisionValue(event, 'switch_reason') }}</p></div></li>
            </ol>
          </template>
        </section>
      </div>
    </main>
  </AppLayout>
</template>

<style scoped>
.scheduler-log-page{max-width:1440px;margin:0 auto;color:rgb(15 23 42)}.dark .scheduler-log-page{color:rgb(226 232 240)}.scheduler-log-header{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem;margin-bottom:1rem}.scheduler-log-header h1{margin:0;font-size:1.5rem;font-weight:700;letter-spacing:0}.scheduler-log-header p{margin:.35rem 0 0;color:rgb(100 116 139);font-size:.875rem}.scheduler-log-kicker{color:rgb(13 148 136)!important;font-size:.75rem!important;font-weight:700;text-transform:uppercase}.scheduler-log-range{display:flex;padding:3px;border:1px solid rgb(203 213 225);border-radius:7px;background:white}.dark .scheduler-log-range{border-color:rgb(51 65 85);background:rgb(15 29 52)}.scheduler-log-range button{padding:.45rem .65rem;border:0;border-radius:5px;color:rgb(100 116 139);background:transparent;font-size:.75rem}.scheduler-log-range button.active{color:white;background:rgb(15 118 110)}.scheduler-log-warning,.scheduler-log-error{margin:0 0 .75rem;padding:.65rem .8rem;border-left:3px solid rgb(217 119 6);background:rgb(255 251 235);font-size:.8125rem}.dark .scheduler-log-warning{background:rgb(66 44 18)}.scheduler-log-error{border-color:rgb(220 38 38);background:rgb(254 242 242)}.scheduler-log-workspace{display:grid;grid-template-columns:minmax(360px,.85fr) minmax(480px,1.15fr);min-height:570px;overflow:hidden;border:1px solid rgb(203 213 225);border-radius:8px;background:white}.dark .scheduler-log-workspace{border-color:rgb(42 59 87);background:rgb(11 21 37)}.scheduler-log-list{border-right:1px solid rgb(226 232 240)}.dark .scheduler-log-list{border-color:rgb(42 59 87)}.scheduler-log-list-head,.scheduler-log-row{display:grid;grid-template-columns:1fr 1fr;gap:.8rem;padding:.75rem}.scheduler-log-list-head{color:rgb(100 116 139);background:rgb(248 250 252);font-size:.7rem;font-weight:700;text-transform:uppercase}.dark .scheduler-log-list-head{background:rgb(15 29 52)}.scheduler-log-row{width:100%;min-height:86px;border:0;border-top:1px solid rgb(226 232 240);color:inherit;background:transparent;text-align:left}.dark .scheduler-log-row{border-color:rgb(32 49 73)}.scheduler-log-row:hover,.scheduler-log-row.selected{background:rgb(240 253 250)}.dark .scheduler-log-row:hover,.dark .scheduler-log-row.selected{background:rgb(16 43 48)}.scheduler-log-row.selected{box-shadow:inset 3px 0 rgb(20 184 166)}.scheduler-log-request,.scheduler-log-runtime{min-width:0}.scheduler-log-request strong,.scheduler-log-runtime strong{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:.8rem}.scheduler-log-row small{display:block;margin-top:.3rem;color:rgb(100 116 139);font-size:.7rem}.log-status-success{color:rgb(5 150 105)!important}.log-status-failure{color:rgb(220 38 38)!important}.scheduler-log-detail{min-width:0;padding:1rem}.scheduler-log-empty{display:grid;min-height:180px;place-items:center;color:rgb(100 116 139);font-size:.875rem}.scheduler-log-more{width:100%;padding:.7rem;border:0;border-top:1px solid rgb(226 232 240);color:rgb(13 148 136);background:transparent}.scheduler-log-detail-head{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem;padding-bottom:.9rem;border-bottom:1px solid rgb(226 232 240)}.dark .scheduler-log-detail-head{border-color:rgb(42 59 87)}.scheduler-log-detail-head h2{margin:0;font-size:1rem}.scheduler-log-detail-head p{margin:.3rem 0 0;color:rgb(100 116 139);font-size:.75rem}.scheduler-log-facts{display:grid;grid-template-columns:repeat(3,1fr);gap:.5rem;margin:1rem 0}.scheduler-log-facts div{min-width:0;padding:.75rem;border:1px solid rgb(226 232 240);border-radius:6px}.dark .scheduler-log-facts div{border-color:rgb(42 59 87)}.scheduler-log-facts small{display:block;color:rgb(100 116 139);font-size:.7rem}.scheduler-log-facts strong{display:block;overflow-wrap:anywhere;margin-top:.35rem;font-size:.8rem}.scheduler-log-detail h3{margin:1rem 0 .75rem;font-size:.8rem}.scheduler-log-timeline{margin:0;padding:0;list-style:none}.scheduler-log-timeline li{position:relative;display:grid;grid-template-columns:12px 1fr;gap:.65rem;padding:0 0 1rem}.scheduler-log-timeline li:not(:last-child)::before{content:'';position:absolute;left:5px;top:12px;bottom:0;width:1px;background:rgb(203 213 225)}.scheduler-log-dot{position:relative;z-index:1;width:11px;height:11px;margin-top:3px;border:2px solid rgb(20 184 166);border-radius:50%;background:white}.dark .scheduler-log-dot{background:rgb(11 21 37)}.scheduler-log-timeline strong{font-size:.78rem}.scheduler-log-timeline p{margin:.3rem 0 0;color:rgb(100 116 139);font-size:.72rem;line-height:1.45}@media(max-width:900px){.scheduler-log-header{flex-direction:column}.scheduler-log-workspace{grid-template-columns:1fr}.scheduler-log-list{border-right:0;border-bottom:1px solid rgb(226 232 240)}.scheduler-log-facts{grid-template-columns:1fr}.scheduler-log-range{width:100%}.scheduler-log-range button{flex:1}}
</style>
