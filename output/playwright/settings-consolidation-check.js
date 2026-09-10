async (page) => {
 // Ant Design closes modal/drawer surfaces over a 300 ms CSS transition.
 const settle = () => page.waitForTimeout(400);
 await page.getByRole('tab', { name: '客户属性', exact: true }).click();
 await page.getByRole('row').filter({ hasText: 'custom_string_1' }).getByRole('button', { name: '修改显示名称', exact: true }).click();
 await page.getByRole('textbox', { name: '显示名称', exact: true }).fill('获客渠道');
 await page.getByRole('dialog').getByRole('button', { name: /确\s*定/ }).click();
 await page.getByRole('dialog').waitFor({ state: 'hidden' });
 await page.getByRole('cell', { name: '获客渠道', exact: true }).waitFor();
 await settle();
 await page.screenshot({ path: 'output/playwright/settings-unified-attributes.png', fullPage: true });
 await page.getByRole('tab', { name: '事件管理', exact: true }).click();
 await page.getByRole('button', { name: '批量扩展已有事件字段', exact: true }).click();
 await page.getByRole('button', { name: '添加设备指纹示例字段', exact: true }).click();
 if (!(await page.getByRole('switch', { name: /device_fingerprint/ }).isDisabled())) throw new Error('Shared field must remain optional');
 await settle();
 await page.screenshot({ path: 'output/playwright/settings-event-fields.png', fullPage: true });
 const applied = page.waitForResponse(response => response.url().includes('/api/businessDefinitions.extendEvents'));
 await page.getByRole('button', { name: '应用到所选事件', exact: true }).click();
 if (!(await applied).ok()) throw new Error('Event field extension failed');
 await page.getByRole('tab', { name: '业务对象类型', exact: true }).click();
 await page.getByRole('button', { name: '合同', exact: true }).click();
 await page.getByRole('textbox', { name: /contract_id/ }).first().waitFor();
 await settle();
 await page.screenshot({ path: 'output/playwright/settings-object-fields.png', fullPage: true });
 await page.getByRole('dialog').getByRole('button', { name: /^关\s*闭$/ }).last().click();
 await page.getByRole('dialog').waitFor({ state: 'hidden' });
 await page.getByRole('tab', { name: '对象关系', exact: true }).click();
 await page.getByText('合同的还款计划', { exact: true }).waitFor();
 await settle();
 await page.screenshot({ path: 'output/playwright/settings-object-relations.png', fullPage: true });
 await page.getByRole('menuitem', { name: /客户与对象状态/ }).click();
}
