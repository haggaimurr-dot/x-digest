<template>
  <div class="shell">
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
const { applyTheme } = useSettings()
onMounted(() => applyTheme())
useHead({ htmlAttrs: { 'data-theme': 'dark' } })
</script>
