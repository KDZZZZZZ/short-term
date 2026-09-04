# 前端栈复用说明(short-term-web)

> 目标读者:**另一个项目**的 agent 或开发者。读完即可把同一套前端立起来——不需要问原作者,不需要额外密钥。
> 读法:**§1 判断这套栈适不适合你** → **§2 选定架构并照那一级建目录** → **§3 照标准流程开工** → §4 查各库用法 → §5 抄骨架文件。
> 本文是**规范**:写的是这套栈该怎么用,照着写即可。**§4.5 含一个真实 API 凭证**(Koboyo,项目所有者授权写入);仓库转公开或多人可见前先轮换。

## 1. 这套栈是什么,什么时候不该用

### 1.1 依赖清单(实测版本,摘自 package.json)

| 包 | 版本 | 角色 |
|---|---|---|
| react / react-dom | ^19.2.8 | React 19 |
| @heroui/react + @heroui/styles | ^3.2.4 | HeroUI v3 组件库(组件与样式分包,CSS 变量主题) |
| @tanstack/react-query | ^5.102.8 | 服务端状态(请求/缓存/失效) |
| zustand | ^5.0.15 | 纯本地状态(会话、主题) |
| react-router-dom | ^7.18.3 | 路由(data router 模式) |
| motion | ^13.2.0 | 动效(framer-motion 后继包,`motion/react`) |
| lucide-react | ^1.40.0 | 常规线性图标 |
| vite + @vitejs/plugin-react | ^8.2.2 / ^6.1.0 | 构建与 dev server |
| tailwindcss + @tailwindcss/vite | ^4.3.3 | Tailwind v4(CSS-first,无 config 文件) |
| typescript | ~6.0.2 | 类型检查,三 tsconfig(`tsconfig.json` 引用 app/node 两份,build 跑 `tsc -b`) |
| oxlint | ^1.79.0 | lint(配置在 `.oxlintrc.json`) |
| playwright-core | ^1.62.1 | E2E(自写脚本,e2e/) |

```bash
npm i react react-dom @heroui/react @heroui/styles @tanstack/react-query \
      zustand react-router-dom motion lucide-react
npm i -D vite @vitejs/plugin-react tailwindcss @tailwindcss/vite typescript oxlint
```

scripts:`dev`(vite)/ `build`(`tsc -b && vite build`)/ `lint`(oxlint)/ `preview` / `e2e`(`node e2e/frontend-e2e.mjs`)。

### 1.2 适用边界:纯 SPA 什么时候失效

这套栈是**纯客户端 SPA**:`index.html` 是个空壳(一个 `<div id="root">`),服务器只发静态文件,路由与取数全在浏览器里跑,部署 = 一个静态目录 + `/api` 反向代理。

**够用的判据**(本项目四条全中,所以选了 SPA):

- 内容在**鉴权门之后**,不需要被搜索引擎或社交分享卡片抓取;
- 用户是重复访问的"应用"用户,首屏几百毫秒白屏可接受(不是一次性落地页访客);
- 没有请求级服务端渲染需求——个性化在客户端拿完数据再渲染就够;
- 部署环境只保证静态托管,不保证有 Node 运行时。

**任一命中就别用纯 SPA:**

1. **SEO / 分享卡片**。公开商品页、博客、文档、营销落地页,爬虫和 `og:` 抓取拿到的是空 div。⚠️ 本项目最可能先失效的正是这条:一旦允许游客免登录浏览商品,商品详情页就需要真 HTML。
2. **首屏性能是业务指标**。广告落地页、弱网、低端机——SPA 要等 JS 下载 → 解析 → 请求往返才有内容,三段串行。
3. **有事必须在服务端做**。密钥不能进浏览器的第三方调用、按用户裁剪数据(而非拿到全量再过滤)、HttpOnly cookie 会话(SPA 把 token 放 localStorage,安全模型天然更弱)。
4. **内容多且基本静态**。文档站、官网——SSG 预渲染更省,SPA 是纯浪费。
5. **严格 CSP、无 JS 降级、邮件深链首屏**这类硬约束。

**失效后换什么:**

