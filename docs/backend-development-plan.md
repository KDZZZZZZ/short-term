# 后端开发计划

## 文档信息

| 项目 | 内容 |
| --- | --- |
| 版本/状态 | 1.0 / M0–M6 已实现，生产验收由 main 部署 workflow 留证 |
| 日期 | 2026-09-01 |
| 依据 | [software-design.md](software-design.md)（目标架构）、[state-machines.md](state-machines.md)（状态与并发规则）、[openapi/openapi.yaml](../openapi/openapi.yaml)（公开契约）、根 `AGENTS.md`（Git 与验收治理） |
| 范围 | 仅后端：五个 Go 服务、数据库、内部 gRPC、Gateway REST 实现、测试与后端部署。前端、推荐算法、邮件通知不在范围内 |

## 1. 实现状态

| 里程碑 | 当前实现 |
| --- | --- |
| M0 | Go workspace、共享平台层、独立数据库/迁移、Proto/OpenAPI/Go CI 已就位 |
| M1 | Account、JWT、Gateway 认证和资料链路已实现 |
| M2 | Marketplace 商品、图片、本地对象存储与全部商品 REST 路由已实现 |
| M3 | Trade 状态机、Product→Trade 锁序、事务幂等快照、Outbox worker 已实现 |
| M4 | Favorite 服务、当前商品投影与 Gateway 路由已实现 |
| M5 | Messaging 会话/消息/已读/游标、Outbox worker 与 Gateway 聚合已实现 |
| M6 | 五个独立镜像、真实 readiness、限流、指标/Trace、容器 E2E、自动部署与回滚已实现 |

生产是否成功不由本表自证；以合入 `main` 后的 `Backend`、`Deploy production`
GitHub run、远端容器状态和公开 HTTP E2E 输出为准。

## 2. 总体原则

1. 契约先行：任何公开行为变化先改 OpenAPI/状态机；内部接口变化先改 Proto。实现不反向覆盖契约。
2. 一个任务、一个 `feature/*` 分支、一个 PR，squash 合入 `main`；下列里程碑内部再切分为小 PR。
3. DTO 只在传输边界；禁止跨服务 `common/dto`、`shared/domain`。
4. 每个里程碑结束必须留下可复现的验收证据（命令、退出码、结果），对应设计文档第 10 章测试矩阵。
5. 不声明未测量的性能结论；不引入未经确认的依赖和基础设施。

## 3. 里程碑

### M0 工程基座

目标：让后续每个服务的开发都有统一的构建、迁移、数据库和 CI 地基。

任务：

- 建立后端 Go CI workflow：`go vet`、`go test -race`（复用根 `package.json` 脚本）、`proto:check`  drift 门禁，并将必需检查加入 `main` 分支保护。
- 确定迁移工具并固化约定（现有 `0001_account.sql` 为纯 SQL 序号风格，需选型 goose / golang-migrate / atlas 之一并写入服务 README 或 AGENTS.md）。
- 本地开发基础设施：`docker-compose` 提供 PostgreSQL；按服务建独立 database（或独立 Schema + 独立账号），验证禁止跨库访问。
- 基础设施代码就位：结构化日志、OpenTelemetry、配置加载、gRPC deadline/重试拦截器。存放方式遵循设计文档约束（不进领域层、不建跨服务共享领域包；小工具允许每服务自持或单一内部 platform 模块，二选一并记录）。
- 统一错误映射骨架：领域错误 → gRPC status → HTTP/ErrorCode（按设计文档 7.3 表）。

验收：`npm run proto:check` 与 `go vet`/`go test` 在 CI 全绿；`docker compose up` 后各服务账号只能访问本库。

### M1 Account Service + Gateway 骨架（认证链路）

目标：打通注册 → 登录 → JWT → `/users/me` 全链路，建立 Gateway 全部横切机制。

任务：

- Account：application 层（Register/Login/GetUser/BatchGetUsers/UpdateProfile/ChangePassword）、postgres adapter、grpc adapter、`cmd/server`。
- 密码哈希采用 Argon2id，工作参数在目标 ECS 实测确定（对应安全设计 8.1）。
- JWT 签发与验证（遵守 RFC 8725）；**前置决策：logout 语义**（仅客户端丢弃 vs 服务端 jti 撤销，设计文档 11.3，需人类确认；若撤销则需 Token Revocation 表）。
- Gateway：HTTP server、OpenAPI 对齐的 HTTP DTO 与 mapper、JWT 认证中间件、请求校验、错误映射、trace_id 透传、gRPC 客户端（全部带 deadline）。
- Gateway 实现 `/auth/*`、`/users/me`、`/users/me/password`。

验收：针对上述路径的契约测试通过；密码不出现在任何响应与日志（日志检查）；非本人不可读写资料的安全测试通过。

### M2 Marketplace 商品域 + Gateway 商品路由

任务：

- Marketplace 迁移：product、product_image、outbox、idempotency 表；商品列表索引（status、created_at、id）。
- 商品领域与应用：发布、修改、上下架（只能动作接口改状态）、倒序列表（确定性次级键 id）、标题搜索、详情、`ListUserProducts`、`BatchGetProducts`。
- 图片：OSS adapter、最多三张、sort_order 1–3、对象删除失败可重试清理。**前置决策：单文件大小/MIME/尺寸限制需人类确认并先补 OpenAPI**（设计文档 8.1）。
- Gateway 商品路由与聚合：详情与列表批量补全卖家公开资料；详情补微信/QQ、禁止返回学号；登录态详情补 `is_favorited`（调 Favorite 的 `IsFavorited`，Favorite 未建前以固定 false 占位并在代码标注，M4 后切换）。

