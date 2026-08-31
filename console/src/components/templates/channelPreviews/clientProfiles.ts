export type PreviewRendererKind = 'phone' | 'desktop' | 'notification' | 'enterprise' | 'http' | 'unknown'

const profileKinds: Record<string, Exclude<PreviewRendererKind, 'unknown'>> = {
  sms_ios: 'phone', sms_android: 'phone', google_messages_rcs: 'phone',
  ios_lock_screen: 'notification', android_notification: 'notification', web_notification: 'notification',
  huawei_notification: 'notification', honor_notification: 'notification', xiaomi_notification: 'notification',
  oppo_notification: 'notification', vivo_notification: 'notification',
  in_app_ios: 'notification', in_app_android: 'notification', in_app_web: 'notification',
  http_request: 'http',
  wechat_mobile: 'phone', wechat_mini_program: 'phone',
  wecom_mobile: 'enterprise', wecom_desktop: 'enterprise',
  dingtalk_mobile: 'enterprise', dingtalk_desktop: 'enterprise',
  feishu_mobile: 'enterprise', feishu_desktop: 'enterprise',
  whatsapp_ios: 'phone', whatsapp_android: 'phone', whatsapp_web: 'desktop',
  telegram_mobile: 'phone', telegram_desktop: 'desktop',
  line_mobile: 'phone', line_desktop: 'desktop', zalo_mobile: 'phone',
  viber_mobile: 'phone', viber_desktop: 'desktop',
  messenger_mobile: 'phone', messenger_web: 'desktop',
  instagram_mobile: 'phone', instagram_web: 'desktop', kakao_mobile: 'phone'
}

export const previewRendererKind = (profileId: string): PreviewRendererKind =>
  profileKinds[profileId] || 'unknown'

