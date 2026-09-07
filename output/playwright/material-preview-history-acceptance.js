async (page) => {
  const requests = [];
  const onRequest = request => { if (request.url().includes('/api/templates.compile')) requests.push(request.postDataJSON()); };
  page.on('request', onRequest);
  await page.getByRole('link', { name: '客户管理', exact: true }).click();
  await page.getByRole('row').filter({ hasText: 'lin@example.com' }).getByRole('button').last().click();
  await page.getByRole('tab', { name: '信件', exact: true }).click();
  await page.getByRole('row').filter({ hasText: 'member-email' }).getByRole('button').click();
  const dialog = page.getByRole('dialog', { name: '会员专属礼遇 · 邮件', exact: true });
  await dialog.locator('[data-preview-device=ios]').waitFor();
  if (requests.at(-1)?.test_data?.customer?.name !== '林女士') throw new Error('History preview did not receive customer message data');
  await dialog.getByRole('combobox', { name: '邮件客户端预览' }).selectOption('email_desktop');
  await dialog.locator('[data-preview-device=desktop]').waitFor();
  await dialog.frameLocator('iframe').locator('h1').waitFor();
  if (await dialog.locator('iframe').getAttribute('sandbox') !== '') throw new Error('History HTML sandbox missing');
  await page.screenshot({ path: 'output/playwright/material-preview-message-history.png', animations: 'disabled' });
  page.off('request', onRequest);
  return { entry: 'Customer details / Messages / Preview', customerDataPreserved: true, devices: ['ios', 'desktop'], sandbox: true };
}
