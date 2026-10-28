// Embedded players for the AI summary chapters (2026-10-28): clicking a
// chapter time plays the video from there inside the summary panel.
// YouTube: IFrame embed with enablejsapi=1, so a later chapter click seeks
// the running player via postMessage (no reload). Bilibili: its iframe has
// no seek API, so the src is reloaded with t=<seconds>.

export function embedUrl (platform: 'yt' | 'bili', id: string, start: number): string {
  const s = Math.max(0, Math.floor(start || 0))
  if (platform === 'bili') {
    const key = /^BV/i.test(id) ? `bvid=${encodeURIComponent(id)}` : `aid=${encodeURIComponent(id.replace(/^av/i, ''))}`
    return `https://player.bilibili.com/player.html?${key}&t=${s}&autoplay=1&high_quality=1`
  }
  return `https://www.youtube.com/embed/${encodeURIComponent(id)}?enablejsapi=1&autoplay=1&start=${s}`
}

// seekMessage is the IFrame API command that moves a YouTube player.
export function seekMessage (seconds: number): string {
  return JSON.stringify({ event: 'command', func: 'seekTo', args: [Math.max(0, Math.floor(seconds)), true] })
}
