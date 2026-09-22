# 前端翻译与 Mock 遗留回归修复

本轮修复 [主题与标题验收](console-theme-and-titles-2026-09-21.md) 中记录的 12 项既有前端失败。实施分支为 `main`，基线为 `3473c7b`，保留实施开始时已有的主题与标题改动。

## 原因与修复

- 8 项语言目录检查失败：法语、西班牙语、德语、加泰罗尼亚语、巴西葡萄牙语、日语、意大利语各缺 429 条，简体中文缺 33 条。本轮补齐共 3,036 条译文，并重新生成 Lingui 编译目录。既有非空译文保持不变；字体名、API 路径和技术标识不强行翻译，不以整批英文回填绕过检查。
- 1 项中文误报：全局禁用“身份证”误伤真实证件业务。仅允许 `National ID` 与 `Identity document uploaded` 两个业务词条，并固定其精确译文；新增正反例，继续拦截消息 ID、会话 ID 的误译及其他机器翻译瑕疵。
- 3 项 flowConverter 失败：公共 `@lingui/core/macro` Mock 缺少 `t`。补齐后复用真实 `i18n._` 运行时，保留原 `msg` 的命名插值行为。新增中文/法语切换、真实节点标签、数字零、空字符串、命名插值、消息描述符和 `msg` 回归。

没有删除或跳过原失败测试，没有修改 flowConverter 的业务行为，也没有改动后端或原开发数据库。

## 验证

- 修复前定向复现：347 项中 335 项通过、12 项失败。
- 修复后同范围及新增回归：4 个测试文件，353 项全部通过。
- 编译目录运行时核对：3,036 条新译文全部通过，逐条确认编译 ID 存在、实际翻译输出正确、插值与源文案一致；8 个目录均无空白译文，相对基线的所有既有非空译文保持一致。
- `npm run lingui:compile`、`npx tsc -b --pretty false`、`npm run lint`、`npm run build` 通过。生产构建仍有既有的 chunk 大于 500 kB 提醒。
- 全量 Console Vitest：201 个测试文件、2,109 项测试全部通过，0 失败、0 跳过。测试于 2026-09-21 16:52 完成，2026-09-22 补录最终报告。
- `git diff --check` 通过。

全量命令（在 `console` 目录执行）：

```powershell
npm exec vitest -- run --maxWorkers=3 --silent --reporter=json --outputFile=../output/playwright/frontend-regressions/.playwright-cli/all-tests.json
```

本轮未执行浏览器端交互验收；上轮主题页面浏览器记录仍见原报告，不能视为本轮翻译的浏览器验收。

本地验证记录位于 `output/playwright/frontend-regressions/.playwright-cli/`（不入版本库）：`baseline.json`、`focused.json`、`all-tests.json`、`all-tests.log`、`compile.log`、`typecheck.log`、`lint.log`、`build.log`、`catalogue-runtime.log`。

本任务未执行 stage、commit 或创建分支。补录报告时仓库已推进至 `e7bbd2d`，修复代码已包含在该提交中；本任务仅补充验收记录，没有执行该提交。
