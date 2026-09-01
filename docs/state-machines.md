# 状态机与一致性规则

本文档与 `openapi/openapi.yaml` 共同构成公开 API 的治理依据。OpenAPI 描述单次请求和响应，本文档负责跨资源状态变化和并发约束。

## 用户身份

用户不存在固定的“买家”或“卖家”角色：

- `product.seller_id == user.id` 时，用户是该商品卖家。
- `trade.buyer_id == user.id` 时，用户是该交易买家。
- 同一用户可以同时购买其他用户的商品并发布自己的商品。
- 用户不能收藏、咨询或购买自己发布的商品。

### 资料与联系方式

- 注册未提供昵称时，服务端使用固定默认值“校园用户”，不得从学号派生公开昵称。
- 微信和 QQ 可以在注册时不填写；资料更新可以首次填写或改成另一个非空值。
- 资料更新不接受用 `null` 或空字符串删除微信/QQ；省略字段表示保持原值。
- 发布商品时仍要求微信或 QQ 至少已有一项。由于已填写的联系方式不可删除，商品发布后不会出现卖家清空全部联系方式的状态。

## 收藏

```text
不存在收藏记录 --收藏--> 存在收藏记录 --取消收藏--> 不存在收藏记录
```

收藏接口幂等，联合键为 `(user_id, product_id)`。任意状态的现存商品均可收藏；
`OFF_SHELF`、`RESERVED` 和 `SOLD` 只影响交易能力，不影响详情或收藏可见性。
收藏自己的商品固定返回 `409 / SELF_ACTION_NOT_ALLOWED`。

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
| 不存在 | 创建购买意向 | 非卖家的登录用户 | `PENDING`，HTTP `201` | 保持 `ON_SALE` |
| 任意已存在状态 | 再次创建同商品意向 | 同一买家 | 状态不变，HTTP `200` | 无 |
| `PENDING` | 接受 | 卖家 | `ACCEPTED` | `ON_SALE -> RESERVED` |
| `PENDING` | 拒绝 | 卖家 | `CANCELLED` | 无 |
| `PENDING` | 取消 | 买家 | `CANCELLED` | 无 |
| `ACCEPTED` | 取消 | 买家或卖家 | `CANCELLED` | `RESERVED -> ON_SALE` |
| `ACCEPTED` | 确认 | 买家或卖家 | 保持 `ACCEPTED` | 记录当前用户确认时间 |
| `ACCEPTED` | 双方均确认 | 系统 | `COMPLETED` | `RESERVED -> SOLD` |

`COMPLETED` 和 `CANCELLED` 是交易终态。

Trade 是购买意向而不是一次 HTTP 尝试。数据库必须保证 `(product_id, buyer_id)` 在商品
整个生命周期内唯一；换用新的 `Idempotency-Key`、先前意向被拒绝或取消、商品后来重新
回到 `ON_SALE`，都不得为同一买家创建第二条 Trade。创建接口采用 create-or-get：首次
创建返回 `201`；已有意向时返回同一 Trade 的当前表示和 `200`，且不改变状态或发送新通知。
同一 `Idempotency-Key` 的重试仍优先重放首次成功的状态码和响应体。

创建请求携带 `conversation_id` 时，服务端必须验证 Conversation 的 `product_id` 与路径商品
一致，且 `buyer_id`、`seller_id` 分别为当前用户与商品卖家。会话不存在或当前用户不可见
返回 `404 / RESOURCE_NOT_FOUND`；会话商品或参与者不匹配返回
`409 / CONVERSATION_MISMATCH`，不得写入 Trade。Trade 已存在时，显式提供的值还必须与
已存 `conversation_id` 相同（包括显式 `null`）；省略字段可以读取既有 Trade，但不能修改绑定。

## 商品状态

```text
创建并发布
    |
    v
 ON_SALE --卖家接受交易--> RESERVED --双方确认完成--> SOLD
    |                         |
    |                         +--交易取消----------> ON_SALE
    |
    +--卖家下架（无 PENDING）--> OFF_SHELF --重新上架--> ON_SALE
```

