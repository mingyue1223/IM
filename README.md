<div align="center">

# GoIM

一个面向 Web 的即时通讯应用，支持私聊、群聊、好友关系、朋友圈动态、文件上传与实时 WebSocket 通信。

[本地开发](#本地开发) · [部署](#部署) · [接口文档](backend/docs/API%20参考文档.md) · [项目文档](docs/)

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=white)
![Vite](https://img.shields.io/badge/Vite-8-646CFF?style=flat-square&logo=vite&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)
![CI](https://img.shields.io/github/actions/workflow/status/Johnny-Cheung/IM/ghcr-publish.yml?branch=main&style=flat-square&label=CI)

</div>

## 项目定位

GoIM 提供一套完整的即时通讯 Web 应用：前端负责会话与社交交互，后端提供 REST API 与 WebSocket 实时连接，MySQL、Redis 与 RabbitMQ 分别承担数据持久化、缓存及异步消息处理。

## 核心功能

- **即时通信**：基于 WebSocket 的私聊、群聊与在线消息推送
- **社交关系**：好友申请、接受、删除与拉黑
- **群组管理**：建群、成员管理、角色与群主转让
- **朋友圈**：发布动态、评论、点赞与时间线
- **账户与设置**：注册、登录、JWT 刷新、个人资料与会话设置

## 技术栈

| 层级 | 技术 |
|---|---|
| 前端 | React 19 · TypeScript · Vite · Tailwind CSS · Zustand · TanStack Query |
| 后端 | Go 1.24+ · Gin · WebSocket · JWT |
| 数据与消息 | MySQL 8.4 · Redis 7.2 · RabbitMQ 3.13 |
| 工程化 | Docker Compose · GitHub Actions · GHCR · Vitest |

## 仓库结构

| 路径 | 说明 |
|---|---|
| `frontend/` | React/Vite 前端应用、API 客户端与前端测试 |
| `backend/cmd/server/` | Go 服务入口 |
| `backend/internal/` | API、业务服务、仓储、缓存、消息队列与 WebSocket 实现 |
| `backend/scripts/migrations/` | MySQL 初始化与迁移脚本 |
| `backend/benchmark/` | 压力测试工具；生成的令牌和数据文件不会提交 |
| `docker-compose.yaml` | 本地开发依赖：MySQL、Redis、RabbitMQ |
| `deploy/` | 生产 Docker Compose 与环境变量模板 |
| `docs/` | 前后端联调与优化计划 |
| `backend/docs/` | API、架构、部署与测试文档 |

## 本地开发

### 1. 准备配置

```powershell
Copy-Item .env.example .env
Copy-Item backend/configs/config.local.example.yaml backend/configs/config.local.yaml
Copy-Item frontend/.env.example frontend/.env.local
```

### 2. 启动基础设施

```powershell
docker compose up -d
```

这会启动 MySQL、Redis 和 RabbitMQ。RabbitMQ 管理页面默认地址为 `http://localhost:15673`。

### 3. 启动后端

```powershell
Set-Location backend
go run ./cmd/server -c configs/config.local.yaml
```

后端健康检查：`http://localhost:18080/health`。

### 4. 启动前端

在另一个终端执行：

```powershell
Set-Location frontend
npm ci
npm run dev
```

打开 `http://localhost:5173` 使用应用。

## 文档

- [接口参考](backend/docs/API%20参考文档.md)
- [项目系统架构](backend/docs/项目系统架构.md)
- [项目部署指南](backend/docs/项目部署指南.md)
- [前后端联调实施计划](docs/前后端联调实施计划.md)
