async (page) => {
  const region = page.getByRole('region', { name: 'Message cards' });
  await page.getByRole('button', { name: 'Show card 3' }).click();
  await page.waitForFunction(() => {
    const rail = document.querySelector('.material-mobile-carousel__rail').getBoundingClientRect();
    const card = document.querySelectorAll('.material-mobile-carousel__card')[2].getBoundingClientRect();
    return card.right <= rail.right + 1 && card.left >= rail.left - 1;
  });
  await region.focus();
  await page.keyboard.press('Home');
  await page.waitForFunction(() => document.querySelector('.material-mobile-carousel__rail').scrollLeft === 0);
  await page.getByRole('button', { name: 'RTL', exact: true }).click();
  await page.getByRole('button', { name: 'Show card 2' }).click();
  await page.waitForFunction(() => document.querySelector('.material-mobile-carousel__rail').scrollLeft < -150);
  await page.screenshot({ path: 'output/playwright/mobile-marketing-rtl.png', fullPage: true });
  await page.getByRole('button', { name: 'RTL', exact: true }).click();
  await page.getByRole('combobox', { name: 'Client preview' }).first().selectOption('in_app_android');
  await page.screenshot({ path: 'output/playwright/mobile-marketing-android.png', fullPage: true });
  await page.getByRole('button', { name: 'Long text / failed image' }).click();
  await page.getByText('Image unavailable', { exact: true }).first().waitFor();
  await page.setViewportSize({ width: 375, height: 1000 });
  const geometry = await page.evaluate(() => {
    const outerOverflow = document.documentElement.scrollWidth > innerWidth;
    const screens = [...document.querySelectorAll('.material-device__viewport')].map(el => ({ overflowX: el.scrollWidth > el.clientWidth + 1, scrollsVertically: el.scrollHeight > el.clientHeight }));
    return { outerOverflow, screens };
  });
  if (geometry.outerOverflow || geometry.screens.some(screen => screen.overflowX || !screen.scrollsVertically)) throw new Error(JSON.stringify(geometry));
  await page.screenshot({ path: 'output/playwright/mobile-marketing-long-375.png', fullPage: true });
  await page.locator('.material-device__viewport').first().evaluate(el => { el.scrollTop = el.scrollHeight; });
  const bannerCTA = page.getByRole('link', { name: '立即领取专属礼遇' }).first();
  if (!(await bannerCTA.isVisible())) throw new Error('Long notification action unavailable');
  await page.screenshot({ path: 'output/playwright/mobile-marketing-long-end.png' });
  await page.getByRole('button', { name: 'Long text / failed image' }).click();
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.getByRole('combobox', { name: 'Client preview' }).first().selectOption('in_app_web');
  if (await page.locator('.material-mobile-marketing').count()) throw new Error('Mobile presentation leaked to desktop');
  await page.screenshot({ path: 'output/playwright/mobile-marketing-desktop-check.png', fullPage: true });
  await page.getByRole('combobox', { name: 'Client preview' }).first().selectOption('in_app_ios');
}
