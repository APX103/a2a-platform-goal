# A2A Platform Demo: Agent Registration and Inter-Agent Communication

This demo shows two LLM agents registering with an A2A platform host and communicating through it.

## Architecture

```
                    +-----------------+
                    |   Host (7860)   |
                    |   A2A Platform   |
                    +--------+--------+
                             |
              +--------------+--------------+
              |                             |
     +--------+--------+          +--------+--------+
     | Agent Alpha     |          | Agent Beta      |
     | Port 10001      |          | Port 10002      |
     | (LLM + Tools)   |          | (LLM + Tools)   |
     +-----------------+          +-----------------+
```

Agents are initially **not connected** to the host. You register them manually via API calls. Once registered, agents can discover each other and communicate through the platform proxy.

## Prerequisites

1. Docker and Docker Compose installed
2. An LLM API key (OpenAI-compatible, e.g. for mimo-v2.5-pro)

## Quick Start

```bash
cd docs/demo

# 1. Create your environment file
cp .env.example .env
# Edit .env and set your LLM_API_KEY

# 2. Build and start all services
docker compose up -d --build

# 3. Wait for all services to be healthy
docker compose ps
```

All three services start independently. Agents are NOT auto-registered.

## Configuration Files

| File | Purpose |
|------|---------|
| `.env` | Your API key (**not committed to git**). Create from `.env.example` |
| `.env.example` | Template showing required/optional environment variables |
| `docker-compose.yml` | Service definitions for host + 2 agents |

The `.env` file contains a single required variable:

```
LLM_API_KEY=your-api-key-here
```

Optional variables (with defaults):

```
LLM_BASE_URL=https://token-plan-cn.xiaomimimo.com/v1
LLM_MODEL=mimo-v2.5-pro
```

---

## Step 1: Register Agents

Agents register themselves with the host by calling the registration endpoint. The host then fetches the agent's card from `/.well-known/agent.json` and stores the connection.

### Register agent-alpha

```bash
curl -X POST http://localhost:7860/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"url":"http://agent-alpha:10001","name":"agent-alpha"}'
```

Expected response:

```json
{
  "name": "agent-alpha",
  "status": "connected",
  "type": "",
  "url": "http://agent-alpha:10001",
  "version": "1.0.0"
}
```

### Register agent-beta

```bash
curl -X POST http://localhost:7860/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"url":"http://agent-beta:10002","name":"agent-beta"}'
```

### Verify both agents are registered

```bash
curl -s http://localhost:7860/api/agents | python3 -m json.tool
```

You should see both agents with `status: connected`.

---

## Step 2: Get an Agent's Card

Use the host's API to query agent information:

```bash
curl -s http://localhost:7860/api/agents/agent-beta | python3 -m json.tool
```

Response:

```json
{
  "name": "agent-beta",
  "url": "http://agent-beta:10002",
  "description": "LLM-backed agent using mimo-v2.5-pro",
  "version": "1.0.0",
  "status": "connected",
  "skills": [
    {"id": "chat", "name": "Chat", "description": "Chat with LLM"}
  ],
  "created_at": "2026-05-15T...",
  "updated_at": "2026-05-15T..."
}
```

---

## Step 3: Agent-to-Agent Communication

The platform provides tools that agents can use to discover and communicate with each other:

| Tool | Description |
|------|-------------|
| `list_agents` | List all registered agents |
| `get_agent_info` | Get details about a specific agent |
| `send_to_agent` | Send a message to another agent and get its response |

### Send a message from agent-alpha to agent-beta

Use the `/api/chat` endpoint. Agent-alpha will receive the message, decide to use the `send_to_agent` tool, call agent-beta through the platform, and synthesize the response.

```bash
curl -N -X POST http://localhost:7860/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"agent_name":"agent-alpha","message":"Use the send_to_agent tool to ask agent-beta: What is the capital of France?"}'
```

You will see SSE events in this order:

```
1. task       -- task created for agent-alpha
2. status_update -- SUBMITTED -> WORKING
3. artifact    -- "Using tool: send_to_agent({\"agent_name\": \"agent-beta\", ...})"
4. artifact    -- "Tool result: Paris"
5. message     -- "Agent-beta responded: Paris"
6. status_update -- WORKING -> COMPLETED
7. response    -- final answer text
```

