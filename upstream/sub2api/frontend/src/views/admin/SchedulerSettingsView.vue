<template>
  <AppLayout>
    <main class="mx-auto max-w-5xl">
      <div v-if="loading" class="flex min-h-64 items-center justify-center" data-testid="scheduler-loading">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-200 border-t-primary-600" />
      </div>

      <section v-else class="scheduler-workbench" aria-labelledby="scheduler-settings-title">
        <header class="scheduler-workbench-header">
          <div>
            <p class="scheduler-kicker">OpenAI</p>
            <h1 id="scheduler-settings-title" class="scheduler-title">{{ t('admin.schedulerSettings.title') }}</h1>
            <p class="scheduler-description">{{ t('admin.schedulerSettings.description') }}</p>
          </div>
          <div class="scheduler-switch-row">
            <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('admin.schedulerSettings.enabled') }}</span>
            <Toggle v-model="schedulerEnabled" />
          </div>
        </header>

        <div v-if="schedulerEnabled" class="scheduler-workbench-body">
          <section aria-labelledby="scheduler-group-heading">
            <div class="scheduler-section-heading">
              <div>
                <h2 id="scheduler-group-heading">{{ t('admin.schedulerSettings.group') }}</h2>
                <p>{{ t('admin.schedulerSettings.groupHint') }}</p>
              </div>
              <span class="scheduler-draft-state">{{ t('admin.schedulerSettings.draft') }}</span>
            </div>

            <div v-if="groups.length" class="scheduler-group-list" role="group" :aria-label="t('admin.schedulerSettings.group')">
              <button
                v-for="group in groups"
                :key="group.id"
                type="button"
                class="scheduler-group"
                :class="{ 'scheduler-option-selected': selectedGroupId === String(group.id) }"
                :aria-pressed="selectedGroupId === String(group.id)"
                :data-testid="`scheduler-group-${group.id}`"
                @click="selectGroup(String(group.id))"
              >
                {{ group.name }}
              </button>
            </div>
            <p v-else class="scheduler-empty">{{ t('admin.schedulerSettings.noGroups') }}</p>
          </section>

          <template v-if="selectedGroup && draft">
            <section class="scheduler-guard" data-testid="scheduler-service-guard" aria-labelledby="scheduler-guard-heading">
              <div class="scheduler-guard-mark" aria-hidden="true">✓</div>
              <div>
                <h2 id="scheduler-guard-heading">{{ t('admin.schedulerSettings.guardTitle') }}</h2>
                <p>{{ t('admin.schedulerSettings.guardDescription') }}</p>
              </div>
              <span class="scheduler-locked">{{ t('admin.schedulerSettings.fixed') }}</span>
            </section>

            <section class="scheduler-recovery" aria-labelledby="scheduler-recovery-heading">
              <div class="scheduler-section-heading">
                <div>
                  <h2 id="scheduler-recovery-heading">{{ t('admin.schedulerSettings.recoveryTitle') }}</h2>
                  <p>{{ t('admin.schedulerSettings.recoveryHint') }}</p>
                </div>
              </div>
              <label class="scheduler-recovery-control" for="scheduler-extra-retry-count">
                <span>{{ t('admin.schedulerSettings.extraRetryCount') }}</span>
                <select
                  id="scheduler-extra-retry-count"
                  v-model.number="draft.extraRetryCount"
                  class="scheduler-select"
                  data-testid="scheduler-extra-retry-count"
                >
                  <option v-for="value in [0, 1, 2, 3]" :key="value" :value="value">{{ value }}</option>
                </select>
              </label>
              <p class="scheduler-recovery-note">{{ t('admin.schedulerSettings.extraRetryCountHint') }}</p>
            </section>
          </template>
        </div>

        <footer class="scheduler-workbench-footer">
          <p v-if="saveError" class="text-sm text-red-700 dark:text-red-300" role="alert">{{ saveError }}</p>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving || (schedulerEnabled && !selectedGroupId)"
            data-testid="scheduler-save"
            @click="save"
          >
            {{ saving ? t('common.saving') : t('admin.schedulerSettings.save') }}
          </button>
        </footer>
      </section>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { adminAPI } from '@/api/admin'
import type { OpenAISchedulerGroupPolicy } from '@/api/admin/settings'
import Toggle from '@/components/common/Toggle.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import type { AdminGroup } from '@/types'

type SchedulerDraft = {
  extraRetryCount: number
}

const { t } = useI18n()
const appStore = useAppStore()
const adminSettingsStore = useAdminSettingsStore()

