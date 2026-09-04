<template>
  <AppLayout>
    <section data-test="hybrid-performance-view" class="mx-auto w-full max-w-[1500px] space-y-6 rounded-2xl text-slate-900 dark:bg-[#07101f] dark:px-3 dark:py-3 dark:text-slate-100 sm:dark:px-4 sm:dark:py-4">
      <HybridPerformancePanel />
      <CodexRadarRecommendations />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import CodexRadarRecommendations from '@/features/monitor-v2/CodexRadarRecommendations.vue'
import HybridPerformancePanel from './HybridPerformancePanel.vue'
</script>
