import { describe, expect, it } from 'vitest'
import zhCatalogSource from './locales/zh-CN.po?raw'
import { parsePOCatalog } from './po'

const priorityTranslations: Record<string, string> = {
  Customer: '客户',
  Customers: '客户',
  Audience: '客群',
  Audiences: '客群',
  Campaign: '营销活动',
  Campaigns: '营销活动',
  Journey: '自动化旅程',
  Journeys: '自动化旅程',
  Delivery: '投递',
  Deliveries: '投递',
  Identity: '身份标识',
  Consent: '授权同意',
  Suppression: '触达抑制',
  'Frequency Cap': '频控',
  'System Settings': '系统设置',
  Dashboard: '工作台',
  Templates: '模板',
  Blog: '博客',
  'File Manager': '文件管理',
  Analytics: '营销分析',
  'Web Analytics': '网站分析',
  Live: '实时',
  Explore: '探索',
  Goals: '目标',
  Filters: '筛选器',
  Annotations: '注释',
  Contacts: '联系人',
  Lists: '列表',
  Broadcasts: '邮件群发',
  Automations: '自动化',
  'Transactional Notifications': '事务通知',
  Content: '内容',
  Logs: '日志',
  Settings: '设置',
  'Add dimension': '添加维度',
  'Add filter': '添加筛选条件',
  Sessions: '会话数',
  'Median TimeScore': 'TimeScore 中位数',
  'Bounce Rate': '跳出率',
  'Median Scroll Depth': '滚动深度中位数',
  'Previous 7 days': '过去 7 天',
  'Previous period': '上一周期',
  Dimension: '维度',
  'Referrer domain': '引荐来源域名',
  'UTM Source': 'UTM 来源',
  'UTM Medium': 'UTM 媒介',
  'UTM Campaign': 'UTM 活动',
  'Create New List': '新建列表',
  'Create List': '创建列表',
  Name: '名称',
  'List ID': '列表 ID',
  Description: '描述',
  Public: '公开',
  'Double Opt-in': '双重确认订阅',
  'Enter list name': '输入列表名称',
  'Enter a unique alphanumeric ID': '输入唯一的字母数字 ID',
  'Enter list description': '输入列表描述',
  'Create a broadcast': '创建群发',
  'Create Broadcast': '创建群发',
  'Broadcast name': '群发名称',
  '1. Audience': '1. 受众',
  '2. Web Analytics': '2. 网站分析',
  '3. Data Feeds': '3. 数据源',
  '4. Content': '4. 内容',
  'Select a list': '选择列表',
  'Belonging to at least one of the following segments': '属于以下至少一个细分群体',
  'Exclude unsubscribed, bounced & complained recipients': '排除已退订、退信及投诉的收件人',
  'No broadcasts found': '未找到群发',
  'Create your first broadcast to get started': '创建第一个群发以开始使用',
  'Create Automation': '创建自动化',
  'Enter automation name': '输入自动化名称',
  'Exit on reply': '收到回复时退出',
  Nodes: '节点',
  Delay: '延迟',
  Email: '邮件',
  SMS: '短信',
  Push: '推送',
  Filter: '筛选',
  'A/B Test': 'A/B 测试',
  'List Status': '列表状态',
  'Add to List': '添加到列表',
  'Remove from List': '从列表移除',
  Webhook: 'Webhook',
  Trigger: '触发器',
  Configure: '配置',
  'Trigger Event': '触发事件',
  'Select an event...': '选择事件...',
  'Once per contact': '每个联系人一次',
  'Every time': '每次',
  'Add entry conditions': '添加进入条件',
  'Create a notification': '创建通知',
  'Create Notification': '创建通知',
  'Notification name': '通知名称',
  'E.g. Password Reset Email': '例如：密码重置邮件',
  'API Identifier': 'API 标识符',
  'E.g. password_reset': '例如：password_reset',
  'Email Template': '邮件模板',
  'Select email template': '选择邮件模板',
  "A brief description of this notification's purpose": '简要描述此通知的用途',
  Tracking: '跟踪',
  'Follow workspace setting': '遵循工作区设置',
  'Define UTM parameters for links in your email for better campaign tracking.':
    '为邮件链接定义 UTM 参数，以便更好地跟踪活动。',
  'No transactional notifications found': '未找到事务通知',
  'Create your first notification to get started': '创建第一条通知以开始使用',
  'Create an email template': '创建邮件模板',
  'Template name': '模板名称',
  'Template ID': '模板 ID',
  Category: '分类',
  'Editor mode': '编辑器模式',
  Visual: '可视化',
  'Code (MJML)': '代码（MJML）',
  Sender: '发件人',
  'Email subject': '邮件主题',
  'Subject preview': '主题预览',
  'Reply to': '回复至',
  'Custom sender (transactional email provider)': '自定义发件人（事务邮件服务商）',
  Template: '模板',
  Next: '下一步',
  Previous: '上一步',
  Save: '保存',
  Cancel: '取消',
  Edit: '编辑',
  Preview: '预览',
  Help: '帮助',
  'Import / Export': '导入 / 导出',
  Undo: '撤销',
  Redo: '重做',
  'Content structure': '内容结构',
  Head: '页头',
  'Default attributes': '默认属性',
  Body: '正文',
  Section: '区块',
  Column: '列',
  Spacer: '间距',
  'Configure {nodeLabel}': '配置 {nodeLabel}',
  '{0} in {1}': '{1} 时间 {0}',
  '{0} of {1}': '{0} / {1}',
  'Select a block to edit its attributes': '选择一个区块以编辑其属性',
  Languages: '语言',
  'Customize the static interface text used throughout this workspace.':
    '自定义此工作区中使用的静态界面文案。',
  'Only workspace owners can manage UI translations.': '只有工作区所有者可以管理界面翻译。',
  'Search translations': '搜索翻译',
  'Enter a translation or use Restore to inherit the default': '输入翻译，或选择“恢复默认”以使用默认值',
  'Restore all translations?': '恢复所有翻译？',
  'All workspace overrides will fall back to the bundled defaults.':
    '所有工作区覆盖项都将恢复为内置默认值。',
  'Restore all overrides': '恢复所有覆盖项',
  'Restore all': '全部恢复',
  'Restore {0} in {1}': '恢复 {1} 中的 {0}',
  'Restore item {displayLabel}': '恢复配置项 {displayLabel}',
  'Restore menu {displayLabel}': '恢复菜单 {displayLabel}',
  'Restore page {displayLabel}': '恢复页面 {displayLabel}',
  'Translation for {0} in {1}': '{1} 中 {0} 的翻译',
  Current: '当前',
  Item: '配置项',
  Override: '覆盖',
  Default: '默认',
  Restore: '恢复默认',
  'No translations found': '未找到翻译',
  'Workspace UI translations': '工作区界面翻译',
  'Save Changes': '保存更改',
  Discard: '放弃更改',
  'Your UI translation changes have not been saved.': '您的界面翻译更改尚未保存。',
  'UI translations saved': '界面翻译已保存',
  'Failed to save UI translations': '保存界面翻译失败',
  'UI translations could not be loaded': '无法加载界面翻译',
  'Reload this page to try again.': '请重新加载此页面后重试。',
  'Translations were saved, but the workspace could not be refreshed':
    '翻译已保存，但无法刷新工作区',
  Team: '团队',
  Integrations: '集成',
  Webhooks: 'Webhook',
  'Custom Fields': '自定义字段',
  'SMTP Bridge': 'SMTP 网关',
  General: '常规',
  'Danger Zone': '危险区域',
  ' · {seconds}s': ' · {seconds} 秒',
  '(empty)': '（空）',
  '{0} of {1} rules match': '{1} 条规则中有 {0} 条匹配',
  '{0}-{1} of {total} automations': '{0}-{1}，共 {total} 个自动化流程',
  '{0}-{1} of {total} broadcasts': '{0}-{1}，共 {total} 个群发任务',
  '{clicks} clicks out of {recipients} recipients':
    '{recipients} 位收件人中共产生 {clicks} 次点击',
  '{hours}h': '{hours} 小时',
  '{hours}h {remainingMinutes}m': '{hours} 小时 {remainingMinutes} 分钟',
  '{liveSessions} live now': '当前有 {liveSessions} 个实时会话',
  '{minutes}m': '{minutes} 分钟',
  '{minutes}m {remainingSeconds}s': '{minutes} 分钟 {remainingSeconds} 秒',
  '{minutes}m {seconds}s': '{minutes} 分钟 {seconds} 秒',
  '{seconds} second': '{seconds} 秒',
  '{totalSeconds}s': '{totalSeconds} 秒',
  'API Token': 'API 令牌',
  'Base64 encoded TLS certificate. Run: cat fullchain.pem | base64 -w 0':
    'Base64 编码的 TLS 证书。运行：cat fullchain.pem | base64 -w 0',
  'Base64 encoded TLS private key. Run: cat privkey.pem | base64 -w 0':
    'Base64 编码的 TLS 私钥。运行：cat privkey.pem | base64 -w 0',
  'Blog styling and SEO settings': '博客样式和 SEO 设置',
  'Broadcast ID': '群发 ID',
  'Delete this Zapier webhook?': '删除此 Zapier Webhook？',
  'Deleting this webhook breaks the Zap that created it. The Zap stays switched on in Zapier and reports no error — it simply stops receiving events.':
    '删除此 Webhook 会使创建它的 Zap 失效。该 Zap 在 Zapier 中仍显示为启用且不会报错，但将停止接收事件。',
  'e.g., My Google Gemini Integration': '例如：我的 Google Gemini 集成',
  'e.g., My OpenAI Integration': '例如：我的 OpenAI 集成',
  'e.g., My Supabase Integration': '例如：我的 Supabase 集成',
  'Enter number...': '输入数字...',
  'Entity ID:': '实体 ID：',
  'Errors ({0} of {1} shown)': '错误（显示 {1} 条中的 {0} 条）',
  'ID:': 'ID：',
  'ID: {0}': 'ID：{0}',
  'ID: {listId}': 'ID：{listId}',
  'ID: {segmentId}': 'ID：{segmentId}',
  'ID: {webhookTemplateId}': 'ID：{webhookTemplateId}',
  Integration: '集成',
  'List ID: {0}': '列表 ID：{0}',
  'Look up a Customer by UUID, Customer number, external user ID or normalized identity':
    '通过 UUID、客户编号、外部用户 ID 或标准化身份查询客户',
  'Message ID': '消息 ID',
  Notes: '备注',
  'Partition {0} of {partitionTotal}': '分区 {0}/{partitionTotal}',
  'Please enter a list ID': '请输入列表 ID',
  'Processing batch {currentBatch} of {totalBatches} ({0} of {1} rows)':
    '正在处理第 {currentBatch}/{totalBatches} 批（第 {0}/{1} 行）',
  'Server Token': '服务器令牌',
  'Subscription ID:': '订阅 ID：',
  'Transactional ID': '事务通知 ID',
  'Turn the blog on or off and change its title, SEO, pagination and feed settings':
    '启用或停用博客，并修改标题、SEO、分页和订阅源设置',
  'Unique clicks ÷ recipients of this variation.': '唯一点击人数 ÷ 此版本的收件人数。',
  'You\'re continuing a previous upload session of "{fileName}". The upload will resume from row {0} of {totalRows}.':
    '您正在继续上一次的“{fileName}”上传任务。上传将从第 {0} 行继续，共 {totalRows} 行。',
  'All conditions must match (AND logic):': '所有条件都必须匹配（AND 逻辑）：',
  'Store city name and Store region/state name are off, so coordinates are stored at country precision (~111km).':
    '由于“存储城市名称”和“存储地区/州名称”均已关闭，坐标将按国家级精度（约 111 公里）存储。',
  'Subscribe imported contacts to lists — required only when the import names lists (also needs Contacts write)':
    '将导入的联系人订阅到列表——仅当导入数据指定了列表时需要（还需要“联系人”写入权限）',
  'Covers more than templates: reusable blocks answer to the same switch, and editing a block edits every template that embeds it. Blocks are kept with the workspace\'s own settings, so their content also comes back from /api/workspaces.get and from /api/workspaces.list, which needs no permission at all — Templates read is not the only way to see block content. And /api/templates.compile only checks this grant for calls arriving over the API; when the workspace compiles a template for itself, nothing is checked.':
    '不仅涉及模板：可复用区块受同一开关控制，编辑区块会修改所有嵌入它的模板。区块保存在工作区自身设置中，因此其内容也会由 /api/workspaces.get 和无需任何权限的 /api/workspaces.list 返回；“模板”读取权限并不是查看区块内容的唯一途径。/api/templates.compile 仅对通过 API 发起的调用检查此授权；工作区为自身编译模板时不会检查。',
  'Your AWS credentials cannot create SES tenants. Grant ses:CreateTenant and ses:CreateTenantResourceAssociation, then save again.':
    '您的 AWS 凭据无法创建 SES 租户。请授予 ses:CreateTenant 和 ses:CreateTenantResourceAssociation 权限，然后再次保存。'
}

