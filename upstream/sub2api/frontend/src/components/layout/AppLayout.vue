<template>
  <div class="min-h-screen" :class="isAdmin ? 'bg-gray-50 dark:bg-dark-950' : 'dark user-app-shell bg-[#040b17]'">
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader v-if="isAdmin" />

      <div v-else class="sticky top-0 z-30 flex h-14 items-center border-b border-gray-200 bg-white/95 px-4 backdrop-blur dark:border-dark-700 dark:bg-dark-900/95 lg:hidden">
        <button
          type="button"
          class="btn-ghost btn-icon"
          :aria-label="$t('common.toggleMenu')"
          @click="appStore.toggleMobileSidebar()"
        >
          <Icon name="menu" size="md" />
        </button>
      </div>

      <!-- Main Content -->
      <main class="p-4 md:p-6 lg:p-8" :class="{ 'user-workspace': !isAdmin }">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import Icon from '@/components/icons/Icon.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
.user-app-shell {
  color: #e8f3fb;
}

.user-workspace {
  min-height: 100vh;
  background: #040b17;
}
</style>
