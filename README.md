# A2A Platform

基于 [Google A2A 协议](https://google.github.io/A2A/) 的多智能体协作平台。Host 作为控制平面，提供 Agent 发现、消息路由、反向代理和全链路追踪能力。

## 架构概览

```
┌─────────────────────────────────────────────────────┐
│  Host 控制平面                                       │
│  ┌──────────┐ ┌────────────┐ ┌────────┐ ┌────────┐ │
│  │ Registry │ │ MessageBus │ │ Tracer │ │ Proxy  │ │
│  └──────────┘ └────────────┘ └────────┘ └────────┘ │
│                     │                               │
│              ┌──────┴──────┐                        │
│              │ SQLite DB   │                        │
│              └─────────────┘                        │
└──────────────┬─────────────────┬────────────────────┘
               │  A2A Protocol   │
          ┌────┴────┐      ┌────┴────┐
          │ Agent A │      │ Agent B │
          └─────────┘      └─────────┘
```

**核心组件：**

- **Registry** — Agent 发现与连接管理，通过 `/.well-known/agent.json` 拉取 AgentCard，支持指数退避重试
- **MessageBus** — A2A 消息编排，创建 Task、发送 JSON-RPC 请求、解析 SSE 流、持久化消息
- **Tracer** — 全链路追踪，记录 send / task_created / task_update / artifact / response / error 事件
- **Proxy** — 反向代理转发，自动注入 `A2A-Version: 1.0` Header，剥离 hop-by-hop Header

## 前置要求

- **Go 1.25.0+**（go.mod 指定 `go 1.25.0`，安装方式参考 [go.dev/dl](https://go.dev/dl/)）

不需要外部数据库——默认使用内嵌 SQLite（纯 Go 实现 `modernc.org/sqlite`）。

## 快速开始

### 1. 克隆并安装依赖

```bash
git clone <repo-url> a2a-platform-goal
cd a2a-platform-goal
go mod download
```

### 2. 启动 Host

```bash
make host
# 或者直接运行：
# go run cmd/host/main.go -f etc/host.yaml
```

Host 默认监听 `0.0.0.0:7860`，日志输出到 stdout。

### 3. 启动 Fake Agent（用于测试/开发）

```bash
make fake-agent
# 或者指定端口和名称：
# go run e2e/fake_agent/main.go -port 10001 -name my-agent
```

Fake Agent 默认监听 `:10001`，支持通过 URL query 参数 `?mode=echo` 切换行为模式。

### 4. 注册 Agent 到 Host

```bash
curl -X POST http://localhost:7860/api/agents/register \
  -H "Content-Type: application/json" \
  -d '{"url": "http://localhost:10001"}'
```

### 5. 发送消息

```bash
curl -X POST http://localhost:7860/api/chat \
  -H "Content-Type: application/json" \
  -d '{"agent_name": "fake-agent", "message": "Hello!"}'
```

## 配置

配置文件位于 `etc/host.yaml`：

```yaml
Name: a2a-host           # 服务名称
Host: "0.0.0.0"          # 监听地址
Port: 7860               # 监听端口
DataSource: "file:a2a_platform.db?cache=shared&_journal_mode=WAL"  # SQLite 路径
Timeout: 30000            # 请求超时（毫秒）
AgentRetryMax: 3          # Agent 连接最大重试次数
AgentRetryBaseDelay: 1    # 重试基础延迟（秒），按 1s → 2s → 4s 指数退避
LogLevel: info            # 日志级别
```

可通过 `-f` 参数指定配置文件路径：

```bash
go run cmd/host/main.go -f /path/to/custom.yaml
```

## API 参考

### Agent 管理

| 方法   | 路径                      | 说明                         |
|--------|---------------------------|------------------------------|
| GET    | `/api/agents`             | 列出所有已注册 Agent          |
| GET    | `/api/agents/:name`       | 获取单个 Agent 详情           |
| POST   | `/api/agents/register`    | 注册新 Agent（提供 `url`）    |
| POST   | `/api/agents`             | 同上（别名）                  |
| DELETE | `/api/agents/:name`       | 删除 Agent                   |

### 通信

| 方法   | 路径                      | 说明                                |
|--------|---------------------------|-------------------------------------|
| POST   | `/api/chat`               | 发送消息（SSE 流式响应）             |
| POST   | `/agent/:name`            | 反向代理到指定 Agent（支持 SSE 透传）|

### 系统信息

| 方法   | 路径                      | 说明                         |
|--------|---------------------------|------------------------------|
| GET    | `/api/capabilities`       | 获取平台能力和工具定义        |
| GET    | `/api/tasks`              | 列出任务（支持 `agent_name`、`state` 过滤）|
| GET    | `/api/tasks/:id/trace`    | 获取任务追踪时间线（纯文本）  |

### 请求/响应示例

**注册 Agent：**

```bash
curl -X POST http://localhost:7860/api/agents/register \
  -H "Content-Type: application/json" \
  -d '{"url": "http://localhost:10001", "type": "helloworld", "name": "my-agent"}'
```

响应：
```json
{"status":"connected","name":"my-agent","url":"http://localhost:10001","type":"helloworld","version":"1.0.0"}
```

**发送消息（SSE 流式）：**

```bash
curl -N -X POST http://localhost:7860/api/chat \
  -H "Content-Type: application/json" \
  -d '{"agent_name": "my-agent", "message": "Hello", "context_id": "ctx-1"}'
```

响应为 SSE 流，每行格式 `data: {...}\n\n`：
```
data: {"local_task_id":"abc-123","agent_name":"my-agent","state":"SUBMITTED"}
data: {"type":"response","data":{"text":"Hello back!"}}
```

## 运行测试

### 全量测试（单元 + E2E）

```bash
make test
# 等价于: go test ./... -v
```

### 仅 E2E 测试

```bash
make e2e
# 等价于: go test ./e2e/... -v
```

E2E 测试启动内存 SQLite 数据库和 httptest 服务器，自动创建 fake agent 实例，无需任何外部依赖。

全量测试（`make test`）共 **153 个**顶层测试用例，其中 E2E 测试覆盖 6 个类别：

| 类别           | 说明                                    |
|----------------|-----------------------------------------|
| Discovery      | Agent 列表、详情、404、能力查询、任务列表/过滤/追踪 |
| Messaging      | Echo/Sequence/Artifact SSE、非 SSE JSON、Header 注入/剥离、路径转发、上下文续传 |
| Tool Call      | list_agents / send_to_agent / get_agent_info 工具调用 |
| Tracing        | 发送/响应/错误追踪、事件排序、跨 Agent 追踪  |
| Registry       | 注册/断开/重连、多 Agent、去重、重试         |
| Error          | 404 代理、Agent 错误、死连接、流中断、并发、自消息 |

### Docker E2E 测试

```bash
make docker-e2e
```

通过 Docker Compose 启动 Host + 多个 Agent + 测试容器，运行集成测试。

### 清理

```bash
make clean
# 删除数据库文件和测试缓存
```

## 项目结构

```
a2a-platform-goal/
├── cmd/host/main.go              # Host 入口
├── etc/host.yaml                 # 默认配置
├── internal/
│   ├── config/config.go          # 配置定义
│   ├── handler/                  # HTTP 处理器
│   │   ├── agents.go             # Agent CRUD
│   │   ├── capabilities.go       # 能力查询
│   │   ├── chat.go               # SSE 聊天
│   │   ├── proxy.go              # 反向代理
│   │   ├── tasks.go              # 任务查询
│   │   └── helper.go             # 路由注册 + 工具函数
│   ├── messagebus/               # 消息总线
│   │   ├── messagebus.go         # 核心通信编排
│   │   └── tools.go              # 平台工具定义
│   ├── model/                    # 数据模型
│   ├── registry/registry.go      # Agent 注册中心
│   ├── repository/               # 数据库操作（SQLite）
│   ├── svc/servicecontext.go     # 依赖注入
│   └── tracer/tracer.go          # 追踪系统
├── pkg/a2a/                      # A2A 协议客户端库
│   ├── client.go                 # HTTP 客户端
│   ├── types.go                  # AgentCard、JSON-RPC 类型
│   └── sse.go                    # SSE 解析/写入
├── e2e/                          # E2E 测试
│   ├── test_helper.go            # 测试基础设施
│   ├── discovery_test.go         # Agent 发现测试
│   ├── messaging_test.go         # 消息流测试
│   ├── tool_call_test.go         # 工具调用测试
│   ├── tracing_test.go           # 追踪测试
│   ├── registry_test.go          # 注册中心测试
│   ├── error_test.go             # 错误处理测试
│   ├── fake_agent/               # 独立 Fake Agent 可执行文件
│   └── llm_agent/                # LLM Agent 示例
├── Makefile                      # 常用命令
└── go.mod
```

## LLM Agent

`e2e/llm_agent/` 包含一个连接真实 LLM 的 Agent 示例，支持 function calling：

```bash
export LLM_API_KEY=your-api-key
export LLM_BASE_URL=https://api.openai.com/v1   # 可选，默认内置
export LLM_MODEL=gpt-4                           # 可选
export HOST_URL=http://localhost:7860             # 启用工具调用

make llm-agent
```

## Fake Agent 模式

Fake Agent 通过 URL query 参数 `?mode=` 切换行为，用于测试不同场景：

| 模式                 | 说明                                    |
|----------------------|-----------------------------------------|
| `echo`               | 回显用户消息                             |
| `sequence`           | 返回 WORKING → message → COMPLETED 序列 |
| `artifact`           | 返回 artifact_update 事件               |
| `full_events`        | 返回完整事件序列 (task → status → artifact → message) |
| `json_response`      | 非 SSE 时返回 JSON                      |
| `record_headers`     | 返回收到的 HTTP headers                 |
| `record_url`         | 返回收到的 URL path 和 query            |
| `error`              | 返回 HTTP 500                           |
| `disconnect_mid_stream` | 发送 1 个事件后断开                   |
| `empty_response`     | 只返回 status_update，无 message        |
| `tool_call:<name>`   | 模拟工具调用                            |
