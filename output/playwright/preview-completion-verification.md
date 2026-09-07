# Material preview continuation — 2026-09-07

Continued the shared task `https://chatgpt.com/s/cx_6a9e4ac2a2c88191ae7dbf9bb1778e3f` from local commit `370adb62`. The earlier email metadata, native Web editor, channel profile mapping and shared device renderer changes were already present in the checkout.

## Additional defect fixed

The visual email preview's horizontal Ant Design Splitter can assign its entire width to the details panel at phone widths. In the real browser, a 375.33px drawer contained a phone only 17.33px wide. The shared CSS now stacks the preview above the details/code panel below 768px and overrides the splitter's inline flex sizes. The same measured drawer now contains a 320px phone. Desktop keeps its resizable columns.

The MJML editor uses the same layout, with a 360px scrollable code panel below the preview. Its toolbar wraps at narrow widths, and the email drawer's step tabs stay in normal document flow rather than overlapping the header. The code splitter now uses `defaultSize`, allowing its internal resize state to work normally.

## Browser acceptance

Used the signed-in local application at `http://localhost:8081/console/`, connected to its actual Docker backend. No response interception or fixture server was used. No template was saved and no message was sent.

- Saved email: sender name/address, subject, summary and compiled body appeared together in the shared client preview. The desktop iframe retained its 1024px layout width and empty sandbox.
- Visual editor: changing the settings subject to `预览验收 {{ "Ada" }}` produced `预览验收 Ada`, together with the new summary and original body from the real compiler.
- Visual editor at 375.33 CSS px: confirmed the repaired 320px phone, readable header/body, scrollable content and separate drawer step tabs.
- MJML code mode: settings subject `代码模式 {{ "Ada" }}` rendered as `代码模式 Ada`; configured sender, summary and compiled starter body were visible. At phone width both stacked panels measured 355.33px, with equal client/scroll widths (355px), so the editor did not overflow horizontally.
- Native Web: typed draft content appeared inside both the H5 and desktop iframe, with the H5 layout width at 375px and the sandbox preserved.
- Telegram: draft text and desktop profile switching were observed. The session reloaded during build/catalog compilation before a final server-rendered chat snapshot was captured; automated tests cover the server preview request and response flow.
- The historical sender override is covered by `TemplatePreviewDrawer.test.tsx`. No completed real-browser historical-message preview was captured in this continuation, so the prior historical browser evidence was not treated as a fresh result.
- Temporary viewport overrides were reset.

## Verification

- 88 tests passed in 12 suites, covering shared HTML/email previews, saved previews, both email editors, native Web, SMS/Push and generic channel drawers/renderers/profile mappings. Log: `preview-completion-tests.log`.
- `npx tsc -b` passed.
- Targeted ESLint passed with `--max-warnings 0` for the touched TSX files.
- `npm run build` passed, including Lingui compilation, TypeScript and Vite. Log: `preview-completion-build.log`. Vite retains its bundle-size warning.
- `git diff --check` passed.
- Independent CodeRabbit review was attempted but the service exited with `Connection failed: WebSocket closed`; no successful external review is claimed.

No user-facing strings were added; the existing Lingui metadata labels and catalogs remain in use. No commit was created.
