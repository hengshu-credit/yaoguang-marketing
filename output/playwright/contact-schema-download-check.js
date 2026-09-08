async (page) => {
  await page.reload();
  await page.getByRole('tab', { name: '导入客户', exact: true }).click();
  const download = async (label, path) => {
    const pending = page.waitForEvent('download');
    await page.getByRole('button', { name: label, exact: true }).click();
    await (await pending).saveAs(path);
  };
  await download('下载 CSV 模板', 'output/playwright/contact-schema-before.csv');
  await page.evaluate(async () => {
    const definition = { kind: 'customer_attribute', key: 'new_limit', name: '新增额度', schema: { type: 'number' }, settings: {}, status: 'active' };
    const created = await fetch('/api/businessDefinitions.save', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ workspace_id: 'import-review', definition }) });
    if (!created.ok) throw new Error('Fixture definition creation failed');
    const renamed = await fetch('/api/workspaces.setCustomFieldLabels', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ workspace_id: 'import-review', custom_field_labels: { custom_string_1: '客户会员编号' } }) });
    if (!renamed.ok) throw new Error('Fixture field rename failed');
  });
  await download('下载 CSV 模板', 'output/playwright/contact-schema-after.csv');
  await download('下载 Excel 模板', 'output/playwright/contact-schema-after.xlsx');
  await page.screenshot({ path: 'output/playwright/contact-schema-import.png', fullPage: true });
  await page.getByRole('tab', { name: /^联系人/ }).click();
  await page.getByRole('button', { name: '导入 / 导出', exact: true }).click();
  await page.getByRole('menuitem', { name: /导出/ }).click();
}
