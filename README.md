# 校园二手交易平台 MVP

本仓库以 OpenAPI 为唯一接口契约，当前范围包括：

- 学号与密码注册登录；
- 用户资料和微信/QQ 联系方式；
- 商品发布、图片、列表、搜索及上下架；
- 收藏；
- 商品上下文中的站内文字聊天与已读状态；
- 购买意向、卖家接受、双方确认、取消等交易状态流转。

明确不包含推荐算法和邮件通知。

## 契约

- 源文件：`openapi/openapi.yaml`
- 生成文件：`openapi/openapi.bundle.json`
- 状态机：`docs/state-machines.md`
- Git 协作规范：`CONTRIBUTING.md`

本地校验：

```bash
npm ci
npm run openapi:check
```

`openapi:check` 会执行规范校验、重新生成 bundle，并通过 Git diff 阻止源契约与生成产物漂移。

## 分支与发布

- `main` 是唯一基线和部署分支。
- 所有工作从最新 `main` 创建 `feature/<short-topic>`。
- 只能通过 PR 合入 `main`，不要求同行审批，但必须通过 CI。
- 仓库只允许 squash merge，合入后自动删除 feature 分支。
- `main` 更新后，GitHub Actions 自动把 Swagger UI 部署到配置的阿里云 ECS。

详细规则见 `CONTRIBUTING.md`。
