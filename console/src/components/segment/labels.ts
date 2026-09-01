import { msg } from '@lingui/core/macro'
import { i18n } from '../../i18n'

const zhSourceLabels: Record<string, string> = {
  contacts: '联系人属性',
  contact_lists: '订阅列表',
  contact_timeline: '活动',
  custom_events_goals: '自定义事件目标'
}

const zhFieldLabels: Record<string, string> = {
  profile_status: '客户档案状态',
  profile_tags: '客户档案标签',
  profile_attributes: '客户档案属性',
  max_scroll: '最大滚动深度',
  os: '操作系统'
}

const zhOperatorLabels: Record<string, string> = {
  is_set: '已设置',
  is_not_set: '未设置',
  equals: '等于',
  not_equals: '不等于',
  contains: '包含',
  not_contains: '不包含',
  gt: '大于',
  lt: '小于',
  gte: '大于等于',
  lte: '小于等于',
  before_date: '早于日期',
  after_date: '晚于日期',
  in_date_range: '在日期范围内',
  not_in_date_range: '不在日期范围内',
  in_the_last_days: '在最近天数内',
  not_in_the_last_days: '不在最近天数内',
  in_array: '在数组中'
}

export const segmentSourceLabel = (source: string, fallback: string): string => {
  if (i18n.locale === 'zh-CN' && zhSourceLabels[source]) return zhSourceLabels[source]
  switch (source) {
    case 'contacts': return i18n._(msg`Contact property`)
    case 'contact_lists': return i18n._(msg`List subscription`)
    case 'contact_timeline': return i18n._(msg`Activity`)
    case 'custom_events_goals': return i18n._(msg`Custom Events Goal`)
    default: return fallback
  }
}

export const segmentFieldLabel = (field: string, fallback: string): string => {
  if (i18n.locale === 'zh-CN' && zhFieldLabels[field]) return zhFieldLabels[field]
  switch (field) {
    case 'email': return i18n._(msg`Email`)
    case 'external_id': return i18n._(msg`External ID`)
    case 'first_name': return i18n._(msg`First Name`)
    case 'last_name': return i18n._(msg`Last Name`)
    case 'phone': return i18n._(msg`Phone`)
    case 'country': return i18n._(msg`Country`)
    case 'language': return i18n._(msg`Language`)
    case 'timezone': return i18n._(msg`Timezone`)
    case 'address_line_1': return i18n._(msg`Address Line 1`)
    case 'address_line_2': return i18n._(msg`Address Line 2`)
    case 'postcode': return i18n._(msg`Postcode`)
    case 'state': return i18n._(msg`State`)
    case 'job_title': return i18n._(msg`Job Title`)
    case 'profile_status': return i18n._(msg`Profile Status`)
    case 'profile_tags': return i18n._(msg`Profile Tag`)
    case 'profile_attributes': return i18n._(msg`Profile Attribute`)
    case 'custom_string_1': return i18n._(msg`Custom String 1`)
    case 'custom_string_2': return i18n._(msg`Custom String 2`)
    case 'custom_string_3': return i18n._(msg`Custom String 3`)
    case 'custom_string_4': return i18n._(msg`Custom String 4`)
    case 'custom_string_5': return i18n._(msg`Custom String 5`)
    case 'custom_number_1': return i18n._(msg`Custom Number 1`)
    case 'custom_number_2': return i18n._(msg`Custom Number 2`)
    case 'custom_number_3': return i18n._(msg`Custom Number 3`)
    case 'custom_number_4': return i18n._(msg`Custom Number 4`)
    case 'custom_number_5': return i18n._(msg`Custom Number 5`)
    case 'custom_datetime_1': return i18n._(msg`Custom Date 1`)
    case 'custom_datetime_2': return i18n._(msg`Custom Date 2`)
    case 'custom_datetime_3': return i18n._(msg`Custom Date 3`)
    case 'custom_datetime_4': return i18n._(msg`Custom Date 4`)
    case 'custom_datetime_5': return i18n._(msg`Custom Date 5`)
    case 'created_at': return i18n._(msg`Created At`)
    case 'updated_at': return i18n._(msg`Updated At`)
    case 'custom_json_1': return i18n._(msg`Custom JSON 1`)
    case 'custom_json_2': return i18n._(msg`Custom JSON 2`)
    case 'custom_json_3': return i18n._(msg`Custom JSON 3`)
    case 'custom_json_4': return i18n._(msg`Custom JSON 4`)
    case 'custom_json_5': return i18n._(msg`Custom JSON 5`)
    case 'path': return i18n._(msg`Path`)
    case 'duration_ms': return i18n._(msg`Time on page (ms)`)
    case 'max_scroll': return i18n._(msg`Maximum scroll depth`)
    case 'page_number': return i18n._(msg`Page number`)
    case 'entry_type': return i18n._(msg`Entry type`)
    case 'session_id': return i18n._(msg`Visit ID`)
    case 'landing_domain': return i18n._(msg`Domain`)
    case 'landing_path': return i18n._(msg`Entry page`)
    case 'exit_path': return i18n._(msg`Exit page`)
    case 'pageview_count': return i18n._(msg`Pages viewed`)
    case 'goal_count': return i18n._(msg`Goals reached`)
    case 'goal_value': return i18n._(msg`Goal value`)
    case 'referrer_domain': return i18n._(msg`Referrer domain`)
    case 'utm_source': return i18n._(msg`UTM source`)
    case 'utm_medium': return i18n._(msg`UTM medium`)
    case 'utm_campaign': return i18n._(msg`UTM campaign`)
    case 'utm_content': return i18n._(msg`UTM content`)
    case 'channel': return i18n._(msg`Channel`)
    case 'channel_group': return i18n._(msg`Channel group`)
    case 'device': return i18n._(msg`Device`)
    case 'browser': return i18n._(msg`Browser`)
    case 'os': return i18n._(msg`Operating system`)
    default: return fallback
  }
}

