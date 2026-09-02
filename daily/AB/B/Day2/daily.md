# D2 日报

> 成员：B

## 遗留问题的回答

昨天的设计文档说清了服务怎么分，但没说用什么实现。技术选型和边界不落成可执行的规范，CI 顶多帮你抓格式错误，拦不住服务职责一天天漂。

## 目标

定下 Go、gRPC、PostgreSQL 的技术选型，明确微服务边界、数据所有权、DTO、CORS、Outbox 和后端目录约定，形成 backend conventions 和 development plan。

## 实际进展

实现路线定了：Go + gRPC + PostgreSQL，REST/JSON 只作为前端唯一入口。五个服务的边界和数据事实源都写清楚了，另外补了 `docs/backend-conventions.md`（目录、迁移、独立库账号、错误模型、日志脱敏、Trace 传播、gRPC deadline、测试层级）和 `docs/backend-development-plan.md`（M0 到 M6 拆分，标好依赖、并行边界和每阶段验收证据）。

## 遇到的问题与解决

选型一开始聊废了。Go 还是 Spring Boot、PostgreSQL 还是 MySQL，两边都能说出一堆优点，纯粹是名词对名词，聊半天没有判据。后来换了个问法：我们就两个人、单机部署、商品和交易要强一致、最后还得答辩讲得清。按这四条一过，结论很快就出来了，也顺便说明白了为什么不上搜索服务和服务网格。

另一个是我差点做错的事。当时想建个 `common/dto` 让几个服务共用结构体，省得重复写。写到 Messaging 要给消息加个字段时才发现，改一处得同时动 Marketplace，服务就重新粘在一起了，独立部署也就白搭了。最后把这条禁掉，改成分层：Gateway HTTP DTO、Protobuf DTO、Application Command、Domain Entity、Persistence Row、Event DTO 各写各的，宁可多敲几行映射代码。

## 后续计划

和 A 的 Platform 实现对齐，同时定义 proto/gRPC 契约和生成流程，给 Messaging、Favorite、Gateway 并行开发准备接口。
