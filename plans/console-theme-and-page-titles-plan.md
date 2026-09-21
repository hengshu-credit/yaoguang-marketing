# 瑶光控制台主题与标题统一

基线：`3473c7b`，当前 `main`；工作区起始干净。不新建分支、不提交。

## 参考与适配

参考本机天枢 `theme/themeConfig.js`、`theme/themeRuntime.js`、`ThemeSettingsDrawer.vue` 和布局标题代码中的明暗/预设/自定义渐变、导航风格、内容宽度及预览/取消/保存交互。瑶光仍采用 React + Ant Design，不引入 Vue/Element Plus 或多页签导航。

瑶光已有 Workspace 品牌、字体和 owner 编辑权限，因此扩展 `settings.console_theme` 保存主题，沿用设置 JSON，不新增表/迁移。默认保留瑶光紫色和明亮导航；增加明亮/深色/跟随系统、预设/纯色/渐变、导航风格、内容宽度、密度和标题风格。危险/成功等语义颜色保持可辨认，不随主题任意互换。

主题入口放在账号菜单和控制台设置提示中。选择时预览，取消/关闭/切换工作空间恢复已保存配置；保存只提交主题字段，不覆盖邮件、字体或其他设置。非 owner 可查看，但不能保存到共享 Workspace。

统一标题视觉层级、说明和操作行，补齐分析/客户页面的主标题，移除重复页边距。浏览器标题同步当前页面和 Workspace；保留 h1 语义、动态详情标题及多语言。主题仅用于控制台框架，不影响素材正文、客户预览、博客和通知中心。

## 实现与回归

- `internal/domain/console_theme.go`、workspace JSON、独立主题 service/handler/repository；`workspaces.setConsoleTheme` 原子更新主题，常规整行更新保留数据库中的新主题。对应 domain/service/http/repository 测试校验枚举、颜色、未知字段、默认/重置/部分更新和持久化。
- `console/src/theme/` 配置/上下文与主题抽屉；对应 Vitest 检查安全色值、系统偏好监听、预览回滚、工作空间隔离、保存失败保留、只读权限。
- App/WorkspaceLayout、统一标题/页头与主题 CSS；对应 App/layout/标题/页面回归，保护原字体和业务交互。
- Lingui 新文案覆盖英文/中文，其他语言保留可编辑回退。
- Go 定向及全量单元回归、Console 类型/Lint/构建、相关组件回归、真实浏览器明暗/渐变/保存/刷新/窄屏检查。原始开发数据库不启动、不变更；浏览器使用实际设置处理链和隔离测试存储。

## 完成记录

实现与验证已完成；Go 5,014 个顶层测试和前端相关 111 项通过，真实 PostgreSQL 及生产前端操作验证通过。全量前端剩余已知失败与验证边界见 [主题与标题验收记录](../docs/operations/console-theme-and-titles-2026-09-21.md)。
