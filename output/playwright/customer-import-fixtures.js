async (page) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
  const now = '2026-09-08T00:00:00Z';
  const workspace = { id: 'import-review', name: '客户导入验收', settings: { timezone: 'Asia/Shanghai', default_language: 'zh-CN', languages: ['zh-CN'], custom_field_labels: { custom_string_1: '会员编号' } }, integrations: [], created_at: now, updated_at: now };
  const attributes = [
    { key: 'credit_score', name: '信用评分', schema: { type: 'number' }, settings: {}, status: 'active', kind: 'customer_attribute' },
    { key: 'vip', name: '会员标识', schema: { type: 'boolean' }, settings: {}, status: 'active', kind: 'customer_attribute' }
  ];
  const contact = { email: 'file-review@example.com', external_id: '000123456789012345678', first_name: '小明', full_name: '小明\n客户', custom_string_1: '00042', custom_number_1: 0, profile: { status: 'active', tags: ['会员'], attributes: { credit_score: 0, vip: false } }, created_at: now, updated_at: now, contact_lists: [] };
  const fileFields = () => {
    const field = (key, type = 'string', header = key, read_only = false) => ({ key, type, header, read_only });
    const fields = ['email', 'external_id', 'first_name', 'last_name', 'full_name', 'phone', 'timezone', 'language', 'country', 'address_line_1', 'address_line_2', 'state', 'postcode', 'job_title'].map(key => field(key));
    for (const type of ['string', 'number', 'datetime', 'json']) for (let i = 1; i <= 5; i++) {
      const key = `custom_${type}_${i}`;
      fields.push(field(key, type, `${workspace.settings.custom_field_labels[key] || `Custom ${type[0].toUpperCase()}${type.slice(1)} ${i}`} (${key})`));
    }
    fields.push(field('created_at', 'string', 'created_at', true), field('updated_at', 'string', 'updated_at', true), field('profile.status'), field('profile.tags', 'json'));
    for (const attribute of attributes) fields.push(field(`profile.attributes.${attribute.key}`, attribute.schema.type, `${attribute.name} (profile.attributes.${attribute.key})`));
    return fields;
  };
  const jobs = [];
  await page.route('**/config.js', route => route.fulfill({ contentType: 'application/javascript', body: 'window.API_ENDPOINT=window.location.origin;window.ROOT_EMAIL="review@example.com";' }));
  await page.route('http://127.0.0.1:8081/api/**', async route => {
    const url = route.request().url();
    const endpoint = url.split('?')[0].split('/').pop();
    const query = Object.fromEntries((url.split('?')[1] || '').split('&').filter(Boolean).map(part => part.split('=').map(decodeURIComponent)));
    let data = {};
    if (endpoint === 'user.me') data = { user: { id: 'review-user', email: 'review@example.com', language: 'zh-CN' }, workspaces: [workspace] };
    else if (endpoint === 'setup.status') data = { is_installed: true };
    else if (endpoint === 'workspaces.get') data = { workspace };
    else if (endpoint === 'workspaces.list') data = { workspaces: [workspace] };
    else if (endpoint === 'workspaces.members') data = { members: [{ user_id: 'review-user', role: 'owner', permissions: Object.fromEntries(['contacts', 'customers', 'lists', 'segments', 'workspace', 'settings', 'custom_fields'].map(key => [key, { read: true, write: true }])) }] };
    else if (endpoint === 'contacts.list') data = { contacts: [contact], next_cursor: '' };
    else if (endpoint === 'contacts.count') data = { total_contacts: 1 };
    else if (endpoint === 'imports.fields') data = { fields: fileFields() };
    else if (endpoint === 'businessDefinitions.save') {
      const definition = route.request().postDataJSON().definition;
      attributes.push(definition);
      contact.profile.attributes[definition.key] = 7;
      data = { definition };
    } else if (endpoint === 'workspaces.setCustomFieldLabels') {
      Object.assign(workspace.settings.custom_field_labels, route.request().postDataJSON().custom_field_labels);
      data = { success: true };
    }
    else if (endpoint === 'lists.list') data = { lists: [] };
    else if (endpoint === 'segments.list') data = { segments: [] };
    else if (endpoint === 'imports.list') data = { items: jobs, total: jobs.length };
    else if (endpoint === 'imports.get') data = jobs.find(job => job.id === query.job_id);
    else if (endpoint === 'imports.upload') {
      const csv = route.request().postData();
      if (!csv.includes('000123456789012345678') || !csv.includes('email,external_id')) throw new Error('Import payload lost template headers or identifier');
      data = { id: `job-${jobs.length + 1}`, filename: query.filename, status: 'completed', list_ids: [], counters: { total: 1, pending: 0, processing: 0, succeeded: 1, failed: 0 } };
      jobs.unshift(data);
    } else if (endpoint === 'businessDefinitions.list') {
      const definitions = query.kind === 'event' && !query.search ? [{ id: 'event-1', kind: 'event', key: 'contact.created', name: '客户已创建', builtin: true, status: 'active', settings: { builtin: true } }] : [];
      data = { definitions, total: definitions.length };
    }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) });
  });
  await page.addInitScript(() => { localStorage.setItem('auth_token', 'local-import-review-fixture'); localStorage.setItem('locale', 'zh-CN'); });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('http://127.0.0.1:8081/console/workspace/import-review/contacts');
}
