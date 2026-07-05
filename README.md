# GoIM — 高并发即时通讯系统

一个基于 Go 语言构建的生产级 IM（即时通讯）系统，采用微信风格的隐私设计、Redis 优先架构配合异步 MySQL 持久化。

## ✨ 功能特性

| 分类 | 功能 |
|----------|----------|
| **消息** | 私聊（推送模式）、群聊（拉取模式）、消息撤回、送达/已读回执、离线同步 |
| **社交** | 好友请求/接受/拒绝/拉黑、双向好友关系、朋友圈发布/推拉结合Feed流/高并发点赞(Lua原子+批量削峰)/评论 |
| **群组** | 创建/更新/退出群组、成员管理（添加/移除/踢出）、角色体系（群主/管理员/成员） |
| **设置** | 通知偏好、消息预览开关、会话免打扰列表 |
| **实时通信** | 基于 JWT 认证的 WebSocket、单设备登录策略（踢出旧连接）、心跳、推送通知 |
| **可靠性** | Redis Lua 原子操作、RabbitMQ 异步持久化、3天 TTL 自动清理 |

## 🏗 架构

```
┌─────────────┐     ┌──────────────────┐     ┌────────────┐
│   客户端      │────▶│   Gin HTTP + WS   │────▶│   Redis    │
│  (浏览器/    │     │   服务端           │     │   (优先)   │
│   移动端)    │◀────│                   │◀────│            │
└─────────────┘     │   ┌──────────┐    │     └────────────┘
                    │   │ MQ 发布  │────│────▶┌────────────┐
                    │   │ (RabbitMQ)│    │     │  RabbitMQ  │
                    │   └──────────┘    │     │  消费者     │
                    │                   │     │ ┌────────┐ │
                    │   ┌──────────┐    │     │ │ MySQL  │ │
                    │   │ Lua EVAL │◀───│     │ │ 持久化 │ │
                    │   └──────────┘    │     │ └────────┘ │
                    └──────────────────┘     └────────────┘

MQ 消费者:
  private_msg_persist  → 写 inbox ZSet + 推 WS + 写 MySQL
  group_msg_fanout     → 写 outbox ZSet + 推在线成员 + 写 MySQL
  moment_push          → 推拉结合: 写寄件箱 + 大V判定 + 普通用户写扩散到好友收件箱
  like_persist         → 攒批削峰: last-action-wins 折叠 + 批量 INSERT IGNORE / DELETE
  comment_persist      → 评论异步写 MySQL
```

### 核心设计决策

1. **Redis 优先模式**：所有读写优先经过 Redis。MySQL 持久化通过 MQ 消费者异步完成。
2. **私聊推送模式**：消息推送到每个用户的 `inbox:{userID}` ZSet。接收方连接实时收到推送。
3. **群聊拉取模式**：消息存储在群组维度的 `outbox:{groupID}` ZSet。成员通过同步请求拉取消息。
4. **微信隐私设计**：发送者**无法**查看接收者的已读状态。`readStatus` 仅对接收者本人可见。
5. **Lua 原子操作**：好友校验、去重、msgID 分配、收件箱写入、标记已读、撤回等操作均通过 Redis Lua 脚本原子执行。
6. **3天 TTL**：收件箱/发件箱/时间线通过 `ZREMRANGEBYSCORE`（按时间）+ `ZREMRANGEBYRANK`（按数量上限）自动过期。
7. **单设备登录策略**：新的 WebSocket 连接会踢出旧连接，发送 `{"type":"kick","reason":"new_login"}`。
8. **朋友圈推拉结合 Feed 流**：普通用户发布时写扩散到好友收件箱 `timeline:{userID}`（推）；所有动态同时写入作者寄件箱 `moment_outbox:{authorID}`（拉）。好友数超阈值的大V用户跳过写扩散，其动态由好友从寄件箱拉取合并。读取时多源（自己的寄件箱 + 收件箱 + 大V好友寄件箱）归并去重，复合游标 `(ts, id)` 分页。
9. **点赞高并发优化**：点赞/取消赞用 Lua 脚本原子执行 SADD/SREM + INCR/DECR，消除竞态。首次访问时从 MySQL 按需预热（loaded 标记 + NX 锁防缓存击穿）。点赞落库采用攒批策略（batchSize + flushInterval），批内 last-action-wins 折叠后批量 INSERT IGNORE / DELETE，削峰且幂等。

### 会话 ID 格式

- 私聊：`p_{较小ID}_{较大ID}`（例如 `p_1_2`）
- 群聊：`g_{groupID}`（例如 `g_42`）

