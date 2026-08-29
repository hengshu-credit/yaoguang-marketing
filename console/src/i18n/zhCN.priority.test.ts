import { describe, expect, it } from 'vitest'
import zhCatalogSource from './locales/zh-CN.po?raw'
import { parsePOCatalog } from './po'

const priorityTranslations: Record<string, string> = {
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
  'Danger Zone': '危险区域'
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
})