const loading = ref(true)
const saving = ref(false)
const saveError = ref('')
const schedulerEnabled = ref(false)
const groups = ref<AdminGroup[]>([])
const selectedGroupId = ref('')
const policies = ref<Record<string, OpenAISchedulerGroupPolicy>>({})
const drafts = reactive<Record<string, SchedulerDraft>>({})
const draft = reactive<SchedulerDraft>({
  extraRetryCount: 0,
})

const selectedGroup = computed(() => groups.value.find((group) => String(group.id) === selectedGroupId.value))

function cloneDraft(source: SchedulerDraft): SchedulerDraft {
  return { extraRetryCount: source.extraRetryCount }
}

function normalizeExtraRetryCount(value: unknown): number {
  return typeof value === 'number' && Number.isInteger(value) ? Math.min(3, Math.max(0, value)) : 0
}

function draftForGroup(group: AdminGroup): SchedulerDraft {
  const policy = policies.value[String(group.id)]
  return { extraRetryCount: normalizeExtraRetryCount(policy?.extra_retry_count) }
}

function storeDraft(): void {
  if (selectedGroupId.value) drafts[selectedGroupId.value] = cloneDraft(draft)
}

function selectGroup(groupId: string): void {
  storeDraft()
  const group = groups.value.find((item) => String(item.id) === groupId)
  if (!group) return
  selectedGroupId.value = groupId
  Object.assign(draft, cloneDraft(drafts[groupId] ?? draftForGroup(group)))
}

