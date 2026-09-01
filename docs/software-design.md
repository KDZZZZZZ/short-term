# 校园二手交易平台微服务软件设计文档

## 文档信息

| 项目 | 内容 |
| --- | --- |
| 版本/状态 | 0.1 / 草案，待人类评审 |
| 日期 | 2026-09-01 |
| 责任角色 | 项目维护者批准；Codex 起草 |
| 目标读者 | 后端、前端、测试、运维与后续架构评审人员 |
| 形成方式 | 需求驱动的目标设计 |
| 事实基准 | 当前 main 分支；公开契约为 [openapi/openapi.yaml](../openapi/openapi.yaml)，状态规则为 [state-machines.md](state-machines.md) |

### 修订记录

| 版本 | 日期 | 修订人/责任角色 | 修订内容 | 状态 |
| --- | --- | --- | --- | --- |
| 0.1 | 2026-09-01 | Codex / 项目维护者待评审 | 建立微服务、REST/gRPC、DTO、数据一致性及部署目标设计 | 草案 |

### 事实状态

- **Human Design**：系统采用微服务；前端继续使用 REST；同一用户可同时作为买家和卖家；实现收藏、站内文字聊天和交易状态流转；商品列表与商品详情返回商品状态。
- **已批准基线**：OpenAPI 是公开 HTTP API 唯一真源；[状态机文档](state-machines.md)治理商品与交易的跨资源状态变化。
- **Agent Self-Claimed**：API Gateway、内部 gRPC、服务边界、DTO 分层、数据库所有权、Outbox、目标目录和部署拓扑均为本设计提出的方案，尚未视为已实现或已批准。

## 1. 背景、目标与范围

### 1.1 背景

校园二手交易平台需要支持学生账号、商品发布与检索、收藏、商品上下文聊天以及线下交易协商。前端需要稳定且易调试的 REST/JSON 契约，后端则希望从项目初期按独立业务能力拆分，为独立部署、故障隔离和后续扩展保留边界。

当前仓库只包含公开 OpenAPI、状态机、契约 CI 和 OpenAPI 文档部署配置，尚无 Go 服务、内部 RPC 契约、数据库迁移或后端运行时。本文因此描述目标设计，不描述既有实现。

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
| 已确认约束 | 商品和交易接受动作必须原子联动 | Product 与 Trade 放在同一个 Marketplace Service 和本地事务中 |
| 已确认约束 | 用户没有固定买家/卖家角色 | 权限根据 seller_id、buyer_id 和当前用户动态判断 |
| 已确认约束 | 商品列表和详情返回 status | 所有 ProductSummary、ProductDetail 和嵌套商品投影保留状态字段 |
| 建议 | Go 微服务内部使用 gRPC | 需要建立 Protobuf、Buf 和生成代码治理 |
| 建议 | 每个服务独占自己的逻辑数据库 | 可以初期共享 PostgreSQL 集群，但禁止跨服务直接访问表或 Schema |
| 待确认 | 峰值请求量、数据规模、延迟和可用性目标 | 暂不能确定副本数、连接池、缓存及容量 |
| 待确认 | 事件总线产品和交付语义 | 本文只固定 Outbox、幂等消费和事件契约，不指定 Kafka、NATS 或其他产品 |

## 2. 需求与质量属性摘要