Key observations:
- Events #3-4 show agent-alpha using the platform tool
- Event #4 shows agent-beta's actual response coming back through the platform
- The entire cross-agent exchange is tracked in the task trace

Note: The `response` event may contain multiple lines if the LLM formats its answer with line breaks. For programmatic parsing, extract JSON from each `data: ` line — the `artifact` events (tool calls and results) are always single-line.

### Alternative: Direct proxy call

You can also send messages through the JSON-RPC proxy endpoint:

```bash
curl -N -X POST http://localhost:7860/agent/agent-alpha \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d '{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"Tell me a joke"}]}}}'
```

---

## Step 4: Verify Traces

After sending messages, you can inspect traces to confirm inter-agent communication.

### List all tasks

```bash
curl -s http://localhost:7860/api/tasks | python3 -c "
import json,sys
tasks=json.load(sys.stdin)
for t in tasks:
    print(f'{t[\"agent_name\"]}: state={t[\"state\"]}, id={t[\"local_task_id\"][:12]}')
print(f'Total: {len(tasks)} tasks')
"
```

You should see tasks for both agent-alpha (who initiated the call) and agent-beta (who received the forwarded message).

### Get agent-alpha's trace

Copy the `local_task_id` for agent-alpha, then:

```bash
curl -s "http://localhost:7860/api/tasks/{TASK_ID}/trace"
```

The trace shows:

```
[timestamp] send {"content":"Use send_to_agent tool to ask agent-beta: What is the capital of France?"}
[timestamp] task_update {"new_state":"WORKING","old_state":"SUBMITTED"}
[timestamp] artifact {"text":"Using tool: send_to_agent({\"agent_name\": \"agent-beta\", ...})"}
[timestamp] artifact {"text":"Tool result: Paris"}
[timestamp] task_update {"new_state":"COMPLETED","old_state":"WORKING"}
[timestamp] response {"source":"agent-alpha","text":"Agent-beta responded: Paris"}
```

### Get agent-beta's trace

Copy the `local_task_id` for agent-beta (created when agent-alpha used `send_to_agent`):

```bash
curl -s "http://localhost:7860/api/tasks/{TASK_ID}/trace"
```

The trace shows:

```
[timestamp] send {"content":"What is the capital of France?"}
[timestamp] task_update {"new_state":"WORKING","old_state":"SUBMITTED"}
[timestamp] task_update {"new_state":"COMPLETED","old_state":"WORKING"}
[timestamp] response {"source":"agent-beta","text":"Paris"}
```

This confirms the complete communication chain: user -> agent-alpha -> platform -> agent-beta -> platform -> agent-alpha -> user.

### Filter tasks by agent

```bash
curl -s "http://localhost:7860/api/tasks?agent_name=agent-beta"
```

---

## Other Useful Endpoints

```bash
# Platform capabilities (includes tool definitions)
curl -s http://localhost:7860/api/capabilities | python3 -m json.tool

# Delete/disconnect an agent
curl -X DELETE http://localhost:7860/api/agents/agent-alpha

# Check platform health
curl -s http://localhost:7860/api/capabilities > /dev/null && echo "Host is up"
```

---

## Cleanup

```bash
docker compose down -v
```

This removes all containers and the database volume.

---

## How It Works Internally

1. **Registration**: `POST /api/agents/register` fetches the agent's `/.well-known/agent.json`, stores the connection, and makes it available for proxying.

2. **Tool calling**: When `HOST_URL` is set, agents include platform tool definitions (`list_agents`, `get_agent_info`, `send_to_agent`) in their LLM API requests. The LLM decides when to use them.

3. **send_to_agent**: When an agent's LLM calls this tool, the agent sends `POST /api/chat` to the host with the target agent's name and message. The host creates a new task, proxies to the target agent, and returns the response via SSE.

4. **Traces**: Every task has a trace accessible via `/api/tasks/{id}/trace`. When agent A sends a message to agent B via `send_to_agent`, two tasks are created (one for each agent), and both traces record the communication.
