<template>
  <component
    :is="inline ? 'div' : BaseDialog"
    v-if="!inline || show"
    :show="show"
    :title="title"
    width="wide"
    @close="$emit('close')"
  >
    <h4 v-if="inline" class="mb-3 text-sm font-medium">{{ title }}</h4>
    <button v-if="inline" data-native-back class="btn btn-secondary mb-4" @click="$emit('close')"><Icon name="arrowLeft" size="sm" />{{ t('common.back') }}</button>
    <div v-if="loading" class="py-8 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="!detail" class="py-8 text-center text-sm text-gray-500">
      {{ t('channelStatus.detailLoadError') }}
      <button v-if="inline && monitorId != null" class="btn btn-secondary" @click="load(monitorId)">{{ t('channelMonitorV2.tool.retry') }}</button>
    </div>
    <div v-else class="overflow-x-auto">
      <table class="w-full text-left text-sm">
        <thead class="border-b border-gray-200 dark:border-dark-700">
          <tr class="text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.model') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.latestStatus') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.latestLatency') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability7d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability15d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.availability30d') }}</th>
            <th class="py-2 pr-3">{{ t('channelStatus.detailColumns.avgLatency7d') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="m in detail.models"
            :key="m.model"
            class="border-b border-gray-100 dark:border-dark-800"
          >
            <td class="py-2 pr-3 font-medium text-gray-900 dark:text-gray-100">{{ formatMonitorModel(m.model) }}</td>
            <td class="py-2 pr-3">
              <span
                class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px]"
                :class="statusBadgeClass(m.latest_status)"
              >
                {{ statusLabel(m.latest_status) }}
              </span>
            </td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatLatency(m.latest_latency_ms) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(inline && m.has_samples_7d !== true ? null : m.availability_7d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(inline && m.has_samples_15d !== true ? null : m.availability_15d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatPercent(inline && m.has_samples_30d !== true ? null : m.availability_30d) }}</td>
            <td class="py-2 pr-3 text-gray-700 dark:text-gray-300">{{ formatLatency(m.avg_latency_7d_ms) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('channelStatus.closeDetail') }}
        </button>
      </div>
    </template>
  </component>
</template>

<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  status as fetchChannelMonitorDetail,
  type UserMonitorDetail,
} from '@/api/channelMonitor'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const props = defineProps<{
  inline?: boolean
  toolId?: string
  show: boolean
  monitorId: number | null
  title: string
}>()

const emit=defineEmits<{
  (e: 'close'): void
  (e: 'load-error'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { statusLabel, statusBadgeClass, formatLatency, formatPercent, formatMonitorModel } = useChannelMonitorFormat()

const detail = ref<UserMonitorDetail | null>(null)
const loading = ref(false)
let requestVersion = 0
let controller: AbortController | undefined

async function load(id: number) {
  controller?.abort();const current = new AbortController();controller = current
  const version = ++requestVersion
  detail.value = null
  loading.value = true
  try {
    const result = await fetchChannelMonitorDetail(id,{signal:current.signal,toolId:props.toolId})
    if (!current.signal.aborted && version === requestVersion) detail.value = result
  } catch (err: unknown) {
    if (current.signal.aborted || version !== requestVersion) return
    if(props.inline)emit('load-error')
    else appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

watch(
  () => [props.show, props.monitorId, props.toolId] as const,
  ([show, id], _, onCleanup) => {
    onCleanup(() => { requestVersion++;controller?.abort() })
    if (!show) {
      detail.value = null
      return
    }
    if (id != null) void load(id)
  },
  { immediate: true },
)
onBeforeUnmount(()=>{requestVersion++;controller?.abort()})
</script>
