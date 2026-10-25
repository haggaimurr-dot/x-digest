<template>
  <div class="shell">
    <header class="topbar">
      <div class="topbar-inner">
        <nav class="topnav" aria-label="主导航">
          <span class="logo"><span class="logo-x">𝕏</span> x-digest</span>
          <NuxtLink to="/">X日报</NuxtLink>
          <NuxtLink to="/repos">仓库</NuxtLink>
          <NuxtLink to="/fuke">复刻</NuxtLink>
        </nav>
      </div>
    </header>
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
const { applyTheme } = useSettings()
onMounted(() => applyTheme())
useHead({ htmlAttrs: { 'data-theme': 'dark' } })
</script>
