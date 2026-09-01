# 校园二手交易平台微服务软件设计文档

## 文档信息

| 项目 | 内容 |
| --- | --- |
| 版本/状态 | 0.5 / 已实现基线；架构取舍仍可评审 |
| 日期 | 2026-09-01 |
| 责任角色 | 项目维护者批准；Codex 起草 |
| 目标读者 | 后端、前端、测试、运维与后续架构评审人员 |
| 形成方式 | 需求驱动的目标设计 |
| 事实基准 | 当前 main 分支；公开契约为 [openapi/openapi.yaml](../openapi/openapi.yaml)，状态规则为 [state-machines.md](state-machines.md) |

### 修订记录

| 版本 | 日期 | 修订人/责任角色 | 修订内容 | 状态 |
| --- | --- | --- | --- | --- |
| 0.5 | 2026-09-01 | Codex / 项目维护者 | 将自动部署验收改为公网 API 三角色业务旅程，补齐跨 Marketplace/Messaging 的公开请求标识传递和失败回滚边界 | 实现基线 |
| 0.4 | 2026-09-01 | Codex / 项目维护者 | 同步 M0–M6 实现、独立容器、CI/部署/回滚、就绪、限流、指标和验收现状 | 实现基线 |
| 0.3 | 2026-09-01 | Codex / 项目维护者 | 明确商品可见性、联系方式不可删除、PENDING 下架保护、商品编辑状态和唯一购买意向；补齐会话校验、错误、图片、分页及限流契约 | 已确认产品语义；代理细节见下文 |
| 0.2 | 2026-09-01 | Codex / 项目维护者待评审 | 补全交易命令幂等结果与 Product/Trade 原子状态转换 | 草案 |
| 0.1 | 2026-09-01 | Codex / 项目维护者待评审 | 建立微服务、REST/gRPC、DTO、数据一致性及部署目标设计 | 草案 |

### 事实状态

- **Human Design**：系统采用微服务；前端继续使用 REST；同一用户可同时作为买家和卖家；实现收藏、站内文字聊天和交易状态流转；商品列表与商品详情返回商品状态；任意状态商品详情保持可见；联系方式只允许新增或修改、不得删除；存在 `PENDING` 意向时商品不得下架；`RESERVED`/`SOLD` 商品不得编辑字段或图片；同一买家对同一商品的重复请求归入同一购买意向；自动部署上线后必须通过公网 API 执行真实 E2E。
- **已批准基线**：OpenAPI 是公开 HTTP API 唯一真源；[状态机文档](state-machines.md)治理商品与交易的跨资源状态变化。
- **Agent Self-Claimed**：API Gateway、内部 gRPC、DTO 分层、数据库所有权、Outbox、五个独立镜像、单机 Podman/systemd 拓扑、服务器本地生成密钥、进程内固定窗口限流、主机回环指标端口与 release manifest 回滚均为代理选择的实现方案。学习期直接绑定明文 `0.0.0.0:18083`、使用三角色合成数据且保留 `19090` 回环亦为代理选择；正式流量前必须改为可信 TLS 入口。`(product_id, buyer_id)` 终生唯一及 create-or-get 的 `200/201` 细节、PENDING 期间冻结商品内容、默认昵称、任意状态可收藏、403/404 隐藏规则、图片连续排序、页码漂移说明和 429 映射亦由代理补充。

## 1. 背景、目标与范围

### 1.1 背景

校园二手交易平台需要支持学生账号、商品发布与检索、收藏、商品上下文聊天以及线下交易协商。前端需要稳定且易调试的 REST/JSON 契约，后端则希望从项目初期按独立业务能力拆分，为独立部署、故障隔离和后续扩展保留边界。

当前仓库已经实现 Gateway、Account、Marketplace、Messaging、Favorite 五个 Go 服务，
版本化内部 gRPC、独立数据库迁移、事务型 Outbox worker、容器级 CI 和单机生产部署。
本文同时描述已实现基线与仍明确后置的能力；具体上线事实必须以对应 GitHub deployment
run 和远端验收证据为准。

### 1.2 建设目标

1. 前端只依赖一个版本化 REST 入口，不感知后端服务拆分。
2. 后端服务按业务一致性边界拆分，服务间同步通信使用版本化 gRPC。
3. 商品与交易保持强一致，跨服务数据采用明确的事实源和最终一致策略。
4. HTTP DTO、gRPC DTO、应用命令、领域对象和持久化对象互相隔离。
5. OpenAPI 和 Protobuf 均通过 CI 防止契约漂移与破坏性变更。
6. 所有商品查询投影返回当前商品状态；交易中的商品使用 **RESERVED** 表示。

### 1.3 范围

范围内：

- 学号与密码注册登录、退出和修改密码；
- 用户昵称、微信和 QQ 联系方式；
- 商品发布、修改、上下架、最多三张图片、分类、列表、关键词搜索和详情；
- 收藏；
- 商品上下文的一对一文字会话、消息和已读状态；
- 购买意向、接受、拒绝、取消、双方确认以及商品联动状态；
- API Gateway、内部 gRPC、服务数据所有权、事件发布和可观测性边界。

范围外：

- 推荐算法和邮件通知；
- 在线支付、担保、物流和争议仲裁；
- 统一身份认证；
- 群聊、语音、视频和文件消息；
- 当前阶段的独立搜索服务、独立媒体服务和服务网格；
- 未经容量与可用性目标验证的 Kubernetes 或多地域部署。

### 1.4 约束与假设

| 类型 | 内容 | 影响 |
| --- | --- | --- |
| 已确认约束 | 前端使用 REST/JSON | API Gateway 必须稳定实现现有 OpenAPI |
| 已确认约束 | 公开 API 以 OpenAPI 为唯一真源 | 不从 Protobuf 反向生成或覆盖公开 OpenAPI |
| 已确认约束 | 商品与交易的耦合状态转换必须原子联动 | 接受、已接受交易取消和双方确认完成均在 Marketplace Service 本地事务中更新 Product 与 Trade |
| 已确认约束 | 用户没有固定买家/卖家角色 | 权限根据 seller_id、buyer_id 和当前用户动态判断 |
| 已确认约束 | 商品列表和详情返回 status | 所有 ProductSummary、ProductDetail 和嵌套商品投影保留状态字段 |
| 已确认约束 | 任意商品状态的详情保持可见 | 详情 404 只表示商品不存在；公开列表仍只返回 ON_SALE |
| 已确认约束 | 已填写联系方式不得删除 | 更新只接受非空微信/QQ；省略字段保持原值 |
| 已确认约束 | PENDING 是进行中的购买意向 | 存在任意 PENDING Trade 时商品不能下架 |
| 已确认约束 | RESERVED/SOLD 商品内容冻结 | PATCH、加图和删图均返回状态冲突 |
| 已确认语义 | 同一买家对同一商品复用购买意向 | 精确唯一键、终生范围和 200/201 返回由代理补充并进入 OpenAPI |
| 建议 | Go 微服务内部使用 gRPC | 需要建立 Protobuf、Buf 和生成代码治理 |
| 建议 | 每个服务独占自己的逻辑数据库 | 可以初期共享 PostgreSQL 集群，但禁止跨服务直接访问表或 Schema |
| 待确认 | 峰值请求量、数据规模、延迟和可用性目标 | 暂不能确定副本数、连接池、缓存及容量 |
| 待确认 | 事件总线产品和交付语义 | 本文只固定 Outbox、幂等消费和事件契约，不指定 Kafka、NATS 或其他产品 |

