# Material preview verification

Verified on 2026-09-06 against the local Vite frontend at http://127.0.0.1:8081/console/.

Browser checks use explicit intercepted API fixtures from `material-preview-fixtures.js`. No live messages were sent and no material was saved. Backend delivery integration was not exercised.

## Checks

- 27 distinct tests passed across ChannelMessagePreview (3), OmnichannelPreview (14), TemplatePreviewDrawer (3), MessageTemplateDrawer (2), OmnichannelTemplateDrawer (3), and HtmlPreview (2). See `material-preview-tests.log` and the final changed-file run in `material-preview-final-tests.log`.
- TypeScript project build passed; targeted ESLint checks passed; Lingui catalogs compiled successfully; Vite production build passed. See `material-preview-types.log`, `material-preview-lint.log` and `material-preview-build.log`.
- `git diff --check` passed.
- Material management: opened SMS, Push, Email, Web/H5 and WhatsApp previews; switched among iOS, Android, PC and H5; desktop chat contains one desktop frame; filtered-empty state works.
- Customer timeline: opened the saved email preview and switched to desktop.
- Customer message history: opened the message preview, verified that the compile request retained the message's customer data, and checked iOS/desktop rendering with a sandboxed HTML iframe.
- At 375px and 320px: controls wrap without horizontal overflow. At 320px the drawer bounds are exactly [0, 320]; long message content scrolls inside a 207px-wide viewport without horizontal overflow.
- Loading, failed media and failed preview response states verified. Failed media retains readable text; a failed request removes the previous preview.

## Evidence

- `material-preview-ios.png`, `material-preview-android.png`, `material-preview-pc.png`
- `material-preview-email.png`, `material-preview-h5.png`, `material-preview-chat-pc.png`, `material-preview-sms.png`
- `material-preview-history.png`, `material-preview-message-history.png`
- `material-preview-375.png`, `material-preview-long-320.png`, `material-preview-error.png`
- Executable browser scenarios and result logs are alongside this report.

Implementation entry points and reuse guidance: `console/src/components/preview/README.md`.
