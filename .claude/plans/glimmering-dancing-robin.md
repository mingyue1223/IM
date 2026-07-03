# 朋友圈 Feed 流改造为推拉结合

## Context（背景与目标）

当前朋友圈是**纯推模式（写扩散）**：发布时 `MomentFeedConsumer` 遍历作者全部好友，逐个 `ZADD` 写入 `timeline:{friendID}`（`internal/consumer/moment_feed_consumer.go:118-138`）；读取时只读自己的 `timeline` ZSet（`moment_service.go:191`）。

三个问题：
1. **写扩散风暴**——好友多的作者发一条会产生 N 次同步 ZADD，单 goroutine 消费会拖垮整个 `moment_push` 队列。
2. **分页有 bug**——`GetMomentFeed` 用 `ZRANGEBYSCORE`（升序、闭区间，`redis_repo.go:425`），返回的是"最旧的一批"且边界会重复；无游标语义、深分页无保护。
3. **读放大 N+1**——`GetFeed` 对每个 momentID 单独 `GetMomentByID`（`moment_service.go:211`）。

**目标**：改造为推拉结合。普通动态（作者好友数 ≤ 阈值）写扩散到好友收件箱；大V动态（好友数 > 阈值）仅存作者寄件箱，好友读取时合并拉取；用复合游标分页规避重复/漏读与深分页衰减。

## 采用的方案

### 数据模型（Redis）
- **收件箱** `timeline:{userID}`（沿用现有 key）：ZSet，member=momentID，score=发布时间戳(ms)。只存**普通好友推来的**动态。
- **寄件箱** `outbox:{authorID}`（新）：ZSet，同结构。**每条动态都写**作者寄件箱（作为拉取源与自己可见的来源）。
- **大V集合** `moment:big_users`（新）：SET，作者好友数首次超阈值时 `SADD`。**只增不删（sticky）**——避免大V掉回普通后历史动态漏读。

由此**收件箱与拉取源天然无重叠**：自己的动态不再推给自己（只在自己 outbox）；大V动态不推（只在其 outbox）；普通好友动态只在收件箱。仍按 momentID 去重兜底。

### 写路径（发布）
- `PublishMoment`（`moment_service.go:45`）**不变**：落库 + 投递 `moment_push`。分类逻辑放到消费者（那里本就要拉好友列表，成本合并）。
- `MomentFeedConsumer.process`（`moment_feed_consumer.go:94`）改为：
  1. `AddToOutbox(authorID, momentID, ts)` —— 替换原"写作者自己 timeline"那步。
  2. `visibility==3`（私密）→ 到此结束（仅在自己寄件箱）。
  3. `CountFriends(authorID)`：
     - `> 阈值` → `MarkBigUser(authorID)`，**跳过扇出**。
     - `≤ 阈值` → 用 **Redis pipeline** 批量 `ZADD` 到各好友 `timeline:{friendID}`（替换逐个 ZADD），并对收件箱做长度裁剪。
  4. 扇出后 `TrimZSetByCount(timeline:{friendID}, maxLen)` 控制收件箱膨胀。

### 读路径（GetFeed，推拉合并 + 游标）
`MomentService.GetFeed(userID, cursor, limit)` 重写为：
1. 解析 cursor → `(maxTs, maxID)`；首页 cursor 空 → `maxTs=+inf`。
2. **拉取源** = 自己的 `outbox:{userID}` ∪ `FilterBigUsers(好友列表)` 得到的大V好友 `outbox:{bigFriend}`。
3. **推取源** = 自己的 `timeline:{userID}`（收件箱）。
4. 对每个 ZSet 用 `ZREVRANGEBYSCORE key max=maxTs min=-inf WITHSCORES LIMIT 0 (limit+1)` 取一页（降序）。
5. Go 内归并：按 `(ts desc, id desc)` 排序，剔除 `ts==maxTs && id>=maxID` 的已读项，按 momentID 去重，取 `limit` 条。
6. **一次** `GetMomentsByIDs(ids)` 批量补全（消除 N+1）。
7. 计算 `next_cursor`（最后一条的 `ts,id`，base64 编码 `{ts}_{id}`）返回。

无 OFFSET → 深分页不衰减；开区间 + 复合游标 → 无重复/漏读。

### 可见性（本次最小范围）
寄件箱/收件箱均排除 `visibility==3`（与现有扇出一致）。`GetMomentsByIDs` 过滤掉"非作者本人的私密动态"。`GetMoment`/`LikeMoment` 等其他读路径的完整鉴权**不在本次范围**，作为独立后续项（见下）。

## 具体改动文件

