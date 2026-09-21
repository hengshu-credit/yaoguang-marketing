# 事件上报配置与交互优化

日期：2026-09-21。当前工作区已有上游适配变更，保留不动；不创建分支，不提交。

## 范围

沿用 `businessDefinitions`、`events.track`、Customer UUID 和既有浏览器凭证协议，不新增事件入口、不改变幂等或历史事实语义。配置优化针对自定义事件及字段映射；内置事件详情保持只读生产者说明。

1. 独立上报配置模型：界面与服务端对齐有效期缺省（旧配置为 required）、时长上限、浏览器/可信后端互斥、精确 Origin、对象路径及可选映射路径。
2. 事件配置分组，时长以秒/分钟/小时/天输入但保存原秒值；显示后端来源及浏览器身份规则。自定义映射支持固定事件选择、可选路径清空和原始/规范化路径边界说明。
3. 生成与字段、对象和过期策略一致的样例，提供真实 `events.track` 请求外壳与 SDK 接入说明，明确草稿/未保存状态。生成器不覆盖手写样例、不发送事件。
4. 校验按当前配置和样例快照生效，编辑/关闭后忽略旧响应；保存防重和禁改，关闭提示未保存内容，错误定位到配置/样例。
5. 扩展事件字段默认不选中所有事件，显式选择目标、提示冲突，不覆盖已有输入中的示例字段，生成匹配当前字段的 data 示例。

## 实现与测试

- `event-reporting.ts` / `event-reporting.test.ts`：配置校验、秒值换算、嵌套样例、映射边界与接入请求。
- `EventReportingSettings.tsx`、`BusinessDefinitionSettings.tsx` / 对应组件测试：互斥模式、单位保值、可选映射清空与错误反馈。
- `BusinessDefinitionEditor.tsx` / `BusinessDefinitionEditor.test.tsx`：样例、迟到响应、只读、草稿与激活、关闭保护、保存失败留存。
- `EventFieldsExtensionDrawer.tsx` / `ConfigurationConsolidation.test.tsx`：显式多选、字段冲突、当前样例和提交状态。
- Go domain 配置校验 / domain 测试，service 预览与引用校验 / service 测试，HTTP 保存与预览回归。
- 新界面文案进入 Lingui，英文与简体中文完整，其他支持语言提供可编辑回退；保留前一轮目录修改。

## 验证

运行相应 Go domain/service/http 测试、Console 定向 Vitest、TypeScript、ESLint、Lingui 和生产构建；使用 Playwright 检查真实页面的填写、纠错、样例、关闭保护与窄屏。区分 UI 夹具、后端 HTTP 回归和真实数据库验收；不接触原开发数据库，不发送真实事件/消息。

## 实施结果

已完成上述实现与针对性验证。最终 Go 全量单元测试 5,010 个顶层测试通过；事件配置相关 Console 55 项通过，类型、Lint、Lingui 和生产构建通过。Playwright 连接实际保存/预览 handler 与内存 repository，完成保存重开、纠错、单位保值、样例校验、关闭保护和 390px 窄屏检查。验证边界及证据见 [验收记录](../docs/operations/event-reporting-configuration-2026-09-21.md)。
