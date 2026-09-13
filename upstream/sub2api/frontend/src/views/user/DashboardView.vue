<template>
  <AppLayout>
    <div class="mx-auto max-w-[1500px] space-y-8">
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :show-platform-breakdown="false" />
        <HybridPerformancePanel />
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import HybridPerformancePanel from '@/features/monitor-v4/HybridPerformancePanel.vue'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null); const loading = ref(false)
const loadStats = async () => { loading.value = true; try { await authStore.refreshUser(); stats.value = await usageAPI.getDashboardStats() } catch (error) { console.error('Failed to load dashboard stats:', error) } finally { loading.value = false } }
const refreshAll = () => { loadStats() }

onMounted(() => { refreshAll() })
</script>
