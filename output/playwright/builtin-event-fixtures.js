async (page) => {
 await page.unrouteAll({ behavior: 'ignoreErrors' });
 await page.route('**/config.js*', route => route.fulfill({ contentType: 'application/javascript', body: 'window.API_ENDPOINT=window.location.origin;window.ROOT_EMAIL="review@example.com";' }));
 const workspace = { id: 'builtin-review', name: '配置验收', settings: { timezone: 'Asia/Shanghai', default_language: 'zh-CN', languages: ['zh-CN'], custom_field_labels: { custom_string_1: '客户来源', custom_number_1: '授信额度' } }, integrations: [] };
 const definitions = [
  ...['email.bounced','email.delivered','email.opened','email.sent','contact.created','state.entered','push.delivered'].map((key,index) => ({ id: 'builtin-'+index, kind: 'event', key, name: key, settings: { builtin: true, subject_type: 'customer' }, schema: key === 'email.bounced' ? { type: 'object', properties: { device_fingerprint: { type: 'string', title: '设备指纹' } } } : { type: 'object' }, status: 'active', revision: 1 })),
  { id: 'custom-login', kind: 'event', key: 'login', name: '登录', settings: { subject_type: 'customer' }, schema: { type: 'object' }, status: 'active', revision: 1 }
 ];
 await page.route('http://127.0.0.1:8081/api/**', async route => {
  const url = route.request().url(), endpoint = url.split('?')[0].split('/').pop();
  const query = Object.fromEntries((url.split('?')[1] || '').split('&').filter(Boolean).map(part => part.split('=').map(value => decodeURIComponent(value.replaceAll('+', ' ')))));
  const body = route.request().method() === 'POST' ? route.request().postDataJSON() : {};
  let data = {};
  if (endpoint === 'user.me') data = { user: { id: 'review', email: 'review@example.com', language: 'zh-CN' }, workspaces: [workspace] };
  else if (endpoint === 'workspaces.get') data = { workspace };
  else if (endpoint === 'workspaces.list') data = { workspaces: [workspace] };
  else if (endpoint === 'workspaces.members') data = { members: [{ user_id: 'review', role: 'owner', permissions: Object.fromEntries(['contacts','customers','lists','segments','workspace'].map(key => [key,{read:true,write:true}])) }] };
  else if (endpoint === 'setup.status') data = { is_installed: true };
  else if (endpoint === 'businessDefinitions.list') { const matches = definitions.filter(item => item.kind === query.kind && (!query.search || `${item.key} ${item.name}`.includes(query.search))); data = { definitions: matches.slice(Number(query.offset || 0), Number(query.offset || 0) + Number(query.limit || 100)), total: matches.length }; }
  else if (route.request().method() !== 'GET') throw new Error('Unexpected mutation: '+endpoint);
  await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
 });
 await page.addInitScript(() => { localStorage.setItem('auth_token','configuration-review-fixture'); localStorage.setItem('locale','zh-CN'); });
 await page.setViewportSize({ width: 1440, height: 1100 });
 await page.goto('http://127.0.0.1:8081/console/workspace/builtin-review/settings/business');
}

