下面是根据 **go-agent-prd.md** 和 **go-agent-cli-prd.md** 两个文档整合生成的 **Redmi.md**（可以理解为项目说明 / README / 架构说明文档）。

---

# Redmi.md — GoAgent 工业级 AI Agent 系统

## 1. 项目简介

**GoAgent** 是一个基于 Go 语言构建的工业级 AI Agent 运行时系统，采用 **PEMT 架构（Planner–Executor–Memory–Tool）**，并结合 **Event Bus + Graph FSM + Pipeline** 构建可控、可扩展、可观测的 Agent 执行框架。

系统提供：

* 长任务执行能力
* 多工具调用能力（Shell / File / Git / Browser / RAG / SQL）
* 分层 Memory 系统
* Context 智能构建
* 可控执行（FSM + TTL + 限流 + 去重）
* SSE / WebSocket 流式输出
* CLI 终端交互客户端

---

## 2. 系统总体架构

### 2.1 架构分层

```
Application Layer
    ├── Gin HTTP API
    ├── WebSocket / SSE
    ├── Auth / Session / RateLimit
    ▼
Agent Runtime Layer
    ├── Event Bus
    ├── Graph FSM
    ├── Scheduler
    ├── Planner
    ├── Executor
    ├── Reflection
    ▼
Pipeline Layer
    ├── Tool Result Pipeline
    ├── Context Builder Pipeline
    ▼
Tool Layer
    ├── Shell / File / Browser / Git / HTTP / SQL / RAG
    ├── MCP Adapter
    ▼
Memory Layer
    ├── Working Memory
    ├── Tool Memory
    ├── Episodic Memory
    ├── Knowledge Memory (RAG)
    ▼
Model Layer
    ├── LLM Adapter
    ├── Embedding
    ├── Reranker
```

系统本质定义：

```
GoAgent = Event Bus + Graph FSM + Pipeline + Memory + LLM + Tools
```



---

## 3. Agent 执行流程（核心闭环）

```
User Request
    │
    ▼
Create Session
    │
    ▼
EventBus.Emit("start")
    │
    ▼
Graph FSM 主循环
    │
    ├── PLANNING        → 生成 Plan
    ├── EXECUTING       → 调用工具 / LLM
    ├── WAIT_TOOL       → 等待工具结果
    ├── PROCESS_RESULT  → 处理工具结果
    ├── UPDATE_MEMORY   → 写入记忆
    ├── REFLECTING      → 判断是否完成/重规划
    ├── REPLANNING      → 重规划
    ├── NEXT_STEP       → 下一步
    └── DONE            → 完成
```

执行过程通过 **SSE 流式返回** 到客户端。

---

## 4. 核心模块

### 4.1 Event Bus（事件总线）

系统解耦核心，所有模块通过事件通信。

功能：

* 优先级队列
* 事件去重
* 限流
* TTL 防止无限循环
* 死循环检测
* 发布 / 订阅机制

事件类型示例：

| 类别 | 事件                           |   |
| -- | ---------------------------- | - |
| 控制 | start / stop / cancel        |   |
| 规划 | plan_created                 |   |
| 执行 | step_start / step_done       |   |
| 工具 | tool_call / tool_result      |   |
| 记忆 | memory_update                |   |
| 推理 | llm_request / llm_response   |   |
| 反思 | reflect_done / replan_needed |   |

---

### 4.2 FSM 状态机

Agent 的“大脑”，只负责状态流转，不执行业务逻辑。

核心状态：

```
IDLE
PLANNING
EXECUTING
WAIT_TOOL
PROCESS_RESULT
UPDATE_MEMORY
BUILD_CONTEXT
LLM_THINKING
REFLECTING
REPLANNING
NEXT_STEP
WAIT_USER_INPUT
DONE
ERROR
CANCELLED
TIMEOUT
```

状态流转示例：

| 当前状态          | 事件             | 下一状态           |   |
| ------------- | -------------- | -------------- | - |
| IDLE          | start          | PLANNING       |   |
| PLANNING      | plan_created   | EXECUTING      |   |
| EXECUTING     | tool_call      | WAIT_TOOL      |   |
| WAIT_TOOL     | tool_result    | PROCESS_RESULT |   |
| UPDATE_MEMORY | memory_updated | REFLECTING     |   |
| REFLECTING    | replan_needed  | REPLANNING     |   |
| NEXT_STEP     | all_done       | DONE           |   |

---

### 4.3 Planner（规划器）

负责把用户 Goal 拆分为可执行步骤：

```go
type Plan struct {
    Goal        string
    Steps       []Step
    CurrentStep int
}
```

每个 Step 必须绑定一个 Tool：

```go
type Step struct {
    Name        string
    Tool        string
    ToolInput   any
    Status      StepStatus
}
```

支持 **局部重规划（Replan）**。

---

### 4.4 Executor（执行器）

采用 **ReAct 模式**：

```
Thought → Action → Observation → Thought → Action ...
```

负责：

* 调用 LLM
* 解析 Tool Call
* 执行工具
* 返回结果

---

### 4.5 Tool System（工具系统）

统一工具接口：

```go
type Tool interface {
    Name() string
    Description() string
    Schema() ToolSchema
    Execute(ctx context.Context, input map[string]any) (*ToolResult, error)
}
```

MVP 工具列表：

