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

            <section aria-labelledby="scheduler-priorities-heading">
              <div class="scheduler-section-heading">
                <div>
                  <h2 id="scheduler-priorities-heading">{{ t('admin.schedulerSettings.priorities') }}</h2>
                  <p>{{ t('admin.schedulerSettings.prioritiesHint') }}</p>
                </div>
              </div>

              <div class="scheduler-priority-list">
                <div v-for="metric in metrics" :key="metric.key" class="scheduler-priority-row">
                  <span class="scheduler-priority-label">{{ metric.label }}</span>
                  <div class="scheduler-number-group" role="group" :aria-label="metric.label">
                    <button
                      v-for="value in [1, 2, 3]"
                      :key="value"
                      type="button"
                      class="scheduler-option scheduler-number"
                      :class="{ 'scheduler-option-selected': draft.priority[metric.key] === value }"
                      :aria-pressed="draft.priority[metric.key] === value"
                      :data-testid="`scheduler-priority-${metric.key}-${value}`"
                      @click="draft.priority[metric.key] = value"
                    >
                      {{ value }}
                    </button>
                  </div>
                </div>
              </div>

              <p class="scheduler-summary" data-testid="scheduler-priority-summary">{{ prioritySummary }}</p>
            </section>

            <section class="scheduler-adjustments" :aria-label="t('admin.schedulerSettings.adjustments')">
              <div v-for="control in controls" :key="control.key" class="scheduler-adjustment">
                <h2>{{ control.label }}</h2>
                <div class="scheduler-segments" role="group" :aria-label="control.label">
                  <button
                    v-for="option in control.options"
                    :key="option.value"
                    type="button"
                    class="scheduler-option scheduler-segment"
                    :class="{ 'scheduler-option-selected': draft.operations[control.key] === option.value }"
                    :aria-pressed="draft.operations[control.key] === option.value"
                    :data-testid="`scheduler-operation-${control.key}-${option.value}`"
                    @click="setOperation(control.key, option.value)"
                  >
                    {{ option.label }}
                  </button>
                </div>
                <p>{{ control.hint }}</p>
              </div>
            </section>

            <section class="scheduler-preview" aria-labelledby="scheduler-preview-heading">
              <div class="scheduler-section-heading">
                <div>
                  <h2 id="scheduler-preview-heading">{{ t('admin.schedulerSettings.preview') }}</h2>
                  <p>{{ selectedGroup.name }} · {{ t('admin.schedulerSettings.draft') }}</p>
                </div>
              </div>
              <div class="scheduler-preview-grid">
                <article class="scheduler-preview-item">
                  <h3>{{ t('admin.schedulerSettings.normal') }}</h3>
                  <p data-testid="scheduler-preview-normal">{{ previews.normal }}</p>
                </article>
                <article class="scheduler-preview-item">
                  <h3>{{ t('admin.schedulerSettings.peakPreview') }}</h3>
                  <p data-testid="scheduler-preview-peak">{{ previews.peak }}</p>
                </article>
                <article class="scheduler-preview-item">
                  <h3>{{ t('admin.schedulerSettings.sessionPreview') }}</h3>
                  <p data-testid="scheduler-preview-session">{{ previews.session }}</p>
                </article>
              </div>
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
import type {
  OpenAISchedulerBusinessPriority,
  OpenAISchedulerGroupPolicy,
  OpenAISchedulerOperations,
} from '@/api/admin/settings'
import Toggle from '@/components/common/Toggle.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import type { AdminGroup } from '@/types'
import {
  createSchedulerScenarioPreviews,
  DEFAULT_SCHEDULER_OPERATIONS,
  hasValidBusinessPriority,
  recommendedBusinessPriority,
  schedulerPrioritySummary,
} from './scheduler/schedulerPolicy'

type SchedulerDraft = {
  priority: OpenAISchedulerBusinessPriority
  operations: OpenAISchedulerOperations
}

type MetricKey = keyof OpenAISchedulerBusinessPriority
type ControlKey = keyof OpenAISchedulerOperations

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
  priority: { profit: 1, ttft: 1, latency: 1 },
  operations: { ...DEFAULT_SCHEDULER_OPERATIONS },
})

const selectedGroup = computed(() => groups.value.find((group) => String(group.id) === selectedGroupId.value))

const metrics = computed((): Array<{ key: MetricKey; label: string }> => [
  { key: 'profit', label: t('admin.schedulerSettings.profit') },
  { key: 'ttft', label: t('admin.schedulerSettings.ttft') },
  { key: 'latency', label: t('admin.schedulerSettings.latency') },
])

