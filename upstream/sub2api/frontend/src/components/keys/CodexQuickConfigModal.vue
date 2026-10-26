<template>
  <BaseDialog
    :show="show"
    :title="t('keys.quickConfigureModal.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/30">
        <Icon name="exclamationCircle" size="md" class="mt-0.5 flex-shrink-0 text-amber-500" />
        <p class="text-sm text-amber-800 dark:text-amber-200">
          {{ t('keys.quickConfigureModal.warning') }}
        </p>
      </div>

      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('keys.quickConfigureModal.description') }}
      </p>

      <div class="grid grid-cols-3 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700" role="tablist">
        <button
          type="button"
          role="tab"
          :aria-selected="activePlatform === 'mac'"
          data-testid="quick-config-unix-tab"
          :class="tabClass(activePlatform === 'mac')"
          @click="activePlatform = 'mac'"
        >
          {{ t('keys.quickConfigureModal.mac') }}
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="activePlatform === 'windows'"
          data-testid="quick-config-windows-tab"
          :class="tabClass(activePlatform === 'windows')"
          @click="activePlatform = 'windows'"
        >
          {{ t('keys.quickConfigureModal.windows') }}
        </button>
        <button type="button" role="tab" :aria-selected="activePlatform === 'unix'" data-testid="quick-config-linux-tab" :class="tabClass(activePlatform === 'unix')" @click="activePlatform = 'unix'">
          Linux
        </button>
      </div>

      <div aria-live="polite" class="text-sm">
        <p v-if="catalogState === 'loading'" data-testid="quick-config-catalog-loading" class="text-primary-600 dark:text-primary-400">
          {{ t('keys.quickConfigureModal.importCatalogLoading') }}
        </p>
        <p v-else-if="catalogState === 'ready'" data-testid="quick-config-catalog-ready" class="text-emerald-600 dark:text-emerald-400">
          {{ t('keys.quickConfigureModal.importCatalogReady', { count: catalogModelCount }) }}
        </p>
        <div v-else-if="catalogState === 'error'" class="flex flex-wrap items-center gap-2">
          <p role="alert" data-testid="quick-config-catalog-error" class="text-red-600 dark:text-red-400">
            {{ t('keys.quickConfigureModal.importCatalogError') }}
          </p>
          <button type="button" class="btn btn-secondary" data-testid="quick-config-catalog-retry" @click="loadCatalog">
            {{ t('keys.quickConfigureModal.retryCatalog') }}
          </button>
        </div>
      </div>

      <div class="overflow-hidden rounded-lg bg-gray-900 dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-700 bg-gray-800 px-4 py-2 dark:bg-dark-800">
          <span class="truncate font-mono text-xs text-gray-400">{{ fileName }}</span>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="!scriptReady"
              class="flex items-center gap-1.5 rounded-lg bg-gray-700 px-2.5 py-1 text-xs font-medium text-gray-300 transition-colors hover:bg-gray-600 hover:text-white disabled:cursor-not-allowed disabled:opacity-50"
              data-testid="quick-config-copy"
              @click="copyScript"
            >
              <Icon :name="copied ? 'check' : 'clipboard'" size="sm" />
              {{ copied ? t('keys.quickConfigureModal.copied') : t('keys.quickConfigureModal.copy') }}
            </button>
          </div>
        </div>
        <pre class="max-h-96 overflow-auto p-4 text-xs leading-5 text-gray-100"><code>{{ script }}</code></pre>
      </div>

      <div
        v-if="actionStatus"
        role="status"
        aria-live="polite"
        data-testid="quick-config-action-status"
        class="flex items-start gap-2 rounded-lg border p-3 text-sm"
        :class="actionStatus === 'error'
          ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-950/30 dark:text-red-300'
          : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-300'"
      >
        <Icon :name="actionStatus === 'error' ? 'exclamationCircle' : 'checkCircle'" size="md" class="mt-0.5 flex-shrink-0" />
        <p>{{ t(`keys.quickConfigureModal.${actionStatus}Message`) }}</p>
      </div>

      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-300">
        <p class="font-medium text-gray-900 dark:text-white">{{ t('keys.quickConfigureModal.runTitle') }}</p>
        <p class="mt-1 text-sm">{{ runInstruction }}</p>
        <code v-if="activePlatform === 'unix'" class="mt-1 block break-all text-xs">{{ runCommand }}</code>
      </div>
    </div>

    <template #footer>
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.quickConfigureModal.downloadHint') }}</p>
        <div class="flex shrink-0 justify-end gap-3">
          <button
            type="button"
            class="btn btn-primary flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!scriptReady"
            data-testid="quick-config-download"
            @click="downloadScript"
          >
            <Icon name="download" size="sm" />
            {{ t('keys.quickConfigure') }}
          </button>
          <button type="button" class="btn btn-secondary" @click="emit('close')">
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { fetchCodexModelsManifest } from '@/api/codex'
import {
  buildMacLinuxCodexQuickConfigScript,
  buildMacCodexQuickConfigArchive,
  buildMacCodexQuickConfigScript,
  buildWindowsCmdCodexQuickConfigScript,
  type CodexQuickConfigPlatform
} from '@/utils/codexQuickConfig'