验收：商品路径契约测试；状态专项 1、5、6（新商品 ON_SALE、下架/上架往返、公开列表只含 ON_SALE）；卖家学号不泄漏测试；上传集成测试（真实 OSS 或本地兼容实现）。

### M3 Marketplace 交易域（核心一致性）

这是全项目风险最高的里程碑，单独占用一个阶段。

任务：

- Trade 领域与状态机，严格实现 state-machines.md 的权限与副作用表。
- 本地事务实现接受/取消已接受/双方确认的原子联动，统一 **Product -> Trade** 加锁顺序。
- Idempotency：`(actor_id, operation, idempotency_key)` 唯一约束；成功结果快照（schema_version + result_code）与领域写入同事务；重放优先于当前状态校验。
- Outbox 表与发布 worker（`cmd/worker` 同镜像不同启动命令）；**Broker 未确认前**，worker 先实现"读已提交事件 → 发布"接口，发布器用日志/空实现占位，Broker 选型确认后替换（记录为 Agent Self-Claimed 的临时方案）。
- Gateway 交易路由与错误映射（403/409/PRODUCT_NOT_AVAILABLE/TRADE_STATE_CONFLICT）。

验收（设计文档第 10 章逐项对应）：

- 并发接受同一商品多笔交易：仅一笔 ACCEPTED，商品 RESERVED，其余 CANCELLED。
- 响应丢失后同键重试：返回首次成功结果而非 409。
- 取消与第二次确认并发：仅一个合法转换提交，无部分副作用。
- 任一步骤失败整体回滚（故障注入测试）。
- 商品状态专项 2、3、4（RESERVED/回退 ON_SALE/SOLD 在全部投影查询可见）。

### M4 Favorite Service

任务：Favorite 迁移与领域；`(user_id, product_id)` 联合主键幂等 PUT/DELETE；添加时经 Marketplace 校验商品存在且非本人商品；Gateway 收藏列表聚合（先读关系，再一次 `BatchGetProducts` 补当前 status，禁止 N+1）；切换 M2 的 `is_favorited` 占位为真实调用。

验收：幂等与自我收藏拒绝测试；收藏列表返回当前商品 status 的契约测试。

### M5 Messaging Service

任务：会话与消息迁移；`GetOrCreateConversation` 经 Marketplace 获取 seller_id、拒绝自我会话、联合唯一约束；发消息、拉消息、已读（单向、幂等、只能标记对方消息）、未读数；Gateway 会话路由与聚合（批量补全商品投影与对方昵称）。

验收：参与者权限安全测试；已读幂等测试；会话商品投影携带当前 status。

### M6 横切收尾与后端部署

任务：

- 限流（登录、注册、发消息、上传、交易动作分别配置；阈值先保守取值并标注待压测校准）。
- 健康检查区分存活/就绪；核心指标（交易转换与冲突数、Outbox 积压等）与跨 REST/gRPC 的 trace 串联验证。
- 部署：各服务与 worker 的容器镜像（固定版本/摘要）、私有容器网络拓扑、迁移执行步骤、扩展现有 deploy workflow 或新增后端 workflow；当前 Gateway 只绑定主机回环并通过 SSH 在目标主机执行验收，接入公网时仅由 TLS 反向代理访问 Gateway。
- 性能基线：在目标 ECS 同环境对真实路径建立基线数据，只报告实测结果，不设虚构 SLA。
- E2E 主流程回归：注册 → 发布 → 收藏 → 会话 → 交易全状态机 → 完成。

验收：合入 `main` 后部署 workflow 成功，远端 Gateway 健康且在目标主机执行的全链路 E2E 通过；提供 GitHub run 日志、容器状态与真实服务器 HTTP 证据。

## 4. 前置决策清单（阻塞项）

以下事项在对应里程碑开工前需人类确认，来自设计文档 11.3 与本计划：

| 决策 | 阻塞里程碑 | 默认取向（未确认时） |
| --- | --- | --- |
| JWT logout 是否服务端撤销 | M1 | 仅客户端丢弃，不建撤销表 |
| 图片大小/MIME/尺寸限制 | M2 | 暂停图片接口，先补 OpenAPI |
| 公开列表是否展示 RESERVED | M2 | 不改契约，仍只返回 ON_SALE |
| 迁移工具选型 | M0 | 由首个实现 PR 提出并记录为 Agent Self-Claimed |
| 事件总线产品与语义 | M3（Outbox 发布器）/M6 | 日志占位发布器，Broker 后置 |
| PostgreSQL 版本与实例形态 | M0/M6 | 本地 compose 固定一个大版本 |
| 是否扩展 503/504 公开错误 | M6 | 不改契约，依赖故障返回 500/INTERNAL_ERROR |

## 5. 主要风险与对策

- M3 事务/幂等实现复杂：优先用数据库行锁与唯一约束而非分布式锁；并发测试先行（红绿重构）。
- Gateway 聚合延迟：只用批量 RPC 加并行查询，禁止 N+1；所有下游调用带 deadline。
- 多服务运维成本：单仓库、统一 CI、同镜像多命令（server/worker）；不引入服务网格与 Kubernetes。
- 事件语义超前设计：MVP 内无硬消费者，Outbox 先保证"可恢复、可重放"，Broker 选型不阻塞主链路。

## 6. 里程碑依赖

```text
M0 ──> M1 ──> M2 ──> M3 ──> M6
              │  └──> M4 ──┘
              └──────> M5 ──┘
```

M4、M5 依赖 M2 的商品批量查询与 Gateway 聚合机制，彼此独立可并行；M3 只依赖 M2，与 M4/M5 可并行。
