# Day3 提交计划

- 人员：A
- 建议 Gitee commit：`feat(platform): build backend platform and core modules`

## 主要内容

### Platform 公共技术设施

- `platform/config`、`config/runtime`：读取环境变量、一次汇报配置错误，并区分本地与生产运行参数。
- `platform/auth`：密码哈希、JWT 签发与验证、issuer/audience/TTL 等认证基础能力；配套密码和 token 测试。
- `platform/errs`、`platform/grpcx`：统一领域错误模型、gRPC status/错误码映射、server/client、deadline、recovery、health、metadata 和请求上下文传播。
- `platform/id`、`platform/logging`、`platform/observability`：生成不透明公开 ID，输出结构化日志并脱敏密码/Token/联系方式，建立 Trace 与日志关联。
- `platform/pg`、`platform/pgtest`：连接池、迁移、SQL trace、事务辅助和每个集成测试独立的临时 PostgreSQL 数据库。

### 本地数据库与服务基础

- `deploy/local/docker-compose.yml`、`postgres-init.sql`：启动 PostgreSQL 18，创建 Account、Marketplace、Messaging、Favorite 独立数据库和账号，撤销跨库连接并提供健康检查。
- 为每个服务保留独立 `go.mod`、迁移目录、`cmd/server`/`cmd/migrate` 和真实 PostgreSQL 测试入口，禁止把业务领域类型塞回 Platform。

### Account 与 Marketplace 核心模块

- Account：实现注册、登录、当前用户资料、联系方式更新、改密、密码哈希、JWT、数据库仓储和 gRPC adapter。
- Marketplace：实现商品/图片/交易领域模型、迁移、仓储、应用服务、gRPC 接口、图片对象存储、Product/Trade 状态机、事务幂等和 Outbox 基础。
- 商品与交易的接受、取消、双方确认等跨资源变化在 Marketplace 单一本地事务中完成，并遵循 `Product -> Trade` 加锁顺序。

### 提交边界与完成标准

- 本提交负责 Platform、Account、Marketplace 和本地 PostgreSQL 基座；Proto、Messaging、Favorite、Gateway 的并行模块由另一条提交计划负责。
- 通过 `gofmt`、`go vet`、单元测试及需要数据库的集成/并发测试，服务可以在本地 Compose 中启动并执行最小链路。

## GitHub 取材

- `e8c0aec` 路径拆分
