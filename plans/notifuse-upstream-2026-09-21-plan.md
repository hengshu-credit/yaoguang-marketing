# Notifuse 增量评估与瑶光适配

日期：2026-09-21。瑶光基线：`3825888`，当前 `main`，不新建分支、不提交。

上游范围：`63d70e5d667fe89381d439e5fd4016038ae3b73b`（不含）至 `d7870faef7988bfef77e5cc95a3eb93b14aa3e8d`（含），25 个提交。GitHub compare API 与固定 SHA 源码归档核查；上游最新分支不作为可重复构建依据。

## 边界

- 已有权限、API Key、Webhook 多实例租约、字段保留和自引荐修复不重复移植。
- 直接移植仅取确认仍为 AGPL 的 `1e73290eb0027dc26dbf27a8f2c45aeba391f4ef` 及更早内容。上游 v40/v41 核心已改 BSL；不复制其实现、授权模块或商业门禁。后续通用问题依据行为描述，在瑶光既有代码上独立修复。
- Customer UUID、执行快照、Delivery Intent、unknown 对账、四层频控、多渠道和中文界面保持现有设计。不得用上游邮件队列重试按钮替代瑶光投递账本。
- 不启动原开发数据库，不向真实 Provider 发信；使用测试容器、回环夹具和已有回归。

## 实施和回归对应

1. 消息日志与 API：`MessageHistoryTable.tsx`、`TemplatePreviewDrawer.tsx`、`transactional_handler.go`；相邻测试验证任意 JSON 元数据、缺失/清除数据、列开关、真实 `status_info`、配置错误与瞬时故障状态码。Zapier 仅调整已有权限授权及说明，不另建发送链路。
2. SMTP：`smtp_service.go`、`pkg/emailerror/smtp.go`；对应 SMTP 协议夹具和分类测试，保留 Provider 原因、区分收件人与发送方/系统拒绝、临时错误可重试。
3. Webhook：在现有安全 HTTP 客户端上统一默认出站策略；`webhook_subscription_service.go`、`webhook_delivery_worker.go` 及瑶光渠道调用点；测试阻断本地/私网、DNS/重定向、显式内网部署开关、不误停订阅，以及保留签名和租约。
4. 请求关联与登录：HTTP middleware 和 `SignInPage.tsx`；测试合法/非法请求 ID、CORS、错误响应及验证码自动填充。
5. AI 编辑器：检查现有 `ai-assistant` continuation、模板用途和工具执行链；只补实际缺口，对应 hook/prompt 测试，尊重自定义营销/事务分类。
6. 安全依赖：根据实际安装版本与包管理器核验，只升级相关补丁，运行受影响模块测试及构建，不回退瑶光 SDK 版本或改许可证。
7. 记录逐提交取舍、适配点及实际验证结果；运行 Go 相关层、Console Vitest/TypeScript/Lingui/ESLint、SDK 和 `git diff --check`。完整套件与定向回归结果分别报告。
