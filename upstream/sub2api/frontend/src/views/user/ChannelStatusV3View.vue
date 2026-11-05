<template>
  <component :is="embedded ? 'div' : AppLayout" ref="surface" :class="{'embedded-v3':embedded}">
    <div v-if="!embedded || !showIncidents" class="w-full min-w-0" :class="{ 'pb-10': !embedded }">
      <p v-if="error" class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-200" role="alert">
        {{ error }}
      </p>
      <StatusPage
        :status="status"
        :loading="loading"
        @refresh="load(end)"
        @navigate="navigate"
        @open-incidents="openIncidents"
      />
    </div>
    <IncidentsDialog :inline="embedded" :tool-id="toolId" :show="showIncidents" :admin="false" @close="closeIncidents" @load-error="emit('load-error')" />
  </component>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import StatusPage from '@/features/channel-monitor-v3/StatusPage.vue'
import IncidentsDialog from '@/features/channel-monitor-v3/IncidentsDialog.vue'
import { getStatus, type MonitorV3StatusPage } from '@/api/channelMonitorV3'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{embedded?:boolean;toolId?:string}>()
const emit = defineEmits<{'load-error':[]}>()
const REFRESH_MS = 60_000
const { t } = useI18n()
const surface=ref<HTMLElement>()
function openIncidents(){showIncidents.value=true;if(props.embedded)void nextTick(()=>surface.value?.querySelector<HTMLButtonElement>('[data-native-back]')?.focus({preventScroll:true}))}
function closeIncidents(){showIncidents.value=false;if(props.embedded)void nextTick(()=>surface.value?.querySelector<HTMLButtonElement>('[data-testid="monitor-v3-incidents"]')?.focus({preventScroll:true}))}
const status = ref<MonitorV3StatusPage | null>(null)
const loading = ref(false)
const error = ref('')
const end = ref<number | null>(null)
const showIncidents = ref(false)
let controller: AbortController | undefined
let timer: ReturnType<typeof setInterval> | undefined

async function load(nextEnd: number | null) {
  controller?.abort()
  const current = new AbortController()
  controller = current
  loading.value = true
  try {
    const result = await getStatus(nextEnd, false, current.signal, props.toolId)
    if (current.signal.aborted) return
    status.value = result
    end.value = result.window.latest ? null : nextEnd
    error.value = ''
  } catch (err: unknown) {
    if (current.signal.aborted) return
    status.value=null
    error.value = extractApiErrorMessage(err, t('channelMonitorV3.page.loadFailed'))
    emit('load-error')
  } finally {
    if (controller === current) loading.value = false
  }
}

function navigate(nextEnd: number | null) {
  void load(nextEnd)
}

// Only the live window refreshes itself; an older window is history.
function tick() {
  if (document.visibilityState !== 'visible' || loading.value || end.value !== null) return
  void load(null)
}

watch(()=>props.toolId,()=>{controller?.abort();status.value=null;end.value=null;showIncidents.value=false;void load(null)})
onMounted(() => {
  void load(null)
  timer = setInterval(tick, REFRESH_MS)
})
onBeforeUnmount(() => {
  controller?.abort()
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.embedded-v3 :deep([data-testid="monitor-v3-refresh"]){background:var(--xq-surface);border:1px solid var(--xq-border);color:var(--xq-secondary);box-shadow:none;font-size:12px;font-weight:400;border-radius:8px}
.embedded-v3 :deep([data-testid="monitor-v3-refresh"]:hover){background:var(--xq-raised);color:var(--xq-text)}
.embedded-v3 :deep([data-testid="monitor-v3-refresh"]:focus-visible){outline:2px solid var(--xq-accent);outline-offset:2px}
</style>