| 编号 | 需求或场景 | 验收标准 | 状态/证据 |
| --- | --- | --- | --- |
| FR-01 | 学号和密码注册登录 | 合法账号可注册、登录并取得访问令牌；密码不出现在响应或日志 | 已确认，OpenAPI |
| FR-02 | 用户维护资料及联系方式 | 用户只能读写自己的完整资料 | 已确认，OpenAPI |
| FR-03 | 发布与查询商品 | 支持分类、价格、描述、最多三张图片、倒序列表和标题搜索 | 已确认，OpenAPI |
| FR-04 | 返回商品状态 | 商品列表、我的商品、收藏中的商品、交易商品投影和商品详情均携带 status | Human Design；OpenAPI Schema 已包含 |
| FR-05 | 收藏 | 收藏和取消收藏幂等，不能收藏自己的商品 | 已确认，OpenAPI 与状态机 |
| FR-06 | 站内聊天 | 仅商品买卖双方访问会话；消息已读单向且幂等 | 已确认，OpenAPI 与状态机 |
| FR-07 | 交易流转 | 状态动作、操作者权限和商品副作用符合状态机 | 已确认，状态机 |
| NFR-01 | 契约兼容 | OpenAPI lint/bundle 无漂移；Proto lint、breaking 和生成代码无漂移 | OpenAPI 已建立；Proto 为建议目标 |
| NFR-02 | 一致性 | 同一商品最多一笔 ACCEPTED 交易，商品与交易状态不分裂 | 已确认，状态机 |
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
    ON_SALE --> OFF_SHELF: 卖家下架
    OFF_SHELF --> ON_SALE: 卖家重新上架
    SOLD --> [*]
~~~

状态只能通过专用动作或交易状态机改变，不能通过通用商品 PATCH 直接赋值。所有返回商品投影的查询读取 Marketplace Service 的当前状态。

### 5.2 商品列表和详情

1. Gateway 验证请求格式和访问令牌，生成或透传 trace_id。
2. Gateway 调用 Marketplace Service 的 ListProducts 或 GetProduct。
3. Marketplace Service 根据授权上下文查询商品真值并返回 seller_id、商品字段和 status。
4. Gateway 通过 Account Service 的批量接口补全公开卖家资料；详情按 OpenAPI 补全微信/QQ，禁止返回学号。
5. 登录用户的商品详情需要 is_favorited 时，Gateway 调用 Favorite Service 查询当前关系。
6. Gateway 将 gRPC DTO 映射为 ProductSummary 或 ProductDetail HTTP DTO，并再次按 OpenAPI 序列化。

Marketplace 是商品状态的唯一事实源。Gateway 不缓存状态，除非后续定义了明确的最大陈旧时间和失效策略。

### 5.3 接受交易的强一致流程

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
    MK->>DB: BEGIN + 锁定商品和目标交易
    MK->>DB: 校验卖家、PENDING、商品 ON_SALE
    alt 校验通过
        MK->>DB: 目标交易 -> ACCEPTED
        MK->>DB: 商品 -> RESERVED
        MK->>DB: 其他 PENDING 交易 -> CANCELLED
        MK->>DB: 写入 Outbox 事件
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
~~~

事务内必须完成目标交易、商品状态、其他待处理交易和 Outbox 写入。事件只能在提交后投递。Outbox 允许至少一次投递，因此消费者必须根据 event_id 或业务幂等键去重。

### 5.4 聊天和收藏

- 创建会话前，Messaging Service 通过 Marketplace gRPC 获取 product_id 对应的 seller_id，拒绝自我会话；相同 product_id、buyer_id、seller_id 使用唯一约束保证只有一个会话。
- 发送和标记已读只依赖 Messaging Service 本地参与者数据，不在每条消息上同步调用 Account 或 Marketplace。
- Favorite Service 使用 user_id、product_id 唯一键实现 PUT/DELETE 幂等；添加时批量或单次向 Marketplace 验证商品存在及 seller_id，拒绝收藏自己的商品。
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
| Account | student_no 唯一；password_hash 为敏感字段，永不进入响应、事件和普通日志 |
| Product | ID 对外是不透明字符串；status 受状态机控制；version 用于并发检测 |
| ProductImage | 同商品最多三条；sort_order 为 1 至 3；对象删除失败需要可重试清理 |
| Trade | buyer_id 不等于 seller_id；price_snapshot 创建后不可变；同商品最多一个 ACCEPTED |
| Conversation | product_id、buyer_id、seller_id 联合唯一 |
| Message | sender_id 必须是会话参与者；只能把对方发给自己的消息标记已读 |
| Favorite | user_id、product_id 联合主键；PUT 和 DELETE 均幂等 |
| Idempotency | actor_id、operation、idempotency_key 联合唯一；保存首次成功结果摘要 |
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
| 非资源所有者或非参与者 | PermissionDenied | 403 / FORBIDDEN |
| 商品或交易不存在 | NotFound | 404 / RESOURCE_NOT_FOUND |
| 商品当前不可交易 | FailedPrecondition 或 Aborted | 409 / PRODUCT_NOT_AVAILABLE |
| 并发交易冲突 | Aborted | 409 / TRADE_STATE_CONFLICT |
| 输入不合法 | InvalidArgument | 400 / VALIDATION_ERROR |

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
- 图片上传采用流式限制，校验数量、真实 MIME、尺寸和对象键；单文件大小和允许类型尚未由 OpenAPI 定义，实施前必须确认并补充契约。
- 密钥只从 GitHub Actions Secrets 和服务器运行环境注入，禁止提交仓库。
- 对登录、注册、消息发送、图片上传和交易动作分别限流；具体阈值依据压测和滥用模型确定。