async function save(): Promise<void> {
  storeDraft()
  saveError.value = ''
  saving.value = true
  try {
    const nextPolicies = { ...policies.value }
    for (const [groupId, currentDraft] of Object.entries(drafts)) {
      nextPolicies[groupId] = {
        ...nextPolicies[groupId],
        extra_retry_count: currentDraft.extraRetryCount,
      }
    }
    const updated = await adminAPI.settings.updateSettings({
      openai_advanced_scheduler_enabled: schedulerEnabled.value,
      openai_advanced_scheduler_group_policies: nextPolicies,
    })
    schedulerEnabled.value = Boolean(updated.openai_advanced_scheduler_enabled)
    policies.value = { ...(updated.openai_advanced_scheduler_group_policies ?? nextPolicies) }
    await Promise.all([
      appStore.fetchPublicSettings(true),
      adminSettingsStore.fetch(true),
    ])
    appStore.showSuccess(t('admin.schedulerSettings.saved'))
  } catch (_error) {
    saveError.value = t('admin.schedulerSettings.saveFailed')
    appStore.showError(saveError.value)
  } finally {
    saving.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  try {
    const [settings, activeGroups] = await Promise.all([
      adminAPI.settings.getSettings(),
      adminAPI.groups.getAll('openai'),
    ])
    schedulerEnabled.value = Boolean(settings.openai_advanced_scheduler_enabled)
    policies.value = { ...(settings.openai_advanced_scheduler_group_policies ?? {}) }
    groups.value = activeGroups.filter((group) => group.status === 'active')
    if (groups.value[0]) selectGroup(String(groups.value[0].id))
  } catch (_error) {
    saveError.value = t('admin.schedulerSettings.loadFailed')
    appStore.showError(saveError.value)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.scheduler-workbench {
  overflow: hidden;
  border: 1px solid rgb(203 213 225);
  border-radius: 12px;
  background: rgb(248 250 252);
  color: rgb(15 23 42);
}

.dark .scheduler-workbench { border-color: rgb(51 65 85); background: rgb(11 21 37); color: rgb(226 232 240); }
.scheduler-workbench-header { display: flex; align-items: center; justify-content: space-between; gap: 1.5rem; padding: 1.5rem; border-bottom: 1px solid rgb(226 232 240); background: rgb(255 255 255); }
.dark .scheduler-workbench-header { border-color: rgb(42 59 87); background: rgb(15 29 52); }
.scheduler-kicker { margin: 0 0 .35rem; color: rgb(71 85 105); font-size: .75rem; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; }
.dark .scheduler-kicker { color: rgb(148 163 184); }
.scheduler-title { margin: 0; font-size: 1.25rem; font-weight: 650; letter-spacing: -.02em; text-wrap: balance; }
.scheduler-description, .scheduler-section-heading p, .scheduler-recovery-note { margin: .35rem 0 0; color: rgb(71 85 105); font-size: .875rem; line-height: 1.5; }
.dark .scheduler-description, .dark .scheduler-section-heading p, .dark .scheduler-recovery-note { color: rgb(165 180 202); }
.scheduler-switch-row { display: flex; align-items: center; gap: .75rem; flex-shrink: 0; }
.scheduler-workbench-body { display: grid; gap: 1.5rem; padding: 1.5rem; }
.scheduler-section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.scheduler-section-heading h2, .scheduler-adjustment h2, .scheduler-preview-item h3, .scheduler-guard h2 { margin: 0; font-size: .9375rem; font-weight: 650; }
.scheduler-draft-state, .scheduler-locked { padding: .3rem .55rem; border-radius: 999px; color: rgb(51 65 85); background: rgb(226 232 240); font-size: .75rem; font-weight: 600; white-space: nowrap; }
.dark .scheduler-draft-state { color: rgb(203 213 225); background: rgb(30 41 59); }
.scheduler-group-list { display: flex; flex-wrap: wrap; gap: .5rem; margin-top: .85rem; }
.scheduler-group, .scheduler-option { border: 1px solid rgb(203 213 225); border-radius: 8px; color: rgb(71 85 105); background: rgb(255 255 255); cursor: pointer; transition: background-color 180ms ease-out, border-color 180ms ease-out, color 180ms ease-out; }
.dark .scheduler-group, .dark .scheduler-option { border-color: rgb(59 76 105); color: rgb(184 198 220); background: rgb(16 30 53); }
.scheduler-group { min-height: 2.5rem; padding: .5rem .75rem; font-size: .875rem; }
.scheduler-option { min-height: 2.5rem; }
.scheduler-group:hover, .scheduler-option:hover { border-color: rgb(96 131 204); color: rgb(30 64 175); }
.dark .scheduler-group:hover, .dark .scheduler-option:hover { border-color: rgb(145 175 255); color: rgb(228 239 255); }
.scheduler-option-selected { border-color: rgb(75 115 198) !important; color: rgb(30 64 175) !important; background: rgb(234 241 255) !important; box-shadow: 0 0 0 2px rgb(219 231 255); }
.dark .scheduler-option-selected { border-color: rgb(145 175 255) !important; color: rgb(228 239 255) !important; background: rgb(41 74 131) !important; box-shadow: 0 0 0 2px rgb(39 63 112); }
.scheduler-group:focus-visible, .scheduler-option:focus-visible { outline: 2px solid rgb(59 130 246); outline-offset: 2px; }
.scheduler-empty { margin: .85rem 0 0; color: rgb(100 116 139); font-size: .875rem; }
.scheduler-guard { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: .75rem; padding: 1rem; border: 1px solid rgb(162 199 176); border-radius: 10px; background: rgb(239 249 242); }
.dark .scheduler-guard { border-color: rgb(49 110 85); background: rgb(16 43 37); }
.scheduler-guard-mark { display: grid; width: 1.65rem; height: 1.65rem; place-items: center; border-radius: 999px; color: white; background: rgb(22 128 84); font-size: .875rem; font-weight: 800; }
.scheduler-guard p { margin: .25rem 0 0; color: rgb(54 102 77); font-size: .8125rem; line-height: 1.45; }
.dark .scheduler-guard p { color: rgb(183 222 197); }
.scheduler-locked { color: rgb(35 100 67); background: rgb(255 255 255); }
.dark .scheduler-locked { color: rgb(166 233 191); background: rgb(18 49 42); }
.scheduler-recovery { display: grid; gap: .9rem; padding: 1rem; border: 1px solid rgb(210 219 232); border-radius: 9px; background: rgb(255 255 255); }
.dark .scheduler-recovery { border-color: rgb(51 68 95); background: rgb(16 29 51); }
.scheduler-recovery-control { display: flex; align-items: center; justify-content: space-between; gap: 1rem; font-size: .9375rem; font-weight: 650; }
.scheduler-select { min-width: 8rem; min-height: 2.5rem; padding: .35rem .6rem; border: 1px solid rgb(148 163 184); border-radius: 7px; color: rgb(15 23 42); background: white; font-size: .9375rem; }
.dark .scheduler-select { border-color: rgb(71 85 105); color: rgb(226 232 240); background: rgb(15 29 52); }
.scheduler-select:focus-visible { outline: 2px solid rgb(59 130 246); outline-offset: 2px; }
.scheduler-workbench-footer { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 1rem 1.5rem; border-top: 1px solid rgb(226 232 240); background: rgb(255 255 255); }
.dark .scheduler-workbench-footer { border-color: rgb(42 59 87); background: rgb(15 29 52); }
@media (max-width: 700px) { .scheduler-workbench-header, .scheduler-workbench-footer { align-items: flex-start; flex-direction: column; } .scheduler-recovery-control { align-items: flex-start; flex-direction: column; } .scheduler-select, .scheduler-workbench-footer .btn { width: 100%; } }
@media (prefers-reduced-motion: reduce) { .scheduler-group, .scheduler-option { transition: none; } }
</style>