const props = defineProps<{
  show: boolean
  apiKey: string
  baseUrl: string
  platform?: CodexQuickConfigPlatform | null
}>()

const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()

type TargetPlatform = 'mac' | 'unix' | 'windows'
type CatalogState = 'idle' | 'loading' | 'ready' | 'error'

const detectPlatform = (): TargetPlatform => /windows/i.test(navigator.userAgent) ? 'windows' : /linux/i.test(navigator.userAgent) ? 'unix' : 'mac'
const activePlatform = ref<TargetPlatform>(detectPlatform())
const copied = ref(false)
type ActionStatus = 'copySuccess' | 'downloadSuccess' | 'error' | null
const actionStatus = ref<ActionStatus>(null)
const catalogState = ref<CatalogState>('idle')
const catalogContent = ref('')
const catalogModelCount = ref(0)
let catalogController: AbortController | null = null
let catalogRequestID = 0

const input = computed(() => ({
  apiKey: props.apiKey,
  baseUrl: props.baseUrl || window.location.origin,
  platform: props.platform,
  modelCatalogContent: catalogState.value === 'ready' ? catalogContent.value : undefined
}))

const script = computed(() => activePlatform.value === 'windows'
  ? buildWindowsCmdCodexQuickConfigScript(input.value)
  : activePlatform.value === 'mac' ? buildMacCodexQuickConfigScript(input.value) : buildMacLinuxCodexQuickConfigScript(input.value))

const fileName = computed(() => activePlatform.value === 'windows'
  ? 'sub2api-codex-config.cmd'
  : activePlatform.value === 'mac' ? 'xingqiao-codex-config.zip' : 'sub2api-codex-config.sh')

const runCommand = computed(() => `chmod +x ${fileName.value} && ./${fileName.value}`)
const runInstruction = computed(() => t(`keys.quickConfigureModal.${activePlatform.value === 'windows' ? 'windowsRun' : activePlatform.value === 'mac' ? 'macRun' : 'linuxRun'}`))

const scriptReady = computed(() => props.show && catalogState.value === 'ready')

const tabClass = (selected: boolean) => [
  'rounded-md px-3 py-2 text-sm font-medium transition-colors',
  selected
    ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
    : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
]

watch(() => [props.show, props.apiKey, props.baseUrl, props.platform] as const, () => {
  copied.value = false
  actionStatus.value = null
  resetCatalog()
  if (props.show) {
    activePlatform.value = detectPlatform()
    void loadCatalog()
  }
}, { immediate: true })

watch(activePlatform, () => {
  copied.value = false
  actionStatus.value = null
})

onBeforeUnmount(resetCatalog)

function resetCatalog() {
  catalogController?.abort()
  catalogController = null
  catalogRequestID += 1
  catalogState.value = 'idle'
  catalogContent.value = ''
  catalogModelCount.value = 0
}

async function loadCatalog() {
  if (!props.show) return
  if (!props.apiKey) {
    catalogState.value = 'error'
    return
  }
  catalogController?.abort()
  const controller = new AbortController()
  const requestID = ++catalogRequestID
  catalogController = controller
  catalogState.value = 'loading'
  try {
    const result = await fetchCodexModelsManifest(props.baseUrl, props.apiKey, controller.signal)
    if (requestID !== catalogRequestID) return
    catalogContent.value = result.content
    catalogModelCount.value = result.modelCount
    catalogState.value = 'ready'
  } catch (error) {
    const errorName = error && typeof error === 'object' && 'name' in error
      ? String((error as { name?: unknown }).name || '')
      : ''
    if (requestID !== catalogRequestID || errorName === 'AbortError') return
    catalogState.value = 'error'
  } finally {
    if (requestID === catalogRequestID) catalogController = null
  }
}

async function copyScript() {
  if (!scriptReady.value) return
  try {
    await navigator.clipboard.writeText(script.value)
    copied.value = true
    actionStatus.value = 'copySuccess'
    window.setTimeout(() => { copied.value = false }, 1800)
  } catch {
    copied.value = false
    actionStatus.value = 'error'
  }
}

function downloadScript() {
  if (!scriptReady.value) return
  try {
    const blob = activePlatform.value === 'mac'
      ? new Blob([buildMacCodexQuickConfigArchive(input.value).buffer as ArrayBuffer], { type: 'application/zip' })
      : new Blob([script.value], { type: 'text/plain;charset=utf-8' })
    saveAs(blob, fileName.value)
    actionStatus.value = 'downloadSuccess'
  } catch {
    actionStatus.value = 'error'
  }
}
</script>
