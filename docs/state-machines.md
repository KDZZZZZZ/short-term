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

## 并发一致性

卖家接受交易必须在同一数据库事务中完成：

1. 以排他锁读取商品。
2. 验证商品仍为 `ON_SALE`，交易仍为 `PENDING`，操作者为卖家。
3. 将目标交易更新为 `ACCEPTED`。
4. 将商品更新为 `RESERVED`。
5. 将同一商品的其他 `PENDING` 交易更新为 `CANCELLED`。
6. 提交事务后再发送非关键通知；通知失败不能回滚已提交交易。

数据库或服务层必须保证同一商品最多存在一个 `ACCEPTED` 交易。并发冲突统一返回 HTTP `409` 和 `TRADE_STATE_CONFLICT` 或 `PRODUCT_NOT_AVAILABLE`。
