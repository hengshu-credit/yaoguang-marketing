async (page) => {
 await page.goto('http://127.0.0.1:8081/console/workspace/config-review/settings/custom-fields');
 const tab = page.getByRole('tab', { name: '客户属性', exact: true });
 await tab.waitFor();
 if (await tab.getAttribute('aria-selected') !== 'true') throw new Error('Legacy route did not open unified attributes');
 if (await page.getByRole('menuitem', { name: /自定义字段/ }).count()) throw new Error('Duplicate sidebar entry remains');
 const search = page.getByPlaceholder('按名称或编码搜索', { exact: true });
 await search.fill('不存在的属性');
 await page.getByText('暂无数据', { exact: true }).last().waitFor();
 await page.screenshot({ path: 'output/playwright/settings-attributes-empty.png', fullPage: true });
 await search.fill('');
}
