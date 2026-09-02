# Day1 提交计划

- 人员：A
- 建议 Gitee commit：`chore(project): establish agent rules and project baseline`

## 主要内容

### 工程规则

- `AGENTS.md`：写明校园二手交易平台 MVP 的业务范围和排除项，规定需求优先级、文档职责、API/数据/安全不变量、验证证据和代理任务路由。
- 固定 `main` 为集成和部署基线，规定 `feature/*` 分支、一个任务一个 PR、Conventional Commits、检查通过后 squash 等协作规则。
- 明确 OpenAPI、状态机、测试、部署等治理文档的职责边界，避免实现先于契约或以当前代码反向定义需求。

### 项目说明与技能锁定

- `README.md`：整理产品目标、范围内能力、明确排除的推荐/邮件等功能、开发入口和文档索引。
- `skills-lock.json`：记录经过审核的 skill 版本和锁定信息，使代理执行方式具备可复现性。
- 必要时同步 `.gitignore` 和文档链接，避免把本地产物、凭据或无关文件带入提交。

### 分工边界与完成标准

- A 只负责工程规则、项目说明和技能锁定，不包含 `docs/software-design.md`、`docs/state-machines.md` 的设计正文。
- 文档能让新成员明确项目做什么、如何协作、哪些行为必须先更新契约，以及后续任务如何留下验证证据。

## GitHub 取材

- `ccc6a18`
- `1abd7e0`
