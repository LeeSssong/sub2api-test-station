<template>
  <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-600" aria-label="AI 工具关联">
    <h3 class="text-sm font-semibold text-gray-900 dark:text-white">AI 工具关联</h3>
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">来源：管理员配置。未配置时不会出现在 AI 工具卡片中，已有密钥不变。</p>
    <fieldset :disabled="loading || saving || !loaded" class="mt-3 flex flex-wrap gap-4">
      <legend class="sr-only">选择此线路可用于哪些 AI 工具</legend>
      <label v-for="tool in tools" :key="tool.id" class="flex items-center gap-2 text-sm">
        <input v-model="selected" type="checkbox" :value="tool.id" class="checkbox" />
        {{ tool.label }}
      </label>
    </fieldset>
    <p v-if="loading" class="mt-3 text-xs text-gray-500" role="status">正在读取工具关联…</p>
    <div v-if="error" class="mt-3 flex items-center gap-3" role="alert">
      <span class="text-sm text-red-600 dark:text-red-400">{{ error }}</span>
      <button v-if="!loaded" type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">重试</button>
    </div>
    <p v-if="saved" class="mt-3 text-sm text-emerald-600 dark:text-emerald-400" role="status">工具关联已保存</p>
    <button type="button" class="btn btn-secondary mt-3" :disabled="loading || saving || !loaded" @click="save">
      {{ saving ? '正在保存…' : '保存工具关联' }}
    </button>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { apiClient } from '@/api/client'

type ToolId = 'codex' | 'claude' | 'grok' | 'deepseek'
interface ToolMappings { group_id: number; tool_ids: ToolId[]; version: number; effective_at: string; updated_by: number }
const props = defineProps<{ groupId: number }>()
const tools: { id: ToolId; label: string }[] = [
  { id: 'codex', label: 'Codex' }, { id: 'claude', label: 'Claude Code' },
  { id: 'grok', label: 'Grok' }, { id: 'deepseek', label: 'DeepSeek' },
]
const selected = ref<ToolId[]>([])
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const error = ref('')
const saved = ref(false)
let requestGeneration = 0

async function load() {
  const generation = ++requestGeneration
  const groupId = props.groupId
  loading.value = true
  loaded.value = false
  error.value = ''
  saved.value = false
  try {
    const { data } = await apiClient.get<ToolMappings>(`/admin/groups/${groupId}/tool-mappings`)
    if (generation !== requestGeneration) return
    selected.value = [...data.tool_ids]
    loaded.value = true
  } catch {
    if (generation === requestGeneration) error.value = '工具关联读取失败，请检查权限或网络后重试。'
  } finally {
    if (generation === requestGeneration) loading.value = false
  }
}
async function save() {
  if (!loaded.value || loading.value || saving.value) return
  const generation = requestGeneration
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    const { data } = await apiClient.put<ToolMappings>(`/admin/groups/${props.groupId}/tool-mappings`, { tool_ids: [...selected.value] })
    if (generation !== requestGeneration) return
    selected.value = [...data.tool_ids]
    saved.value = true
  } catch {
    if (generation === requestGeneration) error.value = '工具关联保存失败，请检查权限或网络后重试；当前选择已保留。'
  } finally {
    saving.value = false
  }
}
watch(() => props.groupId, load, { immediate: true })
watch(selected, () => { saved.value = false }, { deep: true, flush: 'sync' })
</script>
