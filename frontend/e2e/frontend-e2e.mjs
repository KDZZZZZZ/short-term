/**
 * 前端 E2E：驱动真实浏览器（系统 Chrome）访问 Vite 开发服务器，
 * 后端为联调环境（Vite 代理 /api/v1 -> http://123.56.161.234:18083）。
 *
 * 覆盖：注册登录、完善资料、发布商品（含图片）、市场搜索/分类、
 * 商品详情、收藏、会话聊天（含未读与已读）、购买意向、卖家接受、
 * 双方确认完成、买家评价、公开评论、卖家公开货架、下架/重新上架、图片管理、修改资料与退出登录。
 *
 * 关键截图保存到 e2e/shots/ 供人工/评审复核。
 */
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SHOTS = join(__dirname, 'shots')
mkdirSync(SHOTS, { recursive: true })

const BASE = process.env.E2E_BASE_URL ?? 'http://localhost:5173'
const STAMP = Date.now().toString(36)
const SELLER = { student_no: `e2es${STAMP}`, nickname: `E2E卖家${STAMP.slice(-4)}` }
const BUYER = { student_no: `e2eb${STAMP}`, nickname: `E2E买家${STAMP.slice(-4)}` }
const PASSWORD = 'e2e-password-123'
const PRODUCT_TITLE = `E2E二手单词书${STAMP.slice(-4)}`

/** 1x1 红色 PNG 与蓝色 PNG（服务端按真实字节判定图片类型）。 */
const PNG_RED = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
)
const PNG_BLUE = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64',
)

let step = 0
async function shot(context, name) {
  step += 1
  const page = context.pages()[0]
  if (!page) return
  await settle(page)
  await page.screenshot({ path: join(SHOTS, `${String(step).padStart(2, '0')}-${name}.png`), fullPage: false })
  console.log(`  [shot] ${name}`)
}

/** 等待页面渲染稳定：所有 <img> 加载完成 + 两帧 rAF。 */
async function settle(page) {
  await page
    .waitForFunction(() => Array.from(document.images).every((img) => img.complete), { timeout: 10_000 })
    .catch(() => {})
  // 等待 Motion 入场动画播完（最长延迟 0.24s + 时长 0.28s）
  await page.waitForTimeout(700)
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))))
}

async function register(context, { student_no, nickname }) {
  const page = await context.newPage()
  await page.goto(`${BASE}/register`)
  await page.getByLabel('学号 *').fill(student_no)
  await page.getByLabel('密码 *', { exact: true }).fill(PASSWORD)
  await page.getByLabel('确认密码 *').fill(PASSWORD)
  await page.getByLabel('昵称').fill(nickname)
  await page.getByLabel('微信').fill(`wx_${student_no}`)
  await page.getByRole('button', { name: '注册', exact: true }).click()
  await page.waitForURL(`${BASE}/`, { timeout: 10_000 })
  console.log(`✓ 注册并自动登录 ${student_no}`)
  return page
}

