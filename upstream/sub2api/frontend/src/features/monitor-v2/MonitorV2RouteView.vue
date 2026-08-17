<template>
  <ChannelStatusView v-if="officialMode" />

  <div v-else-if="fallback">
    <div
      class="fixed right-4 top-4 z-50 max-w-sm rounded-lg bg-gray-950 px-4 py-3 text-sm text-white shadow-md dark:bg-white dark:text-gray-950"
      role="status"
    >
      {{ t('monitorV2.fallbackNotice') }}
    </div>
    <ChannelStatusView />
  </div>

  <MonitorV2View
    v-else-if="snapshot"
    :initial-snapshot="snapshot"
    @fatal="activateFallback"
  />

  <AppLayout v-else>
    <section
      class="mx-auto max-w-7xl"
      aria-busy="true"
      aria-live="polite"
    >
      <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('monitorV2.loading') }}
      </p>
      <div class="mt-5 grid grid-cols-1 gap-4 xl:grid-cols-2">
        <div
          v-for="index in 4"
          :key="index"
          class="h-72 animate-pulse rounded-lg bg-gray-200/70 motion-reduce:animate-none dark:bg-dark-800"
          aria-hidden="true"
        />
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ChannelStatusView from '@/views/user/ChannelStatusView.vue'
import { isChannelMonitorV2Mode } from '@/utils/featureFlags'
import { getMonitorV2Snapshot } from './api'
import MonitorV2View from './MonitorV2View.vue'
import type { MonitorV2Snapshot } from './types'

const { t } = useI18n()
const officialMode = computed(() => isChannelMonitorV2Mode())
const snapshot = ref<MonitorV2Snapshot | null>(null)
const fallback = ref(false)
let abortController: AbortController | null = null

function activateFallback() {
  fallback.value = true
  snapshot.value = null
}

onMounted(async () => {
  if (officialMode.value) return

  const controller = new AbortController()
  abortController = controller
  try {
    const initial = await getMonitorV2Snapshot('7d', controller.signal)
    if (controller.signal.aborted || abortController !== controller) return
    snapshot.value = initial
  } catch (error: unknown) {
    const candidate = error as { name?: string; code?: string }
    if (candidate?.name === 'AbortError' || candidate?.code === 'ERR_CANCELED') return
    activateFallback()
  } finally {
    if (abortController === controller) abortController = null
  }
})

onBeforeUnmount(() => {
  abortController?.abort()
})
</script>
