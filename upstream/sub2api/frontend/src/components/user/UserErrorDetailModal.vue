<template>
  <BaseDialog :show="show" :title="t('usage.errors.detail.title')" width="wide" @close="handleClose">
    <!-- Loading -->
    <div v-if="loading" class="flex justify-center py-10" role="status" aria-live="polite">
      <svg class="h-7 w-7 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
      </svg>
      <span class="sr-only">{{ t('usage.detail.loading') }}</span>
    </div>

    <!-- Error state -->
    <div v-else-if="loadError" class="py-8 text-center text-sm text-red-500" role="alert">
      <p>{{ t('usage.errors.detail.loadFailed') }}</p>
      <button
        type="button"
        data-testid="retry-user-error-detail"
        class="mt-3 rounded-md border border-red-300 px-3 py-1.5 font-medium text-red-600 transition-colors hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/30"
        @click="retryDetail"
      >
        {{ t('usage.detail.retry') }}
      </button>
    </div>

    <!-- Detail content -->
    <div v-else-if="detail" class="space-y-4 text-sm">
      <div class="grid grid-cols-2 gap-x-6 gap-y-3">
        <!-- Request ID -->
        <div v-if="detail.request_id" class="col-span-2">
          <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.detail.requestId') }}</span>
          <div class="mt-0.5 flex min-w-0 items-start gap-2">
            <span
              data-testid="user-error-request-id"
              class="min-w-0 flex-1 break-all font-mono text-gray-900 dark:text-dark-100"
            >{{ detail.request_id }}</span>
            <button
              type="button"
              data-testid="copy-user-error-request-id"
              class="inline-flex h-8 w-8 flex-none items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-dark-200"
              :title="t('usage.detail.copyRequestId')"
              :aria-label="t('usage.detail.copyRequestId')"
              @click="copyRequestId"
            >
              <Icon name="copy" size="sm" />
            </button>
          </div>
        </div>
        <!-- Time -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.time') }}</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ formatDateTime(detail.created_at) }}</p>
        </div>
        <!-- Model -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.model') }}</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.model || '-' }}</p>
        </div>
        <!-- Endpoint -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.endpoint') }}</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.inbound_endpoint || '-' }}</p>
        </div>
        <!-- Category -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.category') }}</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ t('usage.errors.categories.' + detail.category) }}</p>
        </div>
      </div>

      <div>
        <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.detail.meaning') }}</span>
        <p data-testid="user-error-meaning" class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.meaning }}</p>
      </div>

      <div>
        <span class="font-medium text-gray-500 dark:text-dark-400">{{ t('usage.errors.detail.suggestion') }}</span>
        <p data-testid="user-error-suggestion" class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.suggestion }}</p>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getMyErrorDetail } from '@/api/usage'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import type { UserErrorRequestDetail } from '@/types'

const props = defineProps<{
  show: boolean
  errorId: number | null
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const loading = ref(false)
const loadError = ref(false)
const detail = ref<UserErrorRequestDetail | null>(null)
let requestSequence = 0

function clearState() {
  loading.value = false
  loadError.value = false
  detail.value = null
}

watch(
  () => [props.show, props.errorId] as const,
  ([show, id]) => {
    if (!show || id == null || id <= 0) {
      requestSequence += 1
      clearState()
      return
    }
    void fetchDetail(id)
  }
)

async function fetchDetail(id: number) {
  const sequence = ++requestSequence
  loading.value = true
  loadError.value = false
  detail.value = null
  try {
    const result = await getMyErrorDetail(id)
    if (sequence === requestSequence && props.show) {
      detail.value = result
    }
  } catch (e) {
    if (sequence === requestSequence && props.show) {
      console.error('[UserErrorDetailModal] Failed to load error detail:', e)
      loadError.value = true
    }
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
    }
  }
}

function retryDetail() {
  const id = props.errorId
  if (id == null || id <= 0 || !props.show) return
  void fetchDetail(id)
}

function handleClose() {
  requestSequence += 1
  clearState()
  emit('update:show', false)
}

async function copyRequestId() {
  if (!detail.value?.request_id) return
  await copyToClipboard(detail.value.request_id, t('usage.detail.copied'))
}

onBeforeUnmount(() => {
  requestSequence += 1
})
</script>
