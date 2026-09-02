# Day2 提交计划

- 人员：A
- 建议 Gitee commit：`feat(api): define OpenAPI contract and Swagger documentation`

## 主要内容

### OpenAPI 源契约

- `openapi/openapi.yaml`：定义 API 标题、版本、服务器、基础路径、标签、认证方式和全局响应约定。
- `openapi/paths/auth.yaml`、`users.yaml`：注册、登录、当前用户资料、密码修改和联系方式更新。
- `openapi/paths/products.yaml`：商品发布、详情、列表搜索、分类、上下架、字段修改和最多三张图片。
- `openapi/paths/favorites.yaml`、`conversations.yaml`、`trades.yaml`：收藏、商品上下文会话、消息已读、购买意向和交易动作。
- `openapi/components/schemas.yaml`、`parameters.yaml`、`responses.yaml`：统一 ID、金额、分页、图片、状态、错误码、鉴权和联系人字段的约束，避免各 path 重复定义产生漂移。

### 业务语义与生成产物

- 把联系方式只能新增/修改不能清空、`PENDING` 意向限制下架、`RESERVED/SOLD` 内容冻结、同一买家与商品复用购买意向等规则写进公开契约。
- 明确商品状态在列表、详情、收藏、会话和交易投影中的返回方式，以及会话不匹配、资源隐藏、限流和图片排序等错误/边界语义。
- `redocly.yaml`、`package.json`、`package-lock.json`：固定 Redocly 配置和 npm 脚本；`openapi/openapi.bundle.json` 由源契约生成，禁止手工维护。

### 提交边界与完成标准

- 先改源契约，再生成 bundle；不在此提交中实现 Go handler 或数据库逻辑。
- 执行 `npm ci`、`npm run openapi:check` 后，源文件、lint、bundle 和漂移检查全部通过，前后端可以依同一份契约并行开发。

## GitHub 取材

- 以 `dcb8001` 的版本为准
