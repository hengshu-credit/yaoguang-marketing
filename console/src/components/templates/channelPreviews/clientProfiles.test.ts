import { describe, expect, it } from 'vitest'
import { previewRendererKind } from './clientProfiles'

const expectedProfiles = [
  'sms_ios', 'sms_android', 'ios_lock_screen', 'android_notification', 'web_notification',
  'huawei_notification', 'honor_notification', 'xiaomi_notification', 'oppo_notification', 'vivo_notification',
  'in_app_ios', 'in_app_android', 'in_app_web', 'google_messages_rcs', 'http_request',
  'wechat_mobile', 'wechat_mini_program', 'wecom_mobile', 'wecom_desktop',
  'dingtalk_mobile', 'dingtalk_desktop', 'feishu_mobile', 'feishu_desktop',
  'whatsapp_ios', 'whatsapp_android', 'whatsapp_web', 'telegram_mobile', 'telegram_desktop',
  'line_mobile', 'line_desktop', 'zalo_mobile', 'viber_mobile', 'viber_desktop',
  'messenger_mobile', 'messenger_web', 'instagram_mobile', 'instagram_web', 'kakao_mobile'
]

describe('previewRendererKind', () => {
  it('routes every built-in non-email profile to a renderer', () => {
    for (const profile of expectedProfiles) {
      expect(previewRendererKind(profile), profile).not.toBe('unknown')
    }
  })

  it('does not silently route an unknown profile', () => {
    expect(previewRendererKind('future_client')).toBe('unknown')
  })
})