## 🚀 快速开始

### 前置条件

- Go 1.22+
- Docker 与 Docker Compose
- MySQL 8、Redis 7、RabbitMQ 3（或使用下方的 Docker Compose）

### 1. 启动基础设施

```bash
docker-compose up -d
```

### 2. 运行数据库迁移

```bash
# 按顺序执行迁移文件
for f in scripts/migrations/*.sql; do
  mysql -u goim -pgoim123 goim < "$f"
done
```

### 3. 构建并运行

```bash
go build -o goim-server ./cmd/server
./goim-server -c configs/config.yaml
```

或直接运行：

```bash
go run ./cmd/server -c configs/config.yaml
```

### 4. 验证

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"goim"}
```

### 5. 运行端到端测试

```bash
# 先启动 Docker 服务，然后执行：
go test ./tests/... -v -tags e2e -timeout 120s
```

### 6. 运行单元测试

```bash
go test ./internal/... -v
```

## ⚡ 性能压测

单机 Docker 环境（MySQL 8.4 + Redis 7.2 + RabbitMQ 3.13）下的压测结果。

### 目标达成

| 指标 | 目标 | 实测 | 达成率 |
|------|------|------|--------|
| WebSocket 并发连接 | ≥ 10,000 | **50,000** | 500% |
| 私聊消息 QPS | ≥ 5,000 | **29,000** | 580% |
| P99 延迟 | ≤ 100ms | **7.42ms**（200 连接） | 13.5× 优于目标 |
| goroutine 泄漏 | 无 | ✅ 线性增长 | 通过 |

### WebSocket 并发连接

每连接固定增加 2 个 goroutine（ReadPump + WritePump），零泄漏。

| 同时在线 | goroutine 总数 | 成功率 | 备注 |
|----------|---------------|--------|------|
| 10,000 | 20,018 | 100% | 达到目标 |
| 28,000 | 56,018 | 100% | 单客户端临时端口池极限 |
| 50,000 | 100,008 | 100% | 服务端极限 |

### 消息写入 QPS

采用 发→收 serverAck→立即发下一条 的流水线模式，每个消息走完整同步路径（Redis Lua 原子校验 → MQ 异步发布 → serverAck）。

| 连接数 | QPS | P50 | P95 | P99 | 成功率 |
|--------|-----|-----|-----|-----|--------|
| 10 | 29,532 | 0.23ms | 0.38ms | 0.47ms | 100% |
| 100 | 28,993 | 1.70ms | 2.71ms | 3.70ms | 99.96% |
| 1,000 | 27,432 | 18.73ms | 25.04ms | 577ms | 99.58% |

### 瓶颈分析

- **QPS 天花板 ≈ 29,000 msg/s**：10 连接与 1000 连接产生的 QPS 基本持平，确认瓶颈在 **Redis Lua 单线程** 而非连接调度
- **延迟随连接数恶化**：连接数增加后每连接排队等 Redis，P99 从 0.47ms 升至 577ms
- **少量失败来自 MQ 阻塞**：`PublishWithContext` 在 RabbitMQ 堆积时阻塞最多 5 秒

### 运行压测

压测脚本位于 `benchmark/` 目录，使用方法详见 [压测文档](docs/压力测试总结.md#6-压测脚本使用指南)：

```bash
# 第一轮：WS 并发连接数（k6）
go run benchmark/register_users.go -count=2000 -url=http://localhost:8080
cd benchmark && k6 run --env VUS=10000 --env HOLD=60 ws-conn-hold.js

