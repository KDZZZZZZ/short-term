# Day4 提交计划

- 人员：B
- 建议 Gitee commit：`fix(api): complete production integration and e2e validation`

## 主要内容

### 真实接口 E2E 旅程

- `scripts/backend-e2e.py`：用合成账号完成注册、登录、资料联系方式、商品发布/查询、收藏、商品上下文会话、发消息/已读和完整交易状态流转。
- 覆盖买家、卖家及第二买家角色，检查 HTTP 状态码、响应字段、权限、错误码、商品状态联动、重复请求幂等和交易完成后的最终结果。
- E2E 必须从真实 Gateway API 入口执行，先确认 PostgreSQL、迁移、服务间 gRPC 和 worker 已 ready，不能只以容器处于 running 作为部署成功。

### 接口联调与环境修复

- 修正主机侧 acceptance 的执行位置和脚本参数，确保验收使用已部署的真实版本和公网 API。
- 兼容生产主机 Python 3.6 的语法与标准库能力；将 Python 仅用于验收脚本和日志解析，不把它带入 Go 服务镜像。
- 在本地 Compose、CI Compose 和服务器启动脚本中统一 PostgreSQL TCP readiness，避免入口进程已启动但数据库尚不可用。
- 检查 Gateway、gRPC、Marketplace/Messaging Outbox 的 request/trace 关联、消息会话校验和失败重试；修复后用真实链路重新验证。

### 验收证据与完成标准

- 保存三角色业务旅程的请求结果、最终 Product/Trade 状态、PostgreSQL readiness、Trace 串联、Outbox 积压和指标输出。
- 对失败路径、重复请求、状态冲突、权限边界和服务启动时序分别记录结果；任一步失败都修复后重跑，不用一次手工成功替代 E2E 证据。
- 生产验收通过后，提交才算完成；单元测试通过但公网主流程、持久化或跨服务链路未验证时不能标记为完成。

## GitHub 取材

- `1ff92fe`
- `774bae6`
- `0cc0d12`
- `3c84f74`
