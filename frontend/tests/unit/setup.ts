// Nuxt auto-imports used by components under test.
import * as vue from 'vue'
import { t } from '~/utils/i18n'

const g = globalThis as any
for (const k of ['ref', 'computed', 'watch', 'nextTick', 'onMounted', 'onBeforeUnmount', 'onUnmounted', 'reactive']) g[k] = (vue as any)[k]
g.useLang = () => ({ t, lang: vue.ref('zh') })
