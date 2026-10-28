import { describe, expect, it } from 'vitest'
import { remoteIsNewer } from '~/composables/useSettingsSync'

describe('remoteIsNewer', () => {
  it('server copy wins only when newer than this device', () => {
    expect(remoteIsNewer('2026-10-28T10:00:00Z', null)).toBe(true)
    expect(remoteIsNewer('2026-10-28T10:00:00Z', '2026-10-28T09:00:00Z')).toBe(true)
    expect(remoteIsNewer('2026-10-28T10:00:00Z', '2026-10-28T10:00:00Z')).toBe(false)
    expect(remoteIsNewer('2026-10-28T08:00:00Z', '2026-10-28T09:00:00Z')).toBe(false)
    expect(remoteIsNewer('garbage', null)).toBe(false)
  })
})