## 2. 需求与质量属性摘要

| 编号 | 需求或场景 | 验收标准 | 状态/证据 |
| --- | --- | --- | --- |
| FR-01 | 学号和密码注册登录 | 合法账号可注册、登录并取得访问令牌；密码不出现在响应或日志 | 已确认，OpenAPI |
| FR-02 | 用户维护资料及联系方式 | 用户只能读写自己的完整资料；联系方式只允许新增或修改为非空值，不得删除 | 已确认，OpenAPI |
| FR-03 | 发布与查询商品 | 支持分类、价格、描述、最多三张图片、倒序列表和标题搜索；所有状态详情可见，交易中/已售商品不可编辑 | 已确认，OpenAPI |
| FR-04 | 返回商品状态 | 商品列表、我的商品、收藏中的商品、交易商品投影和商品详情均携带 status | Human Design；OpenAPI Schema 已包含 |
| FR-05 | 收藏 | 任意状态商品可收藏，收藏和取消收藏幂等，不能收藏自己的商品 | 已确认，OpenAPI 与状态机 |
| FR-06 | 站内聊天 | 仅商品买卖双方访问会话；消息已读单向且幂等 | 已确认，OpenAPI 与状态机 |
| FR-07 | 交易流转 | 状态动作、操作者权限和商品副作用符合状态机；同一买家与商品只有一个购买意向 | 已确认语义；唯一键和 HTTP 细节为 Agent Self-Claimed |
| NFR-01 | 契约兼容 | OpenAPI lint/bundle 无漂移；Proto lint、breaking 和生成代码无漂移 | OpenAPI 已建立；Proto 为建议目标 |
| NFR-02 | 一致性 | 同一商品最多一笔 ACCEPTED 交易，商品与交易状态不分裂；重复幂等请求返回首次成功结果 | 已确认，OpenAPI 与状态机 |
| NFR-03 | 安全 | 内部服务不直接公网暴露；资源级授权在拥有资源的服务中执行 | 建议目标 |
| NFR-04 | 可观测性 | REST、gRPC、数据库和异步事件共享可关联的 trace_id | 建议目标 |
| NFR-05 | 性能 | 不虚构 SLA；先建立真实负载模型和同环境基线，再确定目标 | 待确认 |

## 3. 系统上下文与总体架构

### 3.1 目标架构

~~~mermaid
flowchart LR
    User[学生用户]
    Web[Web 前端]
    Gateway[API Gateway / BFF]
    Account[Account Service]
    Market[Marketplace Service]
    Message[Messaging Service]
    Favorite[Favorite Service]
    Bus[事件总线<br/>产品待确认]
    Object[对象存储]

    User --> Web
    Web -->|REST / JSON| Gateway
    Gateway -->|gRPC| Account
    Gateway -->|gRPC| Market
    Gateway -->|gRPC| Message
    Gateway -->|gRPC| Favorite
    Market -->|Outbox 事件| Bus
    Message -->|Outbox 事件| Bus
    Favorite -->|可选订阅| Bus
    Market -->|图片对象| Object
~~~

API Gateway 是唯一公网业务入口，负责 HTTP 协议、OpenAPI 请求/响应、认证入口、限流、错误映射和少量批量聚合。它不保存领域状态，也不决定交易能否接受等业务规则。API Gateway 作为客户端与微服务之间的集中入口、反向代理和聚合层，参考 Microsoft 的 API Gateway 指南。

内部服务只通过版本化 gRPC 和异步事件暴露能力。浏览器不直接访问 gRPC 服务。聊天 MVP 继续通过 OpenAPI 中的 REST 发送与拉取消息；若以后需要实时推送，可以在 Gateway 增加 WebSocket 边路，但不能替换或破坏现有 REST 契约。

### 3.2 服务边界

| 服务 | 职责 | 拥有的数据 | 明确不负责 |
| --- | --- | --- | --- |
| API Gateway | 公开 REST、JWT 解析、DTO 映射、批量聚合、限流、错误与追踪映射 | 无业务数据库；只允许短期技术缓存 | 交易状态机、商品权限、消息参与者规则 |
| Account Service | 学号、密码哈希、令牌会话策略、昵称、微信和 QQ | Account、Credential、可选 Token Revocation | 商品、收藏、会话和交易 |
| Marketplace Service | 商品、图片元数据、搜索、商品状态、交易及交易状态机 | Product、ProductImage、Trade、Idempotency、Outbox | 用户凭据、消息正文、收藏关系 |
| Messaging Service | 商品上下文会话、文字消息、已读状态 | Conversation、Message、Idempotency、Outbox | 商品和用户资料事实 |
| Favorite Service | 用户与商品的收藏关系 | Favorite、Idempotency | 商品详情和商品当前状态 |

**Agent Self-Claimed 决策：Account Service 同时拥有认证与资料。** 注册需要同时创建学号凭据和用户资料；在当前规模下拆成 Identity Service 与 Profile Service 会立即引入分布式注册事务，收益不足。达到独立团队、独立合规边界或认证负载显著独立增长时再复审。

**Agent Self-Claimed 决策：Product 与 Trade 同属 Marketplace Service。** 接受交易时必须原子更新目标交易、商品以及同商品的其他待处理交易。成熟的微服务边界原则也建议将需要强一致、调用频繁的功能放在同一边界内，避免聊天式调用和跨库事务。

### 3.3 数据事实源与聚合

| 数据 | 唯一事实源 | 其他服务如何引用 |
| --- | --- | --- |
| 用户身份与公开资料 | Account Service | 只保存不透明 user_id；Gateway 批量补全当前昵称或联系方式 |
| 商品、价格与状态 | Marketplace Service | 只保存 product_id；需要当前状态时进行一次批量查询 |
| 交易价格 | Marketplace Service 的 Trade.price_snapshot | 创建交易时从商品价格复制，此后不随商品价格变化 |
| 会话与消息 | Messaging Service | Conversation 保存 product_id、buyer_id、seller_id |
| 收藏关系 | Favorite Service | Favorite 保存 user_id、product_id，不复制商品真值 |

Favorites 和 Conversations 的 REST 列表需要商品标题、封面或状态时，由 Gateway 收集 product_id 后调用一次 Marketplace 批量查询，禁止逐条 N+1 RPC。这样可确保用户要求的商品 status 是当前事实，而不是事件延迟造成的旧快照。

## 4. DTO、领域模型与代码结构

### 4.1 模型转换边界

~~~mermaid
flowchart LR
    HTTP[HTTP Request / Response DTO<br/>JSON 与 OpenAPI]
    Proto[gRPC Protobuf DTO<br/>内部传输契约]
    App[Application Command / Query<br/>用例输入输出]
    Domain[Domain Entity / Value Object<br/>业务规则]
    Row[Persistence Row<br/>数据库映射]

    HTTP -->|Gateway Mapper| Proto
    Proto -->|Transport Mapper| App
    App --> Domain
    Domain -->|Repository Mapper| Row
    Row -->|Repository Mapper| Domain
    Domain --> App
    App --> Proto
    Proto -->|Gateway Mapper| HTTP
