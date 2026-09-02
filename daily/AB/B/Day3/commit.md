# Day3 提交计划

- 人员：B
- 建议 Gitee commit：`feat(proto): define grpc contracts and parallel service modules`

## 主要内容

### gRPC 契约与生成流程

- `proto/shortterm/account/v1/account.proto`：定义注册、登录、资料、批量用户和改密 RPC，以及公开资料/私有资料的字段边界。
- `proto/shortterm/marketplace/v1/marketplace.proto`：定义商品、图片、Product/Trade 状态和全部商品/交易动作 RPC。
- `proto/shortterm/messaging/v1/messaging.proto`：定义会话、消息、游标分页、未读数、发送和已读 RPC；`favorite.proto` 定义收藏关系；`events.proto` 定义带 event_id、schema_version、aggregate 和 trace_id 的 Outbox 信封。
- `buf.yaml`、`buf.gen.yaml`：固定模块、STANDARD lint、FILE breaking 规则和 Go/gRPC 插件版本；`scripts/proto-format.mjs`、`proto-breaking.mjs` 固定格式、生成和兼容性检查。
- `gen/go/**`：由 `buf generate` 生成的 Go/gRPC 类型和客户端/服务端接口，禁止手工改生成产物。

### Messaging、Favorite 与 Gateway

- Messaging：实现商品上下文会话、参与者权限、文本消息、游标读取、单向幂等已读、未读数、PostgreSQL 仓储和 Outbox worker。
- Favorite：实现 `(user_id, product_id)` 唯一收藏关系、幂等添加/取消、列表和 `IsFavorited`，添加时通过 Marketplace 校验商品存在且禁止收藏自己的商品。
- Gateway：实现 HTTP router、OpenAPI DTO、mapper、认证/限流/指标/Trace middleware、gRPC clients、错误映射、商品状态批量聚合和收藏/会话/交易路由。
- `go.work`、`go.work.sum`：把生成代码、Platform、五个独立服务接入同一 workspace，同时保持每个服务可单独构建和部署。

### 并行开发与完成标准

- 先固定 Proto 字段号、错误和状态，再由各服务实现；生成代码变化必须回到 Proto 源文件。
- 执行 `npm run proto:check`，确认 build、格式、lint、生成代码和 drift 全部通过，并补充 gRPC/Gateway/业务服务测试后再与其他模块联调。

## GitHub 取材

- `e8c0aec` 路径拆分