const technicalAllowlist = new Set(['Webhook'])

describe('Simplified Chinese priority catalog', () => {
  const entries = parsePOCatalog(zhCatalogSource)
  const translations = new Map(entries.map((entry) => [entry.msgid, entry.msgstr]))

  it.each(Object.entries(priorityTranslations))('translates %s', (source, expected) => {
    expect(translations.get(source), `missing or incorrect translation for ${source}`).toBe(expected)
    if (!technicalAllowlist.has(source)) expect(expected).not.toBe(source)
  })

  it('translates the large majority of English interface copy', () => {
    const eligible = entries.filter(
      ({ msgid }) => /[A-Za-z]/.test(msgid) && !/^(?:https?:\/\/|[A-Z0-9_./:+-]{1,16})$/.test(msgid)
    )
    const translated = eligible.filter(
      ({ msgid, msgstr }) => msgstr !== msgid && /[\u3400-\u9fff]/.test(msgstr)
    )

    expect(translated.length / eligible.length).toBeGreaterThanOrEqual(0.85)
  })

  it('preserves every ICU placeholder', () => {
    const placeholders = (value: string) =>
      [...value.matchAll(/\{([A-Za-z_]\w*|\d+)(?=[},])/g)].map(([, name]) => name).sort()

    for (const entry of entries) {
      expect(placeholders(entry.msgstr), entry.msgid).toEqual(placeholders(entry.msgid))
    }
  })

  it('does not contain corrupted Unicode characters', () => {
    const offenders = entries
      .filter(({ msgstr }) => /[\uE000-\uF8FF\u0400-\u04FF\uFFFD]/.test(msgstr))
      .map(({ msgid, msgstr }) => `${msgid} => ${msgstr}`)

    expect(offenders).toEqual([])
  })

  it.each([
    '页:1',
    '空虚',
    '网瘾',
    '网游',
    '托肯',
    '双子体',
    '一体化',
    'SIO',
    '整合',
    '融合',
    '终点',
    '员额',
    '身份证',
    '单击',
    '交易通知',
    '交易邮件',
    '交易服务商',
    '交易写入',
    '双选入',
    '公共旗帜',
    '散列同步',
    '同位素',
    '邮件机身',
    '独特的点击',
    '数据种子',
    '大楼',
    '竞选',
    '卷轴命令',
    '矩阵索引',
    '对象密钥',
    '服务商证书',
    '联系字段',
    '接触',
    '听众',
    '召回',
    '预切',
    'Webhoks',
    '诱导引擎',
    '今此动词',
    '许可门',
    '网路 ',
    '直播',
    '外传',
    '弹出',
    '赠款',
    '信用',
    '呼叫',
    '点火',
    '名为收件人',
    '重播',
    '暂停播放',
    '外出消息',
    '短消息',
    '无记名',
    '写作',
    '装入',
    '入境',
    '列表博客',
    '列表主题',
    '联系时间线',
    '数据输入',
    '发送者'
  ])('does not contain the machine-translation artifact %s', (artifact) => {
    const offenders = entries
      .filter(({ msgstr }) => msgstr.includes(artifact))
      .map(({ msgid, msgstr }) => `${msgid} => ${msgstr}`)

    expect(offenders).toEqual([])
  })
})
