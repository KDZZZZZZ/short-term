# 状态机与一致性规则

本文档与 `openapi/openapi.yaml` 共同构成公开 API 的治理依据。OpenAPI 描述单次请求和响应，本文档负责跨资源状态变化和并发约束。

## 用户身份

用户不存在固定的“买家”或“卖家”角色：

- `product.seller_id == user.id` 时，用户是该商品卖家。
- `trade.buyer_id == user.id` 时，用户是该交易买家。
- 同一用户可以同时购买其他用户的商品并发布自己的商品。
- 用户不能收藏、咨询或购买自己发布的商品。

## 收藏

```text
不存在收藏记录 --收藏--> 存在收藏记录 --取消收藏--> 不存在收藏记录
```

收藏接口幂等，联合键为 `(user_id, product_id)`。

## 消息已读

```text
UNREAD (read_at = null) --会话参与者查看--> READ (read_at != null)
```

只能将对方发送给自己的消息标记为已读，已读操作幂等且不可逆。

## 交易状态

```text
PENDING --卖家接受--> ACCEPTED --双方均确认--> COMPLETED
   |                      |
   +--买家取消/卖家拒绝--> CANCELLED
                          |
                          +--任一方取消------> CANCELLED
```

权限与副作用：

| 当前状态 | 动作 | 操作者 | 新状态 | 商品副作用 |
| --- | --- | --- | --- | --- |
| 不存在 | 发起交易 | 非卖家的登录用户 | `PENDING` | 保持 `ON_SALE` |
| `PENDING` | 接受 | 卖家 | `ACCEPTED` | `ON_SALE -> RESERVED` |
| `PENDING` | 拒绝 | 卖家 | `CANCELLED` | 无 |
| `PENDING` | 取消 | 买家 | `CANCELLED` | 无 |
| `ACCEPTED` | 取消 | 买家或卖家 | `CANCELLED` | `RESERVED -> ON_SALE` |
| `ACCEPTED` | 确认 | 买家或卖家 | 保持 `ACCEPTED` | 记录当前用户确认时间 |
| `ACCEPTED` | 双方均确认 | 系统 | `COMPLETED` | `RESERVED -> SOLD` |

`COMPLETED` 和 `CANCELLED` 是交易终态。

## 商品状态

```text
创建并发布
    |
    v
 ON_SALE --卖家接受交易--> RESERVED --双方确认完成--> SOLD
    |                         |
    |                         +--交易取消----------> ON_SALE
    |
    +--卖家下架-----------> OFF_SHELF --重新上架--> ON_SALE
```

约束：

- `SOLD` 是终态。
- `RESERVED`、`SOLD`、`OFF_SHELF` 商品不能创建或接受新交易。
- 公开商品列表只返回 `ON_SALE`。
- 商品状态只能由商品动作接口或交易状态机驱动。

## 事务、幂等与并发一致性

### 通用事务边界

所有同时改变 Product 与 Trade 的动作必须在 Marketplace DB 的单个本地事务中完成。事务统一按 **Product -> Trade** 顺序加锁，并包含该动作产生的 Outbox 写入：

| 动作 | 必须原子提交的状态变化 |
| --- | --- |
| 卖家接受 | 目标 Trade `PENDING -> ACCEPTED`；Product `ON_SALE -> RESERVED`；同商品其他 PENDING Trade -> `CANCELLED` |
| 取消已接受交易 | Trade `ACCEPTED -> CANCELLED`；Product `RESERVED -> ON_SALE` |
| 双方完成确认 | Trade `ACCEPTED -> COMPLETED`；Product `RESERVED -> SOLD` |

第一次确认只记录当前用户确认时间，Trade 保持 `ACCEPTED`。第二次确认发现另一方已确认时，确认时间、Trade 和 Product 的最终状态必须在同一事务提交。任何校验、状态写入、Outbox 或幂等结果写入失败都整体回滚，不得暴露中间状态。

### 幂等命令

请求携带 `Idempotency-Key` 时，以 `(actor_id, operation, idempotency_key)` 唯一标识一次命令：

1. 在领域状态校验前查询或声明幂等记录；并发同键请求由唯一约束和行锁串行化。
2. 已存在成功记录时，不再校验当前 Trade/Product 状态，直接返回首次成功的规范化命令结果，由 Gateway 重建首次 HTTP 状态与响应体。
3. 首次成功处理时，领域写入、Outbox 和成功结果在同一事务提交。
4. 事务回滚时不得遗留成功记录；领域状态已提交但成功结果缺失同样视为不允许出现的不一致。

因此，服务提交成功但客户端未收到响应后，使用相同键重试仍返回首次成功结果，而不是因为 Trade 已变为 `ACCEPTED`、`CANCELLED` 或 `COMPLETED` 而返回新的 `409`。

### 并发转换

卖家接受交易时，事务必须锁定商品和目标交易，验证商品仍为 `ON_SALE`、交易仍为 `PENDING` 且操作者为卖家，再执行联动更新。数据库或服务层必须保证同一商品最多存在一个 `ACCEPTED` 交易。

取消已接受交易与第二次确认并发时，只有先取得锁且满足前置状态的一方可以提交；另一方返回 HTTP `409` 和 `TRADE_STATE_CONFLICT` 或 `PRODUCT_NOT_AVAILABLE`，且不得产生部分副作用。提交后再发送非关键通知；通知失败不能回滚已提交交易。