### 8.2 性能与容量

当前没有已确认的 QPS、并发用户、数据规模、p95/p99 延迟或可用性目标，因此本文不声明“高性能”或具体 SLA。实施顺序是：

1. 定义真实用户路径和峰值负载模型。
2. 在目标 ECS、目标数据库和相同镜像环境建立基线。
3. 分别测量 REST 到 gRPC、数据库查询、密码校验、图片上传和交易锁竞争。
4. 只针对已测瓶颈增加索引、缓存、副本或拆分服务。

列表查询必须稳定排序并使用确定性次级键；商品列表建议围绕 status、created_at、id 建索引。标题搜索的数据库能力和索引策略需在数据库版本及中文搜索要求确认后决定。

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

当前 main 更新后的 GitHub Actions 只部署 Swagger UI/OpenAPI 文档。仓库尚无后端容器、数据库、事件总线或 gRPC 部署配置。

### 9.2 目标初始拓扑

建议先在现有阿里云 ECS 上部署多个独立容器：

- 一个 Gateway 容器对公网开放 HTTPS；
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

后端自动部署工作流、数据库迁移工具、镜像仓库和回滚命令尚未建立，不能把本节目标拓扑视为当前能力。

## 10. 测试与验收

| 风险/场景 | 测试层级 | 验证点 | 当前证据 |
| --- | --- | --- | --- |
| OpenAPI 漂移 | 契约 CI | lint、bundle、Git diff | 已有 npm scripts；本次文档变更未修改 OpenAPI |
| Proto 破坏性变更 | 契约 CI | buf lint、buf breaking、buf generate drift | 待 Proto 建立 |
| 商品状态返回 | 契约/集成/E2E | 所有 ProductSummary、ProductDetail、ConversationProduct、TradeProduct 返回当前 status | OpenAPI Schema 已定义；实现待建 |
| 并发接受交易 | 数据库集成测试 | 并发请求只有一个成功；商品 RESERVED；其他交易 CANCELLED | 状态规则已定义；实现待建 |
| 重复命令 | 集成测试 | 同 actor、operation、idempotency_key 返回首次成功结果 | OpenAPI 部分动作已定义；实现待建 |
| 越权访问 | 安全测试 | 非卖家不能改商品，非参与者不能读消息/交易 | 契约已定义；实现待建 |
| 密码和联系方式泄漏 | 契约/日志检查 | 响应、日志、trace 不出现禁泄漏字段 | 实现待建 |
| Outbox 故障恢复 | 集成测试 | 提交后发布失败可恢复；重复投递无重复副作用 | 设计建议；实现待建 |
| 依赖故障 | E2E/故障注入 | deadline 生效，不无限等待，不产生半完成交易 | 设计建议；实现待建 |

商品状态专项验收：

