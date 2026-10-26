<template>
  <div class="shell">
    <header class="topbar">
      <div class="topbar-inner">
        <nav class="topnav" :aria-label="t('nav.main')">
          <span class="logo"><span class="logo-x">𝕏</span> x-digest</span>
          <NuxtLink to="/">{{ t('nav.daily') }}</NuxtLink>
          <NuxtLink to="/repos">{{ t('nav.repos') }}</NuxtLink>
          <NuxtLink to="/fuke">{{ t('nav.fuke') }}</NuxtLink>
          <NuxtLink to="/youtube">{{ t('nav.youtube') }}</NuxtLink>
          <NuxtLink to="/bilibili">{{ t('nav.bilibili') }}</NuxtLink>
          <button class="btn btn-sm lang-toggle" type="button" @click="toggleLang"
            :aria-label="lang === 'en' ? t('lang.switchToZh') : t('lang.switchToEn')">{{ lang === 'en' ? '中文' : 'EN' }}</button>
        </nav>
      </div>
    </header>
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
const { applyTheme, theme } = useSettings()
const { t, lang, toggleLang, loadLang } = useLang()
onMounted(() => { applyTheme(); loadLang() })
useHead({ htmlAttrs: { 'data-theme': () => theme.value, lang: () => lang.value }, title: () => t('app.title') })
</script>
