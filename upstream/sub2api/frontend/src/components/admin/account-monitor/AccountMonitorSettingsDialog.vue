<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accountMonitor.settings.title')"
    width="narrow"
    @close="emit('close')"
  >
    <form id="account-monitor-settings-form" class="space-y-4" @submit.prevent="save">
      <label class="block">
        <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.accountMonitor.settings.intervalLabel') }}
        </span>
        <input
          v-model.number="draft"
          type="number"
          min="15"
          max="3600"
          step="1"
          class="input w-full"
          :aria-describedby="error ? 'account-monitor-interval-error' : undefined"
        />
      </label>
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accountMonitor.settings.intervalHint') }}
      </p>
      <p v-if="error" id="account-monitor-interval-error" class="text-sm text-red-600 dark:text-red-400">
        {{ error }}
      </p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" form="account-monitor-settings-form" class="btn btn-primary" :disabled="saving">
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = withDefaults(defineProps<{
  show: boolean
  intervalSeconds: number
  saving?: boolean
  error?: string | null
}>(), {
  saving: false,
  error: null,
})

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', value: number): void
}>()

const { t } = useI18n()
const draft = ref(props.intervalSeconds)

watch(
  () => [props.show, props.intervalSeconds] as const,
  ([show, interval]) => {
    if (show) draft.value = interval
  },
  { immediate: true },
)

function save() {
  const value = Math.round(Number(draft.value))
  if (!Number.isFinite(value) || value < 15 || value > 3600) return
  emit('save', value)
}
</script>
