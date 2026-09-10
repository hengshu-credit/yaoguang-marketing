async (page) => {
 await page.unrouteAll({ behavior: 'ignoreErrors' });
 await page.route('**/config.js*', route => route.fulfill({ contentType: 'application/javascript', body: 'window.API_ENDPOINT=window.location.origin;window.ROOT_EMAIL="review@example.com";' }));
 const workspace = { id: 'config-review', name: '配置验收', settings: { timezone: 'Asia/Shanghai', default_language: 'zh-CN', languages: ['zh-CN'], custom_field_labels: { custom_string_1: '客户来源', custom_number_1: '授信额度' } }, integrations: [] };
 let definitions = [
  { id: 'event-1', kind: 'event', key: 'contact.created', name: 'Customer created', settings: { builtin: true, subject_type: 'customer', expiry_mode: 'never' }, schema: { type: 'object' }, status: 'active', revision: 1 },
  { id: 'event-2', kind: 'event', key: 'login', name: '登录', settings: { subject_type: 'customer', expiry_mode: 'never' }, schema: { type: 'object' }, status: 'active', revision: 1 },
  { id: 'attr-1', kind: 'customer_attribute', key: 'credit_score', name: '信用评分', settings: { writable: true }, schema: { type: 'number' }, status: 'active', revision: 1 },
  { id: 'contract', kind: 'object_type', key: 'contract', name: '合同', settings: { identity_fields: ['contract_id'] }, schema: { type: 'object', properties: { contract_id: { type: 'string', title: '合同编号' }, amount: { type: 'number', title: '合同金额' } } }, status: 'active', revision: 1 },
  { id: 'plan', kind: 'object_type', key: 'repayment_plan', name: '还款计划', settings: { identity_fields: ['plan_id'] }, schema: { type: 'object', properties: { plan_id: { type: 'string' } } }, status: 'active', revision: 1 },
  { id: 'relation', kind: 'object_relation', key: 'contract_plans', name: '合同的还款计划', settings: { from_type: 'contract', to_type: 'repayment_plan', cardinality: 'one_to_many' }, schema: { type: 'object' }, status: 'active', revision: 1 },
  { id: 'state-1', kind: 'state', key: 'eligible', name: '授信客户', settings: { subject_type: 'customer', rule: { kind: 'all', children: [{ kind: 'attribute', field: 'credit_score', operator: 'gte', operand: 600 }] } }, schema: { type: 'object' }, status: 'active', revision: 1 }
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
  else if (endpoint === 'businessDefinitions.extendEvents') {
   if (body.schema.properties.device_fingerprint?.type !== 'string' || body.event_keys.length !== 2) throw new Error('Incorrect shared field request');
   definitions = definitions.map(item => body.event_keys.includes(item.key) && item.kind === 'event' ? { ...item, schema: { ...item.schema, properties: { ...item.schema.properties, ...body.schema.properties } }, revision: item.revision + 1 } : item);
   data = { definitions: definitions.filter(item => body.event_keys.includes(item.key)) };
  } else if (endpoint === 'businessDefinitions.save') {
   const definition = { ...body.definition, id: body.definition.id || 'created', revision: (body.definition.revision || 0) + 1 };
   definitions = [...definitions.filter(item => item.id !== definition.id), definition]; data = { definition };
  } else if (endpoint === 'workspaces.setCustomFieldLabels') {
   if (body.custom_field_labels.custom_number_1 !== '授信额度') throw new Error('Unrelated label lost');
   workspace.settings.custom_field_labels = body.custom_field_labels; data = { success: true };
  } else if (endpoint === 'imports.fields') data = { fields: [{ key: 'profile.attributes.credit_score', header: '信用评分', type: 'number', read_only: false }, { key: 'custom_string_1', header: '客户来源', type: 'string', read_only: false }] };
  else if (endpoint === 'subjectStates.inspect') data = { states: [{ state_key: 'eligible', value: 'true', subject_type: 'customer', subject_id: 'customer-1', rule_revision: definitions.find(item => item.id === 'state-1').revision, reason: '信用评分 700，满足已保存条件', computed_at: '2026-09-09T10:00:00Z' }] };
  else if (endpoint === 'subjectStates.coverage') data = { total: 1, intersection: 1, union: 1, states: [{ state_key: 'eligible', satisfied: 1, false: 0, unknown: 0 }] };
  await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
 });
 await page.addInitScript(() => { localStorage.setItem('auth_token','configuration-review-fixture'); localStorage.setItem('locale','zh-CN'); });
 await page.setViewportSize({ width: 1440, height: 1100 });
 await page.goto('http://127.0.0.1:8081/console/workspace/config-review/settings/business');
}
