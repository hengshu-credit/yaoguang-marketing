# Mobile In-App marketing preview

- Notification: top-aligned banner with sender, thumbnail, readable text and a full-width action.
- Rich card: large media area, stronger title hierarchy, footer and primary/secondary touch targets.
- Carousel: separate cards with a neighboring-card preview, horizontal snapping, page buttons, count, RTL and keyboard support. No automatic rotation.
- Mobile styles apply to In-App iOS/Android previews. Desktop, native Push and chat-channel renderers retain their existing surfaces.

Validation: 17 preview tests; TypeScript and scoped ESLint; production Vite build; real Chrome rendering of the production OmnichannelPreview component in an isolated fixture.

Browser checks cover direct navigation to card 3, Home navigation, RTL scrolling, Android and iOS, 375px long text with failed images, no page/device horizontal overflow, reachable actions after vertical scrolling, and desktop isolation. Removed `scroll-snap-stop: always` after browser verification exposed skipped multi-page navigation.

Screenshots: `mobile-marketing-ios.png`, `mobile-marketing-android.png`, `mobile-marketing-rtl.png`, `mobile-marketing-long-375.png`, `mobile-marketing-long-end.png`, `mobile-marketing-desktop-check.png`.

The fixture uses synthetic artwork/content and does not send messages. To reproduce, temporarily copy `mobile-marketing-review.html` and `.tsx` to the console root as `mobile-preview-review.html` and `.tsx`, open `/console/mobile-preview-review.html` on the dev server, and run `mobile-marketing-check.js` through playwright-cli. The temporary console entry points were removed after verification.