1. 新商品在列表和详情中返回 ON_SALE。
2. 接受交易后，我的商品列表、收藏列表、会话商品投影、交易商品投影和详情均返回 RESERVED。
3. 已接受交易取消后，上述查询返回 ON_SALE。
4. 双方确认完成后，上述可访问查询返回 SOLD。
5. 卖家下架和重新上架后返回 OFF_SHELF 与 ON_SALE。
6. 公开 GET /products 仍只返回 ON_SALE，除非先修改公开契约。

## 11. 决策、风险与待确认项

### 11.1 关键决策

| 编号 | 决策 | 理由与代价 | 状态/复审条件 |
| --- | --- | --- | --- |
| ADR-01 | 前端 REST，内部 gRPC | 保持浏览器友好契约，并让内部接口强类型；增加双契约及 Mapper 成本 | Human Design + Agent Self-Claimed；待批准 |
| ADR-02 | Gateway 是唯一公网业务入口 | 隐藏服务拆分并集中通用策略；需防止业务逻辑堆积 | Agent Self-Claimed |
| ADR-03 | Account 合并认证和资料 | 避免注册的分布式事务；认证形成独立团队或合规边界时复审 | Agent Self-Claimed |
| ADR-04 | Product 与 Trade 同属 Marketplace | 满足接受交易的本地原子事务；交易规模或团队边界改变时复审 | Agent Self-Claimed |
| ADR-05 | DTO 只存在于传输边界 | 防止协议、数据库和领域模型耦合；需要显式 Mapper | Agent Self-Claimed |
| ADR-06 | 每服务独占逻辑数据 | 允许独立演进；跨服务查询需要 Gateway 聚合或事件 | Agent Self-Claimed |
| ADR-07 | 所有商品投影返回当前 status | 满足用户可见交易中状态，避免过期快照 | Human Design |
| ADR-08 | 本地事务加 Outbox | 避免数据库与事件双写不一致；增加 Worker 和幂等消费成本 | Agent Self-Claimed |

### 11.2 风险

| 风险 | 可能性/影响 | 缓解措施 | 责任角色 |
| --- | --- | --- | --- |
| 小团队维护多个服务的运维成本过高 | 高 / 中 | 单仓库、统一模板、共享 CI 但不共享领域模型；定期复审是否需要合并 | 技术负责人 |
| Gateway 聚合过多导致延迟和单点故障 | 中 / 高 | 批量 RPC、并行独立查询、deadline、追踪；禁止 N+1 | 后端负责人 |
| 商品状态在收藏/会话列表中陈旧 | 中 / 高 | 从 Marketplace 批量读取当前状态，不以异步快照作为验收真值 | Marketplace/Gateway |
| 事件重复或乱序 | 高 / 中 | event_id 去重、aggregate_id 顺序策略、Outbox 和幂等消费者 | 各服务负责人 |
| 单 ECS 故障导致全站不可用 | 中 / 高 | 明确不宣称 HA；确认 SLA 后增加多节点和托管数据库 | 运维负责人 |
| JWT 退出语义不明确 | 中 / 中 | 在实现前决定仅客户端丢弃或服务端 jti 撤销，并同步 OpenAPI | 产品与安全负责人 |

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
| NFR-02 一致性 | Marketplace、Outbox | Product、Trade、Outbox | 并发事务和故障恢复 |
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
| 密码使用慢速加盐哈希并在真实主机调参 | Agent Self-Claimed | [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)，检索于 2026-09-01 | 算法和参数实施前需安全评审与基准 |
| JWT 实现遵守当前最佳实践 | Agent Self-Claimed | [RFC 8725: JSON Web Token Best Current Practices](https://www.rfc-editor.org/rfc/rfc8725.html)，检索于 2026-09-01 | 退出和撤销策略仍待产品确认 |
| 跨进程传播 Trace Context | Agent Self-Claimed | [OpenTelemetry: Context propagation](https://opentelemetry.io/docs/concepts/context-propagation/)，检索于 2026-09-01 | SDK 与版本在 Go 工程建立后固定 |
