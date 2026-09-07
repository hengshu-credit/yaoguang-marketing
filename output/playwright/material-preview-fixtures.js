async (page) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
  const now = '2026-09-06T08:00:00Z';
  const workspace = { id: 'preview-review', name: '瑶光素材预览验收', settings: { timezone: 'Asia/Shanghai', default_language: 'zh-CN', languages: ['zh-CN'], email_tracking_enabled: false }, integrations: [], created_at: now, updated_at: now };
  const common = { version: 1, category: 'marketing', category_purpose: 'marketing', created_at: now, updated_at: now };
  const email = { editor_mode: 'code', mjml_source: '<mjml><mj-body><mj-section><mj-column><mj-text>会员专属礼遇</mj-text></mj-column></mj-section></mj-body></mjml>', subject: '秋日会员礼遇 · 专属优惠已到账', subject_preview: '打开查看您的专属礼遇', visual_editor_tree: {}, compiled_preview: '' };
  const html = '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{margin:0;background:#f4f6fa;font:14px/1.8 -apple-system,sans-serif;color:#273349}.hero{padding:40px 24px;background:#e9edf9;text-align:center}h1{font-size:26px;line-height:1.4;margin:14px 0}.body{padding:24px}a{display:block;text-align:center;background:#334fac;border-radius:8px;padding:12px;color:white;text-decoration:none}small{color:#8691a5}</style></head><body><div class="hero"><small>YAOGUANG · MEMBER BENEFITS</small><h1>秋日好礼<br>为您而来</h1><p>一份专属礼遇，开启美好九月。</p></div><div class="body"><b>林女士，您好</b><p>您的会员专属优惠已到账。即日起至 9 月 30 日，前往活动页面即可领取专属权益。</p><a href="https://example.com">查看我的专属礼遇</a><p><small>感谢您一直以来的信任与陪伴。</small></p></div></body></html>';
  const templates = [
    { ...common, id: 'member-push', name: '会员专属礼遇 · 推送', channel: 'push', push: { title: '您的专属礼遇已到账', body: '林女士，秋日会员活动已开启。\n即日起至 9 月 30 日，领取您的专属权益，让每一份期待都有回应。' } },
    { ...common, id: 'member-sms', name: '会员专属礼遇 · 短信', channel: 'sms', sms: { sender_id: '瑶光会员服务', body: '【瑶光】林女士，您的会员专属优惠已到账！9月30日前前往活动页即可领取，感谢您一直以来的信任与陪伴。退订回复T。' } },
    { ...common, id: 'member-email', name: '会员专属礼遇 · 邮件', channel: 'email', email },
    { ...common, id: 'member-h5', name: '会员专属礼遇 · H5', channel: 'web', web: { html } },
    { ...common, id: 'member-whatsapp', name: '会员专属礼遇 · WhatsApp', channel: 'whatsapp', content_schema_version: 1, content: { family: 'rich_card', title: '秋日会员礼遇', body: '林女士，您的专属优惠已到账。\n前往活动页面即可领取，感谢您的信任与陪伴。', footer: '瑶光会员服务', actions: [{ type: 'url', label: '查看我的专属礼遇', value: 'https://example.com' }] } }
  ];
  const channels = [
    { id: 'email', label_key: 'Email', preview_profiles: [{ id: 'email_mobile', label_key: 'iOS', surface: 'mobile' }], content_families: ['text'] },
    { id: 'web', label_key: 'H5', preview_profiles: [], content_families: ['text'] },
    { id: 'sms', label_key: 'SMS', preview_profiles: [{ id: 'sms_ios', label_key: 'iOS', surface: 'mobile' }, { id: 'sms_android', label_key: 'Android', surface: 'mobile' }], content_families: ['text'] },
    { id: 'push', label_key: 'Push', preview_profiles: [{ id: 'ios_lock_screen', label_key: 'iOS', surface: 'mobile' }], content_families: ['notification'] },
    { id: 'whatsapp', label_key: 'WhatsApp', preview_profiles: [{ id: 'whatsapp_ios', label_key: 'WhatsApp iOS', surface: 'mobile' }, { id: 'whatsapp_android', label_key: 'WhatsApp Android', surface: 'mobile' }, { id: 'whatsapp_web', label_key: 'WhatsApp Web', surface: 'desktop' }], content_families: ['text','rich_card','carousel'] }
  ].map(channel => ({ ...channel, delivery_modes: ['native'], limits: {} }));
  await page.route('**/config.js*', route => route.fulfill({ contentType: 'application/javascript', body: 'window.IS_INSTALLED=true;window.API_ENDPOINT=window.location.origin;window.ROOT_EMAIL="preview@example.com";' }));
  await page.route('http://127.0.0.1:8081/api/**', async route => {
    const url = route.request().url();
    const endpoint = url.split('?')[0].split('/').pop();
    const req = route.request().method() === 'POST' ? route.request().postDataJSON() : {};
    let data = {};
    if (endpoint === 'user.me') data = { user: { id: 'preview-user', email: 'preview@example.com', timezone: 'Asia/Shanghai', language: 'zh-CN' }, workspaces: [workspace] };
    else if (endpoint === 'setup.status') data = { is_installed: true };
    else if (endpoint === 'workspaces.get') data = { workspace };
    else if (endpoint === 'workspaces.list') data = { workspaces: [workspace] };
    else if (endpoint === 'workspaces.members') data = { members: [] };
    else if (endpoint === 'templates.list') data = { templates };
    else if (endpoint === 'templates.get') data = { template: templates.find(t => url.includes('id=' + t.id)) || templates[2] };
    else if (endpoint === 'templateCategories.list') data = { categories: [{ id: 'marketing', name: '营销素材', purpose: 'marketing', is_active: true, is_system: true, usage_count: 5 }] };
    else if (endpoint === 'channels.catalog') data = { channels };
    else if (endpoint === 'templates.compile') data = { html, mjml: req.mjml_source, subject: req.subject, test_data: req.test_data };
    else if (endpoint === 'templates.preview') {
      data = { channel: req.channel, resolved_language: 'zh-CN', fallback_used: false, test_data: req.test_data };
      if (req.push) data.push = { ...req.push, platform: req.platform || 'android', payload_bytes: 246, warnings: [] };
      if (req.sms) data.sms = { ...req.sms, encoding: 'ucs-2', character_count: 72, unit_count: 72, segment_count: 2, per_segment: 67, remaining: 62 };
      if (req.content) data.channel_preview = { profile: req.profile || 'whatsapp_ios', direction: 'ltr', payload_bytes: 328, message: req.content, warnings: [] };
    }
    else if (endpoint === 'contacts.list') data = { contacts: [{ email: 'lin@example.com', first_name: '林', last_name: '女士', contact_lists: [], created_at: now, updated_at: now }], total: 1 };
    else if (endpoint === 'contacts.get') data = { contact: { email: 'lin@example.com', first_name: '林', last_name: '女士', created_at: now, updated_at: now } };
    else if (endpoint === 'timeline.list') data = { timeline: [{ id: 'timeline-1', email: 'lin@example.com', operation: 'insert', entity_type: 'message_history', kind: 'message.sent', changes: {}, entity_id: 'message-1', entity_data: { id: 'message-1', template_id: 'member-email', template_version: 1, template_name: '会员专属礼遇 · 邮件', template_email: email, channel: 'email', sent_at: now, message_data: { customer: { name: '林女士' } } }, created_at: now, db_created_at: now }] };
    else if (endpoint === 'messages.list') data = { messages: [{ id: 'message-1', contact_email: 'lin@example.com', template_id: 'member-email', template_version: 1, channel: 'email', status: 'sent', sent_at: now, created_at: now, updated_at: now, message_data: { data: { customer: { name: '林女士' } } } }], next_cursor: '' };
    else if (endpoint === 'lists.list') data = { lists: [] };
    else if (endpoint === 'segments.list') data = { segments: [] };
    else if (endpoint === 'customers.list') data = { customers: [], total: 0 };
    else if (endpoint === 'customers.get') data = { customer: { customer_id: 'customer-review', customer_no: 'YG-1001', version: 1, identities: [], tags: [], profile: { customer_id: 'customer-review', attributes: {}, version: 1, created_at: now, updated_at: now }, created_at: now, updated_at: now } };
    else if (endpoint === 'broadcasts.list') data = { broadcasts: [] };
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
  });
  await page.addInitScript(() => { if (window.top === window) { localStorage.setItem('auth_token', 'local-preview-fixture'); localStorage.setItem('locale', 'zh-CN'); } });
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto('http://127.0.0.1:8081/console/workspace/preview-review/templates');
}
