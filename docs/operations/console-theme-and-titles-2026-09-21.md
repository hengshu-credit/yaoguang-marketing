# 控制台主题与页面标题

本轮基线 `3473c7b`。参考同机天枢决策引擎的 `themeConfig.js`、`themeRuntime.js`、`ThemeSettingsDrawer.vue` 和布局标题实现，适配为瑶光的 React/Ant Design 控制台。没有修改天枢仓库。

## 使用方式

账户菜单 → **主题设置**：选择明亮、深色或跟随系统；选择瑶光紫等预设、纯色或二/三色线性/径向渐变；调整导航栏风格、内容宽度、界面密度与标题风格。

- 即时预览不写数据库。面板不加遮罩，直接观察页面变化；取消或关闭恢复原主题。
- **保存主题**应用到当前 Workspace，需要 owner 权限。与天枢的个人偏好不同，这里沿用瑶光既有 Workspace 品牌/字体配置边界，其他工作空间不受影响。
- 主题保存失败会保留草稿和错误，允许重试；切换工作空间、退出页面会停止预览，迟到响应不能覆盖新空间。
- **恢复默认主题**只修改预览草稿，仍须保存才能成为共享设置。
- 字体继续在“设置 → 常规”选择或上传，不因主题色重置。用户的素材、邮件正文、设备模拟预览、博客和通知中心不是主题配置目标。

## 标题与阅读体验

所有使用共享标题的页面采用相同字重、行高、颜色和换行规则：强调/简洁标题 22px，紧凑标题 18px；强调标题使用主题色装饰条，保留 `h1` 语义。补齐客户和分析页面的主标题，统一分析标题/描述/操作区，移除列表、日志、分析及内容页的重复外层留白。

浏览器标题格式：`页面名称 · 工作空间名称 · 瑶光营销平台`，跟随当前语言和实际标题变化；详情页保留自己的业务名称。没有照搬天枢的多页签导航、顶部菜单位置或编辑器状态缓存。

明暗主题同时驱动 Ant Design 组件和控制台背景/文字/边框；深色图表适配坐标轴/图例，保留原数据、系列颜色和显式主题。危险、成功、警告等语义色不随主题任意互换。自定义浅色按钮自动选择可读前景；极端明暗渐变不能保证全文对比度时，按钮使用可读纯色，装饰和预览保留渐变。

## 保存协议与兼容

`POST /api/workspaces.setConsoleTheme`：

```json
{
  "workspace_id": "your-workspace",
  "console_theme": {
    "version": 1,
    "color_scheme": "dark",
    "accent_mode": "preset",
    "accent_preset": "nebula",
    "primary_color": "#7763F1",
    "gradient_colors": ["#2639E9", "#873FF2"],
    "gradient_type": "linear",
    "gradient_angle": 135,
    "sidebar_style": "dark",
    "content_width": "fluid",
    "density": "comfortable",
    "title_style": "accent"
  }
}
```

返回规范化后的 `console_theme`。读取通过现有 Workspace 响应的 `settings.console_theme`。空对象重置默认，缺失/null 对象拒绝；枚举、颜色、角度和未知字段由服务端校验，拒绝任意 CSS/URL。零度角是显式有效值，不被默认 135 度覆盖。

主题保存在现有 settings JSON 中，不新增表或数据库版本。独立的 JSONB 原子写入只改主题；通用 Workspace 整行更新保留数据库当前主题，防止旧表单或后台任务将新主题覆盖。不要通过 `workspaces.update` 修改主题，使用专用接口。

原来没有配置主题的空间使用浅色、瑶光紫、浅色导航、自适应宽度。未知版本/损坏的前端主题退回默认，不执行其中的 CSS。

## 验证记录

使用独立 PostgreSQL 测试库检查真实 JSONB 持久化及旧表单不覆盖新主题，没有启动/迁移原开发数据库。浏览器使用生产前端与真实主题 handler/service/repository，认证身份与业务列表是隔离夹具；主题保存和刷新不是 API mock。

验证结果：

- Go `./internal/... ./pkg/... ./config/... -count=1`：5,014 个顶层测试通过；需要外部服务的既有条件跳过仍存在。最后的零度角序列化改动另行通过 domain/service/http/repository 定向复验。
- 真实 PostgreSQL `TestConsoleThemePostgresPreservesConcurrentSettings`：通过。主题更新保持字体；随后的旧 Workspace 更新保持新主题；另一 Workspace 未产生主题字段。
- Console 主题/标题/布局/设置/图表/页面烟测最终定向：111 项通过。覆盖取消、保存失败、晚到响应、只读、StrictMode、跟随系统和监听清理。
- Console TypeScript、Lingui compile、ESLint、生产构建、OpenAPI JSON 解析及 `git diff --check` 通过。保留既有构建大包提醒。
- 全量 Console 首轮：2,088 项通过、15 项失败。三项是本次统一标题后仍要求“没有标题”或旧 Contacts 标题的断言，已更新并在上述 111 项中复验；其余 12 项为既有翻译目录问题 9 项、flowConverter 的 Lingui `t` Mock 缺失 3 项。未将结果描述为全量前端全绿，未删测试或放宽业务断言掩盖失败。
- 后续修复：上述 12 项既有翻译/Mock 问题现已解决，重新执行全量 Console 后 201 个测试文件、2,109 项全部通过（含新增回归）。原始首轮结果保留，详见 [翻译与 Mock 修复验收](frontend-regressions-2026-09-21.md)。
- 浏览器生产页面：明暗/渐变/导航实时预览、18px 紧凑标题与取消恢复 22px、真实保存后刷新、字体保留、跨 Workspace 恢复、客户/分析标题、390px 窄屏操作通过。浏览器控制台无错误或警告。

参考截图：`page-2026-09-21T08-03-33-038Z.png`（浅色标题）、`page-2026-09-21T08-16-03-621Z.png`（保存刷新后的深色）、`page-2026-09-21T08-17-04-608Z.png`（客户页）、`page-2026-09-21T08-18-18-739Z.png`（窄屏）。截图、日志和夹具保留在 `output/playwright/console-theme/.playwright-cli/`（不入版本库）。

未修改原开发数据库、未发送消息，未新建分支或提交 commit。验证结束后停止本轮本地服务与测试容器。
