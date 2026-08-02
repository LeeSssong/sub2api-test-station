<template>
  <div class="flex w-full flex-wrap items-center gap-2">
    <div class="relative min-w-[220px] flex-1">
      <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
      <input
        :value="search"
        type="search"
        class="input w-full pl-9"
        :placeholder="t('admin.accountMonitor.filters.searchPlaceholder')"
        @input="emit('update:search', ($event.target as HTMLInputElement).value)"
      />
    </div>
    <select
      :value="platform"
      class="input w-auto min-w-[140px]"
      :aria-label="t('admin.accountMonitor.filters.platform')"
      @change="emit('update:platform', ($event.target as HTMLSelectElement).value)"
    >
      <option value="">{{ t('common.all') }}</option>
      <option v-for="item in platforms" :key="item.value" :value="item.value">{{ item.label }}</option>
    </select>
    <select
      :value="status"
      class="input w-auto min-w-[140px]"
      :aria-label="t('admin.accountMonitor.filters.status')"
      @change="emit('update:status', ($event.target as HTMLSelectElement).value)"
    >
      <option value="">{{ t('common.all') }}</option>
      <option value="available">可用</option>
      <option value="unavailable">不可用</option>
      <option value="cost_ineligible">成本不合格</option>
      <option value="pending">待确认</option>
      <option value="paused">暂停</option>
    </select>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { AccountMonitorAccount } from '@/api/admin/accountMonitor'

const props = defineProps<{
  search: string
  platform: string
  status: string
  accounts: AccountMonitorAccount[]
}>()

const emit = defineEmits<{
  (event: 'update:search', value: string): void
  (event: 'update:platform', value: string): void
  (event: 'update:status', value: string): void
}>()

const { t } = useI18n()

const platformLabels: Record<string, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
  vertex: 'Vertex AI',
  bedrock: 'Bedrock',
  azure_openai: 'Azure OpenAI',
}
const platforms = computed(() => [...new Set(props.accounts.map((account) => account.platform))]
  .sort()
  .map((value) => ({ value, label: platformLabels[value] ?? '其他平台' })))
</script>
