export interface POEntry {
  msgid: string
  msgstr: string
  references: string[]
  isExplicitId?: true
}

/**
 * Parse the PO fields needed by the workspace translation inventory.
 *
 * Lingui's generated PO files use the standard quoted-string form, including
 * continuation lines for long messages and source references split over more
 * than one comment line. Obsolete entries are deliberately ignored: they are
 * no longer part of the catalog users can edit.
 */
export function parsePOCatalog(source: string): POEntry[] {
  const entries: POEntry[] = []
  let msgid = ''
  let msgstr = ''
  let references: string[] = []
  let reading: 'msgid' | 'msgstr' | null = null
  let obsolete = false
  let started = false
  let isExplicitId = false

  const flush = () => {
    if (started && !obsolete && msgid !== '') {
      entries.push({
        msgid,
        msgstr,
        references,
        ...(isExplicitId ? { isExplicitId: true as const } : {}),
      })
    }
    msgid = ''
    msgstr = ''
    references = []
    reading = null
    obsolete = false
    started = false
    isExplicitId = false
  }

  for (const line of source.replace(/\r\n?/g, '\n').split('\n')) {
    if (line === '') {
      flush()
      continue
    }

    if (line.startsWith('#~')) {
      obsolete = true
      continue
    }
    if (line === '#. js-lingui-explicit-id') {
      isExplicitId = true
      continue
    }
    if (line.startsWith('#:')) {
      references.push(...line.slice(2).trim().split(/\s+/).filter(Boolean))
      continue
    }
    if (line.startsWith('#')) continue

    if (line.startsWith('msgid ')) {
      const entryReferences = references
      const entryIsExplicitId: boolean = isExplicitId
      flush()
      started = true
      references = entryReferences
      isExplicitId = entryIsExplicitId
      msgid = decodePOString(line.slice('msgid '.length))
      reading = 'msgid'
      continue
    }
    if (line.startsWith('msgstr ')) {
      started = true
      msgstr = decodePOString(line.slice('msgstr '.length))
      reading = 'msgstr'
      continue
    }
    if (line.startsWith('"') && reading) {
      const value = decodePOString(line)
      if (reading === 'msgid') msgid += value
      else msgstr += value
      continue
    }

    // Context and plural fields mark the entry as non-simple for Lingui. They
    // do not affect the literal source/references preserved above, so ignore
    // their contents while ensuring continuations are not appended to msgid.
    reading = null
  }

  flush()
  return entries
}

function decodePOString(value: string): string {
  return JSON.parse(value) as string
}