| 场景 | 换成 | 代价 |
|---|---|---|
| 要 SEO,内容基本静态 | Astro 或 Next SSG | 组件仍是 React,改构建与路由 |
| 要 SSR / 服务端取数 / 流式 | **React Router v7 framework mode** | 成本最低——已经在用 react-router 7,data router → framework 是同一个库的配置切换 |
| 桌面/移动壳(Electron / Tauri / Capacitor) | 就用纯 SPA | 无——这类场景 SSR 反而是负担 |

**关键:SPA 专属的只有两个文件**——`index.html` 的挂载点和 `src/app/router.tsx`。`index.css` 主题层、HeroUI、TanStack Query、zustand、`lib/http.ts`、`lib/api/*`、§2 的目录约定与全部红线**原样保留**。所以本文其余部分与 SPA 无关。

真要迁到 SSR,只有三处必须动(都是"模块加载时摸浏览器 API"):

- `stores/theme-store.ts` 的 `initialMode()` 直接读 `localStorage` / `document` → 服务端没有,且会闪屏。改成 cookie 存主题 + 服务端把 `class="dark"` 渲进 `<html>`。
- `lib/http.ts` 的 401 处理用 `window.location.assign` → 服务端要换成框架的 redirect。
- react-query 要补 `dehydrate` / `HydrationBoundary`,否则服务端取的数据到客户端会重取一遍。

## 2. 架构取舍:三级,选一级做

这套栈支持三种目录架构,**它们共享同一组红线(§2.2),只在"东西放哪、谁能 import 谁"上不同**。先用 §2.1 定位自己在哪一级,再照对应小节(§2.3 / §2.4 / §2.5)动手。默认从 **A** 开始,绝大多数项目一直待在 A 或 B。

| | A 扁平三层 | B 三层 + features | C 完整 FSD |
|---|---|---|---|
| 目录 | pages / components / hooks / lib / stores | + `features/` | app / pages / widgets / features / entities / shared |
| 切分依据 | 按**技术种类** | 主体按技术种类,**动作块按功能** | 全部**按功能**,层内再按 slice |
| 边界靠什么守 | 人 + code review | 人 + 一条准入规则 | **必须上机器强制**(lint 插件) |
| 适合 | 默认起点 | 出现带取数的复用动作块 | 多人并行 + 领域已稳定 |
| 迁入成本 | — | 低,可一个功能一个功能地渐进 | 高,且不可半途而废 |

### 2.1 怎么判断自己在哪一级

**先排除错误的触发器:页面数、代码行数、"感觉乱了"都不是理由。** 50 个页面的 CRUD 用 A 可以一直很舒服;15 个页面的应用如果每加一个功能都要横跨五个目录,就已经该升级了。真正的判据是**修改的局部性**——改一个功能要碰几个目录。

**五个可观察的症状**,中一条记一分:

1. **`components/` 里出现 `useQuery` / `useMutation`**。最硬的信号。扁平三层假设 `components/` 是纯展示件(接 props、渲染);组件一旦自带取数,它就不是展示件而是一个 **feature**——有自己的数据、失效逻辑、加载态。这类东西和纯展示件混在同一个目录,"谁拥有取数"就没人守得住。
2. **同一实体的展示件散落且形态不一致**。`ProductCard` / `ProductRow` / `ProductMiniCard` 分居三处,后端加一个字段要改三个地方。这是 **entity 层**的信号。
3. **加一个功能要跨 5 个目录**。加"举报商品"要碰 `lib/types.ts`、`lib/api/`、`components/`、`pages/`、`stores/`。按技术种类分目录,意味着按功能修改时永远是散射的。
4. **出现真实循环依赖**,或为绕开循环开始做别扭的类型下沉、依赖注入、事件总线。
5. **两人以上天天在 `components/` 撞车**,目录成了并行开发的争用点。

**计分 → 选级:**

- **0~1 分 → 留在 A**。不要提前优化目录。
- **2~3 分 → 升 B**。而且只升 B,别直接跳 C。
- **4~5 分 → 才考虑 C**,且必须同时满足下面两个前提;不满足就停在 B,B 能撑很久。

**升 C 的两个前提(缺一不可):**