~~~

DTO 必须存在，但只存在于传输边界：

- HTTP DTO 位于 Gateway 的 transport/http/dto，只包含 OpenAPI 允许的字段和 JSON 约束。
- Protobuf 生成类型本身就是内部 gRPC DTO，放在 gen/go，不再复制一套 grpc-dto。
- Application Command/Query 表达用例，不携带 HTTP、Protobuf 或数据库标签。
- Domain Entity/Value Object 只表达业务不变量。
- Persistence Row 只表达数据库列、索引和可空性。
- Event DTO 单独版本化，不能复用数据库 Row 或公开 HTTP DTO。
- 禁止建立跨服务的 common/dto 或 shared/domain 包。

Protocol Buffers 官方建议 RPC API 消息与存储消息分开，并要求演进时保留已删除字段号、避免复用字段号。该原则也用于阻止生成类型进入领域层。

### 4.2 商品状态在 DTO 中的约束

| REST 查询 | DTO | status 规则 |
| --- | --- | --- |
| GET /products | ProductSummary.status | 必填；当前公开契约只返回 ON_SALE，因此仍显式返回 ON_SALE |
| GET /users/me/products | ProductSummary.status | 必填；按查询条件返回 ON_SALE、RESERVED、SOLD 或 OFF_SHELF |
| GET /favorites | FavoriteItem.product.status | 必填；允许返回已预留、已售或已下架商品的当前状态 |
| GET /products/{productId} | ProductDetail.status | 必填；返回 Marketplace Service 中的当前状态 |
| GET /conversations | ConversationProduct.status | 必填；由 Gateway 批量补全当前状态 |
| GET /trades 和 GET /trades/{tradeId} | TradeProduct.status | 必填；返回关联商品当前状态 |

**RESERVED** 的业务含义是：卖家已经接受一笔交易，商品正在交易中，尚未由双方确认完成。若未来希望公开 GET /products 同时展示 RESERVED 商品，需要先修改 OpenAPI 的列表语义和筛选规则；当前批准契约仍只展示 ON_SALE。

公开列表的状态过滤与详情可见性相互独立。`GET /products/{productId}` 对所有已认证用户返回
任意状态的现存商品，`OFF_SHELF`、`RESERVED` 和 `SOLD` 不因状态返回 404。详情仍按
OpenAPI 返回卖家联系方式且不返回学号；本次按人类决定不扩展 Trade 中的买家联系方式。

### 4.3 目标 Monorepo 结构

~~~text
short-term/
├─ openapi/                          # 公开 REST 唯一真源
├─ proto/
│  └─ shortterm/
│     ├─ account/v1/
│     ├─ marketplace/v1/
│     ├─ messaging/v1/
│     ├─ favorite/v1/
│     └─ events/v1/
├─ gen/go/                           # 生成代码，不手工修改
├─ services/
│  ├─ gateway/
│  │  ├─ go.mod
│  │  ├─ cmd/server/main.go
│  │  └─ internal/
│  │     ├─ transport/http/
│  │     │  ├─ handler/
│  │     │  ├─ dto/
│  │     │  ├─ mapper/
│  │     │  └─ middleware/
│  │     ├─ client/grpc/
│  │     └─ application/aggregation/
│  ├─ account/
│  │  ├─ go.mod
│  │  ├─ cmd/server/main.go
│  │  └─ internal/
│  │     ├─ domain/
│  │     ├─ application/
│  │     └─ adapter/{grpc,postgres}/
│  ├─ marketplace/
│  │  ├─ go.mod
│  │  ├─ cmd/{server,worker,migrate}/main.go
│  │  └─ internal/
│  │     ├─ domain/{product,trade}/
│  │     ├─ application/{command,query,port}/
│  │     └─ adapter/{grpc,postgres,event,oss}/
│  ├─ messaging/
│  └─ favorite/
├─ deploy/
├─ buf.yaml
├─ buf.gen.yaml
└─ go.work
~~~

每个可独立部署服务使用自己的 go.mod，根 go.work 只服务于本地协同开发。cmd 只负责配置、构造器注入和生命周期；业务代码位于 internal。默认使用手工构造器注入，除非依赖图复杂度实测证明需要引入 DI 框架。

## 5. 核心状态与流程

### 5.1 商品状态

~~~mermaid
stateDiagram-v2
    [*] --> ON_SALE: 创建并发布
    ON_SALE --> RESERVED: 卖家接受交易
    RESERVED --> SOLD: 双方确认完成
    RESERVED --> ON_SALE: 已接受交易取消
    ON_SALE --> OFF_SHELF: 卖家下架且不存在 PENDING
    OFF_SHELF --> ON_SALE: 卖家重新上架
    SOLD --> [*]
~~~

状态只能通过专用动作或交易状态机改变，不能通过通用商品 PATCH 直接赋值。所有返回商品投影的查询读取 Marketplace Service 的当前状态。`PENDING` 是进行中的购买意向但不预留商品，Product 仍为 `ON_SALE` 并可接收其他买家的唯一意向；只要存在任意 `PENDING` Trade，商品就不能下架或修改内容/图片。商品字段与图片只允许在 `ON_SALE`、`OFF_SHELF` 且没有 `PENDING` 时修改，在 `RESERVED`、`SOLD` 冻结。

图片顺序始终为连续的 `1..N`，`cover_url` 指向第一张图片。新增图片按上传顺序追加；删除后保持剩余图片相对顺序并重排，删除封面后下一张自动成为封面，无图时封面为 `null`。

### 5.2 商品列表和详情

1. Gateway 验证请求格式和访问令牌，生成或透传 trace_id。
2. Gateway 调用 Marketplace Service 的 ListProducts 或 GetProduct。
3. Marketplace Service 查询商品真值并返回 seller_id、商品字段和 status；任意状态商品详情均可见，只有资源确实不存在才返回 404。
4. Gateway 通过 Account Service 的批量接口补全公开卖家资料；详情按 OpenAPI 补全微信/QQ，禁止返回学号。
5. 登录用户的商品详情需要 is_favorited 时，Gateway 调用 Favorite Service 查询当前关系。
6. Gateway 将 gRPC DTO 映射为 ProductSummary 或 ProductDetail HTTP DTO，并再次按 OpenAPI 序列化。

Marketplace 是商品状态的唯一事实源。Gateway 不缓存状态，除非后续定义了明确的最大陈旧时间和失效策略。

### 5.3 唯一购买意向与创建流程

Trade 表达买家对一个商品挂牌的购买意向，不表达一次 HTTP 尝试。借鉴支付意向将一次
业务购买过程聚合到单一、可追踪资源的做法，本项目为 `(product_id, buyer_id)` 建立终生
唯一约束：首次调用创建 `PENDING` Trade 并返回 201；任何后续调用都返回同一 Trade 的
当前表示和 200，不改变状态、价格快照、会话绑定，也不产生新通知。`CANCELLED` 和
`COMPLETED` 仍为终态，因此重复创建不会重新开启已结束意向。这项限制能防止更换
`Idempotency-Key` 或卖家拒绝后反复创建记录；代价是同一买家不能对同一挂牌再次表达意向，
如将来需要重开，应新增受控动作而不是创建第二条 Trade。

