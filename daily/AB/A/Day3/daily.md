# D3 日报

> 成员：A

## 遗留问题的回答

OpenAPI 管的是对外 HTTP 契约，管不到服务内部的配置、数据库、认证和错误处理。这些几个服务都要用的东西收进 Platform，不然每个服务都得复制一套。

## 目标

完成 Platform 和核心模块：platform 目录、本地 PostgreSQL/Compose、Account、Marketplace，以及配置、ID、日志、Tracing、认证、错误、migration 和 gRPC 的公共封装。

## 实际进展

按模块边界拆好了 `platform/`、`deploy/local/`、`services/account/` 和 `services/marketplace/`。公共能力各有独立目录和测试入口，本地 Compose 起 PostgreSQL 18 并给四个服务建了各自独立的库和账号，跨库连接直接撤掉。Account 的注册登录改密、Marketplace 的商品与交易状态机也跑通了最小链路。

## 遇到的问题与解决

写 Account 登录的集成测试时翻日志，发现结构化日志把整个请求体打出来了，密码明文就躺在里面。虽然是本地环境，但这个习惯带到生产就完了。当天给 `platform/logging` 补了字段脱敏，密码、Token、联系方式统一按 key 打码，顺手加了条测试卡住。

还有一个是集成测试互相串。几个服务的测试并行跑，共用同一个库，前一个用例插的数据把后一个的断言干挂了，重跑又时好时坏。后来做了 `platform/pgtest`，每个测试单独开临时库，跑完销毁，这才稳定。

至于 Platform 本身，最需要克制的是别让业务逻辑往里钻。现在只留跨服务真正复用的那几样，Account 和 Marketplace 的业务规则一律留在各自服务里。

## 后续计划

配合部署那边做镜像和运行检查，确认 Platform、Account、Marketplace 在容器和本地 Compose 里都能起来并跑通最小测试。
