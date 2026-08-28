# Tiptap Editor Components

This directory contains the refactored Tiptap editor components, organized into specialized, focused components for better maintainability and performance.

## 🏗️ Architecture Overview

The original 1,505-line `TiptapComponent` has been refactored into a modular architecture:

```
tiptap/
├── TiptapRichEditor.tsx        # Full-featured block-level editor (~400 lines)
├── TiptapInlineEditor.tsx      # Inline-only text editor (~300 lines)
├── TiptapComponent.tsx         # Backward-compatible wrapper (deprecated)
├── TiptapSchema.ts             # Custom Tiptap extensions and marks
├── components/
│   └── TiptapToolbar.tsx       # Shared toolbar components (~500 lines)
├── shared/
│   ├── types.ts                # TypeScript interfaces and types
│   ├── extensions.ts           # Tiptap extension configurations
│   ├── utils.ts                # Formatting and content utilities
│   └── styles.ts               # Shared CSS styles and styling utilities
├── index.ts                    # Public API exports
└── README.md                   # This file
```

## 📦 Components

### TiptapRichEditor

Full-featured rich text editor for block-level content editing.

```tsx
import { TiptapRichEditor } from './tiptap'
;<TiptapRichEditor
  content="<p>Initial content</p>"
  onChange={(content) => console.log(content)}
  placeholder="Start writing..."
  buttons={['bold', 'italic', 'underline', 'textColor']}
  autoFocus={true}
/>
```

**Features:**

- Paragraph support
- Full formatting toolbar
- Block-level content handling
- Email-compatible HTML output

### TiptapInlineEditor

Specialized editor for inline-only text editing (no paragraphs or block elements).

```tsx
import { TiptapInlineEditor } from './tiptap'
;<TiptapInlineEditor
  content="Button text"
  onChange={(content) => console.log(content)}
  placeholder="Enter text..."
  buttons={['bold', 'italic', 'textColor']}
/>
```

**Features:**

- Inline-only content (no `<p>` tags)
- Prevents line breaks
- Automatic block-to-inline conversion
- Perfect for buttons, labels, headers

### TiptapComponent (Deprecated)

Backward-compatible wrapper that automatically chooses the appropriate editor.

```tsx
import { TiptapComponent } from './tiptap'

// Uses TiptapRichEditor
<TiptapComponent content="<p>Rich content</p>" />

// Uses TiptapInlineEditor
<TiptapComponent inline={true} content="Inline content" />
```

**⚠️ Migration Notice:** Consider using `TiptapRichEditor` or `TiptapInlineEditor` directly for better type safety and performance.

## 🔧 Props Interface

### Common Props (BaseTiptapProps)

```tsx
interface BaseTiptapProps {
  content?: string // HTML content
  onChange?: (content: string) => void // Content change callback
  readOnly?: boolean // Read-only mode
  placeholder?: string // Placeholder text
  containerStyle?: React.CSSProperties // Container styling
  autoFocus?: boolean // Auto-focus on mount
  buttons?: string[] // Toolbar button selection
}
```

### Available Toolbar Buttons

```tsx
type ButtonType =
  | 'undo'
  | 'redo'
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strikethrough'
  | 'textColor'
  | 'backgroundColor'
  | 'emoji'
  | 'link'
  | 'superscript'
  | 'subscript'
```

## 🚀 Benefits of the Refactored Architecture

### ✅ Advantages

- **Single Responsibility**: Each component has a clear, focused purpose
- **Performance**: Smaller bundles and focused re-rendering
- **Type Safety**: More specific prop interfaces for each use case
- **Maintainability**: Easier to modify one editing mode without affecting others
- **Testing**: Components can be unit tested independently
- **Bundle Optimization**: Tree-shaking removes unused code paths
- **API Clarity**: Users know exactly what they're getting

### 📊 Size Comparison

| Component                | Before      | After      |
| ------------------------ | ----------- | ---------- |
| Original TiptapComponent | 1,505 lines | -          |
| TiptapRichEditor         | -           | ~400 lines |
| TiptapInlineEditor       | -           | ~300 lines |
| TiptapToolbar            | -           | ~500 lines |
| Shared utilities         | -           | ~300 lines |

## 🔄 Migration Guide

### From Original TiptapComponent

**Before:**

```tsx
import { TiptapComponent } from '../ui/TiptapComponent'

// Rich text editing
<TiptapComponent
  content="<p>Content</p>"
  onChange={handleChange}
/>

// Inline editing
<TiptapComponent
  inline={true}
  content="Button text"
  onChange={handleChange}
/>
```

**After:**

```tsx
import { TiptapRichEditor, TiptapInlineEditor } from '../ui/tiptap'

// Rich text editing
<TiptapRichEditor
  content="<p>Content</p>"
  onChange={handleChange}
/>

// Inline editing
<TiptapInlineEditor
  content="Button text"
  onChange={handleChange}
/>
```

### Gradual Migration Strategy

1. **Phase 1**: Import from new location

   ```tsx
   import { TiptapComponent } from '../ui/tiptap/TiptapComponent'
   ```

2. **Phase 2**: Replace with specific components

   ```tsx
   import { TiptapRichEditor, TiptapInlineEditor } from '../ui/tiptap'
   ```

3. **Phase 3**: Remove deprecated wrapper

## 🛠️ Advanced Usage

### Custom Extensions

```tsx
import { createRichExtensions, TextStyleMark } from './tiptap'

const customExtensions = [
  ...createRichExtensions()
  // Add your custom extensions here
]
```

### Custom Toolbar

```tsx
import { TiptapToolbar } from './tiptap'
;<TiptapToolbar editor={editor} buttons={['bold', 'italic', 'textColor']} mode="rich" />
```

### Utility Functions

```tsx
import { convertBlockToInline, processInlineContent, expandSelectionToNode } from './tiptap'

// Convert block HTML to inline spans
const inlineHtml = convertBlockToInline('<p>Block content</p>')

// Process inline editor content
const processed = processInlineContent(editorHtml)
```

## 🎨 Styling

The components use shared CSS injection for consistent styling:

```tsx
import { injectTiptapStyles } from './tiptap'

// Styles are automatically injected, but you can call manually if needed
injectTiptapStyles()
```

## 🧪 Testing

Each component can be tested independently:

```tsx
import { render } from '@testing-library/react'
import { TiptapRichEditor } from './tiptap'

test('renders rich editor', () => {
  render(<TiptapRichEditor content="<p>Test</p>" />)
  // Test rich editor specific functionality
})
```

## 📝 Contributing

When contributing to these components:

1. **Keep components focused** - Don't add unrelated functionality
2. **Update shared utilities** - Common logic goes in `shared/`
3. **Maintain backward compatibility** - Until deprecated wrapper is removed
4. **Add proper TypeScript types** - Export from `shared/types.ts`
5. **Test both variants** - Ensure changes work for both rich and inline editors

## 🔍 Troubleshooting

### Common Issues

**Content not updating:**

- Check if `onChange` is properly memoized
- Ensure content comparison logic is correct

**Inline content showing as blocks:**

- Use `TiptapInlineEditor` instead of `TiptapRichEditor`
- Check content processing in `shared/utils.ts`

**Toolbar not showing:**

- Ensure `readOnly={false}` (default)
- Check `buttons` prop is properly set

**Type errors:**

- Import types from `shared/types.ts`
- Use appropriate prop interface for each component

### Debug Mode

Enable debug logging:

```tsx
// Check console for content processing logs
<TiptapInlineEditor content="debug content" />
```
