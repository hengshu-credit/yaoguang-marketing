async (page) => {
  await page.goto('http://127.0.0.1:8081/console/workspace/preview-review/templates');
  let release;
  let fail = false;
  const pending = new Promise(resolve => { release = resolve; });
  const routePattern = 'http://127.0.0.1:8081/api/templates.preview';
  const handler = async route => {
    const req = route.request().postDataJSON();
    await pending;
    if (fail) return route.fulfill({ status: 422, contentType: 'application/json', body: JSON.stringify({ error: '预览服务暂不可用' }) });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ channel: 'push', resolved_language: 'zh-CN', fallback_used: false, push: { title: '长内容及图片异常验证', body: '这是一条需要完整阅读的客户通知。'.repeat(180) + '\n预览内容末尾', image_url: 'http://127.0.0.1:8081/preview-missing-image.png', platform: req.platform, payload_bytes: 3000, warnings: [] } }) });
  };
  await page.route(routePattern, handler);
  await page.route('**/preview-missing-image.png', route => route.fulfill({ status: 404, body: '' }));
  await page.getByRole('row').filter({ hasText: '会员专属礼遇 · 推送' }).getByRole('button').last().click();
  await page.getByRole('dialog').locator('.ant-spin').waitFor();
  release();
  await page.getByText('图片暂时无法加载', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'iOS', exact: true }).click();
  await page.locator('[data-preview-device=ios]').waitFor();
  await page.setViewportSize({ width: 320, height: 900 });
  await page.getByRole('dialog').evaluate(async el => {
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    await Promise.all(el.closest('.ant-drawer').getAnimations({ subtree: true }).filter(a => a.effect.getComputedTiming().iterations !== Infinity).map(a => a.finished.catch(() => {})));
  });
  const scroll = await page.locator('.material-device__viewport').evaluate(el => {
    el.scrollTop = el.scrollHeight;
    return { height: el.clientHeight, contentHeight: el.scrollHeight, top: el.scrollTop, width: el.clientWidth, contentWidth: el.scrollWidth };
  });
  if (scroll.contentHeight <= scroll.height || scroll.top <= 0 || scroll.contentWidth > scroll.width + 1) throw new Error('Long message is clipped: ' + JSON.stringify(scroll));
  const bounds = await page.getByRole('dialog').evaluate(el => ({ left: el.getBoundingClientRect().left, right: el.getBoundingClientRect().right, viewport: window.innerWidth }));
  if (bounds.left < -1 || bounds.right > bounds.viewport + 1) throw new Error('Drawer exceeds the viewport: ' + JSON.stringify(bounds));
  await page.getByRole('dialog').locator('.ant-drawer-body').evaluate(el => { el.scrollTop = el.scrollHeight; });
  await page.screenshot({ path: 'output/playwright/material-preview-long-320.png', animations: 'disabled' });
  fail = true;
  await page.getByRole('button', { name: 'PC / Web', exact: true }).click();
  await page.getByText('预览服务暂不可用', { exact: true }).waitFor();
  if (await page.locator('[data-preview-device]').count()) throw new Error('Stale preview displayed on error');
  await page.screenshot({ path: 'output/playwright/material-preview-error.png', animations: 'disabled' });
  await page.unroute(routePattern, handler);
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  return { loading: true, brokenImageFallback: true, scroll, bounds, staleContentHiddenOnError: true };
}