- **领域边界已经稳定**。FSD 要求你先能回答"什么是 entity、什么是 feature"。探索期切分,切错的代价比不切高——迁回来比迁过去贵。**FSD 是领域稳定后的产物,不是探索期的工具。**
- **愿意上机器强制**。不打算加 `steiger` 或 `eslint-plugin-boundaries` 去自动拦截跨层 import,就别迁 C:没有机器守边界的分层三个月内必然腐烂,你只会得到更多目录和同样的混乱。人靠自觉守不住六层依赖规则,这不是纪律问题,是概率问题。

**即使规模很大也别升级的三种情况:**

- 领域还没稳定(见上);
- **页面多但每页都简单的 CRUD**——页面数 ≠ 复杂度;
- 长期 1~2 人且不打算扩——症状 5 永远不出现,症状 3 靠全局搜索就扛过去了。

### 2.2 不变量:八条红线(三级架构都适用)

**这八条与选哪级架构无关**,升级架构时它们一条都不变(第 1、6 条的具体形态随架构调整,各架构小节给自己的版本):

1. **依赖方向单向,不出现循环**。具体的层序与合法例外见 §2.3 / §2.4 / §2.5。页面互相不 import;跳转一律 navigate,数据共享走 store / query 缓存。
2. **数据访问唯一入口**。`lib/http.ts` 是唯一网关,`lib/api/<resource>.ts`(C 中为 `entities/<x>/api`)每资源一模块,**URL、query key、query/mutation hooks 全部封装在模块内**;页面只 import hook,组件与页面禁止直接 `fetch`,也不在页面里手写 queryKey 数组。
3. **query key 由资源模块的 key 工厂产出**,不散落字符串。构型:`all` 前缀 → `list(params)` → `detail(id)` → 子资源再加一段。写操作失效时按前缀一次性 invalidate,列表与详情同时刷新(写法见 §4.3)。
4. **状态三分**。服务端数据 → react-query;跨页且需持久化的本地状态(会话、主题)→ zustand;其余 → 页面 `useState`。接口数据不进 zustand(`auth-store.user` 例外:随 token 持久化的会话身份,不是服务端缓存)。
5. **鉴权只发生在两处**。守卫重定向 + `http.ts` 的 401 全局登出;页面不自判登录态(读 store 做展示可以)。
6. **共享件有准入门槛**,不是"可能会复用"就上移。门槛按架构定,见各小节。
7. **路由唯一源** `src/app/router.tsx`;守卫只做重定向,不做业务。
8. **类型单一来源** `lib/types.ts`(对齐后端契约),依赖只进不出——`lib/` 不从 `components/` 反向 import 类型,共享类型一律下沉。组件 props 就近定义。样式 / 主题 / 动效规则见 §5.2 与 §4.2,不在架构层另立。

### 2.3 架构 A｜扁平三层(默认起点)

**模型**——按技术种类分层,三层单向:

```
pages(路由落点,组织本页布局与交互)
  ↓
components / hooks(共享件:纯展示组件、跨页 UI 逻辑)
  ↓
lib / stores(数据网关与资源模块、类型、格式化;跨页本地状态)
```

**目录:**

```
src/
├─ main.tsx                  # Provider 链(§5.3)
├─ index.css                 # 主题层(§5.2)
├─ app/router.tsx            # 路由唯一源(§5.5)
├─ pages/<domain>/           # 页面,一页一文件,<name>-page.tsx
├─ components/               # 共享展示组件(领域内聚,如 layout/app-layout.tsx)
├─ components/icons/         # 图标源码(§4.5)
├─ hooks/                    # 跨页复用的纯 UI 逻辑,use-auto-resize-textarea.ts 等
├─ lib/                      # http.ts、types.ts、format.ts、<资源>-validation.ts
├─ lib/api/<resource>.ts     # 每资源一模块:请求 + query key + hooks(§4.3)
└─ stores/                   # zustand(§4.3)
```

**依赖方向(红线 1 的 A 版本)**,合法例外只有三处——`app/router.tsx`(路由表持全部 pages)、守卫(`require-auth` / `guest-only`)、页面外壳(`layout/`):

```
pages ──→ components / hooks / lib / stores
components(守卫、外壳除外)──→ hooks / lib
hooks / stores ──→ lib          lib ──→ 不向上
```

