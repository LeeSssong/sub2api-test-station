<template>
  <component :is="inline ? 'div' : BaseDialog" v-if="!inline || show" :show="show" :title="t('channelMonitorV3.incidents.title')" width="wide" close-on-click-outside @close="emit('close')">
    <h4 v-if="inline" class="mb-3 text-sm font-medium">{{ t('channelMonitorV3.incidents.title') }}</h4>
    <button v-if="inline" data-native-back class="btn btn-secondary mb-4" @click="emit('close')"><Icon name="arrowLeft" size="sm" />{{ t('common.back') }}</button>
    <div class="space-y-3" data-testid="monitor-v3-incident-list">
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('channelMonitorV3.incidents.hint', { since: since ? formatMonitorV3Full(since) : '—' }) }}</p>
      <div v-if="loading && !items.length" class="space-y-2">
        <div v-for="index in 3" :key="index" class="h-16 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-700" />
      </div>
      <p v-else-if="error" role="alert" class="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-300">{{ error }} <button v-if="inline" class="btn btn-secondary" @click="load(page)">{{ t('channelMonitorV2.tool.retry') }}</button></p>
      <p v-else-if="!items.length" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400" data-testid="monitor-v3-incidents-empty">
        {{ t('channelMonitorV3.incidents.empty') }}
      </p>
      <ul v-else class="divide-y divide-gray-100 overflow-hidden rounded-xl border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <li v-for="item in items" :key="`${item.component_id}-${item.started_at}`" class="px-4 py-3" data-testid="monitor-v3-incident">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex min-w-0 items-center gap-2">
              <StatusDot :status="item.ended_at ? 'operational' : 'down'" />
              <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ item.component_name }}</span>
              <span
                class="rounded-full px-2 py-0.5 text-[10px] font-semibold"
                :class="item.ended_at ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300'"
              >
                {{ t(item.ended_at ? 'channelMonitorV3.incidents.resolved' : 'channelMonitorV3.incidents.ongoing') }}
              </span>
            </div>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ duration(item) }}</span>
          </div>
          <p class="mt-1 text-xs text-gray-600 dark:text-gray-300">
            {{ reason(item.top_error) }} · {{ t('channelMonitorV3.incidents.downSlots', { count: item.down_slots }) }}
            <template v-if="item.requests"> · {{ t('channelMonitorV3.incidents.volume', { requests: item.requests, errors: item.errors || 0 }) }}</template>
            <template v-if="item.probes"> · {{ t('channelMonitorV3.incidents.probes', { count: item.probes }) }}</template>
          </p>
          <p class="mt-0.5 text-[11px] text-gray-400 dark:text-gray-500">
            {{ formatMonitorV3Full(item.started_at) }} → {{ item.ended_at ? formatMonitorV3Full(item.ended_at) : t('channelMonitorV3.incidents.now') }}
          </p>
        </li>
      </ul>
    </div>
    <div v-if="inline" class="flex flex-wrap justify-between items-center gap-3 mt-4 text-xs">
      <span>{{ t('channelMonitorV3.incidents.total', {count:total}) }}</span>
      <div class="flex items-center gap-2"><button class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="load(page-1)">{{ t('channelMonitorV3.incidents.prev') }}</button><span>{{ page }} / {{ pages }}</span><button class="btn btn-secondary btn-sm" :disabled="loading || page >= pages" @click="load(page+1)">{{ t('channelMonitorV3.incidents.next') }}</button></div>
    </div>
    <template #footer>
      <div class="flex w-full items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('channelMonitorV3.incidents.total', { count: total }) }}</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="load(page - 1)">{{ t('channelMonitorV3.incidents.prev') }}</button>
          <span class="tabular-nums">{{ page }} / {{ pages }}</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || page >= pages" @click="load(page + 1)">{{ t('channelMonitorV3.incidents.next') }}</button>
        </div>
      </div>
    </template>
  </component>
</template>

<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getIncidents, type MonitorV3Incident } from '@/api/channelMonitorV3'
import { extractApiErrorMessage } from '@/utils/apiError'
import StatusDot from './StatusDot.vue'
import { formatMonitorV3Full } from './monitorV3'

const PAGE_SIZE = 20
const props = defineProps<{ show: boolean; admin: boolean; inline?: boolean; toolId?: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e:'load-error'):void }>()
const { t, te } = useI18n()
const items = ref<MonitorV3Incident[]>([])
const since = ref('')
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')
const pages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
let controller: AbortController | undefined

async function load(next: number) {
  controller?.abort()
  const current = new AbortController()
  controller = current
  loading.value = true
  error.value = ''
  try {
    const result = await getIncidents(next, PAGE_SIZE, props.admin, current.signal, props.toolId)
    if (current.signal.aborted) return
    items.value = result.items || []
    since.value = result.since
    total.value = result.total
    page.value = result.page
  } catch (err: unknown) {
    if (current.signal.aborted) return
    error.value = extractApiErrorMessage(err, t('channelMonitorV3.incidents.loadFailed'))
    emit('load-error')
  } finally {
    if (controller === current) loading.value = false
  }
}

function reason(category?: string) {
  const key = `channelMonitorV3.errors.${category || 'other'}`
  return te(key) ? t(key) : t('channelMonitorV3.errors.other')
}

function duration(item: MonitorV3Incident) {
  const end = item.ended_at ? new Date(item.ended_at).getTime() : Date.now()
  const minutes = Math.max(0, Math.round((end - new Date(item.started_at).getTime()) / 60000))
  if (minutes < 60) return t('channelMonitorV3.incidents.minutes', { count: minutes })
  const hours = Math.floor(minutes / 60)
  return t('channelMonitorV3.incidents.hours', { hours, minutes: minutes % 60 })
}

watch(() => [props.show, props.toolId] as const, ([open]) => {
  items.value=[];total.value=0;since.value=''
  if (open) void load(1)
  else controller?.abort()
}, { immediate: true })
onBeforeUnmount(()=>controller?.abort())
</script>
