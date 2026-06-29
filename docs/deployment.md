# GoIM Deployment Guide

## Docker Compose (Recommended for Development)

### Start Infrastructure

```bash
docker-compose up -d
```

This starts:
- MySQL 8 on port 3306 (user: goim, password: goim123, database: goim)
- Redis 7 on port 6379
- RabbitMQ 3 on port 5672 (management UI on port 15672)

### Initialize Database

```bash
# Apply all migrations in order
for f in scripts/migrations/*.sql; do
  docker exec -i goim-mysql mysql -u goim -pgoim123 goim < "$f"
done
```

Or manually:
```bash
docker exec -it goim-mysql mysql -u goim -pgoim123 goim
# Then paste each migration SQL file content
```

### Start GoIM Server

```bash
go build -o goim-server ./cmd/server
./goim-server -c configs/config.yaml
```

Or run directly:
```bash
go run ./cmd/server -c configs/config.yaml
```

### Verify

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"goim"}
```

### RabbitMQ Management UI

Access at http://localhost:15672 (guest/guest).

View queues: `private_msg_persist`, `group_msg_fanout`, `moment_push`, etc.

---

## Production Deployment

### Environment Variables

Override config values via environment variables or modify `configs/config.yaml`:

| Config Field | Description | Production Notes |
|-------------|-------------|------------------|
| `jwt.secret` | JWT signing secret | **Must change!** Use a strong random string (≥32 chars) |
| `jwt.access_exp_hours` | Access token expiry | 2 hours (default) |
| `jwt.refresh_exp_days` | Refresh token expiry | 7 days (default) |
| `mysql.password` | MySQL password | Use a strong password |
| `redis.password` | Redis password | Enable in production |
| `llm.api_key` | OpenAI API key | Set to your actual key |
| `server.port` | HTTP server port | 8080 (default) |

### Scaling Considerations

#### Horizontal Scaling

GoIM supports multiple server instances with shared Redis + MySQL + RabbitMQ:

```
                    ┌──── Redis Cluster ────┐
                    │   (shared state)      │
                    └───────────────────────┘
                          ▲     ▲     ▲
                    ┌─────┘     │     └─────┐
                    │           │           │
              ┌─────┴───┐ ┌────┴────┐ ┌────┴───┐
              │ GoIM #1 │ │ GoIM #2 │ │ GoIM #3│
              │ (port   │ │ (port   │ │ (port  │
              │  8080)  │ │  8081)  │ │  8082) │
              └─────────┘ └─────────┘ └────────┘
                    │           │           │
                    └───────────┼───────────┘
                                ▼
                    ┌──── RabbitMQ ─────────┐
                    │   (shared MQ)         │
                    └───────────────────────┘
                                ▼
                    ┌──── MySQL ─────────────┐
                    │   (shared DB)          │
                    └───────────────────────┘
```

**Important**: For horizontal scaling with WebSocket, use a reverse proxy with sticky sessions (e.g., nginx with `ip_hash`). Each WS connection must reach the same GoIM instance.

#### Lua Script Limitations

Current Lua scripts use dynamic key construction (`inbox:{receiverID}`), which blocks Redis Cluster migration. For Redis Cluster, modify scripts to pass all keys upfront (Redis Cluster requires all keys in a Lua script to hash to the same slot).

#### Load Balancer Config (nginx)

```nginx
upstream goim {
    ip_hash;  # Sticky sessions for WebSocket
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
    
    # Health check
    location /health {
        proxy_pass http://goim;
    }
}
```

---

## Monitoring

### Health Check Endpoint

```bash
curl http://localhost:8080/health
```

### Redis Monitoring

```bash
redis-cli info memory
redis-cli info stats
# Check inbox/outbox sizes
redis-cli keys "inbox:*" | wc -l
```

### RabbitMQ Monitoring

- Management UI: http://localhost:15672
- Queue depths: check `private_msg_persist`, `group_msg_fanout`, `moment_push` queues
- If queue depth grows, consumers may be lagging

### MySQL Monitoring

```bash
mysql -u goim -pgoim123 goim -e "SHOW TABLE STATUS"
mysql -u goim -pgoim123 goim -e "SELECT COUNT(*) FROM private_messages"
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| MySQL connection refused | MySQL not running | `docker-compose up -d mysql` |
| Redis connection refused | Redis not running | `docker-compose up -d redis` |
| RabbitMQ connection refused | RabbitMQ not running | `docker-compose up -d rabbitmq` |
| JWT auth fails | Wrong secret | Check `jwt.secret` matches config |
| WebSocket disconnects | Token expired | Refresh token before connecting |
| Message not delivered | Users not friends | Create friendship before private messaging |
| Group msg rejected | Not a member | Join group before sending messages |
| Queue depth growing | Consumer lag | Check MQ consumer logs, increase consumer count |

### Log Levels

GoIM uses zap logger. Set `gin.SetMode(gin.TestMode)` for tests, `gin.ReleaseMode()` for production.

```bash
# View server logs
./goim-server -c configs/config.yaml  # logs to stdout
```

---

## Security Checklist

- [ ] Change `jwt.secret` from default
- [ ] Enable Redis password in production
- [ ] Use strong MySQL password
- [ ] Restrict MySQL/Redis/RabbitMQ access to GoIM only
- [ ] Enable HTTPS via reverse proxy
- [ ] Rate-limit auth endpoints (register/login)
- [ ] Set `gin.ReleaseMode()` for production
- [ ] Keep LLM API key out of config file (use env var)