export const segmentOperatorLabel = (operator: string, fallback: string): string => {
  if (i18n.locale === 'zh-CN' && zhOperatorLabels[operator]) return zhOperatorLabels[operator]
  switch (operator) {
    case 'is_set': return i18n._(msg`is set`)
    case 'is_not_set': return i18n._(msg`is not set`)
    case 'equals': return i18n._(msg`equals`)
    case 'not_equals': return i18n._(msg`doesn't equal`)
    case 'contains': return i18n._(msg`contains`)
    case 'not_contains': return i18n._(msg`doesn't contain`)
    case 'gt': return i18n._(msg`greater than`)
    case 'lt': return i18n._(msg`less than`)
    case 'gte': return i18n._(msg`greater than or equal`)
    case 'lte': return i18n._(msg`less than or equal`)
    case 'before_date': return i18n._(msg`before date`)
    case 'after_date': return i18n._(msg`after date`)
    case 'in_date_range': return i18n._(msg`in date range`)
    case 'not_in_date_range': return i18n._(msg`not in date range`)
    case 'in_the_last_days': return i18n._(msg`in the last`)
    case 'not_in_the_last_days': return i18n._(msg`not in the last`)
    case 'in_array': return i18n._(msg`in array`)
    default: return fallback
  }
}

export const segmentControlLabel = (key: 'select_operator' | 'enter_value' | 'days'): string => {
  if (!i18n.locale) {
    return { select_operator: 'select a value', enter_value: 'enter a value', days: 'days' }[key]
  }
  if (i18n.locale === 'zh-CN') {
    return { select_operator: '选择运算符', enter_value: '输入值', days: '天' }[key]
  }
  switch (key) {
    case 'select_operator': return i18n._(msg`select a value`)
    case 'enter_value': return i18n._(msg`enter a value`)
    case 'days': return i18n._(msg`days`)
  }
}