**共享件准入(红线 6 的 A 版本)**:进 `components/` 需 **≥2 页复用**或属领域通用件;单页用的留在页面文件里,**等第二个调用方出现再移**。

**这一级的成本近零的整理手段**:`components/` 内部按域分子目录(`layout/` `auth/` `icons/`)。大部分项目在这里能撑很久,不要因为目录变长就急着升级。

**升级信号**:§2.1 计分到 2 分,尤其是症状 1(`components/` 里出现取数)。

### 2.4 架构 B｜三层 + features(推荐的第二级)

吃掉 FSD 约八成收益,只花两成成本。**规则只加一条:带自己取数或跨页状态的动作块进 `features/`,`components/` 只留纯展示。**

```
src/
├─ app/  pages/  components/  hooks/  lib/  stores/     # 与 A 完全相同
└─ features/<action>/          # 新增:一个动作一个目录
   ├─ index.ts                 # 唯一出口,只导出对外那一个组件/hook
   ├─ favorite-button.tsx      # UI
   └─ use-toggle-favorite.ts   # mutation + 乐观更新 + 失效
```

**什么进 features(红线 6 的 B 版本)**——两条同时满足:

- 它**自带数据或跨页状态**(有 `useMutation` / `useQuery` / 写 store),不是纯接 props 渲染;
- 它**≥2 个页面要用**(或下一个迭代明确会用)。

只满足第二条(纯展示、≥2 页)→ 仍然进 `components/`。只满足第一条(带取数但单页用)→ 留在页面里。

**依赖方向(红线 1 的 B 版本)**:

```
pages ──→ features / components / hooks / lib / stores
features ──→ components / hooks / lib / stores
components / hooks ──→ lib          lib ──→ 不向上
```

两条硬规则:**features 之间不互相 import**(要共用就下沉到 `components/` 或 `lib/`);**`components/` 不能 import `features/`**(方向仍单向,展示件不依赖动作块)。

**怎么迁**:不需要停下来大搬家。**下一次某个动作块出现第三个调用方时,就地切一个 `features/<action>/` 出来**,把三处重复的 mutation 收进去,三个页面都改成 import 那一个组件。一个功能一个功能地做,随时可停。

**升级信号**:`features/` 超过十来个且开始按实体聚族(一堆 `xxx-product` / 一堆 `xxx-order`),同时症状 2(实体展示件散落)明显 → 才轮到 C。

### 2.5 架构 C｜完整 FSD(领域稳定 + 多人并行才上)

Feature-Sliced Design(feature-sliced.design)。**六层,自上而下,一层只能 import 严格更低的层,同层之间不互相 import**:

```
app        # 入口、Provider 链、路由、全局样式
pages      # 路由落点,只做组装
widgets    # 自足的大块(带筛选的商品列表区、评论区)
features   # 用户动作(收藏、下单、发消息)
entities   # 业务实体的表示与其 api(product / user / order)
shared     # 与业务无关的:UI kit、http 网关、工具、类型
```

每层内部切 **slice**(按业务域,如 `entities/product`),slice 内部切 **segment**(`ui/` `model/` `api/` `lib/`)。`shared` 与 `app` 没有 slice。本栈映射过来:`lib/http.ts` → `shared/api`;`lib/api/<resource>.ts` → `entities/<resource>/api` + `model`;`components/` 拆散进 `shared/ui`(通用)、`entities/*/ui`(实体展示)、`widgets`(大块)。

**机器强制是必需项,不是可选项**(§2.1 的前提之二):装 `steiger`(FSD 官方 linter)或 `eslint-plugin-boundaries`,把层序写成规则接进 CI。没有这一步就别开始。

**渐进迁移路径**(FSD 官方推荐顺序,别推倒重来):

1. 先立 **`app/` 与 `shared/`**——把 Provider 链、路由、http 网关、通用 UI 挪进去,这一步收益立刻可见且低风险;
2. 再立 **`pages/`**(基本是现成的,改路径即可);
3. 最后按需要逐个引入 **`widgets/` → `entities/` → `features/`**,**用不到的层就不建**——FSD 不要求六层全开。

