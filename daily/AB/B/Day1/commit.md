# Day1 提交计划

- 人员：B
- 建议 Gitee commit：`docs(project): define software design and state machines`

## 主要内容

### 软件设计文档

- `docs/software-design.md`：建立 Gateway、Account、Marketplace、Messaging、Favorite 五个服务的职责边界和调用关系。
- 说明前端通过 REST/JSON 访问 Gateway，服务之间通过版本化 gRPC 协作；浏览器不直接依赖内部服务。
- 明确各服务的数据事实源、独立数据库/迁移边界、DTO 分层、错误映射、日志与 Trace 关联、Outbox 事件和单机部署假设。
- 把已确认的产品范围与待确认的容量、事件总线、TLS 等架构问题分开记录，避免把代理取舍误写成人类已确认决策。

### 状态机与一致性规则

- `docs/state-machines.md`：定义用户在商品和交易中的动态角色，以及不能收藏、咨询或购买自己商品等权限规则。
- 定义收藏存在/取消、消息未读/已读、商品 `ON_SALE/OFF_SHELF/RESERVED/SOLD` 和交易 `PENDING/ACCEPTED/COMPLETED/CANCELLED` 的状态转换。
- 写清卖家接受、买卖双方取消/确认、商品上下架、内容冻结、最多三张图片及连续排序等跨资源副作用。
- 固定 Product 与 Trade 的 `Product -> Trade` 加锁顺序、同一商品唯一已接受交易、购买意向幂等、Outbox 与失败回滚约束。

### 分工边界与完成标准

- B 承接 A Day1 中的软件设计和状态机部分；不再包含原先的 CI、分支保护、Swagger 部署和服务器基线任务。
- 文档能够直接指导 OpenAPI、Proto、Platform、Marketplace 和 Gateway 的后续拆分，并为并发、权限和持久化测试提供验收依据。

## GitHub 取材

- `38b6e6c`
- `ccc6a18`
