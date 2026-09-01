# 后端工程约定

## 文档信息

| 项目 | 内容 |
| --- | --- |
| 版本/状态 | 0.1 / 草案，随实现演进 |
| 日期 | 2026-09-01 |
| 依据 | [software-design.md](software-design.md)、[backend-development-plan.md](backend-development-plan.md)、根 `AGENTS.md` |
| 定位 | 记录后端实现层的工程约定。公开行为仍以 `openapi/` 为唯一真源，跨资源状态仍以 [state-machines.md](state-machines.md) 为准 |

本文件记录的选择全部为 `Agent Self-Claimed`，除非另有标注；人类可随时覆盖。

## 1. 模块与目录

| 模块 | 路径 | 说明 |
| --- | --- | --- |
| 生成代码 | `gen/go` | 由 `buf generate` 产出，禁止手工修改 |
| 平台设施 | `platform` | 跨服务共享的**技术**设施 |
| 业务服务 | `services/<name>` | 每个可独立部署单元一个 `go.mod` |

`platform` 模块的硬性边界（`Human Design`：单一 platform 模块由用户选定）：

- 只包含配置、日志、追踪、gRPC 传输、PostgreSQL 连接与错误分类等技术设施。
- **不得**包含任何领域类型、业务规则、公开 HTTP DTO 或跨服务共享的聚合模型。设计文档 4.1 禁止 `common/dto` 与 `shared/domain`，本模块不是它们的替代品。
- 新增包前先自问：它是否对五个服务都成立且与业务语义无关？否则放进具体服务的 `internal/`。

| 包 | 职责 |
| --- | --- |
| `platform/config` | 环境变量加载，一次性汇报全部配置错误 |
| `platform/logging` | 结构化 slog 日志与敏感字段脱敏 |
| `platform/observability` | OpenTelemetry TracerProvider、W3C 传播、日志/追踪关联 |
| `platform/grpcx` | gRPC server/client、deadline、recovery、访问日志、错误规范化、actor/请求元数据 |
| `platform/errs` | 内部稳定错误模型与 gRPC 映射 |
| `platform/id` | 不透明公开 ID 生成 |
| `platform/pg` | 连接池、查询追踪、事务助手、迁移执行 |
| `platform/pgtest` | 集成测试用的一次性数据库（仅测试文件可导入） |

## 2. 数据库迁移

**工具：`golang-migrate/migrate` v4**（`Human Design`：由用户在本次开发中选定）。

约定：

1. 迁移文件位于 `services/<name>/migrations/`，通过 `go:embed` 编译进服务，运行时不依赖外部目录。
2. 命名 `NNNNNN_<snake_case_description>.up.sql` 与配对的 `.down.sql`，序号从 `000001` 开始、全服务内单调递增。
3. 只写纯 SQL。`schema_migrations` 表由 golang-migrate 自行创建和维护，迁移文件不得自建同名表。
4. 每个 `.up.sql` 必须有可执行的 `.down.sql`；无法回滚的破坏性变更改用 expand/contract 分阶段迁移（设计文档 9.3）。
5. 迁移由 `platform/pg.Migrate` 执行，其内部使用 golang-migrate 的咨询锁，多副本同时启动是安全的。
6. 迁移只改结构，不写业务数据。

## 3. 数据库账号与本地环境

每个服务拥有独立数据库和独立账号，跨库访问由 PostgreSQL 拒绝而不是靠约定（设计文档 9.2）。

本地环境：

```bash
docker compose -f deploy/local/docker-compose.yml up -d
```

创建 `account_db`、`marketplace_db`、`messaging_db`、`favorite_db` 四个库及同名 `*_svc` 账号，并对每个库 `REVOKE CONNECT ... FROM PUBLIC`，只授予属主。

PostgreSQL 大版本固定为 **18**（本地 compose 与 CI service 一致）。实例形态、备份与恢复目标仍是设计文档 11.3 的待确认项。

## 4. 测试

| 层级 | 位置 | 依赖 |
| --- | --- | --- |
| 领域与纯逻辑单元测试 | 与被测包同目录 | 无外部依赖 |
| 仓储、事务、并发、幂等集成测试 | 服务 `adapter/postgres` 及应用层 | 真实 PostgreSQL |
| gRPC 服务测试 | 服务 `adapter/grpc` | 真实 PostgreSQL + 进程内 gRPC |
| Gateway 契约测试 | `services/gateway` | 进程内假下游或真实服务 |

集成测试通过 `platform/pgtest.New` 获取**每个测试独立**的数据库：它创建临时库、执行该服务的迁移、测试结束后强制删除，因此测试可以并行。

需要管理员 DSN：

```bash
export SHORTTERM_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable'
npm run go:test:race
```

未设置该变量时数据库测试会 `t.Skip`。CI 的 `Backend / go` job 始终提供该变量，因此设计文档第 10 章要求的事务、并发与幂等证据不会在关键门禁中静默缺席。

## 5. 错误模型

领域错误一律使用 `platform/errs.Error`，其 `Code` 直接复用 OpenAPI 的 `ErrorCode` 枚举，服务因此无法发明契约无法表达的错误。

- 跨进程传输：错误码写入 `google.rpc.ErrorInfo` 的 `reason`（`domain = "shortterm"`），Gateway 不解析人类可读消息。
- gRPC 状态码映射见设计文档 7.3，实现见 `platform/errs`。
- `grpcx` 的 `normalizeErrorInterceptor` 保证离开服务的每个错误都带契约错误码；未分类错误一律降级为 `INTERNAL_ERROR` 并使用通用消息，数据库或驱动的原始文本不会外泄。
- HTTP 状态码与响应体由 Gateway 依据同一个 `Code` 生成，是公开契约到内部模型的唯一映射点。

## 6. 日志与追踪

- 统一字段：`service`、`environment`、`trace_id`、`span_id`、`request_id`、`actor_id`、`error_code`。
- `platform/logging` 在写入时脱敏：密码、令牌、`Authorization`、学号、微信、QQ、消息正文、DSN 与 OSS 密钥。脱敏发生在 handler 层，不依赖调用点自觉。
- 数据库 span 记录静态 SQL 文本，**不记录**查询参数（参数携带敏感值）。
- 所有 gRPC 客户端调用必须有 deadline：`grpcx.Dial` 在缺少 `DefaultTimeout` 时直接拒绝构造，已继承的 deadline 原样向下传播、不在每一跳延长。

## 7. 标识符

公开 ID 由 `platform/id` 生成：`<前缀>_<26 位 Crockford base32 ULID>`，例如 `p_01ARZ3NDEKTSV4RRFFQ69G5FAV`。

- 对客户端不透明，不暴露数据库类型或自增序列。
- 字典序与创建顺序一致，可直接用作分页的确定性次级键。
- 长度远小于 OpenAPI `Identifier` 的 64 字符上限。
