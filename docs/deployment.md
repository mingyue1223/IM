# GoIM 部署指南

## Docker Compose（推荐用于开发环境）

### 启动基础设施

```bash
docker-compose up -d
```

这会启动：
- MySQL 8，端口 3306（用户名：goim，密码：goim123，数据库：goim）
- Redis 7，端口 6379
- RabbitMQ 3，端口 5672（管理界面端口 15672）

### 初始化数据库

```bash
# 按顺序执行所有迁移脚本
for f in scripts/migrations/*.sql; do
  docker exec -i goim-mysql mysql -u goim -pgoim123 goim < "$f"
done
```

或手动执行：
```bash
docker exec -it goim-mysql mysql -u goim -pgoim123 goim
# 然后粘贴每个迁移 SQL 文件的内容
```

### 启动 GoIM 服务器

```bash
go build -o goim-server ./cmd/server
./goim-server -c configs/config.yaml
```

或直接运行：
```bash
go run ./cmd/server -c configs/config.yaml
```

### 验证

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"goim"}
```

### RabbitMQ 管理界面

通过 http://localhost:15672 访问（guest/guest）。

查看队列：`private_msg_persist`、`group_msg_fanout`、`moment_push` 等。

---

## 生产环境部署

### 环境变量

通过环境变量覆盖配置值，或修改 `configs/config.yaml`：

| 配置字段 | 描述 | 生产环境注意事项 |
|----------|------|------------------|
| `jwt.secret` | JWT 签名密钥 | **必须更改！** 使用强随机字符串（≥32 字符） |
| `jwt.access_exp_hours` | 访问令牌过期时间 | 2 小时（默认） |
| `jwt.refresh_exp_days` | 刷新令牌过期时间 | 7 天（默认） |
| `mysql.password` | MySQL 密码 | 使用强密码 |
| `redis.password` | Redis 密码 | 在生产环境中启用 |
| `llm.api_key` | OpenAI API 密钥 | 设置为您的实际密钥 |
| `server.port` | HTTP 服务器端口 | 8080（默认） |

### 扩展考量

#### 水平扩展

GoIM 支持多个服务器实例共享 Redis + MySQL + RabbitMQ：

```
                    ┌──── Redis 集群 ────┐
                    │   （共享状态）      │
                    └─────────────────────┘
                          ▲     ▲     ▲
                    ┌─────┘     │     └─────┐
                    │           │           │
              ┌─────┴───┐ ┌────┴────┐ ┌────┴───┐
              │ GoIM #1 │ │ GoIM #2 │ │ GoIM #3│
              │ （端口  │ │ （端口  │ │ （端口  │
              │  8080） │ │  8081） │ │  8082） │
              └─────────┘ └─────────┘ └─────────┘
                    │           │           │
                    └───────────┼───────────┘
                                ▼
                    ┌──── RabbitMQ ─────────┐
                    │   （共享 MQ）          │
                    └───────────────────────┘
                                ▼
                    ┌──── MySQL ─────────────┐
                    │   （共享数据库）        │
                    └───────────────────────┘
```

**重要提示**：对于 WebSocket 的水平扩展，请使用带有粘性会话的反向代理（例如，nginx 配置 `ip_hash`）。每个 WS 连接必须到达同一个 GoIM 实例。

#### Lua 脚本限制

当前的 Lua 脚本使用动态键构造（`inbox:{receiverID}`），这会阻止 Redis 集群的迁移。对于 Redis 集群，需要修改脚本以预先传递所有键（Redis 集群要求 Lua 脚本中的所有键哈希到同一个槽位）。

#### 负载均衡器配置（nginx）

```nginx
upstream goim {
    ip_hash;  # WebSocket 粘性会话
    server goim1:8080;
    server goim2:8081;
    server goim3:8082;
}

server {
    listen 80;
    
    # REST API
    location /api/ {
        proxy_pass http://goim;
    }
    
    # WebSocket
    location /ws {
        proxy_pass http://goim;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
    
    # 健康检查
    location /health {
        proxy_pass http://goim;
    }
}
```

---

## 监控

### 健康检查接口

```bash
curl http://localhost:8080/health
```

### Redis 监控

```bash
redis-cli info memory
redis-cli info stats
# 检查收件箱/发件箱大小
redis-cli keys "inbox:*" | wc -l
```

### RabbitMQ 监控

- 管理界面：http://localhost:15672
- 队列深度：检查 `private_msg_persist`、`group_msg_fanout`、`moment_push` 队列
- 如果队列深度不断增长，消费者可能存在滞后

### MySQL 监控

```bash
mysql -u goim -pgoim123 goim -e "SHOW TABLE STATUS"
mysql -u goim -pgoim123 goim -e "SELECT COUNT(*) FROM private_messages"
```

---

## 故障排查

### 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| MySQL 连接被拒绝 | MySQL 未运行 | `docker-compose up -d mysql` |
| Redis 连接被拒绝 | Redis 未运行 | `docker-compose up -d redis` |
| RabbitMQ 连接被拒绝 | RabbitMQ 未运行 | `docker-compose up -d rabbitmq` |
| JWT 认证失败 | 密钥错误 | 检查 `jwt.secret` 是否与配置匹配 |
| WebSocket 断开连接 | 令牌过期 | 连接前先刷新令牌 |
| 消息未送达 | 用户不是好友 | 发送私聊消息前先建立好友关系 |
| 群聊消息被拒绝 | 不是群成员 | 发送消息前先加入群组 |
| 队列深度不断增长 | 消费者滞后 | 检查 MQ 消费者日志，增加消费者数量 |

### 日志级别

GoIM 使用 zap 日志库。测试时设置 `gin.SetMode(gin.TestMode)`，生产环境使用 `gin.ReleaseMode()`。

```bash
# 查看服务器日志
./goim-server -c configs/config.yaml  # 日志输出到 stdout
```

---

## 安全清单

- [ ] 更改默认的 `jwt.secret`
- [ ] 在生产环境中启用 Redis 密码
- [ ] 使用强 MySQL 密码
- [ ] 限制 MySQL/Redis/RabbitMQ 仅允许 GoIM 访问
- [ ] 通过反向代理启用 HTTPS
- [ ] 对认证接口（注册/登录）进行频率限制
- [ ] 生产环境设置 `gin.ReleaseMode()`
- [ ] 不要在配置文件中存放 LLM API 密钥（使用环境变量）
