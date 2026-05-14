# A2A Platform - Go Implementation Design

## Overview

A2A (Agent-to-Agent) protocol platform implemented in Go 1.21.13 using go-zero framework. Provides a Host control plane with Registry, MessageBus, Tracer, and Proxy for multi-agent discovery, communication, and observability. SQLite for persistence, Docker Compose for E2E testing.

## Architecture

```
User/UI ──► API Gateway ──► Registry (agent discovery)
              │                 MessageBus (A2A send/receive)
              │                 Tracer (observability)
              │                 Proxy (transparent A2A forwarding)
              ▼
         SQLite DB (agents, tasks, messages, traces)
              │
              ▼
         Agent A / Agent B / Agent N (A2A JSON-RPC + SSE)
```

## Project Structure

```
a2a_platform/
├── cmd/host/main.go
├── etc/host.yaml
├── host.api
├── internal/
│   ├── config/config.go
│   ├── svc/servicecontext.go
│   ├── handler/                  # goctl-generated + custom SSE handlers
│   ├── logic/                    # Business logic
│   ├── model/                    # Data models
│   ├── repository/               # SQLite repository pattern
│   ├── registry/                 # Agent connection lifecycle
│   ├── messagebus/               # A2A message orchestration
│   ├── tracer/                   # Trace event system
│   └── middleware/               # A2A-Version injection, hop-by-hop stripping
├── pkg/
│   ├── a2a/                      # A2A protocol types + client
│   │   ├── types.go              # AgentCard, Task, Message, Part, Event
│   │   ├── jsonrpc.go            # JSON-RPC 2.0
│   │   ├── client.go             # A2A HTTP client
│   │   └── sse.go                # SSE parser/writer
│   └── httputil/
├── e2e/
│   ├── docker-compose.yml
│   ├── fake_agent/               # Go fake A2A agent binary
│   ├── test_helper.go
│   ├── messaging_test.go         # 11 cases
│   ├── discovery_test.go         # 12 cases
│   ├── tool_call_test.go         # 6 cases
│   ├── tracing_test.go           # 8 cases
│   ├── registry_test.go          # 6 cases
│   └── error_test.go             # 8 cases
├── docs/usage.html
├── Makefile
├── go.mod
└── go.sum
```

## Tech Stack

- **Language**: Go 1.21.13
- **Framework**: go-zero (rest.Server, ServiceContext, goctl codegen)
- **Database**: SQLite via modernc.org/sqlite (pure Go, no CGO)
- **Testing**: Go testing package + Docker Compose for E2E
- **LLM for test agents**: OpenAI-compatible API (mimo-v2.5-pro)

## API Routes (host.api)

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/agents | List all agents |
| GET | /api/agents/:name | Get agent by name |
| POST | /api/agents/register | Agent self-registration |
| POST | /api/agents | Dynamic agent add |
| DELETE | /api/agents/:name | Remove agent |
| GET | /api/capabilities | Platform capabilities |
| GET | /api/tasks | List tasks (filterable) |
| GET | /api/tasks/:id/trace | Task trace |
| POST | /api/chat | Chat with agent (SSE) |
| POST | /agent/:name | Proxy to agent (SSE) |

## Data Models

### AgentRecord
- id (auto), name (unique), url, description, version, type, status (connected/disconnected/error), skills_json, error_message, created_at, updated_at

### TaskRecord
- id (auto), local_task_id (UUID), agent_name, context_id, state (SUBMITTED/WORKING/INPUT_REQUIRED/COMPLETED/FAILED/ERROR/RESPONDED), display_id, created_at, updated_at

### MessageRecord
- id (auto), task_id, role (user/agent), content, created_at

### TraceEventRecord
- id (auto), task_id, context_id, agent_name, target_agent, event_type (send/task_created/task_update/artifact/response/error), data_json, created_at

## Core Components

### Registry
- connect_agent_by_url(url) → fetch /.well-known/agent.json → parse AgentCard → persist → store in-memory connection
- disconnect_agent(name) → remove from memory → mark DB as disconnected
- list_agents() → cross-check DB + memory connections
- get_client(name) → return in-memory connection or nil
- Retry: exponential backoff 1s→2s→4s, 3 attempts

### MessageBus
- send_streaming(sender, agent_name, content, context_id) → create/reuse Task → call A2A client → parse SSE events → record traces → yield events
- send(sender, agent_name, content, context_id) → send_streaming → collect → return text
- create_task(agent_name) → generate UUID → persist
- handle_tool_call(tool_name, params, host_url) → list_agents / send_to_agent / get_agent_info
- Context continuation: reuse Task when context_id matches

### Tracer
- record_send(task_id, content, context_id)
- record_task_created(task_id, server_task_id, context_id)
- record_task_update(task_id, old_state, new_state)
- record_artifact(task_id, text)
- record_response(task_id, text, source)
- record_error(task_id, err)
- get_timeline(task_id) → ordered events
- get_debug_trace(task_id) → formatted string
- get_recent_by_agent(agent_name, limit) → recent events

### Proxy
- Transparent forwarding of POST /agent/:name to target agent
- Inject A2A-Version: 1.0 header if missing
- Strip hop-by-hop headers (host, content-length, connection, accept-encoding, transfer-encoding)
- Forward path suffix and query params
- Support both SSE and non-SSE responses

## A2A Protocol

- JSON-RPC 2.0 over HTTP
- Methods: SendStreamingMessage, SendMessage
- SSE events: task, status_update, artifact_update, message
- A2A-Version: 1.0 header required (proxy auto-injects)
- AgentCard via GET /.well-known/agent.json

## Fake Agent Modes

| Mode | Behavior |
|------|----------|
| echo | Returns input text in SSE |
| sequence | status_update(WORKING) → message("processed") → status_update(COMPLETED) |
| artifact | status_update(WORKING) → artifact_update(text="generated artifact") → status_update(COMPLETED) |
| json_response | Returns plain JSON (non-SSE) |
| record_headers | Records received headers, returns them |
| record_url | Records received URL path+query |
| full_events | task → status_update → artifact_update → message |
| error | Returns HTTP 500 |
| tool_call:list_agents | Calls Host GET /api/agents, embeds result |
| tool_call:send_to_agent | Calls Host POST /agent/:name, embeds result |
| tool_call:get_agent_info | Calls Host GET /api/agents/:name, embeds result |
| tool_call:unknown | Calls unknown tool, embeds error |
| tool_call:send_to_nonexistent | Calls POST /agent/nonexistent, embeds error |
| disconnect_mid_stream | Sends 1 SSE event then closes |
| empty_response | Only status updates, no message |
| delayed_start | Doesn't accept connections for first 2 attempts |

## E2E Testing

53 test cases across 6 categories:
- Messaging: 11 cases
- Discovery: 12 cases (actually 13, some listed in discovery section)
- Tool Call: 6 cases
- Tracing: 8 cases
- Registry: 6 cases
- Error/Edge: 8 cases

Docker Compose: Host container + Fake Agent container(s). Test binary connects to Host via HTTP.

## LLM Configuration (for test agents)

```json
{
  "llm": {
    "baseUrl": "https://token-plan-cn.xiaomimimo.com/v1",
    "apiKey": "tp-cvg1l8m7kmdevg8xts5j9qt34i9tpvxdj5o57q668da3g5om",
    "model": "mimo-v2.5-pro"
  }
}
```
