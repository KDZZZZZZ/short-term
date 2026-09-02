# D3 日报

> 成员：B

## 遗留问题的回答

后端架构定了，但服务之间没有独立的内部契约，Account、Marketplace、Messaging、Favorite 还是会各自猜字段和错误码。先把 proto 固定下来，再让几个模块并行实现。

## 目标

完成 proto、gen、Buf、Messaging、Favorite、Gateway 和 workspace：定义 gRPC 契约，生成 Go 代码，加上格式和 breaking change 检查，让服务能照契约并行开发。

## 实际进展

四个服务的 proto 加一份带 `event_id`/`schema_version`/`trace_id` 的 Outbox 事件信封都定完了，`buf.yaml`、`buf.gen.yaml` 固定了 lint 规则、breaking 规则和插件版本，`gen/go/**` 全部由 `buf generate` 产出。Messaging、Favorite、Gateway 三个模块也实现了，`go.work` 把生成代码、Platform 和五个服务接进同一个 workspace，同时每个服务还能单独构建。

## 遇到的问题与解决

一开始我把 proto、protoc、Buf 当成一个东西了，配置抄来抄去总有地方对不上。拆开之后才清楚：proto 是契约，protoc 负责生成，Buf 管项目级的格式、lint、生成和兼容性检查，各干各的就顺了。

真正踩到的坑是字段号。Messaging 的会话消息里删了个没用上的字段，我顺手把它的编号给下一个新字段用了，本地编译一切正常。`buf breaking` 直接把 PR 拦下来，报字段号复用不兼容——要是这版发出去，旧客户端解出来的数据会是错的，而且不报错，是静默错。后来规矩改成：字段号只增不复用，废弃的留着标记；生成代码一律不手改，要改回 proto 源文件再 `buf generate`，CI 里 drift 检查兜底。

## 后续计划

准备生产部署后的端到端联调，重点记 Gateway、服务间 gRPC、PostgreSQL、Trace 和 Outbox 在真实请求链路里的表现。
