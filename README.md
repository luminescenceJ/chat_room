# 聊天室项目 (ChatRoom)

一个基于 Go 语言开发的实时聊天室应用，支持私聊和群聊功能。

## 功能特性

- 🚀 实时消息传输（WebSocket）
- 👥 用户注册和登录
- 💬 私聊和群聊
- 🏠 群组管理（创建、加入、离开）
- 📱 在线用户状态
- 🔄 消息持久化存储
- ⚡ Redis 缓存优化
- 📊 Kafka 消息队列
- 🔐 JWT 身份验证
- 🛡️ 请求限流

## 技术栈

- **后端框架**: Gin (Go)
- **数据库**: MySQL + GORM
- **缓存**: Redis
- **消息队列**: Kafka
- **实时通信**: WebSocket
- **身份验证**: JWT

## 项目结构

```
chatroom/
├── api/                 # API 控制器
│   ├── auth.go         # 认证相关
│   ├── user.go         # 用户管理
│   ├── message.go      # 消息处理
│   ├── group.go        # 群组管理
│   ├── websocket.go    # WebSocket 处理
│   ├── monitor.go      # 监控接口
│   └── routes.go       # 路由配置
├── config/             # 配置管理
│   └── config.go
├── middleware/         # 中间件
│   └── rate_limiter.go # 限流中间件
├── models/             # 数据模型
│   ├── user.go
│   ├── message.go
│   └── group.go
├── services/           # 业务逻辑层
│   ├── user_service.go
│   ├── message_service.go
│   ├── group_service.go
│   ├── kafka_service.go
│   ├── websocket_manager.go
│   ├── websocket.go
│   ├── client.go
│   └── server.go
├── .env.example        # 环境变量示例
├── go.mod
├── go.sum
├── main.go
└── README.md
```

## 快速开始

### 方式一：最小化运行（推荐新手）

如果你只想快速体验项目，可以使用最小化配置：

1. **安装 Go 1.19+**

2. **克隆项目并安装依赖**
```bash
git clone <项目地址>
cd chatroom
go mod tidy
```

3. **直接运行（使用默认配置）**
```bash
go run main.go
```

项目会自动：
- 使用 SQLite 内存数据库（无需安装 MySQL）
- 跳过 Redis 和 Kafka（功能会降级但不影响基本使用）
- 在 http://localhost:8080 启动服务

### 方式二：完整功能运行

#### 1. 环境要求
- Go 1.19+
- MySQL 8.0+
- Redis 6.0+
- Kafka 2.8+（可选）

#### 2. 安装依赖
```bash
go mod download
```

#### 3. 配置环境变量
复制并编辑配置文件：
```bash
cp .env.example .env
# 编辑 .env 文件，配置数据库连接等信息
```

#### 4. 数据库初始化
```sql
CREATE DATABASE chatroom CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

#### 5. 启动服务
```bash
go run main.go
```

### 方式三：Docker 运行

使用 Docker Compose 一键启动所有服务：
```bash
docker-compose up -d
```

服务将在 `http://localhost:8080` 启动。

## API 接口

### 认证接口

- `POST /api/register` - 用户注册
- `POST /api/login` - 用户登录

### 用户接口

- `GET /api/users` - 获取所有用户
- `GET /api/users/:id` - 获取用户信息
- `PUT /api/users/:id` - 更新用户信息
- `GET /api/users/online` - 获取在线用户

### 消息接口

- `GET /api/messages` - 获取消息列表
- `POST /api/messages` - 发送消息
- `GET /api/messages/:id` - 获取单个消息

### 群组接口

- `GET /api/groups` - 获取群组列表
- `POST /api/groups` - 创建群组
- `GET /api/groups/:id` - 获取群组信息
- `PUT /api/groups/:id` - 更新群组信息
- `DELETE /api/groups/:id` - 删除群组
- `POST /api/groups/:id/members` - 添加群组成员

### WebSocket

- `GET /api/ws` - WebSocket 连接

## WebSocket 消息格式

### 发送消息

```json
{
  "type": "chat_message",
  "content": {
    "content": "Hello, World!",
    "type": "text",
    "receiver_id": 123,
    "group_id": 0
  },
  "timestamp": "2023-01-01T00:00:00Z"
}
```

### 接收消息

```json
{
  "id": 1,
  "content": "Hello, World!",
  "type": "text",
  "sender_id": 456,
  "sender": {
    "id": 456,
    "username": "alice",
    "avatar": "https://example.com/avatar.jpg",
    "online": true
  },
  "receiver_id": 123,
  "group_id": 0,
  "created_at": "2023-01-01T00:00:00Z"
}
```

## 部署

### Docker 部署

```bash
# 构建镜像
docker build -t chatroom .

# 运行容器
docker run -d \
  --name chatroom \
  -p 8080:8080 \
  --env-file .env \
  chatroom
```

### 生产环境配置

1. 设置 `MODE=release`
2. 配置强密码的 `JWT_SECRET`
3. 使用生产级别的数据库和缓存配置
4. 配置负载均衡和反向代理
5. 启用 HTTPS

## 监控

应用提供了监控接口：

- `GET /api/monitor/system` - 系统状态
- `GET /api/monitor/connections` - 连接统计

## 开发

### 添加新功能

1. 在 `models/` 中定义数据模型
2. 在 `services/` 中实现业务逻辑
3. 在 `api/` 中添加控制器
4. 在 `api/routes.go` 中注册路由

## 中间件

### kafka
```shell
.\kafka-server-start.bat D:\lumin\kafka_2.13-4.0.0\config\server.properties
```



### 测试

```bash
go test ./...
```

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License