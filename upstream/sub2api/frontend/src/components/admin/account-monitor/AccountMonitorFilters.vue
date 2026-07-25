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
      <option v-for="item in platforms" :key="item" :value="item">{{ item }}</option>
    </select>
    <select
      :value="group"
      class="input w-auto min-w-[140px]"
      :aria-label="t('admin.accountMonitor.filters.group')"
      data-test="group-filter"
      @change="emit('update:group', ($event.target as HTMLSelectElement).value)"
    >
      <option value="">{{ t('admin.accountMonitor.filters.allGroups') }}</option>
      <option v-for="item in groups" :key="item.id" :value="String(item.id)">
        {{ item.name }}
      </option>
    </select>
    <select
      :value="status"
      class="input w-auto min-w-[140px]"
      :aria-label="t('admin.accountMonitor.filters.status')"
      @change="emit('update:status', ($event.target as HTMLSelectElement).value)"
    >
      <option value="">{{ t('common.all') }}</option>
      <option value="success">{{ t('admin.accountMonitor.status.success') }}</option>
      <option value="failed">{{ t('admin.accountMonitor.status.failed') }}</option>
      <option value="unavailable">{{ t('admin.accountMonitor.status.noHistory') }}</option>
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
  group: string
  accounts: AccountMonitorAccount[]
}>()

const emit = defineEmits<{
  (event: 'update:search', value: string): void
  (event: 'update:platform', value: string): void
  (event: 'update:status', value: string): void
  (event: 'update:group', value: string): void
}>()

const { t } = useI18n()

const platforms = computed(() => [...new Set(props.accounts.map((account) => account.platform))].sort())
const groups = computed(() => {
  const byID = new Map<number, string>()
  for (const account of props.accounts) {
    account.group_ids.forEach((id, index) => {
      const name = account.group_names[index]?.trim()
      const current = byID.get(id)
      if (!current || current.startsWith('#')) {
        byID.set(id, name || `#${id}`)
      }
    })
  }
  return [...byID.entries()]
    .map(([id, name]) => ({ id, name }))
    .sort((left, right) => left.name.localeCompare(right.name) || left.id - right.id)
})
</script>