业务唯一性与请求幂等是两层规则：

1. 相同 `Idempotency-Key` 优先重放首次成功的 HTTP 状态与响应体，因此首次 201 的重试仍为 201。
2. 不同 key 或无 key 的请求通过 `(product_id, buyer_id)` 命中既有意向并返回当前表示和 200。
3. 并发首次创建由唯一约束串行化；唯一冲突必须转为读取既有 Trade，不能泄漏为 500。
4. 首次创建先按统一顺序锁定 Product，验证 `ON_SALE` 后创建 Trade；商品下架和内容/图片更新使用同一把 Product 锁并验证不存在 PENDING，保证不会提交 `OFF_SHELF + PENDING` 或交错的商品版本。

请求携带 `conversation_id` 时，Marketplace 必须向 Messaging 获取或验证该会话的
`product_id`、`buyer_id`、`seller_id`，三者必须与本次商品、当前买家和商品卖家一致。
不存在或不可见返回 404，不匹配返回 409，校验失败不得创建 Trade。首次创建后会话绑定
不可由重复 create-or-get 请求修改；已有 Trade 时，显式值必须与已存绑定相同，省略字段
只读取既有 Trade。

### 5.4 接受交易的强一致与幂等流程

~~~mermaid
sequenceDiagram
    autonumber
    actor Seller as 卖家
    participant GW as API Gateway
    participant MK as Marketplace Service
    participant DB as Marketplace DB
    participant OB as Outbox Worker
    participant BUS as 事件总线

    Seller->>GW: POST /trades/{id}/accept
    GW->>MK: AcceptTrade(actor_id, trade_id, idempotency_key)
    MK->>DB: BEGIN
    MK->>DB: 若携带 key，查询或声明幂等记录
    alt 已存在 SUCCESS 结果
        DB-->>MK: 首次成功的规范化命令结果
        MK->>DB: COMMIT
        MK-->>GW: 重放首次成功结果
        GW-->>Seller: 与首次成功等价的 HTTP 响应
    else 首次请求或未携带 key
        MK->>DB: 按 Product -> Trade 顺序加锁
        MK->>DB: 校验卖家、PENDING、商品 ON_SALE
        alt 校验通过
            MK->>DB: 目标交易 -> ACCEPTED
            MK->>DB: 商品 -> RESERVED
            MK->>DB: 其他 PENDING 交易 -> CANCELLED
            MK->>DB: 写入 Outbox 事件
            MK->>DB: 若携带 key，保存 SUCCESS 与规范化命令结果
            MK->>DB: COMMIT
            MK-->>GW: ACCEPTED + 商品 RESERVED
            GW-->>Seller: HTTP 200
            OB->>DB: 读取已提交事件
            OB->>BUS: 发布 TradeAccepted / ProductReserved
        else 状态或权限冲突
            MK->>DB: ROLLBACK
            MK-->>GW: 领域错误
            GW-->>Seller: HTTP 403 / 409
        end
    end
~~~

事务内必须完成目标交易、商品状态、其他待处理交易、Outbox 以及本次成功响应对应的 Idempotency 写入。幂等键由 `(actor_id, operation, idempotency_key)` 唯一约束串行化；并发同键请求在首个事务完成后读取其结果。Marketplace 不保存 HTTP DTO，而是保存带 schema_version 的规范化命令结果快照和 result_code，Gateway 据此确定性重建首次 HTTP 状态与响应体。重放检查发生在当前交易状态校验之前，因此即使交易后来已进入其他状态，也必须返回首次成功结果。失败事务不保留成功结果，领域写入与幂等记录任一失败都整体回滚。

事件只能在提交后投递。Outbox 允许至少一次投递，因此消费者必须根据 event_id 或业务幂等键去重。

### 5.5 取消与确认完成的强一致流程

所有同时改变 Product 与 Trade 的动作都使用 Marketplace DB 的单个本地事务，并统一按 **Product -> Trade** 顺序加锁：

| 动作 | 前置状态 | 同一事务中的领域写入 |
| --- | --- | --- |
| 卖家接受 | `Product.ON_SALE`、`Trade.PENDING` | 目标 Trade -> `ACCEPTED`，Product -> `RESERVED`，其他 PENDING Trade -> `CANCELLED` |
| 取消已接受交易 | `Product.RESERVED`、`Trade.ACCEPTED` | Trade -> `CANCELLED`，Product -> `ON_SALE` |
| 第一次确认 | `Trade.ACCEPTED` 且另一方未确认 | 只记录当前用户确认时间，Trade 保持 `ACCEPTED` |
| 第二次确认 | `Product.RESERVED`、`Trade.ACCEPTED` 且另一方已确认 | 记录当前用户确认时间，Trade -> `COMPLETED`，Product -> `SOLD` |

事务还必须写入该动作产生的 Outbox；请求携带幂等键时，还必须在同一事务写入首次成功的规范化命令结果。任何校验、领域更新、Outbox 或 Idempotency 写入失败都回滚整个动作，不允许出现 `CANCELLED + RESERVED`、`COMPLETED + RESERVED` 或 `ACCEPTED + SOLD`。取消与第二次确认并发时，只有先取得锁并满足前置状态的一方提交，另一方返回状态冲突且不得产生部分副作用。

### 5.6 聊天和收藏

- 创建会话前，Messaging Service 通过 Marketplace gRPC 获取 product_id 对应的 seller_id，拒绝自我会话；相同 product_id、buyer_id、seller_id 使用唯一约束保证只有一个会话。
- 发送和标记已读只依赖 Messaging Service 本地参与者数据，不在每条消息上同步调用 Account 或 Marketplace。
- Favorite Service 使用 user_id、product_id 唯一键实现 PUT/DELETE 幂等；添加时批量或单次向 Marketplace 验证商品存在及 seller_id，拒绝收藏自己的商品，但不按商品状态拒绝，`OFF_SHELF`、`RESERVED`、`SOLD` 均可收藏。
- 收藏列表由 Gateway 先读取收藏关系，再一次性批量查询 Marketplace，从而返回当前商品 status。

## 6. 数据设计

### 6.1 逻辑实体关系

