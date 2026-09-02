# Day2 提交计划

- 人员：B
- 建议 Gitee commit：`docs(backend): finalize backend technology and architecture`

## 主要内容

### 技术选型与总体取舍

- 固定 Go 作为后端实现语言，gRPC 作为服务间调用协议，PostgreSQL 作为事务数据库，REST/JSON 继续作为前端唯一接入方式。
- 结合两人协作、单机部署、商品与交易强一致、服务可独立构建等约束，说明为什么不把前端直接连接内部 gRPC，也不在 MVP 引入搜索服务、服务网格或多地域基础设施。

### 服务边界与数据所有权

- Account 负责学号凭据、密码哈希、JWT 和用户公开资料；Marketplace 负责商品、图片、Trade、状态机和 Outbox；Messaging 负责会话、消息、已读；Favorite 负责收藏关系；Gateway 负责公开 REST、认证入口、错误映射和聚合。
- 明确 Product 与 Trade 同属 Marketplace 本地事务边界；其他服务只能通过版本化 gRPC 或事件引用 `user_id`、`product_id`，不能跨服务直接访问对方表。
- 规定 Gateway HTTP DTO、Protobuf DTO、Application Command、Domain Entity、Persistence Row 和 Event DTO 的分层，禁止建立跨服务 `common/dto` 或 `shared/domain`。

### 后端工程约定与开发计划

- `docs/backend-conventions.md`：记录模块目录、迁移、独立数据库账号、错误模型、日志脱敏、Trace 传播、gRPC deadline、测试层级和本地 PostgreSQL 使用方式。
- `docs/backend-development-plan.md`：将实现拆成 M0 工程基座、M1 Account、M2 Marketplace、M3 Trade、M4 Favorite、M5 Messaging、M6 部署收尾，标注依赖、并行边界、阻塞决策和每阶段验收证据。
- 同步 OpenAPI 唯一真源、状态机治理、事务幂等、Outbox、资源级授权和“不虚构性能结论”等规则。

### 提交边界与完成标准

- 本提交只落地技术与架构文档，不提前提交业务实现。
- 每个服务的职责、数据事实源、调用方式、测试入口和里程碑依赖都能在文档中找到，后续并行开发不会因边界不清反复返工。

## GitHub 取材

- `38b6e6c`
- `e8c0aec` 中的 docs
