# Preview repair verification — 2026-09-07

This round used the signed-in Chrome page at `http://localhost:8081/console/workspace/hscredit/templates`, connected to the real Docker backend via the frontend gateway. API preview responses were not intercepted or mocked in browser acceptance. No templates or messages were submitted through the browser.

## Confirmed defects and fixes

- Native Web was incorrectly routed through the generic message editor, resulting in an unsupported `web_page` renderer and the wrong save payload. It now has a native rich text editor and uses `web.content/html/plain_text`.
- Switching from Web to Telegram retained Web form values and preview state. Channel-specific editor keys and profile resolution now keep the form and renderer aligned.
- Existing generic templates could initialize against the first catalogue entry (Email). They now initialize against their own channel, retain hidden action/card data and reject stale requests immediately after draft changes.
- Email compilation sent redundant requests and could display an old result. A single debounced, generation-checked compilation path now owns the visual editor preview.
- Desktop HTML rendered at the width of its containing panel. It now has a 1024px layout viewport and scales to fit; mobile uses 375px/393px viewports.
- The compact inbox preview collapsed to a vertical strip because inline-size containment has no intrinsic width in a flex parent. Before the fix, the stage measured 41.33px and the phone 17.33px within a 1236px parent. After the fix, the stage measured 402px and the phone 360px. At a 375px browser viewport, the stage measured 296px and the phone 274.67px, with equal screen client/scroll widths (257px). Settings and translation forms stack the preview on narrow screens.

## Validation

- 40 frontend tests passed across 8 relevant suites: see `preview-repair-tests.log`.
- Backend domain/service template validation and preview tests passed: see `preview-repair-backend-tests.log`.
- TypeScript and targeted ESLint passed; production Vite build passed. See the corresponding `preview-repair-*.log` files.
- Browser: Web draft text rendered in H5 and PC; PC iframe measured a 1024px layout width inside a 463px panel.
- Browser: Telegram replaced a Liquid variable using the real server renderer and switched to desktop; WhatsApp then opened with fresh form/profile state and rendered through its real preview endpoint.
- Browser: Email visual and MJML code modes both produced actual compiled content. Code preview verified 1024px desktop and 375px mobile layout widths with the sandbox retained.
- Browser: the exact Email creation settings-page inbox preview was visually inspected and measured after the compact-width repair, at desktop and 375px sizes. Browser viewport overrides were reset.

The referenced design discussion informed the separation of content rendering from device chrome and the fixed layout viewport approach. No cross-project or cross-framework migration was included.