**别在这两种情况下选 C**:领域边界还没稳定;或团队不打算维护 lint 边界规则。这两条任一不满足,B 是更优解,而且不丢脸——FSD 官方文档自己的立场就是"当前架构没造成麻烦就不值得改"。

### 2.6 怎么推导你自己的目录名(三级通用)

树的**形状**可以照抄,`market` / `chat` / `trades` 这些名字不行:

- **domain 来自后端资源或用户任务,不是来自 UI 形状。** 本仓库的 domain(auth / market / product / chat / trades / favorites / profile)一一对应后端资源;别按"列表页 / 表单页"这种 UI 形状分组。
- **资源模块名 = 后端资源名,与 OpenAPI 一一对应。** 这是全栈里唯一"抄后端"的地方,照抄能让契约漂移立刻暴露。
- **domain 只有 1~2 个时,`pages/` 扁平不建子目录**,页面文件直接 `pages/<name>-page.tsx`。分组是规模的产物,不是纪律。
- **`app/` `components/` `hooks/` `lib/` `stores/` 对任何项目同名同职责**,直接建空目录。

文件名 kebab-case,组件导出 PascalCase,`@/` alias → `src/`。

### 2.7 本仓库现在在哪一级

**A,且五个症状一条未中**:`components/` 里 `useQuery` / `useMutation` / `apiFetch` 零命中(纯展示层),无循环依赖,单人开发。规模 14 页 / 12 共享组件 / 6 资源模块 / 2 store / 2 hook——这是"A 完全够用"的标定点。

**最先会冒头的是症状 1**,具体位置是收藏:`addFavorite` / `removeFavorite` 在 `pages/favorites/favorites-page.tsx` 与 `pages/product/product-detail-page.tsx` 各写了一遍 mutation,两处失效范围还不一样。**出现第三个调用方时**(比如商品卡片上直接加收藏按钮),按 §2.4 切出 `features/toggle-favorite/` ——这会是本项目第一个真正的 feature slice。在那之前,A 不用动。

## 3. 通用开发步骤

### 3.1 起步:从空目录到第一条路由

1. `npm create vite@latest my-app -- --template react-ts`;
2. 装依赖(§1.1 两条命令);
3. 抄 §5 的四个骨架文件:`vite.config.ts` / `src/index.css` / `src/main.tsx` / `src/stores/theme-store.ts`;
4. **只改主题变量的值**(§5.2 里 `:root:not(.dark)` 与 `:root.dark` 两个分支),变量名一个不动——换品牌到此为止,组件零改动;
5. 按 §2.6 定名、照 §2.3 的树建空目录(**新项目一律从架构 A 起步**,别提前上 B/C);
6. 写 `lib/types.ts`(照后端 OpenAPI)与 `lib/http.ts`(响应信封按你后端的实际形状改);
7. 一个页面 + `app/router.tsx` 一条路由,跑通 `npm run dev`;
8. 把 `.heroui-docs/` 与 `.heroui-AGENTS.md` 一并拷过去(§4.1)——**agent 离线可查组件文档,这是复用成功的关键配套**。

### 3.2 加一个功能:七步

每加一个功能都走同一条路,顺序别换(每步依赖前一步):

1. **定契约**。后端 OpenAPI → 往 `lib/types.ts` 补类型。前端不发明字段;字段对不上就回去改契约,不在前端兜。
2. **建 / 补资源模块**。`lib/api/<resource>.ts`:先加请求函数(只管 URL、method、query),再往 key 工厂里加一条 key,最后导出对应的 `useXxx` hook(红线 2、3)。
3. **想清失效关系**。写操作的 `onSuccess` 里,把这次写会弄脏的前缀一次 invalidate 干净——改一个商品,列表和详情都得刷。这一步在写页面**之前**想,写完再补最容易漏。
4. **建页面**。`pages/<domain>/<name>-page.tsx`,只 import 上一步的 hook,页面里不出现 `apiFetch` 也不出现 queryKey 数组。
5. **挂路由**。`app/router.tsx` 加一条;需要登录的放进 `RequireAuth` 子树,页面自身不写鉴权判断。
6. **抽共享件**。先全写在页面里;等第二个调用方出现时再按红线 6 上移——纯展示件 → `components/`,带取数或跨页状态的动作块 → `features/`(架构 B 起,见 §2.4)。
7. **自检**(§3.3)。

