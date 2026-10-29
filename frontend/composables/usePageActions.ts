// Page-level toolbar actions (import / sync / add …) shown in the side drawer
// (AppDrawer) instead of an icon row on the page, so they never stack up on
// narrow screens. A page registers a getter while it is mounted; the getter
// reads the page's reactive state, so labels / busy / disabled stay live.
import { shallowRef, onMounted, onBeforeUnmount } from 'vue'

export interface PageAction {
  key: string
  label: string
  icon: string
  plus?: boolean
  primary?: boolean
  busy?: boolean
  disabled?: boolean
  run: () => void
}

// Module-level, only written on the client (onMounted), so SSR never shares it.
const source = shallowRef<(() => PageAction[]) | null>(null)

export function usePageActions (getter: () => PageAction[]) {
  onMounted(() => { source.value = getter })
  onBeforeUnmount(() => { if (source.value === getter) source.value = null })
}

export function currentPageActions (): PageAction[] {
  return source.value?.() ?? []
}
