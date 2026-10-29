<template>
  <!-- 侧边汉堡菜单：导航 + 当前页操作 + 外观/语言。图标 + 文字横排，窄屏不会把按钮竖着堆起来。 -->
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="open" class="drawer-backdrop" @click.self="close">
        <aside ref="panel" class="drawer" role="dialog" aria-modal="true" :aria-label="t('drawer.title')" @keydown.esc="close">
          <div class="drawer-head">
            <span class="logo"><span class="logo-x">𝕏</span> x-digest</span>
            <IconBtn class="btn-sm btn-quiet tip-below" icon="x" :size="18" :label="t('drawer.close')" @click="close" />
          </div>

          <nav class="drawer-section" :aria-label="t('nav.main')">
            <NuxtLink v-for="n in NAV" :key="n.to" :to="n.to" class="drawer-item" @click="close">
              <UiIcon :name="n.icon" :size="18" /><span>{{ t(n.label) }}</span>
            </NuxtLink>
          </nav>

          <section v-if="actions.length" class="drawer-section">
            <div class="drawer-title">{{ t('drawer.pageActions') }}</div>
            <button v-for="a in actions" :key="a.key" type="button" class="drawer-item" :class="{ 'is-primary': a.primary, 'is-busy': a.busy }"
              :disabled="a.disabled" @click="run(a)">
              <span class="btn-icon-glyph"><UiIcon :name="a.icon" :size="18" /><span v-if="a.plus" class="btn-icon-plus" aria-hidden="true"><UiIcon name="plus" :size="8" :stroke="3.2" /></span></span>
              <span>{{ a.label }}</span>
            </button>
          </section>

          <section class="drawer-section">
            <div class="drawer-title">{{ t('settings.appearance') }}</div>
            <div class="drawer-label">{{ t('settings.theme') }}</div>
            <div class="drawer-seg" role="group" :aria-label="t('settings.theme')">
              <button type="button" :class="{ on: theme === 'dark' }" :aria-pressed="theme === 'dark'" @click="setTheme('dark')"><UiIcon name="moon" />{{ t('drawer.dark') }}</button>
              <button type="button" :class="{ on: theme === 'light' }" :aria-pressed="theme === 'light'" @click="setTheme('light')"><UiIcon name="sun" />{{ t('settings.light') }}</button>
            </div>
            <div class="drawer-label">{{ t('settings.fontSize') }}</div>
            <div class="drawer-seg" role="group" :aria-label="t('settings.fontSize')">
              <button v-for="sz in ['13px', '15px', '17px', '19px']" :key="sz" type="button" :class="{ on: fontSize === sz }" :aria-pressed="fontSize === sz" @click="setFontSize(sz)">{{ sz }}</button>
            </div>
            <div class="drawer-label">{{ t('settings.font') }}</div>
            <select class="field" :value="fontFamily" :aria-label="t('settings.font')" @change="setFontFamily(($event.target as HTMLSelectElement).value)">
              <option value="">{{ t('settings.fontSystem') }}</option>
              <option value="'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif">{{ t('settings.fontPingFang') }}</option>
              <option value="Georgia,serif">{{ t('settings.fontSerif') }}</option>
              <option value="ui-monospace,Menlo,monospace">{{ t('settings.fontMono') }}</option>
            </select>
            <button type="button" class="drawer-item" style="margin-top:8px" @click="toggleLang">
              <UiIcon name="languages" :size="18" /><span>{{ lang === 'en' ? t('lang.switchToZh') : t('lang.switchToEn') }}</span>
            </button>
          </section>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { currentPageActions, type PageAction } from '~/composables/usePageActions'
import { useScrollLock } from '~/composables/useScrollLock'

const open = defineModel<boolean>({ default: false })
const { theme, fontSize, fontFamily, setTheme, setFontSize, setFontFamily } = useSettings()
const { t, lang, toggleLang } = useLang()
const route = useRoute()
const panel = ref<HTMLElement | null>(null)

const NAV = [
  { to: '/', icon: 'newspaper', label: 'nav.daily' },
  { to: '/repos', icon: 'book', label: 'nav.repos' },
  { to: '/fuke', icon: 'fork', label: 'nav.fuke' },
  { to: '/youtube', icon: 'video', label: 'nav.youtube' },
  { to: '/bilibili', icon: 'tv', label: 'nav.bilibili' }
]
const actions = computed(() => currentPageActions())

function close () { open.value = false }
function run (a: PageAction) { close(); a.run() }

useScrollLock(open)
watch(() => route.fullPath, close)
// Move focus into the drawer so Esc / Tab work right away.
watch(open, async (v) => {
  if (!v) return
  await nextTick()
  panel.value?.querySelector<HTMLElement>('.drawer-item')?.focus()
})
</script>
