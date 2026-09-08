# 客户管理、设置布局与文件导入验收

## 实现

- 去掉联系人页和导入客户面板的重复标题。
- 联系人总数放到联系人标签右侧；导入 / 导出、批量更新、添加通过 Tabs 右侧操作区呈现。
- 设置页移除 700px 最大宽度，内容随可用宽度伸缩，宽表格保留内部横向滚动。
- 两个导入入口均接受 CSV、XLSX、XLS，提供字段对应的 CSV 和 Excel 空模板。
- Excel 读取第一个工作表，以字符串保留格式化数据；用户提示将编号、电话设为文本。
- 后台导入入口将 Excel 转成 CSV，保留原文件名；CSV 原字节传输，保留既有错误行记录行为。
- 后台 CSV 解析接受空的可选标识及 UTF-8 BOM，拒绝重复、缺失标识的表头和没有数据的文件。

## 自动验证

- Vitest：contactImportFile、CustomerImportPanel、ContactsPage，共 30 项通过；操作栏迁移后单独重跑 ContactsPage 的 10 项通过。
- Go：`go test -p 1 ./internal/service -run TestImportJob -count=1` 通过。
- TypeScript build、修改文件 ESLint、git diff --check 通过。
- 生产构建通过，保留项目已有的大 chunk 提示；Excel 库单独动态加载。

## 浏览器验证

使用本地运行页面与隔离 API fixtures；浏览器验证请求内容及 UI，数据库持久化未在这些浏览器步骤中执行。后台 staging 和 worker 映射另由 Go 测试验证。

- 客户页在 1440、1024px 下，联系人数量与标签相邻，操作按钮与标签垂直居中且靠右。
- 设置页在 1920、1440、1024px 下，可用内容宽度分别为 1405、925、509px；内容容器等宽，没有整页横向溢出。
- 设置搜索无结果状态正常显示。
- 点击下载两种后台客户导入模板，填写后实际选择 CSV、XLSX、XLS 文件上传。请求保留 `000123456789012345678` 和空的可选标识；导入历史保留各自扩展名。
- 联系人菜单可正常打开导入抽屉，两种模板均可下载。下载的 Excel 模板填写后自动映射邮箱、外部 ID、中文姓名、电话；实际提交请求精确保留字段，显示 1 成功、0 失败。
- 联系人抽屉沿用原有静态 Ant Design message，成功提示时有已有的上下文警告，不影响导入结果。

## 截图

- `customer-contacts-1440.png`、`customer-contacts-1024.png`
- `customer-import-form.png`、`customer-contact-import-form.png`
- `customer-settings-1920.png`、`customer-settings-1440.png`、`customer-settings-1024.png`、`customer-settings-empty.png`

测试数据及模板仅位于 output/playwright，不含真实客户数据。
