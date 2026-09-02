# D4 日报

> 成员：A

## 遗留问题的回答

本地测试全绿不代表生产没问题，镜像、权限、端口和持久化都是本地看不见的坑。今天把后端容器化，并且拿真实服务器跑一遍部署流程。

## 目标

完成 Dockerfile、镜像构建、backend compose、Podman/systemd、Backend CI 和生产 deploy，让 Account、Marketplace、Messaging、Favorite、Gateway 加 PostgreSQL 在单机服务器上稳定起来。

## 实际进展

多阶段构建的 `Dockerfile.backend` 按 `SERVICE` 出五个服务的二进制，最终镜像用 `scratch` 和非 root 的 `65532:65532`。`deploy/backend` 下编排、启停脚本、systemd 单元都齐了，部署步骤、环境变量、数据库卷和健康检查各有位置，Backend CI 也串上了 OpenAPI、Proto、vet、race test 和容器主流程。

## 遇到的问题与解决

最费时间的是权限。第一次自动部署直接红了，原因是流水线想拿免密 sudo 装东西，服务器上没给——这其实是对的，不该为了省事把 root 权限交出去。改成 rootless Podman 跑整套服务，配 systemd 用户级单元加 linger，退出 SSH 会话或者主机重启都能自己起来，另外写了个一次性的 `bootstrap-host.sh` 做主机初始化，密码不进 Git、不进 release、不落在服务器上。

第二个是端口。本来打算用 18080，起来发现被机器上一个无关的 `intellex-api` 占着，18081 又是 sshd/Workbench 在听。没去动别人的服务，Gateway 改绑 `127.0.0.1:18083`，管理指标留在回环，公网只开 API 那一个口，顺手在部署前加了端口归属检查，免得下次再撞。

## 后续计划

把生产环境交给 B 做公开 API 和 E2E 验收，帮着一起定位接口联调、PostgreSQL readiness、Trace 和 Outbox 的问题。
