// Real browser flow against scripts/smoke-local.py's disposable fixture only.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const path = require('node:path');
(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.SMOKE_BROWSER ? { executablePath: process.env.SMOKE_BROWSER } : {}) });
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage(); const errors = []; page.on('pageerror', e => errors.push(e.message));
  const origin = process.env.SMOKE_ORIGIN, suffix = 'fixture-private-entry', password = 'fixture-password-long';
  const screenshot = async name => { await page.waitForFunction(() => document.querySelectorAll('.ant-message-notice').length === 0); await page.waitForTimeout(300); return page.screenshot({ path: path.join(process.env.SMOKE_OUTPUT, name+'.png'), fullPage: true }); };
  const visible = async locator => { await locator.waitFor({state:'visible'}); };
  try {
    await page.goto(`${origin}/install/${process.env.SMOKE_SETUP_TOKEN}/`);
    await visible(page.getByRole('heading',{name:'建立你的专属入口'}));
    await page.locator('#name').fill('Portal 测试站'); await page.locator('#suffix').fill(suffix);
    await page.locator('#username').fill('owner'); await page.locator('#password').fill(password); await page.locator('#confirm').fill(password);
    await screenshot('setup-desktop'); await page.getByRole('button',{name:/继\s*续/ }).click();
    await page.locator('#panelUrl').fill(process.env.SMOKE_PANEL); await page.locator('#panelToken').fill('fixture-panel-token-for-test-only');
    await page.locator('#supportUrl').fill('https://github.com/AngeloSakura'); await page.getByRole('button',{name:'检查连接'}).click();
    await visible(page.getByText('连接成功',{exact:true})); await page.getByRole('button',{name:/继\s*续/ }).click();
    await page.getByRole('button',{name:'完成安装',exact:true}).click(); await visible(page.getByRole('heading',{name:'一切就绪。'}));
    await page.getByRole('link',{name:'前往登录'}).click(); await visible(page.getByRole('heading',{name:'欢迎回来'}));
    assert.equal(await page.getByRole('link',{name:'AngeloSakura',exact:true}).getAttribute('href'),'https://github.com/AngeloSakura');
    await screenshot('login-dark-desktop'); await page.getByRole('button',{name:/明\s*亮/}).click(); assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),'light'); await page.reload(); await visible(page.getByRole('heading',{name:'欢迎回来'})); assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),'light'); await screenshot('login-light-desktop'); await page.setViewportSize({width:390,height:844}); await screenshot('login-mobile');
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mobile page overflows');
    await page.setViewportSize({width:1440,height:1000});
    await page.getByLabel('账号',{exact:true}).fill('owner');await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录账户'}).click();
    await visible(page.getByRole('heading',{name:'用户管理',exact:true}));await page.getByRole('button',{name:'站点设置',exact:true}).click();
    await visible(page.getByRole('heading',{name:'HTTPS 证书',exact:true})); await visible(page.getByText('导入证书 · 信任由签发方决定',{exact:true}));
    // Save config, but never request an external ACME certificate in this fixture.
    await page.getByLabel('证书联系邮箱').fill('admin@example.com');await page.getByRole('checkbox',{name:/我同意/}).check();
    await page.getByRole('button',{name:'保存配置',exact:true}).click();await visible(page.getByText('配置已保存，自动续期会使用新配置',{exact:true}));
    await screenshot('admin-certificate-desktop');
    await page.getByRole('button',{name:'用户管理',exact:true}).click();await page.getByRole('button',{name:/添加用户/}).click();
    await page.getByLabel('查找面板客户端').fill('demo'); await page.getByRole('button',{name:/查\s*找/ }).click();
    await page.getByLabel('绑定客户端').click();await page.getByTitle('demo-client',{exact:true}).click();
    await page.getByLabel('用户登录账号').fill('member');await page.getByRole('button',{name:'创建用户',exact:true}).click();
    await visible(page.locator('.invitation-address'));const invitation=await page.locator('.invitation-address').textContent();
    await page.getByRole('button',{name:'退出登录',exact:true}).click();await visible(page.getByRole('heading',{name:'欢迎回来'}));
    await page.goto(invitation);await page.getByRole('button',{name:'验证邀请并继续'}).click();await visible(page.getByRole('heading',{name:'最后一步，设置密码'}));
    await page.getByLabel('设置登录密码').fill(password);await page.getByLabel('确认密码').fill(password);await page.getByRole('button',{name:'设置密码，前往登录'}).click();
    await visible(page.getByRole('heading',{name:'欢迎回来'}));await page.getByLabel('账号',{exact:true}).fill('member');await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录账户'}).click();
    await visible(page.getByRole('heading',{name:'订阅概览',exact:true}));await visible(page.getByText('本站中转',{exact:false}).first());
    const summary=await page.evaluate(async()=>await (await fetch('api/summary')).json());assert.equal(summary.routes.length,1);assert.ok(summary.routes[0].primary.startsWith(origin+'/sub/'));assert.ok(summary.routes[0].backup.startsWith(process.env?.SMOKE_PANEL||'http://'));
    const sub=await context.request.get(summary.routes[0].primary);assert.equal(sub.status(),200,await sub.text());assert.equal(await sub.text(),'fixture-subscription-content');
    await screenshot('dashboard-light-desktop'); await page.getByRole('button',{name:/暗\s*黑/}).click(); await screenshot('dashboard-dark-desktop');await page.setViewportSize({width:390,height:844});await screenshot('dashboard-mobile');assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'dashboard overflows');
    assert.equal((await context.request.get(origin+'/')).status(),404);assert.equal((await context.request.get(`${origin}/install/${process.env.SMOKE_SETUP_TOKEN}/`)).status(),404);
    assert.deepEqual(errors,[]);console.log('PASS: HTTPS install, HTTP panel, login, certificate settings, invitation, mandatory password, relay, mobile and author links');
  } catch(e) {await screenshot('smoke-failure');throw e;} finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
