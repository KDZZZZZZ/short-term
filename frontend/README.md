# short-term-web

校园二手交易平台 MVP 的 Web 前端。

## 技术栈

- Vite + React 19 + TypeScript
- HeroUI v3（Tailwind CSS v4，品牌主题见 `src/index.css`）
- TanStack React Query（服务端状态）
- Zustand（会话与主题的本地状态）
- react-router-dom v7
- Motion（进场动画）、lucide-react（图标）

## 接口约定

- 前端源码只请求相对路径 `/api/v1`（图片 `/media`），不写死后端地址。
- 开发时由 Vite 代理转发到联调后端（默认 `http://123.56.161.234:18083`，可用环境变量 `DEV_API_ORIGIN` 覆盖）。
- 生产环境由 Nginx 将同一路径转发到 `127.0.0.1:18083`，部署不改写前端源码。
- API 类型与 `openapi/openapi.yaml` 一一对应，见 `src/lib/types.ts`。

## 常用命令

```bash
npm ci            # 安装依赖
npm run dev       # 开发服务器（默认监听全部网卡，便于 Tailscale 联调）
npm run build     # 类型检查 + 生产构建
npm run lint      # oxlint
npm run e2e       # 浏览器端到端测试（需本地 Chrome + 已启动 dev server）
```

## E2E

`e2e/frontend-e2e.mjs` 使用系统 Chrome（playwright-core）驱动真实页面走完整业务闭环：
注册 → 发布商品（含图片）→ 下架/上架 → 搜索 → 收藏 → 会话聊天 → 购买意向 →
卖家接受 → 双方确认 → 商品售出 → 资料修改 → 退出登录。
截图输出到 `e2e/shots/`（已 gitignore）。
