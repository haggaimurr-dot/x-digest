<template>
  <div class="bell-wrap" @click.stop>
    <button class="btn btn-sm btn-icon lang-toggle bell-btn" :class="{ 'btn-on': open }" type="button" :aria-label="`通知（${unread} 条未读）`" data-tip="通知" :aria-expanded="open" @click="toggle">
      <UiIcon name="bell" /><span v-if="unread" class="bell-count">{{ unread > 99 ? '99+' : unread }}</span>
    </button>
    <div v-if="open" class="bell-pop" role="dialog" aria-label="通知">
      <div class="bell-head">
        <strong>通知</strong>
        <IconBtn v-if="unread" class="btn-sm" icon="check-check" label="全部已读" @click="markAll" />
      </div>
      <p v-if="!items.length" class="bell-empty">暂无通知（关注的仓库发布新 Release 时会提醒）</p>
      <a v-for="n in items" :key="n.id" class="bell-item" :class="{ unread: !n.read_at }" :href="n.url || undefined" target="_blank" rel="noopener" @click="markOne(n)">
        <span>{{ n.title }}</span>
        <small>{{ shanghaiTime(n.created_at.replace(' ', 'T') + '+08:00') }}</small>
      </a>
    </div>
  </div>
</template>

<script setup lang="ts">
// Top-bar bell (2026-10-28): GET /api/v1/notifications every 5 minutes;
// marking read is a write and goes through the API token like any write.
import { shanghaiTime } from '~/composables/useFormat'

interface Note { id: string, kind: string, title: string, url: string, created_at: string, read_at: string | null }
const store = useStore()
const open = ref(false)
const unread = ref(0)
const items = ref<Note[]>([])
let timer: ReturnType<typeof setInterval> | null = null

async function load () {
  try {
    const r = await store.loadNotifications()
    unread.value = r.unread; items.value = r.notifications
  } catch { /* the bell is optional */ }
}
function toggle () { open.value = !open.value; if (open.value) load() }
async function markAll () { try { await store.markNotificationsRead([]); await load() } catch { /* token prompt declined */ } }
async function markOne (n: Note) { if (!n.read_at) { try { await store.markNotificationsRead([n.id]); await load() } catch { /* ignore */ } } }
function close () { open.value = false }
onMounted(() => {
  if (store.mock.value) return
  load()
  timer = setInterval(load, 5 * 60 * 1000)
  document.addEventListener('click', close)
})
onUnmounted(() => { if (timer) clearInterval(timer); document.removeEventListener('click', close) })
</script>

<style scoped>
.bell-wrap { position: relative; }
.bell-btn { position: relative; }
.bell-count { position: absolute; top: -6px; right: -6px; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 9999px; background: #f4212e; color: #fff; font-size: 11px; line-height: 18px; text-align: center; }
.bell-pop { position: absolute; right: 0; top: calc(100% + 6px); z-index: 300; width: min(340px, 90vw); max-height: 60vh; overflow-y: auto; background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 12px; padding: 8px; box-shadow: 0 8px 24px rgba(0, 0, 0, .4); }
.bell-head { display: flex; justify-content: space-between; align-items: center; padding: 4px 4px 8px; }
.bell-empty { color: var(--text-muted); font-size: 13px; margin: 4px; }
.bell-item { display: flex; flex-direction: column; gap: 2px; padding: 8px; border-radius: 8px; color: var(--text); text-decoration: none; }
.bell-item:hover { background: var(--bg); }
.bell-item.unread span { font-weight: 700; }
.bell-item small { color: var(--text-muted); }
</style>
