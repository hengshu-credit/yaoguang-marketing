function readBlob(blob: Blob, text = false) { return new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onerror = () => reject(reader.error); reader.onload = () => resolve(String(reader.result)); if (text) reader.readAsText(blob); else reader.readAsDataURL(blob) }) }
export async function archivedMediaDataUrls(blobs: Map<string, Blob>) {
 const resolved = new Map<string, Promise<string>>()
 const render = (source: string, parents = new Set<string>()): Promise<string> => {
  if (parents.has(source)) return Promise.resolve('')
  const cached = resolved.get(source)
  if (cached) return cached
  const promise = (async () => {
   const blob = blobs.get(source)
   if (!blob) return ''
   if (!blob.type.includes('text/css') && !blob.type.includes('svg')) return readBlob(blob)
   let content = await readBlob(blob, true)
   const next = new Set([...parents, source])
   const matches = [...content.matchAll(/(?:url\(\s*["']?|@import\s+["'])([^"'()\s]+)["']?\s*\)?/gi)]
   for (const match of matches) {
    const raw = match[1]
    let absolute: string
    try { absolute = new URL(raw, source).href } catch { continue }
    if (blobs.has(absolute)) content = content.split(raw).join(await render(absolute, next))
   }
   if (blob.type.includes('svg')) for (const asset of blobs.keys()) if (content.includes(asset)) content = content.split(asset).join(await render(asset, next))
   return readBlob(new Blob([content], { type: blob.type }))
  })()
  resolved.set(source, promise)
  return promise
 }
 return Object.fromEntries(await Promise.all([...blobs.keys()].map(async (source) => [source, await render(source)])))
}
