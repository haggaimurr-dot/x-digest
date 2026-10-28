// PPTX helpers for AttachmentPreview (pure, unit-tested in pptx.test.ts).

// ppt/slides/slide10.xml sorts after slide9.xml, not after slide1.xml.
export function sortSlideNames (names: string[]): string[] {
  const num = (n: string) => Number(n.match(/slide(\d+)\.xml$/)?.[1] || 0)
  return names.filter(n => /^ppt\/slides\/slide\d+\.xml$/.test(n)).sort((a, b) => num(a) - num(b))
}

// Images referenced by one slide: ppt/slides/_rels/slideN.xml.rels lists
// relationships; image ones point at ../media/imageX.ext. Returns zip paths
// (ppt/media/imageX.ext), in document order, without duplicates.
export function slideImageTargets (relsXml: string): string[] {
  const out: string[] = []
  for (const m of relsXml.matchAll(/<Relationship\b[^>]*>/g)) {
    const tag = m[0]
    if (!/Type="[^"]*\/image"/.test(tag)) continue
    const target = tag.match(/Target="([^"]+)"/)?.[1]
    if (!target || /TargetMode="External"/.test(tag)) continue
    const path = resolvePath('ppt/slides/', target)
    if (!out.includes(path)) out.push(path)
  }
  return out
}

function resolvePath (base: string, rel: string): string {
  if (rel.startsWith('/')) return rel.slice(1)
  const parts = base.split('/').filter(Boolean)
  for (const seg of rel.split('/')) {
    if (seg === '..') parts.pop()
    else if (seg !== '.') parts.push(seg)
  }
  return parts.join('/')
}

export function relsPathFor (slidePath: string): string {
  return slidePath.replace(/^ppt\/slides\/(slide\d+\.xml)$/, 'ppt/slides/_rels/$1.rels')
}
