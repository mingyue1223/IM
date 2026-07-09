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

> **所有命令在 `backend/` 目录下执行。**

### 1. 启动基础设施

```bash
cd backend
docker-compose up -d
```

### 2. 运行数据库迁移

```bash
cd backend
for f in scripts/migrations/*.sql; do
  mysql -u goim -pgoim123 goim < "$f"
done
```

### 3. 构建并运行

```bash
cd backend
go build -o goim-server ./cmd/server
./goim-server -c configs/config.yaml
```

或直接运行：

```bash
cd backend
go run ./cmd/server -c configs/config.yaml
```

### 4. 验证

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"goim"}
```

### 5. 运行测试

```bash
cd backend
# 单元测试
go test ./internal/... -v

# 端到端测试（需先启动 Docker）
go test ./tests/... -v -tags e2e -timeout 120s
```

### 运行压测

压测脚本位于 `backend/benchmark/`，详见 [压测文档](backend/docs/压力测试总结.md)。

```bash
cd backend

# 注册压测用户
go run benchmark/register_users.go -count=2000 -url=http://localhost:8080

# WebSocket 长连接压测
cd benchmark && k6 run --env VUS=10000 --env HOLD=60 ws-conn-hold.js && cd ..

# 配对好友 + 消息 QPS 压测
go run benchmark/setup_friends.go -pairs=1000 -url=http://localhost:8080
go run benchmark/msg_bench.go -conns=100 -duration=15s
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

## 📁 项目结构

```
GoIM/
├── backend/                  # Go 后端服务
│   ├── cmd/server/           # 入口点 (main.go)
│   ├── configs/              # 配置 YAML 文件
│   │   ├── config.example.yaml  # 配置模板
│   │   └── config.test.yaml     # 测试配置
│   ├── internal/             # 内部包
│   │   ├── api/              # Gin HTTP 处理器
│   │   ├── config/           # 配置加载
│   │   ├── conn/             # 连接管理器 + 客户端连接
│   │   ├── consumer/         # MQ 消费者
│   │   ├── infra/            # MySQL/Redis/RabbitMQ 连接 + 清理
│   │   ├── llm/              # LLM 客户端 (OpenAI 兼容)
│   │   ├── middleware/       # JWT 认证 + CORS 中间件
│   │   ├── model/            # 数据模型
│   │   ├── protocol/         # WebSocket 消息类型 + 编解码
│   │   ├── redis/            # Lua 脚本封装
│   │   ├── repository/       # MySQL/Redis/MQ 仓库
│   │   ├── service/          # 业务逻辑 (8 个服务)
│   │   └── ws/               # WebSocket 升级 + 消息分发器
│   ├── scripts/              # SQL 迁移 + Lua 脚本
│   │   ├── migrations/       # MySQL DDL
│   │   └── lua/              # Redis Lua 脚本
│   ├── benchmark/            # 压测工具 (k6 + Go)
│   ├── tests/                # 端到端集成测试
│   ├── docs/                 # 项目文档 + Swagger
│   ├── go.mod / go.sum
│   └── docker-compose.yaml
│
└── frontend/                 # 前端 (浏览器客户端)
    └── goim-ws-types.ts      # WebSocket 协议 TS 类型定义
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
| API 文档 | Swagger / OpenAPI 3.0 | — |

## 📊 API 概览

完整详情请参见 [backend/docs/](backend/docs/)，或启动服务后访问 Swagger UI：`http://localhost:8080/swagger/index.html`

| 分类 | 端点 | 认证 |
|----------|-----------|------|
| 健康检查 | `GET /health` | 无 |
| 认证 | register, login, refresh | 无 |
| 好友 | request, accept, reject, list, block, unblock | JWT |
| 群组 | create, update, info, members, add/remove member, leave | JWT |
| 朋友圈 | publish, get, like, comment, feed | JWT |
| 消息操作 | revoke, delete, search | JWT |
| 设置 | get, update, mute, unmute | JWT |
| 文件 | upload avatar, get avatar | JWT/公开 |
| AI | chat, chat/stream, profile, summary | JWT |
| WebSocket | `GET /ws?token=JWT` | JWT |

## 🔧 配置

完整配置模板参见 `backend/configs/config.example.yaml`。

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
