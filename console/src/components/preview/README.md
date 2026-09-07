# Shared material preview

`DeviceFrame` owns the device chrome, status bar, browser/window bar, preview canvas and scrollable screen. Use `device="ios" | "android" | "desktop" | "h5"` and place content in its children. `appearance="notification"` adds the lock screen or desktop notification background; `compact` is for inbox summaries. All CSS is scoped to `material-*` classes in `preview.css`.

`HtmlPreview` renders HTML inside the same device with an empty iframe sandbox. Keep the sandbox when adding new HTML surfaces. `PreviewToolbar` provides shared controls and the simulation label.

`EmailPreview` combines that sandboxed body with the sender name/address, rendered subject and preview text. Email editors pass the metadata from their settings and use the subject/summary returned by the same compile request as the body. Web pages continue to use `HtmlPreview` without a mail header.

The HTML layout viewport is independent of the available panel width: desktop uses 1024px, Android 393px, and iOS/H5 375px, then scales to the device screen. This keeps desktop media queries from falling back to mobile layouts inside narrow drawers.

The preview stage has an explicit width because `container-type: inline-size` suppresses intrinsic sizing inside a flex parent. Compact inbox previews are capped at 402px, including the canvas around the 360px phone. Keep their natural height; do not reintroduce a fixed-height scaled wrapper in translation forms.

The visual preview panel and MJML code editor share the `material-email-editor-split` layout. Below 768px it stacks the full-width preview above a scrollable details/code panel, overriding the splitter's inline flex sizes so a 300px minimum panel cannot collapse the phone. Desktop retains draggable columns. Keep the editor wrapper scrollable and the drawer's step tabs in normal flow on narrow screens.

Existing entry points reuse these components:

- `TemplatePreviewDrawer`: material management, campaign/template selectors, message history and customer timeline.
- `ChannelMessagePreview`: SMS and Push drafts and saved material.
- `OmnichannelPreview`: structured messages, cards and channel/client selection. Device mapping lives in `channelPreviews/clientProfiles.ts`.
- `EmailClientFrame`: email/H5 preview with iOS, Android and PC selection.
- `WebTemplateDrawer`: native Web material editing using the existing rich text editor; saves `web.content`, `web.html` and `web.plain_text`. Native channels must not be sent to the generic message editor.
- `PhonePreview`: compact email inbox summary.
- Email builder preview panel and MJML code editor: `EmailPreview` with their existing viewport controls.

Content renderers must not nest a second device inside a desktop frame. Keep long content scrollable inside the screen; do not truncate the underlying message. These are client simulations, not proprietary client renderers. Web templates display their saved `web.html`, with a fallback to rendering stored Tiptap JSON when the HTML cache is absent; Email and message channels retain their server compile/preview requests.