| 工具           | 功能      |
| ------------ | ------- |
| shell.exec   | 执行命令    |
| file.read    | 读文件     |
| file.write   | 写文件     |
| file.list    | 列目录     |
| file.search  | 搜索      |
| git.clone    | 克隆仓库    |
| http.request | HTTP 请求 |
| sql.query    | 数据库查询   |
| rag.search   | 向量检索    |
| browser.open | 打开网页    |

支持 **MCP 协议扩展工具生态**。 

---

### 4.6 Memory 系统

分层记忆：

| Memory           | 作用      |
| ---------------- | ------- |
| Working Memory   | 当前任务上下文 |
| Tool Memory      | 工具结果    |
| Episodic Memory  | 历史任务    |
| Knowledge Memory | RAG 知识库 |

存储技术：

* Vector DB：Qdrant
* Relational DB：PostgreSQL
* Cache：Redis 

---

### 4.7 Pipeline 系统

三条核心 Pipeline：

#### 1. Tool Result Pipeline

```
Tool Result
 → Filter
 → Extract
 → Summarize
 → Embed
 → Store
```

#### 2. Context Builder Pipeline

```
Load Goal
Load Plan
Load Step
Retrieve Memory
Retrieve Knowledge
Rerank
Compress History
Token Budget Control
Build Prompt
```

用于构建 LLM Prompt。 

---

## 5. GoAgent CLI（终端客户端）

GoAgent CLI 是终端交互式客户端，类似：

* claude-code
* cursor agent
* open-interpreter

CLI 功能：

| 功能             | 描述                     |
| -------------- | ---------------------- |
| 会话保持           | Session ID             |
| SSE 流式输出       | 实时显示 Thought / Tool    |
| Slash Commands | /model /session /clear |
| 文件上传           | /upload                |
| 文件下载           | /download              |
| 审批机制           | 高风险操作确认                |
| 本地服务自启         | 自动启动 go-agent          |
| 非阻塞输入          | Agent 执行时可继续输入         |

TUI 使用 **bubbletea** 实现。 

### CLI 界面布局

```
┌──────────────────────────────────────┐
│ Conversation Stream                  │
│--------------------------------------│
│ Thought: ...                         │
│ Action: shell.exec                   │
│ Result: ...                          │
│                                      │
├──────────────────────────────────────┤
│ Input                                │
│ >                                    │
├──────────────────────────────────────┤
│ Status Bar                           │
│ Session | Tokens | Time | API Addr   │
└──────────────────────────────────────┘
```



---

## 6. API 接口

核心 API：

| Method | API                      | 说明   |   |
| ------ | ------------------------ | ---- | - |
| POST   | /api/v1/agent/run        | 启动任务 |   |
| GET    | /api/v1/agent/:id/stream | SSE  |   |
| GET    | /api/v1/agent/:id/status | 状态   |   |
| DELETE | /api/v1/agent/:id        | 取消   |   |
| POST   | /api/v1/tools/call       | 调用工具 |   |
| GET    | /api/v1/memory/search    | 记忆检索 |   |
| GET    | /api/v1/health           | 健康检查 |   |

SSE 返回格式：

```
data: {"type":"thought","content":"..."}
data: {"type":"tool_call","tool":"shell.exec"}
data: {"type":"tool_result","success":true}
data: {"type":"done","result":"任务完成"}
```



---

## 7. 技术栈

| 层             | 技术                          |
| ------------- | --------------------------- |
| Web           | Gin                         |
| FSM           | 自研 Graph FSM                |
| Event         | Event Bus                   |
| Pipeline      | Middleware                  |
| Memory        | PostgreSQL + Qdrant + Redis |
| LLM           | OpenAI / Anthropic / Ollama |
| CLI           | Bubbletea                   |
| Sandbox       | Docker                      |
| Observability | Prometheus + Logs           |

---

## 8. 项目目录结构（建议）

```
go-agent/
├── cmd/
│   ├── server/
│   └── cli/
├── internal/
│   ├── agent/
│   ├── planner/
│   ├── executor/
│   ├── memory/
│   ├── tools/
│   ├── eventbus/
│   ├── fsm/
│   ├── pipeline/
│   ├── context/
│   ├── reflection/
│   ├── api/
│   └── scheduler/
├── pkg/
├── configs/
├── deployments/
└── docs/
```

---

## 9. 系统设计总结

GoAgent 的核心思想可以总结为一句话：

> **用 Event 驱动 FSM，用 FSM 驱动 Pipeline，用 Pipeline 驱动 LLM + Tools，用 Memory 让 Agent 变聪明。**

系统关键设计：

| 模块              | 作用        |
| --------------- | --------- |
| Event Bus       | 解耦        |
| FSM             | 控制流程      |
| Planner         | 任务拆解      |
| Executor        | 执行        |
| Tool            | 行动        |
| Memory          | 记忆        |
| Pipeline        | 数据处理      |
| Context Builder | Prompt 构建 |
| Reflection      | 自我修正      |

---

## 10. 项目目标

GoAgent 目标不是一个简单 AI 调用框架，而是一个：

* 可执行复杂任务
* 可长时间运行
* 可控
* 可观测
* 可扩展
* 可本地运行
* 可接入工具生态（MCP）

的 **工业级 Agent Runtime**。

---

如果你愿意，下一步我可以帮你生成：

* `ARCHITECTURE.md`
* `CLI_DESIGN.md`
* `FSM_DESIGN.md`
* `EVENT_SYSTEM.md`
* `ROADMAP.md`

这些文档，把这个项目变成一个非常完整的工程设计文档体系。
