// Settings cloud sync (backend settings_sync.go). The synced payload is the
// same JSON as 「导出设置」 (utils/settingsTransfer.ts), so secrets never
// travel. Needs the API token: without one the page keeps local-only
// settings, exactly as before.
import { apiToken } from './useApi'
import { exportSettings, importSettings } from '~/utils/settingsTransfer'

const LOCAL_AT = 'xdigest.settingsUpdatedAt'

// remoteIsNewer: should the server copy replace the local settings?
export function remoteIsNewer (remoteAt: string, localAt: string | null): boolean {
  const r = Date.parse(remoteAt)
  if (Number.isNaN(r)) return false
  const l = localAt ? Date.parse(localAt) : NaN
  return Number.isNaN(l) || r > l
}

function readLocalAt (): string | null { try { return localStorage.getItem(LOCAL_AT) } catch { return null } }
function writeLocalAt (v: string) { try { localStorage.setItem(LOCAL_AT, v) } catch { /* private mode */ } }

export function useSettingsSync (onApplied: () => void) {
  const { mock, apiFetch } = useApi()
  let timer: ReturnType<typeof setTimeout> | null = null
  const enabled = () => !mock.value && typeof localStorage !== 'undefined' && !!apiToken()

  function apply (settings: unknown, at: string) {
    importSettings(JSON.stringify(settings), localStorage)
    writeLocalAt(at)
    onApplied()
  }
  // Start-up: take the server copy when it is newer than what this device has.
  async function pull () {
    if (!enabled()) return
    try {
      const r = await apiFetch<{ settings: unknown, updated_at: string }>('settings')
      if (remoteIsNewer(r.updated_at, readLocalAt())) apply(r.settings, r.updated_at)
    } catch { /* 404 = never saved; offline = keep local */ }
  }
  // After a local change: upload (debounced). A 409 means another device
  // saved later — its copy wins and is applied here.
  function push () {
    if (!enabled()) return
    if (timer) clearTimeout(timer)
    timer = setTimeout(async () => {
      const at = new Date().toISOString()
      try {
        await apiFetch('settings', { method: 'PUT', body: JSON.stringify({ settings: JSON.parse(exportSettings(localStorage)), updated_at: at }) })
        writeLocalAt(at)
      } catch (e: any) {
        if (e?.status === 409) await pull()
      }
    }, 1500)
  }
  return { pull, push }
}
