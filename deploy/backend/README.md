# 单机后端部署运行手册

本目录把五个 Go 服务部署为彼此独立的容器。Marketplace 与 Messaging 的
Outbox worker 复用所属服务镜像，但以独立容器和 `/usr/local/bin/worker` 启动。
这套拓扑面向当前单台阿里云 ECS 和学习用途，不提供节点级高可用，也不引入
Kubernetes 或服务网格。

## 拓扑和边界

| 单元 | 容器入口 | 网络/端口 | 持久化 |
| --- | --- | --- | --- |
| Gateway | `server` | 主机回环 `127.0.0.1:18083 -> 8080`；指标 `127.0.0.1:19090 -> 9090` | 只读媒体卷 |
| Account | `server` | 私有网络 `account:9001` | `account_db` |
| Marketplace | `server`、独立 `worker` | 私有网络 `marketplace:9002` | `marketplace_db`、媒体目录 |
| Messaging | `server`、独立 `worker` | 私有网络 `messaging:9003` | `messaging_db` |
| Favorite | `server` | 私有网络 `favorite:9004` | `favorite_db` |
| PostgreSQL 18 | 官方固定摘要镜像 | 私有网络 `postgres:5432` | Podman named volume |

四个业务库使用不同登录角色，并撤销 `PUBLIC` 的数据库连接权限。应用容器以
UID/GID `65532:65532`、只读根文件系统、移除 capabilities、`no-new-privileges`
和进程数上限运行。Gateway 和管理端口当前都只绑定主机回环地址，外部验收通过
SSH 隧道访问；接入用户流量前应由带 TLS 的反向代理转发到 Gateway。

## 一次性主机初始化

生产容器和 backend systemd unit 都以部署账号运行，不给 CI 任意 sudo 权限。新机器
只需由管理员执行一次仓库中的初始化脚本：

```bash
sudo deploy/backend/bootstrap-host.sh "$(id -un)"
```

脚本只创建部署账号拥有的 `/opt/short-term` 目录并启用 systemd linger，使 rootless
Podman 用户服务在退出登录和主机重启后仍能自动运行；它不保存 sudo 密码，也不写
sudoers。当前 Swagger UI 仍是既有的 root system service，CI 只保留精确白名单的
`systemctl restart short-term-openapi.service` 权限。

## 镜像与 CI

`Dockerfile.backend` 是多阶段构建文件，最终 `scratch` 镜像只包含 CA、时区和该
服务实际需要的 `server`/`migrate`/`worker` 二进制。下面的命令构建五个独立镜像：

```bash
scripts/build-backend-images.sh local-dev
```

传入第二个参数时，脚本还会加入固定摘要的 PostgreSQL 镜像，生成可离线传输的
压缩镜像包和 SHA-256 校验文件：

```bash
scripts/build-backend-images.sh <40位Git SHA> /tmp/backend-images.tar.gz
```

Pull Request 的 `Backend / containers` 检查会验证脚本与 Compose、构建全部镜像、
检查非 root 用户和镜像文件边界，然后用真实 PostgreSQL 跑完整 REST 主流程。
OpenAPI、Proto breaking/drift、Go vet 和 `go test -race` 是同一 Backend workflow
的前置门禁。

## 自动发布顺序

`Deploy production` 只在 `main` 对应的 `Backend` workflow 成功后触发，并精确
检出已验证的提交 SHA：

1. 构建、校验并上传不可变 release bundle；
2. 在服务器生成或复用 `/opt/short-term/state/backend-secrets.env`，密钥不回传 CI；
3. 启动 PostgreSQL并配置四个隔离数据库；
4. 依次执行 Account、Marketplace、Messaging、Favorite 的显式迁移；
5. 启动四个服务、两个 worker 和 Gateway，等待标准 gRPC Health 与 `/readyz`；
6. 更新 Swagger UI；
7. 从 GitHub runner 通过 SSH 隧道连接真实服务器 REST 接口，创建真实用户、商品、
   收藏、会话、消息和完整交易；再在服务器验证同一 Trace 跨
   Gateway/gRPC/Outbox 串联且积压归零。

主机前置校验、迁移、启动、readiness、服务器 E2E、Trace 或 Outbox 任一验收失败都会让 workflow
失败。已有上一版本时，脚本交换 release manifest 并重启旧应用镜像；数据库迁移
不会回滚，因此迁移必须遵循 expand/contract。首次发布没有可回滚的旧应用版本。

## 运维命令

以部署账号登录服务器后，运维入口为 `/opt/short-term/bin/short-term-backend`：

```bash
/opt/short-term/bin/short-term-backend status
/opt/short-term/bin/short-term-backend metrics
/opt/short-term/bin/short-term-backend verify-outbox
/opt/short-term/bin/short-term-backend verify-trace <request-id>
/opt/short-term/bin/short-term-backend rollback
systemctl --user status short-term-backend.service
```

`status` 显示真实容器、存活/就绪和 Outbox 积压。`metrics` 输出 Prometheus 文本：
Gateway 的固定路由/方法/状态计数可计算交易动作成功、409 冲突和限流结果；数据库
快照提供各交易状态、持久化转换总数以及 Marketplace/Messaging Outbox 待发布与
重试数量。指标端口仅绑定服务器回环地址。

## 当前限制

- 这是单节点部署；主机、Podman 网络或 PostgreSQL 故障都会导致整体不可用。
- 当前 Outbox 发布器把版本化事件信封写入结构化日志，尚未连接消息代理。
- Gateway 当前只在服务器回环地址提供 HTTP `18083`，尚未作为公网业务入口；接入
  正式前端前必须配置域名和可信 TLS 终止层，由反向代理访问这个回环端口。
- 限流器是单 Gateway 进程内的固定窗口实现；扩为多副本前必须换成共享限流状态。
- 媒体使用主机持久目录；迁移到对象存储时应保持公开 URL 和所有权边界。
