// Locks body scroll while a modal is open, so scrolling inside the modal
// (or on its backdrop) can't scroll the page behind it.
// Ref-counted: several modals may hold the lock at once and the page only
// becomes scrollable again when the last one releases it.
import { watch, onBeforeUnmount } from 'vue'
import type { Ref } from 'vue'

let locks = 0
let prevOverflow = ''

function applyLock () {
  if (typeof document === 'undefined') return
  if (locks > 0) {
    if (document.body.style.overflow !== 'hidden') {
      prevOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
  } else {
    document.body.style.overflow = prevOverflow
  }
}

export function useScrollLock (locked: Ref<boolean>) {
  let current = false
  const sync = (v: boolean) => {
    if (v) locks++
    else locks = Math.max(0, locks - 1)
    applyLock()
  }
  const stop = watch(locked, (v) => {
    if (v === current) return
    current = v
    sync(v)
  }, { immediate: true })
  onBeforeUnmount(() => {
    stop()
    if (current) {
      current = false
      sync(false)
    }
  })
}
