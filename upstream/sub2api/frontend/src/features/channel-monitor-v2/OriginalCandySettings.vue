<template>
  <details class="original-candy-settings" data-testid="original-candy-settings">
    <summary @click="ensureLoaded">{{ t('channelMonitorV2.originalCandy.title') }}</summary>
    <div class="original-candy-content">
      <p>{{ t('channelMonitorV2.originalCandy.hint') }}</p>
      <p v-if="loading" role="status">{{ t('channelMonitorV2.settings.loading') }}</p>
      <p v-if="error" role="alert">{{ error }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <template v-if="draft">
        <MonitorCandySettings v-model="draft.candy_probes" v-model:history-limit="draft.candy_history_limit" :groups="groups" />
        <div class="original-candy-actions">
          <button type="button" class="btn btn-secondary" :disabled="loading || saving" data-testid="original-candy-reload" @click="load"><Icon name="refresh" size="sm" />{{ t('channelMonitorV2.originalCandy.reload') }}</button>
          <button type="button" class="btn btn-primary" :disabled="saving || loading || conflicted || !dirty" data-testid="original-candy-save" @click="save">{{ saving ? t('common.saving') : t('channelMonitorV2.settings.save') }}</button>
        </div>
      </template>
      <button v-else-if="!loading" type="button" class="btn btn-secondary" @click="load">{{ t('channelMonitorV2.tool.retry') }}</button>
    </div>
  </details>
</template>
<script setup lang="ts">
import { computed, ref, toRaw } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import MonitorCandySettings from './MonitorCandySettings.vue'
import { getConfig, updateConfig, type MonitorConfig } from '@/api/channelMonitorV2'
import { adminAPI } from '@/api/admin'
const { t } = useI18n()
const draft = ref<MonitorConfig>(), original = ref(''), groups = ref<Array<{id:number;name:string;platform:string}>>([])
const loading = ref(false), saving = ref(false), conflicted = ref(false), error = ref(''), notice = ref('')
const dirty = computed(() => !!draft.value && JSON.stringify(draft.value) !== original.value)
function clone<T>(value: T): T { return structuredClone(toRaw(value)) }
function ensureLoaded() { if (!draft.value && !loading.value) void load() }
async function load() {
  loading.value = true; error.value = ''; notice.value = ''
  try {
    const [config, available] = await Promise.all([getConfig(), adminAPI.groups.getAllIncludingInactive()])
    draft.value = clone({ ...config, candy_probes: config.candy_probes || [], candy_history_limit: config.candy_history_limit || 100 })
    groups.value = available; original.value = JSON.stringify(draft.value); conflicted.value = false
  } catch { error.value = t('channelMonitorV2.settings.loadFailed') }
  finally { loading.value = false }
}
async function save() {
  if (!draft.value || saving.value || conflicted.value) return
  const probes = draft.value.candy_probes || [], identities = new Set<string>()
  for (const probe of probes) {
    const identity = `${probe.group_id}\0${probe.model.trim()}\0${probe.reasoning_effort.trim().toLowerCase()}`
    if (!probe.model.trim() || !Number.isInteger(probe.interval_minutes) || probe.interval_minutes < 1 || probe.interval_minutes > 1440) { error.value = t('channelMonitorV2.candy.invalid'); return }
    if (identities.has(identity)) { error.value = t('channelMonitorV2.candy.duplicateModel'); return }
    identities.add(identity)
  }
  const limit = draft.value.candy_history_limit
  if (limit == null || !Number.isInteger(limit) || limit < 15 || limit > 100) { error.value = t('channelMonitorV2.candy.historyLimitInvalid'); return }
  saving.value = true; error.value = ''; notice.value = ''
  try {
    // Full read-version payload preserves unrelated monitor policy. Server-side
    // optimistic concurrency rejects changes made since this editor was loaded.
    const updated = await updateConfig(clone(draft.value))
    draft.value = clone(updated); original.value = JSON.stringify(draft.value); notice.value = t('channelMonitorV2.settings.saveSuccess')
  } catch (failure) {
    const conflict = failure as {status?:number;response?:{status?:number}}
    conflicted.value = (conflict?.status ?? conflict?.response?.status) === 409
    error.value = t(conflicted.value ? 'channelMonitorV2.originalCandy.conflict' : 'channelMonitorV2.settings.saveFailed')
  } finally { saving.value = false }
}
</script>
<style scoped>
.original-candy-settings{border-top:1px solid var(--xq-border);padding-top:18px;color:var(--xq-text)}summary{cursor:pointer;font-size:14px;font-weight:600;padding:8px 0}summary:focus-visible{outline:2px solid var(--xq-accent);outline-offset:4px}.original-candy-content>p{font-size:12px;line-height:1.7;color:var(--xq-secondary);margin:12px 0}.original-candy-content [role="alert"]{color:var(--xq-danger)}.original-candy-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:14px}
</style>
