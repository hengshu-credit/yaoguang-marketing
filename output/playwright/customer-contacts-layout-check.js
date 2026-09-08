async (page) => {
  for (const width of [1440, 1024]) {
    await page.setViewportSize({ width, height: 1000 });
    const tab = page.getByRole('tab', { name: /^联系人/ });
    const action = page.getByRole('button', { name: '导入 / 导出', exact: true });
    const tabBox = await tab.boundingBox();
    const actionBox = await action.boundingBox();
    if (Math.abs(tabBox.y + tabBox.height / 2 - actionBox.y - actionBox.height / 2) > 3 || actionBox.x < tabBox.x + tabBox.width) throw new Error('Contact actions are not beside tabs');
    if (!(await tab.innerText()).includes('0')) throw new Error('Contact count missing from tab');
    await page.screenshot({ path: `output/playwright/customer-contacts-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole('button', { name: '导入 / 导出', exact: true }).click();
}
