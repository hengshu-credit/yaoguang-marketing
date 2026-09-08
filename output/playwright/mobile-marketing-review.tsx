import React, { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import OmnichannelPreview from './src/components/templates/OmnichannelPreview'
import type { ChannelTemplateContent } from './src/services/api/template'

i18n.load('en', {})
i18n.activate('en')
const art = (color: string, label: string) => 'data:image/svg+xml,' + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400" viewBox="0 0 600 400"><rect width="600" height="400" fill="${color}"/><circle cx="480" cy="40" r="220" fill="white" opacity=".2"/><circle cx="50" cy="370" r="170" fill="white" opacity=".16"/><path d="M300 100v170M235 160h130M240 155q-40-85 25-45l35 45q95-95 50-20l-50 20" fill="none" stroke="white" stroke-width="12" stroke-linecap="round"/><rect x="218" y="161" width="165" height="120" rx="16" fill="none" stroke="white" stroke-width="12"/><text x="40" y="350" font-family="sans-serif" font-size="23" fill="white">${label}</text></svg>`)
const definition = { id: 'in_app', label_key: '瑶光会员', content_families: ['notification', 'rich_card', 'carousel'] as const, preview_profiles: [{ id: 'in_app_ios', label_key: 'iOS', surface: 'mobile' }, { id: 'in_app_android', label_key: 'Android', surface: 'mobile' }, { id: 'in_app_web', label_key: 'PC', surface: 'desktop' }], delivery_modes: ['signed_webhook'], limits: {} }
const action = { type: 'url' as const, label: '立即领取专属礼遇', value: 'https://example.com/offer' }
const base = { title: '为你准备的秋日礼遇', body: '会员专享好礼已到账。\n即日起至 9 月 30 日，领取你的专属权益。', media: { type: 'image' as const, url: art('#9484d4', 'MEMBER DAYS · 2026'), alt_text: '会员礼遇' }, actions: [action] }
function Review() {
  const [profile, setProfile] = useState('in_app_ios')
  const [edge, setEdge] = useState(false)
  const [rtl, setRTL] = useState(false)
  const message = { ...base, ...(edge ? { title: base.title.repeat(3), body: (base.body + '\nhttps://example.com/' + 'long-path-'.repeat(8) + '\n').repeat(8), media: { ...base.media, url: '/console/nonexistent-review-image.png' } } : {}) }
  const items: ChannelTemplateContent[] = [{ ...message, family: 'notification' }, { ...message, family: 'rich_card', footer: '每位会员限领一次', actions: [action, { ...action, label: '了解活动详情' }] }, { family: 'carousel', cards: [{ ...message }, { ...base, title: '周末焕新计划', body: '探索更多会员专属权益。', media: { ...base.media, url: art('#71969b', 'WEEKEND PICKS') }, actions: [{ ...action, label: '查看周末精选' }] }, { ...base, title: '与你分享好时光', media: { ...base.media, url: art('#bc9179', 'JUST FOR YOU') }, actions: [{ ...action, label: '查看活动详情' }] }] }]
  return <App><main style={{ padding: 24, background: '#fafbfe', minHeight: '100vh', fontFamily: 'sans-serif' }}>
    <div style={{ display: 'flex', gap: 16, marginBottom: 16 }}><button onClick={() => setEdge(!edge)}>Long text / failed image</button><button onClick={() => setRTL(!rtl)}>RTL</button></div>
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(min(100%,340px),1fr))', gap: 20 }}>
      {items.map((content, index) => <section key={content.family}><h2 style={{ fontSize: 18 }}>{['通知横幅', '富卡片', '轮播卡片'][index]}</h2><OmnichannelPreview definition={{ ...definition, content_families: [...definition.content_families] }} preview={{ profile, message: content, direction: rtl ? 'rtl' : 'ltr', payload_bytes: 480, warnings: [] }} onProfileChange={setProfile} /></section>)}
    </div>
  </main></App>
}
createRoot(document.getElementById('root')!).render(<I18nProvider i18n={i18n}><Review /></I18nProvider>)