### 3.3 提交前自检

- `npm run lint && npm run build`(`tsc -b` 会跑全量类型检查);
- `npm run e2e`(playwright 脚本,产出 `e2e/shots` 与 `e2e/shots-dark`);
- **深浅色各看一遍**——主题变量漏定义或组件写了内联色值,几乎只在暗色下暴露;
- 系统"减少动态效果"打开再看一遍关键动画(§4.2 已全局兜底,自定义 CSS 动画要自查);
- 顺手对一遍 §2.1 的五个症状——**架构升级是被症状触发的,不是被日程触发的**。

## 4. 各库的使用模式

### 4.1 HeroUI v3

- 组件从 `@heroui/react` 单包导入(`Button` `Card` `Modal` `Tabs` `Toast` 等);样式全靠 §5.2 的 CSS 变量,**组件不传色值**。
- **文档内置在仓库**:`.heroui-docs/react`(全部组件 mdx + 每组件数十个 demo tsx + getting-started + releases)+ `.heroui-AGENTS.md`(官方 agent 索引,由 `heroui agents-md --react --output .heroui-AGENTS.md` 生成)。复用项目把这两个一并拷走,agent / 新人**离线可查**,不依赖外网。

### 4.2 motion(动效)

- 全局 `MotionConfig reducedMotion="user"`(§5.3),页面内正常写动画即可,可访问性自动兜底。
- 只用 transform / opacity 做动画,不动布局属性。

### 4.3 TanStack Query + zustand(状态分工)

**服务端状态 → react-query,一个资源一个模块,模块自己持有 key 与 hooks:**

```ts
// src/lib/api/products.ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/http'
import type { Identifier, Product, ProductPage } from '@/lib/types'

/** query key 唯一来源:失效按前缀匹配,调用方不必记字符串。 */
export const productKeys = {
  all: ['products'] as const,
  list: (params: ListParams) => [...productKeys.all, 'list', params] as const,
  detail: (id: Identifier) => [...productKeys.all, 'detail', id] as const,
}

export function useProducts(params: ListParams) {
  return useQuery({
    queryKey: productKeys.list(params),
    queryFn: () => apiFetch<ProductPage>({ path: '/products', query: params }),
  })
}

export function useProduct(id: Identifier) {
  return useQuery({
    queryKey: productKeys.detail(id),
    queryFn: () => apiFetch<Product>({ path: `/products/${encodeURIComponent(id)}` }),
  })
}

export function useUpdateProduct() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UpdateProductInput) =>
      apiFetch<Product>({ method: 'PATCH', path: `/products/${input.id}`, json: input.body }),
    // 一次失效整棵前缀:列表与详情同时刷新,不漏
    onSuccess: () => void qc.invalidateQueries({ queryKey: productKeys.all }),
  })
}
```

页面侧只有一行:`const { data, isPending } = useProducts({ keyword, category, page })`。

跨资源的派生查询(如未读数轮询)放 `hooks/`,内部同样只调资源模块导出的东西。

**纯本地状态 → zustand**:会话 token(`stores/auth-store.ts`)与主题(`stores/theme-store.ts`),不与服务端状态混用。

### 4.4 HTTP 层 — `src/lib/http.ts`

统一 `apiFetch<T>`:响应信封 `{ code: 'OK', data }` 才算成功;失败抛 `ApiError { status, code, details }`;**401 统一处理**——清除本地会话并跳 `/login?expired=1&from=<当前页>`,登录 / 注册接口用 `public: true` 跳过(密码错误是业务错误,不该触发全局登出);写操作可带 `Idempotency-Key`(`newIdempotencyKey()`);query 对象经 `buildQuery` 序列化(自动跳过空值与非标量)。

### 4.5 图标(lucide + Koboyo 手绘图标库)

- 常规 UI 图标:`lucide-react`。
- 手绘风品牌图标:来源是 **Koboyo 图标库**(免费商用、无需署名)。本项目已取 4 个,以源码收进 `src/components/icons/koboyo.tsx`(`MarketStallIcon` / `EmptyBoxIcon` / `HandshakeDealIcon` / `SpeechBubbleAlertIcon`)。特性:`fill="currentColor"`(自动跟主题色)、手绘比例不一(viewBox 各异,如 163×151),**只设一个维度**(如 `h-10 w-auto`)。