**配置**
- `internal/config/config.go`：新增 `MomentConfig{ BigUserFriendThreshold int; TimelineMaxLen int }`，`Config` 加 `Moment MomentConfig \`yaml:"moment"\``。yaml 加载自动识别，无需改 `LoadConfig`。
- `configs/config.example.yaml` + `configs/config.test.yaml`：新增 `moment:` 段（如 `big_user_friend_threshold: 500`、`timeline_max_len: 1000`）。

**MySQL 仓储**（`internal/repository/mysql_repo.go`，接口在 `:13-68`）
- 新增 `CountFriends(ctx, userID) (int, error)`：`SELECT COUNT(*) FROM friendships WHERE user_id=?`（走 `idx_user`）。
- 新增 `GetMomentsByIDs(ctx, ids []int64) ([]model.Moment, error)`：`WHERE id IN (...)`，动态占位符。消除 `GetFeed` 的 N+1（替代 `moment_service.go:211` 循环）。

**Redis 仓储**（`internal/repository/redis_repo.go`，接口在 `:53,61-62`）
- 新增 `AddToOutbox(ctx, authorID, momentID, ts)`：`ZADD outbox:{authorID}`。
- 新增 `MarkBigUser` / `FilterBigUsers(ctx, ids []int64) ([]int64, error)`：`SADD moment:big_users` / `SMISMEMBER`。
- 新增 `GetFeedPage(ctx, key, maxTs, maxID, limit)`：`ZREVRANGEBYSCORE ... WITHSCORES LIMIT 0 N`，返回 `[]{ID,Ts}`。收件箱与寄件箱共用。
- 新增 `TrimZSetByCount(ctx, key, maxLen)`：`ZREMRANGEBYRANK key 0 -(maxLen+1)`。复用现有 `TrimTimelineByTime`（`:388`）模式。
- 现有 `PublishMomentFeed`/`GetMomentFeed` 保留或标记废弃（`GetFeed` 不再用后者）。

**消费者**（`internal/consumer/moment_feed_consumer.go`）
- 构造函数加 `threshold`、`timelineMaxLen` 参数。
- `process` 按上文"写路径"重写：outbox 优先，按 `CountFriends` 分流，pipeline 批量扇出，扇出后 trim。

**服务层**（`internal/service/moment_service.go`）
- `GetFeed` 签名改为 `(ctx, userID, cursor string, limit int) ([]model.Moment, string, error)`（返回 `next_cursor`），按"读路径"重写。
- `PublishMoment` 不变。

**游标编解码**：新增 `internal/model/moment.go` 或服务内 `encodeCursor(ts,id)/decodeCursor(s)`，base64(`{ts}_{id}`)。

**Handler**（`internal/api/moment_handler.go:295` `GetFeed`）
- query 参数 `last_sync_time` → `cursor`（string，默认空）。
- 响应 `momentFeedResponse` 增 `next_cursor` 字段（`:63`）。

**装配**（`cmd/server/main.go`）
- `:104` `NewMomentService(...)` 与 `:132` `NewMomentFeedConsumer(...)` 传入 `cfg.Moment.*`（仿 `:101` authSvc 注入 JWT 配置的写法）。

## 状态转换的正确性说明
- 普通→大V：切换前已推入好友收件箱的旧动态保留可见；之后新动态只进 outbox，读取时靠"该好友已在 big_users → 拉其 outbox"补齐。按 momentID 去重防重复。
- 大V→普通：`moment:big_users` **sticky 不删**，故仍会拉其 outbox，历史不漏读。（代价：该作者转普通后不再享受推的即时性，可接受。）

## 冷启动/降级（记录，不在本次实现）
outbox/timeline 是 Redis 缓存，清空后 Feed 为空。可后续加"Redis 未命中时从 `moments`+好友关系回填"的兜底。本次不做，仅在注释标注。

## 后续独立项（不在本次范围）
- 读路径完整鉴权（`GetMoment` 私密动态暴露）。
- 发布投递失败的 outbox 补偿。
- 评论 ID 改 Snowflake、`LikeMoment` 错误映射修正。

## 验证方式
1. `go build ./...` 与 `go vet ./...` 通过。
2. 单测：更新 `internal/service/moment_service_test.go`、`internal/consumer/consumer_test.go`，覆盖：
   - 好友数 ≤ 阈值 → 走扇出、好友收件箱有该 momentID、未进 big_users。
   - 好友数 > 阈值 → 未扇出、进 big_users、仅 outbox 有。
   - `GetFeed` 合并：自己 outbox + 大V好友 outbox + 普通好友收件箱三源归并、按 (ts,id) 降序、去重。
   - 游标：连续两页无重复无漏读；同一 ms 内多条按 id 正确分界。
   - 私密动态不进任何好友源。
3. `go test ./...` 全绿。
4. 端到端（`tests/e2e_test.go` 若覆盖 moment）：发布→拉 feed→带 next_cursor 拉下一页，断言顺序与去重。
