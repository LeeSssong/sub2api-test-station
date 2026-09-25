<template>
  <header class="user-page-header">
    <h1>{{ title }}</h1>
    <p v-if="description">{{ description }}</p>
  </header>
</template>

<script setup lang="ts">
defineProps<{ title: string; description?: string }>()
</script>

<style scoped>
.user-page-header h1 { font-size: 20px; font-weight: 600; }
</style>