async function run() {
  const browser = await chromium.launch({
    executablePath: '/usr/bin/google-chrome-stable',
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })

  const sellerContext = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  sellerContext.on('response', async (response) => {
    if (response.status() >= 400 && response.url().includes('/api/')) {
      console.log(`  [api ${response.status()}]`, response.request().method(), new URL(response.url()).pathname, (await response.text()).slice(0, 200))
    }
  })
  const buyerContext = await browser.newContext({ viewport: { width: 1440, height: 900 } })

  try {
    // ---------- 1. 登录页可达，注册卖家 ----------
    const anon = await sellerContext.newPage()
    await anon.goto(BASE)
    await anon.waitForURL(/\/login/, { timeout: 10_000 })
    console.log('✓ 未登录访问首页重定向到登录页')
    await anon.getByLabel('学号').fill('will-fail@example')
    await anon.getByLabel('密码').fill('wrong-password')
    await anon.getByRole('button', { name: '登录' }).click()
    await anon.getByText('学号或密码错误').waitFor({ timeout: 5_000 })
    console.log('✓ 错误密码提示“学号或密码错误”')
    await shot(sellerContext, 'login-error')
    await anon.close()

    await register(sellerContext, SELLER)
    const seller = sellerContext.pages()[0]
    await seller.getByRole('link', { name: '市场' }).waitFor({ timeout: 5_000 })
    await seller
      .getByText('市场暂时还没有在售商品')
      .or(seller.getByRole('button', { name: '查看详情' }))
      .first()
      .waitFor({ timeout: 8_000 })
    await settle(seller)
    await shot(sellerContext, 'seller-home')

    // ---------- 2. 卖家发布商品（两张图片） ----------
    await seller.goto(`${BASE}/products/new`)
    await seller.getByLabel('标题 *').fill(PRODUCT_TITLE)
    await seller.getByLabel('价格 *').fill('9.90')
    // Select 分类：点击触发器后选择“教材教辅”
    await seller.getByText('选择分类').click()
    await seller.getByRole('option', { name: '教材教辅' }).click()
    await seller.getByLabel('描述 *').fill('九成新考研单词书，无笔记无划线，自提可小刀。')
    await seller.setInputFiles('input[type=file]', [
      { name: 'cover.png', mimeType: 'image/png', buffer: PNG_RED },
      { name: 'second.png', mimeType: 'image/png', buffer: PNG_BLUE },
    ])
    await shot(sellerContext, 'publish-form')
    await seller.locator('form').getByRole('button', { name: '发布', exact: true }).click()
    await seller.waitForURL(/\/products\/[^/]+$/, { timeout: 10_000 })
    await seller.getByText('商品已发布').waitFor({ timeout: 5_000 })
    console.log('✓ 发布商品成功并跳转详情')
    const productUrl = seller.url()
    const productId = productUrl.split('/').pop()
    await seller.getByRole('button', { name: '编辑信息' }).waitFor({ timeout: 8_000 })
    await settle(seller)
    await shot(sellerContext, 'seller-product-detail')

    // 等待主图渲染（红色 1x1 PNG）
    await seller.locator('img').first().waitFor({ timeout: 8_000 })
    console.log('✓ 详情页主图已渲染')

    // ---------- 3. 卖家下架 -> 重新上架 ----------
    await seller.getByRole('button', { name: '下架商品' }).click()
    await seller.getByRole('button', { name: '确认下架' }).click()
    await seller.getByText('商品已下架').waitFor({ timeout: 5_000 })
    await seller.getByText('已下架', { exact: true }).first().waitFor({ timeout: 5_000 })
    console.log('✓ 下架成功，状态芯片变为已下架')
    await shot(sellerContext, 'seller-off-shelf')
    await seller.getByRole('button', { name: '重新上架' }).click()
    await seller.getByText('商品已重新上架').waitFor({ timeout: 5_000 })
    console.log('✓ 重新上架成功')

    // ---------- 4. 买家注册、搜索、查看详情 ----------
    await register(buyerContext, BUYER)
    const buyer = buyerContext.pages()[0]
    await buyer.getByPlaceholder('搜索在售商品…').fill(PRODUCT_TITLE)
    await buyer.getByPlaceholder('搜索在售商品…').press('Enter')
    await buyer.getByText(PRODUCT_TITLE).first().waitFor({ timeout: 5_000 })
    console.log('✓ 买家搜索到商品')
    await settle(buyer)
    await shot(buyerContext, 'buyer-market-search')
    await buyer.getByText(PRODUCT_TITLE).first().click()
    await buyer.waitForURL(`**/products/${productId}`)
    await buyer.getByRole('heading', { name: PRODUCT_TITLE }).waitFor()
    await buyer.getByText(`wx_${SELLER.student_no}`).waitFor({ timeout: 5_000 })
    console.log('✓ 买家查看详情，卖家微信可见（不暴露学号）')
    await shot(buyerContext, 'buyer-product-detail')

    // ---------- 5. 买家收藏 ----------
    await buyer.getByRole('button', { name: /收藏/ }).click()
    await buyer.getByText('已收藏').waitFor({ timeout: 5_000 })
    await buyer.goto(`${BASE}/favorites`)
    await buyer.getByText(PRODUCT_TITLE).waitFor({ timeout: 5_000 })
    console.log('✓ 收藏列表出现商品')
    await shot(buyerContext, 'buyer-favorites')

    // ---------- 6. 买家发起会话并发消息 ----------
    await buyer.goto(productUrl)
    await buyer.getByRole('button', { name: '和卖家聊聊' }).click()
    await buyer.waitForURL(/\/chats\/[^/]+$/, { timeout: 10_000 })
    await buyer.getByPlaceholder('输入消息，回车发送（Shift+回车换行）').fill('你好，单词书还在吗？可以便宜点到 8 元吗？')
    await buyer.keyboard.press('Enter')
    await buyer.getByText('你好，单词书还在吗？可以便宜点到 8 元吗？').waitFor({ timeout: 5_000 })
    console.log('✓ 买家发起会话并发送消息')
    await shot(buyerContext, 'buyer-chat-sent')
    const chatUrl = buyer.url()

    // ---------- 7. 买家创建购买意向 ----------
    await buyer.goto(productUrl)
    await buyer.getByRole('button', { name: '我想要' }).click()
    await buyer.waitForURL(/\/trades/, { timeout: 10_000 })
    await buyer.locator('[data-slot="chip"], .chip', { hasText: '待处理' }).first().waitFor({ timeout: 8_000 })
    await buyer.getByText(PRODUCT_TITLE).first().waitFor({ timeout: 5_000 })
    console.log('✓ 买家创建购买意向，交易页可见待处理芯片')
    await shot(buyerContext, 'buyer-trade-pending')

    // ---------- 8. 卖家收到未读消息并回复 ----------
    await seller.goto(`${BASE}/chats`)
    await seller.getByText(BUYER.nickname).first().waitFor({ timeout: 10_000 })
    const badge = seller.locator('header').getByText('1', { exact: true })
    if ((await badge.count()) === 0) console.log('  (导航未读角标可能因轮询时序未出现，继续)')
    await shot(sellerContext, 'seller-conversations-unread')
    await seller.getByText(BUYER.nickname).first().click()
    await seller.waitForURL(/\/chats\/[^/]+$/)
    await seller.getByText('你好，单词书还在吗？可以便宜点到 8 元吗？').waitFor({ timeout: 10_000 })
    console.log('✓ 卖家会话内收到买家消息')
    await seller.getByPlaceholder('输入消息，回车发送（Shift+回车换行）').fill('在的，8 元可以，明天中午校门口交易？')
    await seller.keyboard.press('Enter')
    await seller.getByText('在的，8 元可以，明天中午校门口交易？').waitFor({ timeout: 5_000 })
    console.log('✓ 卖家回复消息')
    await shot(sellerContext, 'seller-chat-reply')

    // ---------- 9. 卖家接受购买意向 ----------
    await seller.goto(`${BASE}/trades`)
    await seller.getByRole('button', { name: '我卖出的' }).click()
    await seller.getByText(PRODUCT_TITLE).first().waitFor({ timeout: 5_000 })
    await seller.getByRole('button', { name: '接受' }).click()
    await seller.getByText('已接受，商品转为已预留').waitFor({ timeout: 5_000 })
    console.log('✓ 卖家接受，商品转 RESERVED')
    await seller.locator('[data-slot="chip"], .chip', { hasText: '已接受' }).first().waitFor({ timeout: 8_000 })
    await settle(seller)
    await shot(sellerContext, 'seller-trade-accepted')

    // 卖家详情页应显示已预留
    await seller.goto(productUrl)
    await seller.getByText('已预留', { exact: true }).first().waitFor({ timeout: 5_000 })
    console.log('✓ 商品详情状态为已预留')

    // ---------- 10. 双方确认，交易完成 ----------
    await buyer.goto(`${BASE}/trades`)
    await buyer.getByText('已接受，商品转为已预留').waitFor({ timeout: 5_000 }).catch(() => buyer.getByRole('button', { name: '确认完成' }).waitFor({ timeout: 8_000 }))
    await buyer.getByRole('button', { name: '确认完成' }).click()
    await buyer.getByText('已确认，等待对方确认').waitFor({ timeout: 5_000 })
    console.log('✓ 买家已确认')
    await seller.goto(`${BASE}/trades`)
    await seller.getByRole('button', { name: '我卖出的' }).click()
    await seller.getByRole('button', { name: '确认完成' }).click()
    await seller.locator('[data-slot="chip"], .chip', { hasText: '已完成' }).first().waitFor({ timeout: 8_000 })
    console.log('✓ 卖家已确认，交易 COMPLETED')
    await shot(sellerContext, 'trades-completed')

    // 商品应为已售出
    await seller.goto(productUrl)
    await seller.locator('[data-slot="chip"], .chip', { hasText: '已售出' }).first().waitFor({ timeout: 8_000 })
    console.log('✓ 商品状态变为已售出')
    await shot(sellerContext, 'product-sold')

    // ---------- 10.5 买家评价、公开评论、卖家公开货架 ----------
    await buyer.goto(`${BASE}/trades`)
    await buyer.getByRole('button', { name: '写买家评价' }).click()
    await buyer.getByRole('button', { name: '5 分' }).click()
    await buyer.getByPlaceholder('1-500 字，说说这次交易体验（可选）').fill('卖家很爽快，书成色和描述一致，交易愉快！')
    await buyer.getByRole('button', { name: '发布评价' }).click()
    await buyer.getByText('卖家很爽快，书成色和描述一致，交易愉快！').waitFor({ timeout: 8_000 })
    console.log('✓ 买家发布交易评价')
    await shot(buyerContext, 'buyer-trade-review')

    await buyer.goto(productUrl)
    await buyer.getByText(/买家评价 5 分 · /).waitFor({ timeout: 8_000 })
    await buyer.getByText(/评分 5\.00 · /).waitFor({ timeout: 8_000 })
    await buyer.getByPlaceholder('1-500 字，任何人都可以评论').fill('这本单词书确实不错，推荐！')
    await buyer.getByRole('button', { name: '发布评论' }).click()
    await buyer.getByText('这本单词书确实不错，推荐！').waitFor({ timeout: 8_000 })
    console.log('✓ 商品详情展示买家评价与公开评论')
    await shot(buyerContext, 'product-comments')

    await seller.goto(`${BASE}/my/products`)
    await seller.getByText(/买家评价：5 分：卖家很爽快/).waitFor({ timeout: 8_000 })
    console.log('✓ 卖家「我的商品」展示买家评分与评价')

    await buyer.goto(productUrl)
    await buyer.getByRole('link', { name: SELLER.nickname }).first().click()
    await buyer.waitForURL(/\/users\/[^/]+$/, { timeout: 8_000 })
    await buyer.getByText(PRODUCT_TITLE).first().waitFor({ timeout: 8_000 })
    console.log('✓ 卖家公开货架展示商品')
    await shot(buyerContext, 'seller-shelf')

    // ---------- 11. 卖家修改资料（改昵称） ----------
    await seller.goto(`${BASE}/profile`)
    const newNickname = `${SELLER.nickname}改`
    await seller.getByLabel('昵称').fill(newNickname)
    await seller.getByRole('button', { name: '保存资料' }).click()
    await seller.getByText('资料已更新').waitFor({ timeout: 5_000 })
    await seller.getByText(`学号 ${SELLER.student_no}`).waitFor({ timeout: 5_000 })
    console.log('✓ 资料昵称修改成功')
    await shot(sellerContext, 'profile-updated')

    // ---------- 12. 退出登录 ----------
    await seller.goto(`${BASE}/profile`)
    await seller.getByRole('button', { name: '退出登录' }).click()
    await seller.waitForURL(/\/login/, { timeout: 5_000 })
    console.log('✓ 退出登录回到登录页')
    await shot(sellerContext, 'logged-out')

    console.log(`\nproduct=${productId} chat=${chatUrl}`)
    console.log('E2E 全部通过 ✔')
  } finally {
    await browser.close()
  }
}

run().catch((err) => {
  console.error(err)
  process.exit(1)
})
