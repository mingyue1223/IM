# GoIM — 高并发即时通讯系统

一个基于 Go 语言构建的生产级 IM（即时通讯）系统，采用微信风格的隐私设计、Redis 优先架构配合异步 MySQL 持久化，并集成 AI 驱动的对话智能。

## ✨ 功能特性

| 分类 | 功能 |
|----------|----------|
| **消息** | 私聊（推送模式）、群聊（拉取模式）、消息撤回、送达/已读回执、离线同步 |
| **社交** | 好友请求/接受/拒绝/拉黑、双向好友关系、朋友圈发布/点赞/评论/动态流 |
| **群组** | 创建/更新/退出群组、成员管理（添加/移除/踢出）、角色体系（群主/管理员/成员） |
| **AI** | 4层记忆架构、集成大语言模型的 AI 聊天、对话摘要、用户画像提取 |
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
```

### 核心设计决策

1. **Redis 优先模式**：所有读写优先经过 Redis。MySQL 持久化通过 MQ 消费者异步完成。
2. **私聊推送模式**：消息推送到每个用户的 `inbox:{userID}` ZSet。接收方连接实时收到推送。
3. **群聊拉取模式**：消息存储在群组维度的 `outbox:{groupID}` ZSet。成员通过同步请求拉取消息。
4. **微信隐私设计**：发送者**无法**查看接收者的已读状态。`readStatus` 仅对接收者本人可见。
5. **Lua 原子操作**：好友校验、去重、msgID 分配、收件箱写入、标记已读、撤回等操作均通过 Redis Lua 脚本原子执行。
6. **3天 TTL**：收件箱/发件箱/时间线通过 `ZREMRANGEBYSCORE`（按时间）+ `ZREMRANGEBYRANK`（按数量上限）自动过期。
7. **单设备登录策略**：新的 WebSocket 连接会踢出旧连接，发送 `{"type":"kick","reason":"new_login"}`。

### 会话 ID 格式

- 私聊：`p_{较小ID}_{较大ID}`（例如 `p_1_2`）
- 群聊：`g_{groupID}`（例如 `g_42`）

### AI 4层记忆架构

| 层级 | 存储位置 | 内容 | 用途 |
|-------|---------|---------|---------|
| 0 | MySQL `private_messages` | 原始消息 | 完整对话历史 |
| 1 | MySQL `ai_summaries` | 话题、关键点、结论 | 结构化摘要 |
| 2 | MySQL `ai_user_profiles` | 字段、值、置信度、来源 | 带置信度评分的用户画像 |
| 3 | Redis `ai_memory:{userID}:{key}` | 带 TTL 的工作记忆 | AI 响应的快速上下文 |

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

## 📁 项目结构

```
GoIM/
├── cmd/server/           # 入口点 (main.go)
├── configs/              # 配置 YAML 文件
│   ├── config.yaml       # 生产环境配置模板
│   └── config.test.yaml  # 端到端测试配置
├── docker-compose.yaml   # Docker 基础设施
├── docs/                 # 文档
│   ├── architecture.md   # 系统架构详情
│   ├── api_reference.md  # REST + WS API 参考
│   └── deployment.md     # 部署指南
├── scripts/migrations/   # MySQL 迁移 SQL 文件
│   ├── 001_create_users.sql
│   ├── 002_create_friendships.sql
│   ├── 003_create_groups.sql
│   ├── 004_create_messages.sql
│   ├── 005_create_moments.sql
│   ├── 006_create_misc.sql
│   ├── 007_create_ai.sql
│   └── 008_create_user_settings.sql
├── tests/                # 端到端集成测试
│   ├── e2e_helper.go     # 测试辅助函数
│   └── e2e_test.go       # 端到端测试套件
└── internal/
    ├── api/              # Gin HTTP 处理器 (7 个文件)
    ├── config/           # 配置加载
    ├── conn/             # 连接管理器 + 客户端连接
    ├── consumer/         # MQ 消费者 (3 个文件)
    ├── infra/            # MySQL/Redis/RabbitMQ 连接 + 清理
    ├── llm/              # 大语言模型客户端 (兼容 OpenAI)
    ├── middleware/        # JWT 认证中间件
    ├── model/            # 数据模型 (7 个文件)
    ├── protocol/         # WebSocket 消息类型 + 编解码
    ├── redis/            # Lua 脚本 (4 个文件 + 加载器)
    ├── repository/       # MySQL/Redis/MQ 仓库接口 + 实现
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
| AI | chat, profile, summary | JWT |
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
- `llm`：provider, api_key, base_url, model, max_tokens
- `file`：max_size_mb, allowed_exts, upload_dir

## 📄 许可证

本项目仅供学习和演示目的。