约束：

- `SOLD` 是终态。
- `RESERVED`、`SOLD`、`OFF_SHELF` 商品不能创建或接受新交易。
- `PENDING` 是进行中的购买意向。只要商品存在任意 `PENDING` Trade，卖家必须先逐笔拒绝，不能将商品从 `ON_SALE` 下架。
- `PENDING` 不预留商品：Product 仍为 `ON_SALE` 并继续出现在公开列表，其他买家仍可各自创建唯一购买意向，直到卖家接受其中一笔。
- 公开商品列表只返回 `ON_SALE`。
- 已认证用户可以通过详情接口查看任意状态的现存商品；`OFF_SHELF`、`RESERVED` 和 `SOLD` 不因状态而返回 `404`。
- 商品字段和图片只能在 `ON_SALE` 或 `OFF_SHELF` 且不存在 `PENDING` Trade 时修改；存在 `PENDING` 时返回 `409 / TRADE_STATE_CONFLICT`，`RESERVED`、`SOLD` 时返回 `409 / PRODUCT_STATE_CONFLICT`。
- 商品状态只能由商品动作接口或交易状态机驱动。

### 商品图片顺序

- 同一商品的图片按 `sort_order` 升序返回，顺序始终为连续的 `1..N`。
- `cover_url` 等于 `sort_order = 1` 的图片 URL；没有图片时为 `null`。
- 新图片按上传顺序追加。删除图片后保持其余图片的相对顺序并连续重排；删除封面后新的第一张图片自动成为封面。

## 事务、幂等与并发一致性

### 通用事务边界

所有同时改变 Product 与 Trade 的动作必须在 Marketplace DB 的单个本地事务中完成。事务统一按 **Product -> Trade** 顺序加锁，并包含该动作产生的 Outbox 写入：

| 动作 | 必须原子提交的状态变化 |
| --- | --- |
| 首次创建购买意向 | 锁定 Product 并验证 `ON_SALE`；按 `(product_id, buyer_id)` 唯一创建 Trade `PENDING`；写入 Outbox 与幂等结果 |
| 再次创建同一购买意向 | 读取并返回既有 Trade；不得更新状态、价格快照、会话或产生新 Outbox 事件 |
| 商品下架 | 锁定 Product 并验证 `ON_SALE`；确认不存在 `PENDING` Trade 后更新为 `OFF_SHELF`；检查与更新之间不得插入新意向 |
| 修改商品或图片 | 锁定 Product，验证状态为 `ON_SALE`/`OFF_SHELF` 且不存在 `PENDING` Trade；变更与新意向创建不得交错提交 |
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

创建购买意向与商品下架、商品内容/图片变更必须先锁定同一 Product：创建先提交时，
下架或编辑看到 `PENDING` 并返回 `409 / TRADE_STATE_CONFLICT`；下架先提交时，创建看到
`OFF_SHELF` 并返回 `409 / PRODUCT_NOT_AVAILABLE`；编辑先提交时，新意向读取编辑后的
同一版本并生成价格快照。不得提交 `OFF_SHELF + PENDING`，也不得让意向指向交错更新的商品内容。

并发创建同一买家、同一商品的意向由 `(product_id, buyer_id)` 唯一约束串行化；一个请求
创建并返回 `201`，其他请求读取同一 Trade 并返回 `200`，不得把唯一约束错误泄漏为 `500`。

卖家接受交易时，事务必须锁定商品和目标交易，验证商品仍为 `ON_SALE`、交易仍为 `PENDING` 且操作者为卖家，再执行联动更新。数据库或服务层必须保证同一商品最多存在一个 `ACCEPTED` 交易。

取消已接受交易与第二次确认并发时，只有先取得锁且满足前置状态的一方可以提交；另一方返回 HTTP `409` 和 `TRADE_STATE_CONFLICT` 或 `PRODUCT_NOT_AVAILABLE`，且不得产生部分副作用。提交后再发送非关键通知；通知失败不能回滚已提交交易。
