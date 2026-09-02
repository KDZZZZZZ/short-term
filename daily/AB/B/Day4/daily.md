# D4 日报

> 成员：B

## 遗留问题的回答

部署"成功"不能只看容器是不是 running。得从公开 API 走一条真实业务链，确认数据库真的 ready、服务间调用通、Trace 和 Outbox 没异常，才算数。

## 目标

完成 E2E、接口联调和生产验收，覆盖 PostgreSQL readiness、生产主机 Python 环境、公网 API 和 trace/outbox，发现的问题修完重新验。

## 实际进展

`scripts/backend-e2e.py` 用合成账号跑通了注册、登录、改联系方式、发商品、搜索、收藏、商品上下文会话、发消息已读，一直到交易完整状态流转，覆盖买家、卖家和第二买家三个角色，也验了权限、错误码、重复请求幂等和商品状态联动。最终结果 `status=COMPLETED projection=SOLD`，readiness、Trace 串联、Outbox 积压和指标都留了证据。

## 遇到的问题与解决

之前判断部署成功就是看八个容器是不是全绿，这次上了真实链路验收，立刻抓出两个只在生产才暴露的问题。

一个是 PostgreSQL 的 readiness。容器健康检查明明过了，Account 的迁移却报 `connection refused`，而且只有全新数据卷才复现，重跑一次又好了，一开始还怀疑是迁移脚本的时序问题。翻官方镜像的 entrypoint 才看明白：初始化阶段它会先起一个 `listen_addresses=` 的临时服务，只监听 Unix socket，压根不听 TCP。我们的探测走的正是 socket，所以数据库还没对外开门就被判成 ready 了。把本地 Compose、CI 和生产脚本统一改成 `pg_isready -h 127.0.0.1`，fresh volume 连跑五次都稳。

另一个是验收脚本在服务器上直接跑不起来。主机 Python 是 3.6.8，`from __future__ import annotations` 和 dataclass 一个都不认，`py_compile` 当场退 1。把脚本降回 3.6 兼容语法，再用固定摘要的 3.6.8 镜像验一遍，同时说明 Python 只用于验收和日志解析，不带进 Go 服务镜像。

两个问题修完都重跑了完整 E2E，没拿一次手工成功顶替验收。

## 后续计划

如果还要 D5/D6，就接着记中期检查、答辩演示和最终验收；如果项目已经能演示了，就整理个人贡献、未完成项和学习总结。
