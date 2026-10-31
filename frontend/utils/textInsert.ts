// insertAt puts `insert` into `text` at `pos` on its own line (adds line
// breaks around it unless already at a line boundary).
export function insertAt (text: string, pos: number, insert: string): string {
  const p = Math.max(0, Math.min(pos, text.length))
  const before = text.slice(0, p)
  const after = text.slice(p)
  const lead = before && !before.endsWith('\n') ? '\n' : ''
  const trail = after && !after.startsWith('\n') ? '\n' : ''
  return before + lead + insert + trail + after
}
