async (page) => {
  await page.getByRole('tab', { name: '导入客户', exact: true }).click();
  for (const [label, extension] of [['下载 CSV 模板', 'csv'], ['下载 Excel 模板', 'xlsx']]) {
    const pending = page.waitForEvent('download');
    await page.getByRole('button', { name: label, exact: true }).click();
    const download = await pending;
    await download.saveAs(`output/playwright/customer-import-template.${extension}`);
    console.log(`Downloaded ${download.suggestedFilename()}`);
  }
  await page.screenshot({ path: 'output/playwright/customer-import-form.png', fullPage: true });
}