# 第二轮：消息 QPS（Go 脚本）
go run benchmark/setup_friends.go -pairs=1000 -url=http://localhost:8080
go run benchmark/msg_bench.go -conns=100 -duration=15s
```

## 📁 项目结构

```
GoIM/
├── cmd/server/           # 入口点 (main.go)
├── configs/              # 配置 YAML 文件
│   ├── config.yaml       # 生产环境配置模板
│   └── config.test.yaml  # 端到端测试配置
├── docker-compose.yaml   # Docker 基础设施
├── benchmark/            # 压测脚本 (k6 + 自研 Go)
│   ├── register_users.go        # 批量注册 + 生成 JWT
│   ├── setup_friends.go         # 配对好友 + 预热 Redis
│   ├── ws-conn-test.js          # k6 建连吞吐
│   ├── ws-conn-hold.js          # k6 长连接保持
│   ├── msg_bench.go             # 消息 QPS + 端到端延迟
│   └── msg_debug.go             # 单连接诊断
├── docs/                 # 文档
│   ├── 产品技术设计.md     # 系统架构详情
│   ├── 产品功能需求.md     # 产品需求
│   ├── 项目系统架构.md     # 架构说明
│   ├── API 参考文档.md    # REST + WS API 参考
│   ├── 压力测试总结.md     # 压测报告
│   ├── 项目部署指南.md     # 部署指南
│   └── 项目开发计划.md     # 开发计划
├── scripts/migrations/   # MySQL 迁移 SQL 文件
│   ├── 001_create_users.sql
│   ├── 002_create_friendships.sql
│   ├── 003_create_groups.sql
│   ├── 004_create_messages.sql
│   ├── 005_create_moments.sql
│   ├── 006_create_misc.sql
│   └── 008_create_user_settings.sql
├── tests/                # 端到端集成测试
│   ├── e2e_helper.go     # 测试辅助函数
│   └── e2e_test.go       # 端到端测试套件
└── internal/
    ├── api/              # Gin HTTP 处理器 (7 个 handler)
    ├── config/           # 配置加载 (含 MomentConfig)
    ├── conn/             # 连接管理器 + 客户端连接
    ├── consumer/         # MQ 消费者 (5 个文件)
    │   ├── private_msg_consumer.go
    │   ├── group_msg_consumer.go
    │   ├── moment_feed_consumer.go    # 推拉结合 Feed 扇出
    │   ├── like_persist_consumer.go   # 点赞攒批削峰落库
    │   └── consumer_test.go
    ├── infra/            # MySQL/Redis/RabbitMQ 连接 + 清理
    ├── middleware/       # JWT 认证中间件
    ├── model/            # 数据模型 (7 个文件)
    ├── protocol/         # WebSocket 消息类型 + 编解码
    ├── redis/            # Lua 脚本 (5 个文件 + 加载器)
    │   ├── lua_private_msg.go
    │   ├── lua_group_msg.go
    │   ├── lua_inbox_mark_read.go
    │   ├── lua_revoke_msg.go
    │   ├── lua_moment_like.go        # 点赞/取消赞原子 Lua
    │   └── lua_scripts.go
    ├── repository/       # MySQL/Redis/MQ 仓库接口 + 实现
    │   ├── mysql_repo.go
    │   ├── redis_repo.go             # 含推拉 Feed + 点赞预热 + 大V筛选
    │   └── mq_repo.go
    ├── service/          # 业务逻辑 (8 个服务)
    └── ws/               # WebSocket 升级 + 消息分发器
```

## 🛠 技术栈

| 组件 | 技术 | 版本 |
|-----------|------------|---------|
| 语言 | Go | 1.24+ |
| HTTP 框架 | Gin | v1.10+ |
| WebSocket | gorilla/websocket | v1.5 |
| MySQL | go-sql-driver/mysql | v1.8+ |
| Redis | go-redis/v9 | v9.7+ |
| RabbitMQ | amqp091-go | v1.10+ |
| JWT | golang-jwt/jwt/v5 | v5.2+ |
| 配置 | yaml.v3 | v3 |
| 日志 | zap | v1.27+ |
| 密码 | bcrypt | — |
| 容器 | Docker Compose | v3.8 |

## 📊 API 概览

完整详情请参见 [docs/api_reference.md](docs/api_reference.md)。

| 分类 | 端点 | 认证 |
|----------|-----------|------|
| 健康检查 | `GET /health` | 无 |
| 认证 | register, login, refresh | 无 |
| 好友 | request, accept, reject, list, block, unblock | JWT |
| 群组 | create, update, info, members, add/remove member, leave | JWT |
| 朋友圈 | publish, get, like, comment, feed | JWT |
| 消息操作 | revoke, delete, search | JWT |
| 设置 | get, update, mute, unmute | JWT |
| WebSocket | `GET /ws?token=JWT` | JWT |

## 🔧 配置

完整配置模板及所有字段说明请参见 `configs/config.yaml`。

主要配置项：
- `server`：port, ws_path, upload_dir
- `mysql`：host, port, user, password, db_name
- `redis`：addr, password, db
- `rabbitmq`：url (amqp://)
- `jwt`：secret, access_exp_hours, refresh_exp_days
- `file`：max_size_mb, allowed_exts, upload_dir
- `moment`：big_user_friend_threshold (大V阈值, 默认500), timeline_max_len (收件箱/寄件箱上限, 默认1000), like_persist_batch_size (点赞落库攒批, 默认200), like_persist_flush_ms (攒批间隔ms, 默认500), like_cache_ttl_hours (点赞缓存TTL, 默认168h/7天)

## 📄 许可证

本项目仅供学习和演示目的。
