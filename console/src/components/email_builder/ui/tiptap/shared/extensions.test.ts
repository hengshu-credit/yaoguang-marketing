import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { createInlineExtensions } from './extensions'

/**
 * Regression tests for the inline (button) editor schema.
 *
 * Issue #352: typing into a button produced one character per line
 * ("Example", then "h", "o", "l" each on a new line). Root cause: StarterKit
 * enables the TrailingNode extension by default. In the inline schema there is
 * no paragraph, so TrailingNode resolved its node to the schema's default inline
 * type (hardBreak) and appended a <br> after every transaction — i.e. after every
 * keystroke. Disabling trailingNode in createInlineExtensions fixes it.
 */
describe('createInlineExtensions', () => {
  let editor: Editor | undefined

  afterEach(() => {
    editor?.destroy()
    editor = undefined
  })

  const makeEditor = (content: string) => {
    const element = document.createElement('div')
    document.body.appendChild(element)
    return new Editor({
      element,
      extensions: createInlineExtensions(),
      content
    })
  }

  it('does not insert hard breaks when typing characters (issue #352)', () => {
    editor = makeEditor('<span data-inline-doc="">Example</span>')

    // Type "hol" character-by-character at the end of the content,
    // exactly like the user did in the bug report.
    editor.commands.setTextSelection(editor.state.doc.content.size)
    for (const char of 'hol') {
      editor.commands.insertContent(char)
    }

    const html = editor.getHTML()
    expect(html).not.toContain('<br')
    expect(editor.getText()).toBe('Examplehol')
  })

  it('keeps the inline schema free of block/break trailing nodes', () => {
    editor = makeEditor('<span data-inline-doc="">Example</span>')

    // The trailing-node behaviour appends a node after the content on every
    // transaction. A no-op selection change must not grow the document.
    const sizeBefore = editor.state.doc.content.size
    editor.commands.setTextSelection(1)
    editor.commands.setTextSelection(editor.state.doc.content.size)
    expect(editor.state.doc.content.size).toBe(sizeBefore)
    expect(editor.getHTML()).not.toContain('<br')
  })
})
