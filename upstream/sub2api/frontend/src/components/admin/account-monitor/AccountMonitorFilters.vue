<template>
  <div class="grid w-full grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_152px]">
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
      :value="status"
      class="input w-full"
      data-test="status-filter"
      :aria-label="t('admin.accountMonitor.filters.status')"
      @change="emit('update:status', ($event.target as HTMLSelectElement).value)"
    >
      <option value="">全部状态</option>
      <option value="available">可用</option>
      <option value="unavailable">不可用</option>
      <option value="cost_ineligible">成本不合格</option>
      <option value="pending">待确认</option>
      <option value="paused">暂停</option>
    </select>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  search: string
  status: string
}>()

const emit = defineEmits<{
  (event: 'update:search', value: string): void
  (event: 'update:status', value: string): void
}>()

const { t } = useI18n()
</script>
