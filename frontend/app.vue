<template>
  <div class="shell">
    <header class="topbar">
      <div class="topbar-inner">
        <nav class="topnav" :aria-label="t('nav.main')">
          <IconBtn class="btn-sm btn-quiet menu-btn" icon="menu" :size="20" :label="t('drawer.menu')" :aria-expanded="drawerOpen" aria-haspopup="dialog" @click="drawerOpen = true" />
          <span class="logo"><span class="logo-x">𝕏</span> x-digest</span>
          <!-- 宽屏保留横排导航；窄屏（≤620px）只在侧边菜单里显示 -->
          <span class="topnav-links">
            <NuxtLink to="/">{{ t('nav.daily') }}</NuxtLink>
            <NuxtLink to="/repos">{{ t('nav.repos') }}</NuxtLink>
            <NuxtLink to="/youtube">{{ t('nav.youtube') }}</NuxtLink>
            <NuxtLink to="/bilibili">{{ t('nav.bilibili') }}</NuxtLink>
            <NuxtLink to="/fuke">{{ t('nav.fuke') }}</NuxtLink>
          </span>
          <NotificationBell />
        </nav>
      </div>
    </header>
    <AppDrawer v-model="drawerOpen" />
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
import { useSettingsSync } from '~/composables/useSettingsSync'
// 外观（主题 / 字号 / 字体）与语言是全站设置：在侧边菜单（AppDrawer）里修改，存本机，所有页面生效。
const { load, theme, fontSize, fontFamily } = useSettings()
const { t, lang, loadLang } = useLang()
const drawerOpen = ref(false)
// Settings cloud sync: pull a newer server copy at start-up, push after any
// change of theme / font / language (debounced).
// Only user changes after start-up are pushed (load() itself sets the refs).
let syncReady = false
const sync = useSettingsSync(() => { load(); loadLang() })
watch([theme, fontSize, fontFamily, lang], () => { if (syncReady) sync.push() })
onMounted(async () => {
  load(); loadLang()
  await sync.pull()
  await nextTick()
  syncReady = true
})
useHead({ htmlAttrs: { 'data-theme': () => theme.value, lang: () => lang.value }, title: () => t('app.title') })
</script>