~~~mermaid
erDiagram
    ACCOUNT ||--o{ PRODUCT : publishes
    PRODUCT ||--o{ PRODUCT_IMAGE : has
    PRODUCT ||--o{ TRADE : receives
    ACCOUNT ||--o{ TRADE : buys
    ACCOUNT ||--o{ FAVORITE : creates
    PRODUCT ||--o{ FAVORITE : referenced_by
    PRODUCT ||--o{ CONVERSATION : contextualizes
    ACCOUNT ||--o{ CONVERSATION : participates
    CONVERSATION ||--o{ MESSAGE : contains

    ACCOUNT {
        string id PK
        string student_no UK
        string password_hash
        string nickname
        string wechat
        string qq
    }
    PRODUCT {
        string id PK
        string seller_id
        string title
        int64 price_minor
        string category
        string status
        int64 version
    }
    PRODUCT_IMAGE {
        string id PK
        string product_id
        string object_key
        int sort_order
    }
    TRADE {
        string id PK
        string product_id
        string buyer_id
        string seller_id
        int64 price_snapshot_minor
        string status
        datetime buyer_confirmed_at
        datetime seller_confirmed_at
    }
    CONVERSATION {
        string id PK
        string product_id
        string buyer_id
        string seller_id
    }
    MESSAGE {
        string id PK
        string conversation_id
        string sender_id
        string content
        datetime read_at
    }
    FAVORITE {
        string user_id PK
        string product_id PK
        datetime created_at
    }
~~~

图中的跨服务关系是逻辑引用，不是跨数据库外键。每个服务只维护自己数据库中的外键和唯一约束。

### 6.2 关键约束

| 实体 | 关键约束 |
| --- | --- |
| Account | student_no 唯一；password_hash 为敏感字段，永不进入响应、事件和普通日志；未提供 nickname 时使用“校园用户”且不得从学号派生；已填写微信/QQ 只能改为另一非空值，不得删除 |
| Product | ID 对外是不透明字符串；status 受状态机控制；version 用于并发检测 |
| ProductImage | 同商品最多三条；sort_order 为连续 1..N；第一张是封面；删除后保持相对顺序并重排；对象删除失败需要可重试清理 |
| Trade | buyer_id 不等于 seller_id；price_snapshot 创建后不可变；product_id、buyer_id 终生联合唯一；同商品最多一个 ACCEPTED；conversation_id 非空时商品和双方必须匹配 |
| Conversation | product_id、buyer_id、seller_id 联合唯一 |
| Message | sender_id 必须是会话参与者；只能把对方发给自己的消息标记已读 |
| Favorite | user_id、product_id 联合主键；PUT 和 DELETE 均幂等 |
| Idempotency | actor_id、operation、idempotency_key 联合唯一；保存 result_code、schema_version 与首次成功的规范化命令结果快照；与对应领域写入同事务 |
| Outbox | event_id 唯一；与领域写入同事务；发布器可重复投递 |

公开 HTTP 金额继续使用十进制字符串。领域层建议使用最小货币单位整数，Gateway 和 transport mapper 负责无损转换，禁止使用二进制浮点。

## 7. API、gRPC 与事件契约

### 7.1 公开 REST

- [openapi/openapi.yaml](../openapi/openapi.yaml) 继续作为唯一公开 API 真源。
- Gateway 的 HTTP DTO、状态码、错误码、分页和鉴权必须精确实现 OpenAPI。
- 现有 OpenAPI 变更顺序为：源契约和状态机、生成 bundle、Gateway、内部 RPC、客户端和测试。
- 不使用 grpc-gateway 自动生成结果覆盖 OpenAPI；REST 与内部 RPC 通过显式 Mapper 解耦。

### 7.2 内部 gRPC

建议的 Proto 包：

- shortterm.account.v1
- shortterm.marketplace.v1
- shortterm.messaging.v1
- shortterm.favorite.v1
- shortterm.events.v1

约定：

1. 每个 RPC 使用独立 Request/Response 消息。
2. 每个 enum 的 0 值为 UNSPECIFIED；删除字段后 reserve 字段号和名称。
3. 公开不透明 ID 在 Proto 中仍使用 string，不能泄漏数据库类型。
4. Gateway 向所有下游调用设置 deadline，并向更深层调用传播剩余 deadline。
5. 只有幂等查询或明确设计为幂等的命令允许自动重试；交易动作不能因网络错误盲目重放，必须依赖 idempotency_key。
6. gRPC metadata 传播 actor_id、trace context 和服务身份；业务授权仍由资源拥有服务执行。
7. 禁止同步调用链超过 Gateway 到一个业务服务再到一个必要事实源；列表聚合使用批量 RPC，避免 N+1。

### 7.3 错误映射

领域错误先映射为稳定内部错误，再由 Gateway 映射为 OpenAPI 的 HTTP status 和 ErrorCode。例如：

| 领域错误 | gRPC status | HTTP / ErrorCode |
| --- | --- | --- |
| 未认证 | Unauthenticated | 401 / UNAUTHORIZED |
| 已知资源的参与者角色不允许执行动作 | PermissionDenied | 403 / FORBIDDEN |
| 商品不存在；交易/会话不存在或当前用户不是参与者 | NotFound | 404 / RESOURCE_NOT_FOUND |
| 收藏自己的商品或购买自己的商品 | FailedPrecondition | 409 / SELF_ACTION_NOT_ALLOWED |
| 商品当前不可交易 | FailedPrecondition 或 Aborted | 409 / PRODUCT_NOT_AVAILABLE |
| 并发交易冲突 | Aborted | 409 / TRADE_STATE_CONFLICT |
| PENDING 意向期间下架或修改商品内容/图片 | FailedPrecondition 或 Aborted | 409 / TRADE_STATE_CONFLICT |
| RESERVED/SOLD 商品内容或图片变更 | FailedPrecondition | 409 / PRODUCT_STATE_CONFLICT |
| 会话商品或参与者与购买意向不匹配 | FailedPrecondition | 409 / CONVERSATION_MISMATCH |
| 输入不合法 | InvalidArgument | 400 / VALIDATION_ERROR |
| 达到请求频率限制 | ResourceExhausted | 429 / RATE_LIMITED，并尽可能返回 Retry-After |

交易和会话是参与者私有资源。根据 RFC 9110 允许用 404 隐藏禁止访问资源存在性的语义，
非参与者统一得到 404；已知参与者调用错误角色的专用动作（例如买家调用卖家 accept）
才返回 403。公开商品详情不采用隐藏规则，任意状态都可见，404 只表示不存在。

当前 OpenAPI 没有 503/504 和依赖不可用错误码。在增加这些公开错误前，Gateway 只能按现有契约返回 500 / INTERNAL_ERROR；是否扩展契约列为待确认项。

### 7.4 事件

初始事件至少包含：

- ProductPublished、ProductUpdated、ProductStatusChanged；
- TradeCreated、TradeAccepted、TradeCancelled、TradeCompleted；
- MessageSent、ConversationRead。

事件 envelope 包含 event_id、event_type、schema_version、aggregate_id、occurred_at、trace_id 和 payload。事件是通知既成事实，不允许消费者通过修改收到的事件反向改变源服务状态。Broker 产品、分区键、保留期和重放策略在容量及恢复目标确认后选择。

## 8. 非功能设计

### 8.1 安全与隐私

资产包括密码哈希、JWT 签名密钥、微信/QQ、消息内容、OSS 凭据和部署密钥。攻击者可控输入包括所有 HTTP 字段、搜索词、消息内容、图片、ID、幂等键和 JWT。

建议措施：

- 仅 Gateway 暴露公网端口；gRPC、数据库、事件总线和管理接口只在私网或主机内部网络可达。
- Gateway 验证 JWT 签名、允许算法、issuer、audience、exp 和其他必需声明；服务执行资源级授权，不能只信任前端角色。
- 密码使用自带随机盐的慢速、内存困难哈希。建议优先评估 Argon2id，工作参数必须在实际 ECS 上基准测试，不能照抄为未验证的性能结论。
- 商品详情可按当前 OpenAPI 返回卖家微信/QQ，但不得返回 seller.student_no；联系方式不得进入公共缓存键、追踪属性或错误日志。
- 微信/QQ 更新只接受非空值，防止卖家发布商品后清空全部联系方式；客户端省略字段表示保持原值。
- 图片上传采用流式限制，校验数量、真实 MIME、尺寸和对象键；单文件大小和允许类型尚未由 OpenAPI 定义，实施前必须确认并补充契约。
- 密钥只从 GitHub Actions Secrets 和服务器运行环境注入，禁止提交仓库。
- 对登录、注册、消息发送、图片上传和交易动作分别限流；具体阈值依据压测和滥用模型确定。触发限制时按 OpenAPI 返回 `429 / RATE_LIMITED` 并尽可能携带 `Retry-After`，而不是退化为 400 或 500。

### 8.2 性能与容量

当前没有已确认的 QPS、并发用户、数据规模、p95/p99 延迟或可用性目标，因此本文不声明“高性能”或具体 SLA。实施顺序是：

1. 定义真实用户路径和峰值负载模型。
2. 在目标 ECS、目标数据库和相同镜像环境建立基线。
3. 分别测量 REST 到 gRPC、数据库查询、密码校验、图片上传和交易锁竞争。
4. 只针对已测瓶颈增加索引、缓存、副本或拆分服务。

列表查询必须稳定排序并使用确定性次级键：商品和交易使用 `created_at DESC, id DESC`，收藏使用 `favorited_at DESC, product.id DESC`，会话使用 `coalesce(last_message_at, created_at) DESC, id DESC`。除消息外，MVP 继续使用非快照页码分页；并发新增、删除或排序变化可能导致跨页重复或遗漏，客户端刷新时从第一页重新读取。这是已记录的 MVP 取舍，不得宣称页码结果是时间点快照。商品列表建议围绕 status、created_at、id 建索引。标题搜索的数据库能力和索引策略需在数据库版本及中文搜索要求确认后决定。

### 8.3 可用性与韧性

- 所有 gRPC 调用必须有 deadline；下游不得无限等待。
- 查询类调用可以有限重试并使用抖动；命令类调用只有在幂等语义得到证明时重试。
- 数据库提交与事件发布通过 Transactional Outbox 解决双写问题。
- 消费者按 event_id 幂等，重复事件不得重复产生收藏、消息或状态副作用。
- Gateway 聚合时，联系方式、交易状态等关键字段获取失败则整体失败；非关键装饰字段是否降级需在 OpenAPI 中明确定义。
- 单 ECS 初始部署不提供主机级高可用；多副本和故障转移必须在可用性目标确认后设计。

### 8.4 可观测性

- 采用结构化日志，统一 service、environment、trace_id、span_id、request_id、actor_id 和 error_code 字段。
- 使用 OpenTelemetry 在 REST、gRPC、数据库和事件 envelope 之间传播上下文。
- 禁止在日志和 trace 中记录密码、完整 JWT、微信/QQ、消息全文、OSS 密钥和数据库连接串。
- 核心业务指标包括注册/登录结果、商品发布结果、交易状态转换次数与冲突数、消息发送失败、Outbox 积压和事件重复消费。
- 健康检查区分进程存活和接流量就绪；依赖暂时不可用时不得伪报 ready。

## 9. 部署、发布与迁移

### 9.1 现状

仓库提供五个独立的非 root `scratch` 服务镜像；Marketplace 和 Messaging 各自再用
同一镜像启动独立 Outbox worker。生产工作流只消费已经在 `main` 的 Backend workflow
验证成功的精确 SHA，通过带 SHA-256 校验的离线镜像 bundle 发布到当前阿里云 ECS。
服务器使用 Podman 私有网络、一个持久化 PostgreSQL 18 实例内的四个隔离数据库账号、
持久媒体目录和用户级 systemd 生命周期管理。部署账号运行 rootless Podman；管理员只需
一次性启用 systemd linger，CI 不获得任意 sudo 权限。密钥首次部署时在服务器本地生成，
以部署账号所有的 0600 文件保存，不进入仓库、GitHub 日志或 release bundle。

### 9.2 目标初始拓扑

当前在现有阿里云 ECS 上部署多个独立容器：

- 一个 Gateway 容器把学习验收用的明文 HTTP 18083 绑定到所有 IPv4 地址，管理指标仍仅绑定主机回环 19090；GitHub runner 直接通过公网 API 执行所检出 SHA 的三角色验收脚本，再通过 SSH 核验服务器内部证据。该入口只使用随机合成账号；正式用户流量接入前必须改由可信 TLS 终止层代理并关闭安全组对 18083 的公网直连；
- Account、Marketplace、Messaging、Favorite 只加入私有容器网络；
- 每个服务使用独立数据库或独立 Schema 和独立数据库账号，禁止跨 Schema 查询；
- 对象存储只由 Marketplace 的 OSS Adapter 访问；
- Outbox Worker 可以先与对应服务使用同一镜像、不同启动命令；
- 所有镜像固定版本或摘要，配置和密钥由环境注入。

单机部署可以验证服务边界和独立发布，但不能提供节点故障容错。只有当容量、可用性或团队边界要求成立时，再迁移到 ACK/Kubernetes 或其他编排平台。

### 9.3 发布顺序

1. 向后兼容地发布 Proto 和事件消费者。
2. 执行向前兼容数据库迁移，旧版本服务仍能运行。
3. 发布业务服务并通过 readiness 与烟雾测试。
4. 发布 Gateway。
5. 观察错误率、交易冲突、Outbox 积压和关键延迟。
6. 回滚只回滚应用镜像；不可逆 Schema 变更必须采用 expand/contract 分阶段迁移。

自动部署按上述顺序执行显式迁移和 readiness，随后从 GitHub runner 通过公网 18083
运行真实 REST 主流程，并在服务器验证同一 Trace 跨 Gateway、Marketplace、Messaging、Account 和
Marketplace Outbox worker 串联，等待两个 Outbox 积压归零。迁移、启动、E2E、Trace
或 Outbox 任一失败都会使 workflow 失败；存在上一 release 时自动回滚应用镜像。
当前不使用镜像仓库，而是传输校验后的离线 bundle；这是单节点规模下减少外部依赖的
取舍，不改变每个服务的独立镜像和容器边界。

## 10. 测试与验收

| 风险/场景 | 测试层级 | 验证点 | 当前证据 |
| --- | --- | --- | --- |
| OpenAPI 漂移 | 契约 CI | lint、bundle、Git diff | 已有 npm scripts；源契约和生成 bundle 同步维护 |
| Proto 破坏性变更 | 契约 CI | buf lint、buf breaking、buf generate drift | `proto:check` 与 PR 对 main breaking 门禁已接入 Backend workflow |
| 商品状态返回 | 契约/集成/E2E | 所有 ProductSummary、ProductDetail、ConversationProduct、TradeProduct 返回当前 status | Gateway 聚合测试、真实 PostgreSQL 测试与容器 REST E2E 已覆盖 |
| 非在售详情可见 | 契约/E2E | 非卖家访问 OFF_SHELF、RESERVED、SOLD 商品详情均为 200；不存在才为 404 | Marketplace/Gateway 测试与 SOLD 生产验收路径已覆盖 |
| 联系方式不可删除 | 契约/Account 集成 | 已填写或未填写联系方式都不能通过 null/空字符串删除；非空新增和修改成功 | Account domain/application/PostgreSQL/gRPC 测试已覆盖 |
| PENDING 与下架/编辑并发 | 数据库集成/并发测试 | 创建意向与下架或内容变更只有一个合法顺序提交；永不出现 OFF_SHELF + PENDING 或交错商品版本 | 真实 PostgreSQL 并发与 race 测试已覆盖 Product→Trade 锁序 |
| 唯一购买意向 | 数据库集成/并发测试 | 同一 buyer/product 并发及换 key 请求只产生一个 Trade；首次 201，后续 200；终态不重开 | 唯一索引、create-or-get gRPC/Gateway 与并发测试已覆盖 |
| 会话绑定校验 | 契约/集成测试 | conversation 的商品或任一参与者不匹配时不创建 Trade 并返回 409/CONVERSATION_MISMATCH | Marketplace 经 Messaging verifier 的网络 gRPC 测试已覆盖 |
| 并发接受交易 | 数据库集成测试 | 并发请求只有一个成功；商品 RESERVED；其他交易 CANCELLED | 真实 PostgreSQL 并发与事务测试已覆盖 |
| 响应丢失后的重复命令 | 数据库集成测试 | 首次事务提交但响应丢失后，同 actor、operation、idempotency_key 返回首次成功的 HTTP 状态与响应体，不因当前状态变化返回 409 | schema v2 命令结果和 Product 投影快照测试已覆盖 |
| 取消与完成原子性 | 数据库集成/并发测试 | 任一步骤失败时 Product、Trade、Outbox、Idempotency 全部回滚；取消与第二次确认并发时仅一个合法转换提交 | 故障注入、并发和 Outbox 原子性测试已覆盖 |
| 越权访问 | 安全测试 | 非卖家不能改商品，非参与者不能读消息/交易 | 各服务网络 gRPC 与 Gateway REST 权限测试已覆盖 403/隐藏式 404 |
| 图片顺序与封面 | 数据库/对象存储集成测试 | 上传按顺序追加；删除后连续重排；cover_url 始终对应第一张或 null | Marketplace 真实 PostgreSQL/文件存储测试已覆盖 |
| 限流响应 | Gateway 集成测试 | 登录、注册、消息、上传和交易动作触发阈值后返回 429/RATE_LIMITED 与可用 Retry-After | 有界进程内固定窗口实现和路由集成测试已覆盖；阈值仍待真实流量校准 |
| 密码和联系方式泄漏 | 契约/日志检查 | 响应、日志、trace 不出现禁泄漏字段 | 日志强制脱敏、DTO/聚合测试和容器 E2E 的学号泄漏断言已覆盖 |
| Outbox 故障恢复 | 集成测试 | 提交后发布失败可恢复；重复投递无重复副作用 | Marketplace/Messaging 真实 PostgreSQL 重试与原子性测试；消费者去重留给未来 Broker |
| 依赖故障 | E2E/故障注入 | deadline 生效，不无限等待，不产生半完成交易 | gRPC deadline、标准 Health readiness 与事务失败测试已覆盖；整栈网络分区注入仍后置 |

商品状态专项验收：

1. 新商品在列表和详情中返回 ON_SALE。
2. 接受交易后，我的商品列表、收藏列表、会话商品投影、交易商品投影和详情均返回 RESERVED。
3. 已接受交易取消后，上述查询返回 ON_SALE。
4. 双方确认完成后，上述可访问查询返回 SOLD。
5. 卖家下架和重新上架后返回 OFF_SHELF 与 ON_SALE。
6. 公开 GET /products 仍只返回 ON_SALE，除非先修改公开契约。
7. 已认证用户通过详情接口读取 OFF_SHELF、RESERVED、SOLD 商品均返回 200。
8. 商品存在任意 PENDING Trade 时下架返回 409；逐笔拒绝后才可下架。
9. 商品存在任意 PENDING Trade 时字段修改、加图和删图均返回 409。
10. RESERVED、SOLD 商品的字段修改、加图和删图均返回 409。

## 11. 决策、风险与待确认项

### 11.1 关键决策

| 编号 | 决策 | 理由与代价 | 状态/复审条件 |
| --- | --- | --- | --- |
| ADR-01 | 前端 REST，内部 gRPC | 保持浏览器友好契约，并让内部接口强类型；增加双契约及 Mapper 成本 | Human Design + Agent Self-Claimed；待批准 |
| ADR-02 | Gateway 是唯一公网业务入口 | 隐藏服务拆分并集中通用策略；需防止业务逻辑堆积 | Agent Self-Claimed |
| ADR-03 | Account 合并认证和资料 | 避免注册的分布式事务；认证形成独立团队或合规边界时复审 | Agent Self-Claimed |
| ADR-04 | Product 与 Trade 同属 Marketplace | 满足接受、已接受交易取消和双方确认完成的本地原子事务；交易规模或团队边界改变时复审 | Agent Self-Claimed |
| ADR-05 | DTO 只存在于传输边界 | 防止协议、数据库和领域模型耦合；需要显式 Mapper | Agent Self-Claimed |
| ADR-06 | 每服务独占逻辑数据 | 允许独立演进；跨服务查询需要 Gateway 聚合或事件 | Agent Self-Claimed |
| ADR-07 | 所有商品投影返回当前 status | 满足用户可见交易中状态，避免过期快照 | Human Design |
| ADR-08 | 本地事务加 Outbox | 避免数据库与事件双写不一致；增加 Worker 和幂等消费成本 | Agent Self-Claimed |
| ADR-09 | 幂等成功结果与领域写入同事务 | 满足响应丢失后的安全重试，避免领域状态已提交但幂等结果缺失；增加结果存储与保留成本 | Agent Self-Claimed；实现前复审保留周期 |
| ADR-10 | 同一 buyer/product 终生只有一个 Trade 意向 | 把重复 HTTP 尝试归入同一业务意向并阻断拒绝后重复建单；代价是终态不能再次表达意向，未来如需重开必须新增受控动作 | Human Design 确认“同一 intent”；唯一键、终生范围和 200/201 为 Agent Self-Claimed |
| ADR-11 | 私有交易/会话对非参与者返回 404 | 隐藏私有资源存在性；参与者调用错误角色动作仍用 403，增加授权错误映射分支 | Agent Self-Claimed；依据 RFC 9110 |

### 11.2 风险

| 风险 | 可能性/影响 | 缓解措施 | 责任角色 |
| --- | --- | --- | --- |
| 小团队维护多个服务的运维成本过高 | 高 / 中 | 单仓库、统一模板、共享 CI 但不共享领域模型；定期复审是否需要合并 | 技术负责人 |
| Gateway 聚合过多导致延迟和单点故障 | 中 / 高 | 批量 RPC、并行独立查询、deadline、追踪；禁止 N+1 | 后端负责人 |
| 商品状态在收藏/会话列表中陈旧 | 中 / 高 | 从 Marketplace 批量读取当前状态，不以异步快照作为验收真值 | Marketplace/Gateway |
| 事件重复或乱序 | 高 / 中 | event_id 去重、aggregate_id 顺序策略、Outbox 和幂等消费者 | 各服务负责人 |
| 单 ECS 故障导致全站不可用 | 中 / 高 | 明确不宣称 HA；确认 SLA 后增加多节点和托管数据库 | 运维负责人 |
| JWT 退出语义不明确 | 中 / 中 | 在实现前决定仅客户端丢弃或服务端 jti 撤销，并同步 OpenAPI | 产品与安全负责人 |
| 终生唯一 Trade 限制买家再次表达意向 | 中 / 中 | MVP 保持终态不可重开；出现真实重开需求时设计显式 reopen 动作及卖家反骚扰规则，不通过创建第二条 Trade 绕过 | 产品与 Marketplace 负责人 |

### 11.3 待确认项

| 问题 | 影响 | 需要的决定 |
| --- | --- | --- |
| 公开 GET /products 是否需要展示 RESERVED 商品 | 当前契约只返回 ON_SALE；status 字段存在但公开列表不会出现 RESERVED | 产品负责人确认是否修改列表语义 |
| JWT logout 是否必须立即使令牌失效 | 决定是否需要 jti denylist、会话表或仅客户端清除 | 产品与安全负责人 |
| 图片单文件大小、总大小、MIME 和尺寸 | 决定 Gateway、OSS 和错误契约 | 产品与安全负责人 |
| PostgreSQL 版本、实例形态和备份目标 | 决定迁移、索引、连接池和恢复方案 | 运维与后端负责人 |
| 事件总线及至少一次/顺序要求 | 决定 Broker、分区、重放和运维成本 | 架构负责人 |
| QPS、数据规模、延迟、RPO 和 RTO | 决定副本、缓存、数据库和是否引入编排平台 | 产品与运维负责人 |
| 内部服务身份使用主机私网、mTLS 还是工作负载身份 | 决定服务间认证和证书运维 | 安全与运维负责人 |
| 是否扩展 503/504 公开错误 | 决定依赖故障是否可被前端区分和重试 | 前后端负责人 |

## 12. 需求追踪矩阵

| 需求/用例 | 模块 | 数据/API | 验证 |
| --- | --- | --- | --- |
| FR-01 注册登录 | Account、Gateway | /auth/register、/auth/login、Account | 契约、密码哈希集成、安全测试 |
| FR-02 资料与联系方式 | Account、Gateway | /users/me、Account | 所有权与泄漏测试 |
| FR-03 商品发布查询 | Marketplace、Gateway | /products、Product、ProductImage | 契约、数据库和上传测试 |
| FR-04 商品状态返回 | Marketplace、Gateway | 所有商品投影 DTO 的 status | 状态专项契约与 E2E |
| FR-05 收藏 | Favorite、Marketplace、Gateway | /favorites、Favorite | 幂等、自我收藏、当前状态测试 |
| FR-06 聊天 | Messaging、Marketplace、Gateway | /conversations、Conversation、Message | 参与者、幂等、已读测试 |
| FR-07 交易流转 | Marketplace、Gateway | /trades、Trade、Product | 状态机、事务、并发测试 |
| NFR-01 契约兼容 | OpenAPI、Proto、CI | openapi、proto、gen | lint、breaking、drift |
| NFR-02 一致性 | Marketplace、Outbox | Product、Trade、Idempotency、Outbox | 并发事务、响应丢失重试和故障恢复 |
| NFR-03 安全 | Gateway、各服务 | JWT、授权、日志和上传 | 安全测试与日志检查 |
| NFR-04 可观测性 | 全部运行单元 | trace context、指标、日志 | 跨 REST/gRPC/事件追踪测试 |

## 13. 设计依据与成熟参考

| 结论 | 状态 | 证据或参考 | 适用说明 |
| --- | --- | --- | --- |
| REST 路径、字段、错误和鉴权 | 已确认 | [openapi/openapi.yaml](../openapi/openapi.yaml) 及引用文件，OAS 3.1.0 | 本项目公开契约真源 |
| 商品、消息和交易状态规则 | 已确认 | [state-machines.md](state-machines.md) | 本项目跨资源状态治理 |
| API Gateway 作为集中入口和聚合层 | Agent Self-Claimed | [Microsoft: Use API gateways in microservices](https://learn.microsoft.com/en-us/azure/architecture/microservices/design/gateway)，检索于 2026-09-01 | 借鉴入口、路由、聚合和通用能力；本项目不绑定 Azure 产品 |
| 按限界上下文、内聚性和一致性划分服务 | Agent Self-Claimed | [Microsoft: Identify microservice boundaries](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/microservice-boundaries)，检索于 2026-09-01 | 用于 Account 合并及 Product/Trade 同服务的判断 |
| RPC DTO 与存储模型分离 | Agent Self-Claimed | [Protocol Buffers: Proto Best Practices](https://protobuf.dev/best-practices/dos-donts/)，检索于 2026-09-01 | 用于 Proto 演进和 DTO 边界 |
| gRPC 调用必须设置 deadline | Agent Self-Claimed | [gRPC: Deadlines](https://grpc.io/docs/guides/deadlines/)，检索于 2026-09-01 | 具体时长必须通过真实链路测量 |
| Proto 破坏性变更检查 | Agent Self-Claimed | [Buf: Detecting breaking changes](https://buf.build/docs/breaking/)，检索于 2026-09-01 | Buf 版本在引入工具时固定 |
| 数据库与事件双写使用 Outbox | Agent Self-Claimed | [AWS Prescriptive Guidance: Transactional outbox](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)，检索于 2026-09-01 | 借鉴模式，不绑定 AWS 服务 |
| 幂等令牌与领域写入构成单个 ACID 操作 | Agent Self-Claimed | [AWS Builders' Library: Making retries safe with idempotent APIs](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/)，检索于 2026-09-01 | 用于响应丢失后的结果重放；本项目键空间额外包含 actor_id 与 operation |
| 一个业务购买过程复用同一意向资源 | Human Design + Agent Self-Claimed | [Stripe: The Payment Intents API](https://docs.stripe.com/payments/payment-intents)，检索于 2026-09-01 | 借鉴“一个订单/会话一个 intent”和中断后复用；本项目不接入 Stripe，并自行选择 buyer/product 终生唯一与终态不重开 |
| 私有资源可用 404 隐藏存在性 | Agent Self-Claimed | [RFC 9110 §15.5.4–15.5.5](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4)，发布于 2022-06，检索于 2026-09-01 | 交易/会话非参与者用 404；公开商品和已知参与者错误角色不套用 |
| 限流使用 429 与可选 Retry-After | Agent Self-Claimed | [RFC 6585 §4](https://www.rfc-editor.org/rfc/rfc6585.html#section-4)，发布于 2012-04，检索于 2026-09-01 | 阈值仍由压测和滥用模型决定；契约固定可表达的状态与错误码 |
| 密码使用慢速加盐哈希并在真实主机调参 | Agent Self-Claimed | [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)，检索于 2026-09-01 | 算法和参数实施前需安全评审与基准 |
| JWT 实现遵守当前最佳实践 | Agent Self-Claimed | [RFC 8725: JSON Web Token Best Current Practices](https://www.rfc-editor.org/rfc/rfc8725.html)，检索于 2026-09-01 | 退出和撤销策略仍待产品确认 |
| 跨进程传播 Trace Context | Agent Self-Claimed | [OpenTelemetry: Context propagation](https://opentelemetry.io/docs/concepts/context-propagation/)，检索于 2026-09-01 | SDK 与版本在 Go 工程建立后固定 |
