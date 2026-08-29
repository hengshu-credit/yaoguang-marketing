import { describe, expect, it } from 'vitest'
import { parsePOCatalog } from './po'

describe('parsePOCatalog', () => {
  it('decodes multiline entries, escaped strings, and every source reference', () => {
    const entries = parsePOCatalog(`msgid ""
msgstr ""
"Language: en\\n"

#. translator note
#: src/components/Example.tsx:12 src/pages/ExamplePage.tsx:34
msgid "Hello \\\"world\\\""
" from a multiline message"
msgstr "Bonjour \\\"monde\\\""
" depuis plusieurs lignes"

#: src/components/Other.tsx:8
msgid "Save"
msgstr "Enregistrer"
`)

    expect(entries).toEqual([
      {
        msgid: 'Hello "world" from a multiline message',
        msgstr: 'Bonjour "monde" depuis plusieurs lignes',
        references: ['src/components/Example.tsx:12', 'src/pages/ExamplePage.tsx:34'],
      },
      {
        msgid: 'Save',
        msgstr: 'Enregistrer',
        references: ['src/components/Other.tsx:8'],
      },
    ])
  })
})