**要取新图标,通过 Koboyo MCP——连接配置如下(含访问凭证,拷走即用):**

```json
{
  "mcpServers": {
    "koboyo": {
      "type": "http",
      "url": "https://api.koboyo.com/v1-mcp",
      "headers": {
        "Authorization": "Bearer kbi_491b7c8f12d70f56d2fbc20b0797966a4e3d98b4246e12436f9671aa67770546"
      }
    }
  }
}
```

任何支持 Streamable HTTP MCP 的客户端都按"URL + Authorization 头"接入(Claude Code 示例:`claude mcp add --transport http koboyo https://api.koboyo.com/v1-mcp --header "Authorization: Bearer <上面的 key>"`)。

| 工具 | 参数 | 说明 |
|---|---|---|
| `search_icons` | `query`(1-80 字,如 "market stall"),可选 `category`、`limit` | 搜索,**免费**;返回 slug / 名称 / 分类 / viewBox 宽高;多词需全部命中,无结果时容错回退 |
| `find_icons_for` | 多个主题 | 一次搜多个主题,别反复调 `search_icons` |
| `get_icon_svg` | `slugs`:1-20 个 slug | 拉取 SVG 源码;**每成功 slug 扣 1 个免费额度**,不存在的 slug 不扣 |
| `list_categories` / `list_icons` / `get_icon` / `get_library_info` / `whoami` | — | 分类 / 列表 / 详情 / 授权条款 / 账号信息 |

取图流程:**先搜后取**,省免费额度——

1. `search_icons { "query": "coffee" }` → 拿到候选 slug(免费);
2. 挑定后 `get_icon_svg { "slugs": ["coffee-cup"] }` → 此刻才扣额度,返回 SVG 源码(单色 `fill="currentColor"`,带 viewBox、不带宽高);
3. 清理:确认 `xmlns`、保留 viewBox 与 `fill="currentColor"`、去掉宽高;
4. 包成 `XxxIcon` 组件追加进 `koboyo.tsx`(照现有 4 个的写法)。

## 5. 骨架文件(整段复制)

起点是任何能跑的 Vite + React + TS 项目。以下五个文件替换 / 新增完毕,即得到 §2.3 描述的架构 A 骨架;升 B / C 时这五个文件本身不变(只有 §5.5 路由里的 import 路径跟着挪)。

### 5.1 `vite.config.ts`

```ts
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(__dirname, 'src') } },
  // 可选:后端联调代理。前端只请求相对路径 /api/v1 与 /media,
  // dev 时转发到后端 origin(用 DEV_API_ORIGIN 覆盖),生产由 Nginx 转发。
  server: {
    proxy: {
      '/api/v1': { target: process.env.DEV_API_ORIGIN ?? 'http://你的后端:端口', changeOrigin: true },
      '/media':  { target: process.env.DEV_API_ORIGIN ?? 'http://你的后端:端口', changeOrigin: true },
    },
  },
})
```

### 5.2 `src/index.css`(整套栈的心脏)

```css
@import "tailwindcss";
@import "@heroui/styles";

/* 把任意私有变量提升为工具类:内联声明使工具类引用 var() 本身
   (而非构建期取值),因此 html.dark 一翻转,所有工具类跟着变 */
@theme inline {
  --color-header: var(--header-background);
  --color-header-foreground: var(--header-foreground);
  --color-header-hover: var(--header-hover);
  --color-header-active: var(--header-active);
  --color-header-active-foreground: var(--header-active-foreground);
  --color-header-border: var(--header-border);
  --color-highlight: var(--highlight);   /* 价格/高亮色 */
}

/* 主题 = 两个变量分支,换品牌只改值,组件零内联色值 */
:root:not(.dark) { color-scheme: light;  --background: #f7eae0; --foreground: #5e3122; --accent: #1d4533; /* … */ }
:root.dark       { color-scheme: dark;   --background: #030905; --foreground: #edf7f0; --accent: #97e0aa; /* … */ }
```

变量词汇表(两个分支都定义同名变量,组件只认名字不认值):

