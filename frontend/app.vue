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
          <NotificationBell />
          <div class="appearance-wrap">
            <button class="btn btn-sm lang-toggle" type="button" :aria-expanded="showAppearance" aria-haspopup="dialog" @click.stop="showAppearance = !showAppearance">Aa {{ t('settings.appearance') }}</button>
            <div v-if="showAppearance" class="appearance-pop" role="dialog" :aria-label="t('settings.appearance')" @click.stop>
              <div class="appearance-label">{{ t('settings.theme') }}</div>
              <div class="appearance-row">
                <button class="btn btn-sm" :class="{ 'btn-primary': theme === 'dark' }" type="button" @click="setTheme('dark')">{{ t('settings.dark') }}</button>
                <button class="btn btn-sm" :class="{ 'btn-primary': theme === 'light' }" type="button" @click="setTheme('light')">{{ t('settings.light') }}</button>
              </div>
              <div class="appearance-label">{{ t('settings.fontSize') }}</div>
              <div class="appearance-row">
                <button v-for="sz in ['13px', '15px', '17px', '19px']" :key="sz" class="btn btn-sm" :class="{ 'btn-primary': fontSize === sz }" type="button" @click="setFontSize(sz)">{{ sz }}</button>
              </div>
              <div class="appearance-label">{{ t('settings.font') }}</div>
              <select class="field" :value="fontFamily" @change="setFontFamily(($event.target as HTMLSelectElement).value)">
                <option value="">{{ t('settings.fontSystem') }}</option>
                <option value="'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif">{{ t('settings.fontPingFang') }}</option>
                <option value="Georgia,serif">{{ t('settings.fontSerif') }}</option>
                <option value="ui-monospace,Menlo,monospace">{{ t('settings.fontMono') }}</option>
              </select>
            </div>
          </div>
          <button class="btn btn-sm lang-toggle" type="button" @click="toggleLang"
            :aria-label="lang === 'en' ? t('lang.switchToZh') : t('lang.switchToEn')">{{ lang === 'en' ? '中文' : 'EN' }}</button>
        </nav>
      </div>
    </header>
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
import { useSettingsSync } from '~/composables/useSettingsSync'
// 外观（主题 / 字号 / 字体）是全站设置：在顶栏统一修改，存本机，所有页面生效。
const { load, theme, fontSize, fontFamily, setTheme, setFontSize, setFontFamily } = useSettings()
const { t, lang, toggleLang, loadLang } = useLang()
const showAppearance = ref(false)
function closeAppearance () { showAppearance.value = false }
function onKey (e: KeyboardEvent) { if (e.key === 'Escape') closeAppearance() }
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
  document.addEventListener('click', closeAppearance)
  document.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  document.removeEventListener('click', closeAppearance)
  document.removeEventListener('keydown', onKey)
})
useHead({ htmlAttrs: { 'data-theme': () => theme.value, lang: () => lang.value }, title: () => t('app.title') })
</script>
