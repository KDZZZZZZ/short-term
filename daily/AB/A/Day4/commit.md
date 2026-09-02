# Day4 提交计划

- 人员：A
- 建议 Gitee commit：`feat(deploy): containerize and deploy backend services`

## 主要内容

### 镜像构建

- `Dockerfile.backend`：采用多阶段 Go 构建，按 `SERVICE` 生成 account、marketplace、messaging、favorite、gateway 的 `server`，并按需生成 `migrate`/`worker`。
- 最终镜像使用 `scratch`、固定 CA/时区文件和非 root `65532:65532` 用户，只携带该服务所需二进制；通过版本和提交 SHA 标签追踪发布内容。
- `scripts/build-backend-images.sh`：统一构建五个服务镜像，可选打包固定摘要的 PostgreSQL 镜像和 SHA-256 校验文件，保证 CI 与服务器使用同一 release。

### Compose、Podman 与 systemd

- `deploy/backend/compose.ci.yml`：编排 PostgreSQL、四个迁移任务、五个服务、Marketplace/Messaging worker 和 Gateway，配置私有网络、独立数据库、媒体卷、健康检查和就绪依赖。
- `deploy/backend/short-term-backend.sh`、`bootstrap-host.sh`：初始化部署目录、rootless Podman 网络/卷、运行时环境、随机密钥、迁移、启动/停止、状态、指标、Trace/Outbox 检查和 release 回滚。
- `deploy/backend/short-term-backend.service`：以部署用户运行后台服务，配合 systemd linger 保证退出会话或主机重启后仍能恢复。
- `deploy/local/**`：保留本地 PostgreSQL 和服务开发方式，使本地环境与 CI/生产的数据库和 readiness 语义一致。

### CI 与生产发布

- `.github/workflows/backend-ci.yml`：串联 OpenAPI、Proto、Go vet、race test、镜像边界检查和容器 REST 主流程；验证非 root、只读根文件系统、迁移和 worker 入口。
- `.github/workflows/deploy.yml`：只发布已通过 Backend 检查的提交，上传不可变 release bundle，执行迁移/就绪检查，启动服务并保留失败回滚路径。
- 生产 Gateway 只暴露 API 端口，管理指标保持主机回环；数据库、gRPC 和密钥文件不进入公网暴露面。

### 提交边界与完成标准

- 本提交负责容器化、后端运行拓扑、CI 和生产部署基线；接口语义与 E2E 缺陷修复由下一条提交计划负责。
- 通过 shell 语法、Compose 配置、镜像文件边界、readiness、迁移和容器启动检查后，才能交给真实公网接口验收。

## GitHub 取材

- `e8c0aec`
- `5c84064`