const controls = computed((): Array<{
  key: ControlKey
  label: string
  hint: string
  options: Array<{ value: string; label: string }>
}> => [
  {
    key: 'balance',
    label: t('admin.schedulerSettings.balanceTitle'),
    hint: t('admin.schedulerSettings.balanceHint'),
    options: ['low', 'standard', 'high'].map((value) => ({ value, label: t(`admin.schedulerSettings.balance.${value}`) })),
  },
  {
    key: 'peak_protection',
    label: t('admin.schedulerSettings.peakTitle'),
    hint: t('admin.schedulerSettings.peakHint'),
    options: ['strict', 'standard', 'open'].map((value) => ({ value, label: t(`admin.schedulerSettings.peak.${value}`) })),
  },
  {
    key: 'session_continuity',
    label: t('admin.schedulerSettings.sessionTitle'),
    hint: t('admin.schedulerSettings.sessionHint'),
    options: ['keep', 'standard', 'switch'].map((value) => ({ value, label: t(`admin.schedulerSettings.session.${value}`) })),
  },
])

const prioritySummary = computed(() => schedulerPrioritySummary(draft.priority, {
  profit: t('admin.schedulerSettings.profit'),
  ttft: t('admin.schedulerSettings.ttft'),
  latency: t('admin.schedulerSettings.latency'),
}))

const previews = computed(() => createSchedulerScenarioPreviews(draft))

function cloneDraft(source: SchedulerDraft): SchedulerDraft {
  return {
    priority: { ...source.priority },
    operations: { ...source.operations },
  }
}

function draftForGroup(group: AdminGroup): SchedulerDraft {
  const policy = policies.value[String(group.id)]
  return {
    priority: hasValidBusinessPriority(policy?.priority)
      ? { ...policy.priority }
      : recommendedBusinessPriority(group.name),
    operations: {
      ...DEFAULT_SCHEDULER_OPERATIONS,
      ...(policy?.operations ?? {}),
    },
  }
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

function setOperation(key: ControlKey, value: string): void {
  draft.operations[key] = value as never
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
        priority: { ...currentDraft.priority },
        operations: { ...currentDraft.operations },
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
.scheduler-description, .scheduler-section-heading p, .scheduler-adjustment > p, .scheduler-preview-item p { margin: .35rem 0 0; color: rgb(71 85 105); font-size: .875rem; line-height: 1.5; }
.dark .scheduler-description, .dark .scheduler-section-heading p, .dark .scheduler-adjustment > p, .dark .scheduler-preview-item p { color: rgb(165 180 202); }
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
.scheduler-priority-list { display: grid; gap: .5rem; margin-top: .85rem; }
.scheduler-priority-row { display: grid; grid-template-columns: minmax(7rem, .8fr) 1fr; align-items: center; gap: .75rem; padding: .65rem .75rem; border: 1px solid rgb(210 219 232); border-radius: 9px; background: rgb(255 255 255); }
.dark .scheduler-priority-row { border-color: rgb(51 68 95); background: rgb(16 29 51); }
.scheduler-priority-label { font-size: .875rem; font-weight: 600; }
.scheduler-number-group { display: flex; gap: .4rem; }
.scheduler-number { min-width: 2.2rem; padding: .25rem .65rem; font-size: .875rem; font-weight: 700; }
.scheduler-summary { margin: .85rem 0 0; padding: .7rem .85rem; border-radius: 8px; color: rgb(49 72 111); background: rgb(242 246 255); font-size: .875rem; line-height: 1.5; }
.dark .scheduler-summary { color: rgb(219 232 255); background: rgb(20 41 73); }
.scheduler-adjustments, .scheduler-preview-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .65rem; }
.scheduler-adjustment, .scheduler-preview-item { padding: .85rem; border: 1px solid rgb(210 219 232); border-radius: 9px; background: rgb(255 255 255); }
.dark .scheduler-adjustment, .dark .scheduler-preview-item { border-color: rgb(51 68 95); background: rgb(16 29 51); }
.scheduler-segments { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .25rem; margin-top: .7rem; }
.scheduler-segment { min-height: 2.65rem; padding: .35rem; font-size: .75rem; line-height: 1.2; }
.scheduler-preview { padding-top: 1.25rem; border-top: 1px solid rgb(214 222 234); }
.dark .scheduler-preview { border-color: rgb(45 62 90); }
.scheduler-preview-grid { margin-top: .85rem; }
.scheduler-preview-item h3 { font-size: .8125rem; }
.scheduler-workbench-footer { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 1rem 1.5rem; border-top: 1px solid rgb(226 232 240); background: rgb(255 255 255); }
.dark .scheduler-workbench-footer { border-color: rgb(42 59 87); background: rgb(15 29 52); }
@media (max-width: 700px) { .scheduler-workbench-header, .scheduler-workbench-footer { align-items: flex-start; flex-direction: column; } .scheduler-adjustments, .scheduler-preview-grid { grid-template-columns: 1fr; } .scheduler-priority-row { grid-template-columns: 1fr; } .scheduler-workbench-footer .btn { width: 100%; } }
@media (prefers-reduced-motion: reduce) { .scheduler-group, .scheduler-option { transition: none; } }
</style>
