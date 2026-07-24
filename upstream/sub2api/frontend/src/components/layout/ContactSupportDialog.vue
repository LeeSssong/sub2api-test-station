<template>
  <BaseDialog
    :show="show"
    :title="t('common.contactSupportDialog.title')"
    width="narrow"
    close-on-click-outside
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div
        class="mx-auto aspect-square w-full max-w-[22rem] overflow-hidden rounded-lg border border-gray-200 bg-gray-950 dark:border-dark-600"
      >
        <img
          src="/support/qq-group-1080152144.png"
          :alt="t('common.contactSupportDialog.qrAlt')"
          class="h-full w-full object-cover"
          style="object-position: center 41%"
        >
      </div>

      <p class="text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('common.contactSupportDialog.scanQrCode') }}
      </p>

      <div
        class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900/50"
      >
        <div class="min-w-0 flex-1">
          <span class="block text-xs text-gray-500 dark:text-dark-400">
            {{ t('common.contactSupportDialog.groupNumber') }}
          </span>
          <strong class="mt-1 block font-mono text-lg text-gray-900 dark:text-white">
            {{ QQ_GROUP_NUMBER }}
          </strong>
        </div>
        <button
          type="button"
          class="btn btn-primary min-w-[7.5rem]"
          data-testid="copy-qq-group"
          @click="copyGroupNumber"
        >
          <Icon :name="copied ? 'check' : 'copy'" size="sm" />
          <span>{{ copyButtonLabel }}</span>
        </button>
      </div>

      <p class="sr-only" aria-live="polite">{{ copyStatus }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const QQ_GROUP_NUMBER = '1080152144'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
const copyFailed = ref(false)

const copyButtonLabel = computed(() => {
  if (copied.value) return t('common.copied')
  if (copyFailed.value) return t('common.copyFailed')
  return t('common.contactSupportDialog.copyGroupNumber')
})

const copyStatus = computed(() => {
  if (copied.value) return t('common.copiedToClipboard')
  if (copyFailed.value) return t('common.copyFailed')
  return ''
})

async function copyGroupNumber() {
  copyFailed.value = false
  copyFailed.value = !(await copyToClipboard(QQ_GROUP_NUMBER))
}

watch(
  () => props.show,
  (show) => {
    if (!show) copyFailed.value = false
  },
)
</script>