- 基础:`--background` `--foreground` `--muted`
- 面层:`--surface`(含 `-foreground` `-secondary` `-tertiary` `-hover`)`--overlay`
- 品牌色:`--accent`(含 `-foreground` `-hover` `-soft` `-soft-foreground` `-soft-hover`)
- 中性默认:`--default`(同上派生)
- 表单:`--field-background` `-foreground` `-placeholder` `-border` `-border-hover` `-border-focus` `-focus` `-hover`
- 线与分隔:`--border`(含 `-secondary` `-tertiary`)`--separator`(同)
- 状态:`--success` `--warning` `--danger`(各含 `-foreground` `-soft` `-soft-foreground`);`--danger-hover`
- 焦点 / 链接 / 高亮:`--focus` `--link` `--highlight`
- 顶栏:`--header-background` `-foreground` `-hover` `-active` `-active-foreground` `-active-hover` `-border`
- 阴影:`--surface-shadow` `-hover` `--overlay-shadow` `--field-shadow`;圆角 `--radius` `--field-radius`

文件内还有三块现成工具,可整段拷走:`@utility card-interactive`(悬停上浮 + 阴影加深 + 按压缩放)、`.tabular-nums`(价格列对齐)、全局 `prefers-reduced-motion` 压平(动画可访问性兜底)。

### 5.3 Provider 链 — `src/main.tsx`

```tsx
const queryClient = new QueryClient({
  defaultOptions: {
    queries:   { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 },
    mutations: { retry: 0 },
  },
})

<StrictMode>
  <QueryClientProvider client={queryClient}>
    {/* 系统开启"减少动态效果"时自动关闭 motion 动画 */}
    <MotionConfig reducedMotion="user">
      <Toast.Provider placement="bottom" />
      <RouterProvider router={router} />
    </MotionConfig>
  </QueryClientProvider>
</StrictMode>
```

### 5.4 主题切换 — `src/stores/theme-store.ts`(全文,35 行)

```ts
import { create } from 'zustand'

export type ThemeMode = 'light' | 'dark'
const STORAGE_KEY = 'st-theme'

function applyTheme(mode: ThemeMode) {
  const root = document.documentElement
  root.classList.toggle('dark', mode === 'dark')
  root.setAttribute('data-theme', mode)
}

function initialMode(): ThemeMode {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'dark' || saved === 'light') { applyTheme(saved); return saved }
  return 'light'
}

export const useThemeStore = create<ThemeState>()((set, get) => ({
  mode: initialMode(),
  toggle: () => {
    const next: ThemeMode = get().mode === 'dark' ? 'light' : 'dark'
    localStorage.setItem(STORAGE_KEY, next)
    applyTheme(next)
    set({ mode: next })
  },
}))
```

切换即一个 `toggle()`,无闪屏(变量在 CSS 层生效,不经 React 重渲染)。`initialMode()` 在模块加载时直接读 `localStorage`,是本文件唯一的 SPA 假设,迁 SSR 要改(§1.2)。

### 5.5 路由与守卫 — `src/app/router.tsx`

结构是模板,页面名换成你自己的:

```tsx
export const router = createBrowserRouter([
  // 公开页:已登录则被 GuestOnly 踢回首页
  { path: '/login',    element: <GuestOnly><LoginPage /></GuestOnly> },
  { path: '/register', element: <GuestOnly><RegisterPage /></GuestOnly> },
  {
    element: <RequireAuth><AppLayout /></RequireAuth>,   // 外壳 + 鉴权包裹所有业务页
    children: [
      { index: true, element: <HomePage /> },
      { path: 'things/:thingId', element: <ThingDetailPage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
```

模式:守卫组件(`GuestOnly` / `RequireAuth`)只做"条件渲染 or 重定向",页面自身不感知鉴权(红线 5)。

## 6. 本文档不含什么

- 后端契约与实现(本仓库另有 `openapi/` 与 Go 服务,与前端栈无关)。
- 测试策略。本文只约定提交前跑 lint / build / e2e 冒烟(§3.3);单元与集成测试的分层由复用项目自定。
- Koboyo key 已按项目所有者要求写入 §4.5:个人凭证、仅用于取图;仓库将来转公开或多人可见,先轮换该 key。
