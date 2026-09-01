# Git 协作规范

## 唯一基线

- `main` 是唯一集成和部署基线，不创建 `dev`、`develop` 或长期发布分支。
- 禁止直接推送、强推或删除 `main`；所有变更必须通过 PR。
- PR 不要求同行审批，但必需状态检查必须全部通过。
- 只允许 squash merge，PR 标题会成为 `main` 上的提交标题。

## Feature 分支

每项任务使用一个短期分支和一个 PR：

```bash
git fetch origin
git switch main
git pull --ff-only origin main
git switch -c feature/<short-topic>
```

分支要求：

- 统一使用 `feature/<short-topic>`，主题使用小写英文和连字符。
- 必须从最新 `origin/main` 创建。
- 一个分支只处理一个目标，不混入顺手重构或无关格式化。
- 提 PR 前将分支同步到最新 `origin/main`；需要重写个人 feature 分支时，只能使用 `--force-with-lease`。
- PR squash 合入后由 GitHub 自动删除远端 feature 分支，本地分支确认合入后再删除。

## 提交与 PR

提交及 PR 标题使用 Conventional Commits：

```text
feat: add trade confirmation endpoint
fix: reject self-purchase requests
docs: clarify product state transitions
test: cover concurrent trade acceptance
ci: enforce OpenAPI bundle drift check
refactor: isolate contact visibility policy
chore: update development tooling
```

禁止使用含义不清的标题，例如 `update`、`misc`、`changes`。

PR 正文必须包含：

```markdown
## Human Design

- 用户明确要求或确认的产品、API、架构及治理决策。

## Agent Self-Claimed

- 实现者自行选择的文件、命名、工具、抽象和未被用户明确指定的行为。

## Validation

- 实际执行的命令、退出码、结果和未执行的关键检查。
```

## OpenAPI 契约

- `openapi/openapi.yaml` 是源契约，不能手工编辑 `openapi/openapi.bundle.json`。
- 接口、字段、错误码、权限或状态流转变化必须先更新 OpenAPI 和状态机文档。
- 提交前必须运行 `npm ci` 和 `npm run openapi:check`。
- 后端实现加入仓库后，接口实现必须增加针对 OpenAPI 的契约测试；不能以 Controller 注解反向覆盖契约。
- 状态不能通过通用 `PATCH` 任意改写，必须调用 OpenAPI 中的动作接口。

## 安全与历史

- 禁止提交访问密钥、SSH 私钥、密码、Token、服务器清单或 `.env`。
- 禁止对共享分支使用 `git push --force`。
- 已进入 `main` 的错误用新 PR 修复；不改写 `main` 历史。
- 如发现疑似凭据，停止推送并先轮换凭据，再清理历史。
