async (page) => {
  const pending = page.waitForEvent('download');
  await page.getByRole('dialog', { name: '导出联系人到 CSV' }).getByRole('button', { name: '导 出', exact: true }).click();
  await (await pending).saveAs('output/playwright/contact-schema-export.csv');
}
