async (page) => {
  const widths = [];
  for (const width of [1920, 1440, 1024]) {
    await page.setViewportSize({ width, height: 1000 });
    const heading = page.getByRole('heading', { name: '客户与事件配置', exact: true });
    await heading.waitFor();
    const layout = await heading.evaluate(element => {
      const content = element.closest('main');
      const section = content.firstElementChild;
      return { viewport: innerWidth, content: content.getBoundingClientRect().width, section: section.getBoundingClientRect().width, overflow: document.documentElement.scrollWidth > innerWidth };
    });
    if (layout.overflow || Math.abs(layout.content - layout.section) > 1) throw new Error(`Settings width mismatch: ${JSON.stringify(layout)}`);
    widths.push(layout);
    await page.screenshot({ path: `output/playwright/customer-settings-${width}.png`, fullPage: true });
  }
  if (widths[0].section - widths[1].section < 450) throw new Error('Settings content did not grow with viewport');
  await page.getByRole('searchbox', { name: '搜索定义' }).fill('不存在的定义');
  await page.getByRole('searchbox', { name: '搜索定义' }).press('Enter');
  await page.getByText('暂无数据', { exact: true }).last().waitFor();
  await page.screenshot({ path: 'output/playwright/customer-settings-empty.png', fullPage: true });
  await page.evaluate(value => { document.documentElement.dataset.settingsWidths = JSON.stringify(value); }, widths);
}
