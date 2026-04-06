# GoAgent — 工业级 Go AI Agent 系统产品需求文档（PRD）

**版本**: v1.0  
**状态**: 待评审  
**目标读者**: 后端工程师、架构师  
**语言**: Go 1.22+  

---

## 目录

1. [产品概述](#1-产品概述)
2. [系统架构总览](#2-系统架构总览)
3. [核心模块详细设计](#3-核心模块详细设计)
   - 3.1 Web API 层
   - 3.2 Event Bus（事件总线）
   - 3.3 FSM 状态机引擎
   - 3.4 Planner（规划器）
   - 3.5 Executor（执行器）
   - 3.6 Tool System（工具系统）
   - 3.7 Pipeline 系统
   - 3.8 Memory 系统
   - 3.9 Context Builder（上下文构建器）
   - 3.10 Reflection（反思模块）
   - 3.11 LLM Adapter（模型适配层）
   - 3.12 Scheduler（调度器）
4. [数据结构设计](#4-数据结构设计)
5. [接口规范（Interface Contracts）](#5-接口规范)
6. [技术栈选型](#6-技术栈选型)
7. [工程目录结构](#7-工程目录结构)
8. [数据库 Schema](#8-数据库-schema)
9. [API 接口文档](#9-api-接口文档)
10. [配置规范](#10-配置规范)
11. [安全与限流](#11-安全与限流)
12. [可观测性](#12-可观测性)
13. [部署架构](#13-部署架构)
14. [开发实施路线](#14-开发实施路线)
15. [附录：设计决策说明](#15-附录设计决策说明)

---

## 1. 产品概述

### 1.1 背景与目标

GoAgent 是一套基于 **PEMT 架构**（Planner–Executor–Memory–Tool）的工业级 AI Agent 运行时平台，采用 Go 语言实现。参考 OpenDevin、LangGraph、Claude Code 的核心设计，实现一个：

- 支持复杂长任务的 Agent 运行时
- 具备完整 Memory 分层与 Context 智能构建
- 基于 Event-driven Graph FSM 的可控执行引擎
- 可通过 MCP 协议接入丰富工具生态
- 以标准 Gin Web 后端对外提供服务

### 1.2 核心设计原则

| 原则 | 说明 |
|------|------|
| Event-First | 所有状态变更由事件驱动，模块间解耦 |
| Pipeline-based | 数据流处理全部链式中间件化 |
| Memory-aware | 工具结果经处理后入库，按需注入上下文 |
| Controllable | 内置防爆机制：TTL、限流、去重、最大步数 |
| Observable | 全链路追踪、结构化日志、Prometheus 指标 |
| Extensible | Tool、Memory、LLM 均面向接口编程 |

### 1.3 系统本质定义

```
GoAgent = Event Bus + Graph FSM + Pipeline + Memory + LLM + Tools
```

或用软件工程语言描述：

```
GoAgent = Controller（Gin）
        + State Machine（FSM Engine）
        + Middleware Pipeline（Tool/Context/Reflection）
        + Tool System（MCP + 内置工具）
        + Memory System（分层记忆）
        + LLM Adapter（多模型）
```

---

## 2. 系统架构总览

### 2.1 分层架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        Application Layer                      │
│              HTTP API (Gin) · WebSocket · SSE                │
│            Auth · Session · Rate Limit · Router              │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                       Agent Runtime Layer                     │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │ Event Bus   │  │  Graph FSM   │  │   Scheduler        │  │
│  │ (优先队列)   │  │ (状态控制器)  │  │   (并发调度)       │  │
│  └─────────────┘  └──────────────┘  └────────────────────┘  │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │  Planner    │  │  Executor    │  │  Reflection        │  │
│  │  (规划器)    │  │  (执行器)    │  │  (反思/重规划)      │  │
│  └─────────────┘  └──────────────┘  └────────────────────┘  │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                       Pipeline Layer                          │
│  Tool Result Pipeline · Context Builder Pipeline             │
│  Reflection Pipeline · (Filter→Extract→Summarize→Embed→Store)│
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                        Tool Layer                             │
│   Shell · File · Browser · HTTP · Git · SQL · Search · RAG   │
│                  MCP Protocol Adapter                        │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                       Memory Layer                            │
│  Working Memory · Tool Memory · Episodic · Knowledge (RAG)   │
│  Vector DB (Qdrant) · Relational DB (PostgreSQL) · Redis     │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                        Model Layer                            │
│         LLM Adapter · Embedding Model · Reranker             │
│         OpenAI / Anthropic / Ollama (本地)                   │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Agent 核心工作流（完整闭环）

```
User Request（HTTP POST /agent/run）
        │
        ▼
  [创建 AgentSession]
        │
        ▼
  EventBus.Emit(event: "start")
        │
        ▼
  ┌──────────────────────────────────────┐
  │           Graph FSM 主循环            │
  │                                      │
  │  IDLE → PLANNING                     │
  │           │                          │
  │           ▼ plan_created             │
  │        EXECUTING                     │
  │           │                          │
  │           ▼ tool_called              │
  │        WAIT_TOOL ──────────────┐     │
  │           │ tool_result        │     │
  │           ▼                   │     │
  │    PROCESS_RESULT             async  │
  │           │                   │     │
  │           ▼ processed         │     │
  │      UPDATE_MEMORY            │     │
  │           │                   │     │
  │           ▼ memory_updated    │     │
  │       REFLECTING              │     │
  │        /     \                │     │
  │  success     failure          │     │
  │      │           │            │     │
  │  NEXT_STEP   REPLANNING       │     │
  │      │           │            │     │
  │   (all done)  (retry)         │     │
  │      │           │            │     │
  │    DONE      EXECUTING        │     │
  └──────────────────────────────────────┘
        │
        ▼
  SSE/WebSocket 流式推送结果至前端
```

---

## 3. 核心模块详细设计

### 3.1 Web API 层

**职责**：接收 HTTP 请求，管理 Session，负责 SSE 流式输出。

**框架**：Gin + 标准中间件

#### 路由设计

```
POST   /api/v1/agent/run          # 启动 Agent 任务
GET    /api/v1/agent/:session_id/stream  # SSE 流式接收执行过程
GET    /api/v1/agent/:session_id/status  # 查询当前状态
DELETE /api/v1/agent/:session_id        # 取消/停止任务

POST   /api/v1/agent/:session_id/plan   # 触发重规划
POST   /api/v1/tools/call               # 直接调用单个工具（调试）

GET    /api/v1/memory/search            # 检索记忆
DELETE /api/v1/memory/:session_id       # 清理 Session 记忆

GET    /api/v1/health                   # 健康检查
GET    /metrics                         # Prometheus 指标
```

#### Gin 中间件栈

```
RequestID → Logger → Recovery → Auth → RateLimit → CORS → Handler
```

#### SSE 流式输出格式

```
data: {"type":"thought","content":"正在分析需求...","step":1}
data: {"type":"tool_call","tool":"shell.exec","input":"go build ./..."}
data: {"type":"tool_result","success":true,"content":"BUILD OK"}
data: {"type":"step_done","step":1,"total":3}
data: {"type":"done","result":"任务完成"}
```

---

### 3.2 Event Bus（事件总线）

**职责**：系统解耦核心。所有模块只通过事件通信，不直接调用彼此。

#### 事件结构定义

```go
type Event struct {
    ID        string                 // 唯一 ID（UUID）
    Type      EventType              // 事件类型
    Payload   map[string]interface{} // 事件载荷
    SessionID string                 // 归属 Session
    Priority  int                    // 优先级（0最高）
    TTL       int                    // 剩余处理次数
    Hash      string                 // 去重指纹（payload hash）
    CreatedAt time.Time
    ExpiresAt time.Time              // 超时时间
}
```

#### 事件类型清单

| 类别 | 事件类型 | 触发时机 |
|------|---------|---------|
| 控制 | `start` / `stop` / `pause` / `resume` / `cancel` | 用户操作 |
| 控制 | `timeout` / `loop_detected` | 系统防护 |
| 规划 | `plan_requested` / `plan_created` / `plan_updated` | Planner |
| 执行 | `step_start` / `step_done` / `step_failed` | Executor |
| 工具 | `tool_call` / `tool_result` / `tool_error` | Tool System |
| 记忆 | `memory_update` / `memory_batch_update` | Memory |
| 推理 | `llm_request` / `llm_response` / `llm_error` | LLM Adapter |
| 反思 | `reflect_start` / `reflect_done` / `replan_needed` | Reflection |

#### 事件总线核心功能

```
EventBus
 ├── 优先级队列（P0=cancel/error > P2=tool_result > P4=memory）
 ├── 去重（Deduplication by Hash，滑动窗口 30s）
 ├── 限流（Rate Limit：replan≤3/min，tool_retry≤2/min）
 ├── 防抖（Debounce：memory_update 合并 2s 内事件）
 ├── TTL 检测（每次 dispatch 减 TTL，归零则丢弃）
 ├── 死循环检测（相同事件路径重复 3 次 → emit loop_detected）
 └── 发布/订阅（Subscribe by EventType or SessionID）
```

#### EventBus 接口

```go
type EventBus interface {
    Emit(ctx context.Context, event Event) error
    Subscribe(eventType EventType, handler EventHandler) (unsubscribe func())
    SubscribeSession(sessionID string, handler EventHandler) (unsubscribe func())
    Shutdown(ctx context.Context) error
}

type EventHandler func(ctx context.Context, event Event)
```

---

### 3.3 FSM 状态机引擎

**职责**：Agent 的"大脑"，根据当前状态和事件决定下一状态，不执行业务逻辑。

#### 状态定义

```go
type AgentState string

const (
    StateIdle           AgentState = "IDLE"
    StatePlanning       AgentState = "PLANNING"
    StateReady          AgentState = "READY"
    StateExecuting      AgentState = "EXECUTING"
    StateWaitTool       AgentState = "WAIT_TOOL"
    StateProcessResult  AgentState = "PROCESS_RESULT"
    StateUpdateMemory   AgentState = "UPDATE_MEMORY"
    StateBuildContext   AgentState = "BUILD_CONTEXT"
    StateLLMThinking    AgentState = "LLM_THINKING"
    StateReflecting     AgentState = "REFLECTING"
    StateReplanning     AgentState = "REPLANNING"
    StateNextStep       AgentState = "NEXT_STEP"
    StateWaitUserInput  AgentState = "WAIT_USER_INPUT"
    StateDone           AgentState = "DONE"
    StateError          AgentState = "ERROR"
    StateCancelled      AgentState = "CANCELLED"
    StateTimeout        AgentState = "TIMEOUT"
)
```

#### 状态转移表（核心）

| 当前状态 | 事件 | 下一状态 |
|---------|------|---------|
| IDLE | start | PLANNING |
| PLANNING | plan_created | EXECUTING |
| EXECUTING | tool_call | WAIT_TOOL |
| EXECUTING | llm_request | LLM_THINKING |
| WAIT_TOOL | tool_result | PROCESS_RESULT |
| WAIT_TOOL | tool_error | PROCESS_RESULT |
| WAIT_TOOL | timeout | REFLECTING |
| PROCESS_RESULT | processed | UPDATE_MEMORY |
| UPDATE_MEMORY | memory_updated | REFLECTING |
| REFLECTING | step_done | NEXT_STEP |
| REFLECTING | replan_needed | REPLANNING |
| NEXT_STEP | has_next_step | BUILD_CONTEXT |
| NEXT_STEP | all_done | DONE |
| BUILD_CONTEXT | context_built | EXECUTING |
| BUILD_CONTEXT | build_failed | ERROR |
| REPLANNING | plan_updated | BUILD_CONTEXT |
| REPLANNING | replan_failed | ERROR |
| ANY | cancel | CANCELLED |
| ANY | loop_detected | ERROR |
| ANY | cost_exceeded | ERROR |

#### FSM 接口

```go
type FSMEngine interface {
    // 处理一个事件，返回新状态和需发送的事件
    Transition(ctx context.Context, session *AgentSession, event Event) (AgentState, []Event, error)
    // 注册状态处理器
    RegisterHandler(state AgentState, handler StateHandler)
    // 当前状态快照
    CurrentState(sessionID string) AgentState
}

type StateHandler func(ctx context.Context, session *AgentSession, event Event) ([]Event, error)
```

---

### 3.4 Planner（规划器）

**职责**：接收用户 Goal，调用 LLM 生成结构化 Plan，支持局部重规划。

#### Plan 数据结构

```go
type Plan struct {
    ID          string    `json:"id"`
    Goal        string    `json:"goal"`
    Steps       []Step    `json:"steps"`
    CurrentStep int       `json:"current_step"`
    Version     int       `json:"version"` // 重规划次数
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Step struct {
    ID          int        `json:"id"`
    Name        string     `json:"name"`
    Description string     `json:"description"`
    Tool        string     `json:"tool"`       // 绑定工具名
    ToolInput   any        `json:"tool_input"` // 预期工具入参（可选）
    Status      StepStatus `json:"status"`     // pending/running/done/failed/skipped
    Result      string     `json:"result"`     // 执行结果摘要
    RetryCount  int        `json:"retry_count"`
}

type StepStatus string
const (
    StepPending  StepStatus = "pending"
    StepRunning  StepStatus = "running"
    StepDone     StepStatus = "done"
    StepFailed   StepStatus = "failed"
    StepSkipped  StepStatus = "skipped"
)
```

#### Planner 接口

```go
type Planner interface {
    // 生成整体 Plan
    CreatePlan(ctx context.Context, goal string, agentCtx *AgentContext) (*Plan, error)
    // 局部重规划（从 currentStep 开始修改）
    Replan(ctx context.Context, plan *Plan, failureReason string, agentCtx *AgentContext) (*Plan, error)
    // 拆分步骤粒度校验（防止步骤过大/过小）
    ValidatePlan(plan *Plan) error
}
```

#### Planner Prompt 设计

**Create Plan Prompt**:
```
You are a task planner for an AI agent system.

User Goal: {goal}

Available Tools: {tool_list_json}

Current Context:
{relevant_memory}

Break this goal into concrete, executable steps.
Each step MUST bind to exactly one tool.
Steps should be atomic (not too large, not too small).

Return ONLY valid JSON:
{
  "steps": [
    {"id": 1, "name": "...", "description": "...", "tool": "...", "tool_input": {...}},
    ...
  ]
}
```

**Replan Prompt**:
```
You are replanning an AI agent task.

Goal: {goal}
Current Plan: {plan_json}
Current Step: {current_step}
Failure Reason: {failure_reason}
Step History: {step_history}

Fix the plan from step {current_step} onwards.
Do NOT change completed steps.
Return ONLY the updated steps as JSON array.
```

---

### 3.5 Executor（执行器）

**职责**：读取 Plan 当前步骤，确定工具和参数，触发工具调用。采用 ReAct 推理模式。

#### ReAct 推理循环

```
Thought: 分析当前步骤需要做什么
Action: 选择工具和参数
Observation: 工具返回结果
Thought: 分析结果，决定下一步
Action: ...（直到当前 Step 完成）
```

#### Executor 接口

```go
type Executor interface {
    // 执行当前 Plan Step
    ExecuteStep(ctx context.Context, session *AgentSession) (*ToolCall, error)
    // 解析 LLM ReAct 输出，提取工具调用
    ParseToolCall(llmOutput string) (*ToolCall, error)
}

type ToolCall struct {
    ToolName  string         `json:"tool_name"`
    Input     map[string]any `json:"input"`
    SessionID string         `json:"session_id"`
    StepID    int            `json:"step_id"`
}
```

---

### 3.6 Tool System（工具系统）

**职责**：统一管理和执行所有 Agent 工具，支持 MCP 协议扩展。

#### 工具接口

```go
type Tool interface {
    Name() string
    Description() string
    Schema() ToolSchema      // JSON Schema，用于 function calling
    Execute(ctx context.Context, input map[string]any) (*ToolResult, error)
    Validate(input map[string]any) error
}

type ToolResult struct {
    Success    bool           `json:"success"`
    Stdout     string         `json:"stdout,omitempty"`
    Stderr     string         `json:"stderr,omitempty"`
    ExitCode   int            `json:"exit_code,omitempty"`
    Data       map[string]any `json:"data,omitempty"`
    RawOutput  string         `json:"raw_output"`
    FilesChanged []string     `json:"files_changed,omitempty"`
    TokensUsed int            `json:"tokens_used,omitempty"`
    Duration   time.Duration  `json:"duration"`
}
```

#### 内置工具清单（MVP 必须实现）

| 工具名 | 类别 | 说明 | 安全级别 |
|--------|------|------|---------|
| `shell.exec` | 系统 | 在 Docker sandbox 执行 shell 命令 | 高风险，需 sandbox |
| `shell.exec_safe` | 系统 | 白名单命令执行（不需 sandbox） | 低风险 |
| `file.read` | 文件 | 读取文件内容 | 低风险 |
| `file.write` | 文件 | 写入/创建文件 | 中风险 |
| `file.list` | 文件 | 列出目录结构 | 低风险 |
| `file.search` | 文件 | grep 搜索文件内容 | 低风险 |
| `file.patch` | 文件 | 应用 diff patch | 中风险 |
| `browser.open` | 浏览器 | 打开 URL，返回页面内容 | 中风险 |
| `browser.click` | 浏览器 | 点击页面元素 | 中风险 |
| `browser.find` | 浏览器 | 查找页面元素 | 低风险 |
| `search.web` | 搜索 | 调用 Tavily/Serper 搜索 | 低风险 |
| `rag.search` | 知识库 | 向量检索本地知识库 | 低风险 |
| `git.clone` | Git | 克隆仓库 | 中风险 |
| `git.commit` | Git | 提交代码 | 中风险 |
| `http.request` | 网络 | 发起 HTTP 请求 | 中风险 |
| `sql.query` | 数据库 | 只读 SQL 查询 | 中风险 |
| `python.exec` | 代码 | 执行 Python 代码（sandbox） | 高风险 |

#### ToolResult Shell 返回格式（标准化）

```json
{
  "success": true,
  "stdout": "...",
  "stderr": "",
  "exit_code": 0,
  "files_changed": ["main.go"],
  "duration": "1.2s"
}
```

#### Shell Sandbox 设计（Docker）

```
每个 Session 对应一个 Docker 容器（按需启动）
容器配置：
  - 资源限制：CPU 1核，内存 512MB
  - 网络：可配置隔离或允许
  - 超时：单命令 60s，整体 10min
  - 挂载：工作目录 /workspace（持久化到宿主机）
  - 镜像：goagent-sandbox（含常用开发工具）
```

#### MCP 协议适配器

```go
type MCPAdapter interface {
    // 从 MCP Server 拉取工具定义
    LoadTools(serverURL string) ([]Tool, error)
    // 代理工具调用到 MCP Server
    CallTool(ctx context.Context, serverURL, toolName string, input map[string]any) (*ToolResult, error)
}
```

支持的 MCP Servers（优先级）：
- `filesystem` — 文件操作
- `git` — Git 操作  
- `github` — GitHub API
- `postgres` — 数据库操作
- `browser` — 浏览器控制
- `shell` — 终端执行

---

### 3.7 Pipeline 系统

**职责**：链式数据处理，以中间件模式组合。系统存在三条核心 Pipeline。

#### Pipeline 接口定义

```go
type Middleware func(ctx context.Context, agentCtx *AgentContext, next func(context.Context, *AgentContext) error) error

type Pipeline struct {
    middlewares []Middleware
}

func (p *Pipeline) Use(m Middleware) *Pipeline
func (p *Pipeline) Run(ctx context.Context, agentCtx *AgentContext) error
```

#### Pipeline 1：Tool Result Pipeline

处理工具原始输出，将其转化为可存储的知识。

```
ToolResult（原始输出）
    │
    ▼ [LengthFilter] 超长截断/跳过
    │
    ▼ [ContentFilter] 安全过滤（敏感信息脱敏）
    │
    ▼ [Extractor] 提取关键信息（调用 LLM 小模型）
    │
    ▼ [Summarizer] 生成摘要（调用 LLM）
    │
    ▼ [Embedder] 向量化 summary（调用 Embedding 模型）
    │
    ▼ [Storage] 存储到 ToolMemory + VectorDB
```

Extractor/Summarizer Prompt：
```
Tool: {tool_name}
User Goal: {goal}
Raw Output: {truncated_output}

Extract:
1. Summary（≤100 words）
2. Key points（≤5 bullets）
3. Important entities（names, paths, errors）
4. Metrics/numbers（if any）
5. Relevance to goal（high/medium/low）

Return ONLY JSON.
```

#### Pipeline 2：Context Builder Pipeline

在每次 LLM 调用前，智能组装 Prompt。

```
[LoadGoal]          → 加载用户目标
[LoadPlan]          → 加载当前 Plan 和进度
[LoadCurrentStep]   → 加载当前步骤详情
[RetrieveMemory]    → Vector Search（Tool Memory + Episodic）
[RetrieveKnowledge] → RAG 检索（Knowledge Memory）
[Rerank]            → bge-reranker 精排 Top 20 → Top 5
[CompressHistory]   → 压缩历史对话（Sliding Window）
[TokenBudgetControl]→ Token 分配和截断（见预算表）
[BuildPrompt]       → 拼装最终 Prompt
```

**Token Budget 分配（128k context）**：

| 区域 | Token 分配 | 说明 |
|------|-----------|------|
| System Prompt | 2,000 | 固定 |
| Goal | 1,000 | 固定 |
| Plan + Current Step | 3,000 | 固定 |
| Tool Results Summary | 15,000 | 最近 5 条 |
| RAG Knowledge | 40,000 | 向量检索结果 |
| Conversation History | 20,000 | Sliding Window |
| Scratchpad (ReAct) | 20,000 | Thought/Action |
| Buffer | 27,000 | 预留 |

超出预算时的压缩顺序：
1. 压缩/截断 Conversation History
2. 减少 RAG 返回数量
3. 删除最旧 Tool Result
4. 压缩 Scratchpad

**最终 Prompt 结构**：
```
[System]
You are an expert AI agent. You reason carefully and use tools effectively.
Always think step by step. Return tool calls as JSON.

[Goal]
{goal}

[Current Plan]
{plan_with_progress}
Current Step: {current_step_detail}

[Relevant Knowledge]
{rag_results}

[Recent Tool Results]
{tool_memory_summaries}

[Conversation History]
{compressed_history}

[Scratchpad]
Thought:
Action:
Observation:
```

#### Pipeline 3：Reflection Pipeline

评估步骤结果，决定是继续还是重规划。

```
[EvaluateToolResult] → 检查工具是否成功（exit_code, error）
[LLMJudge]           → LLM 评估是否达成步骤目标
[DetectLoop]         → 检查是否有重复失败模式
[DecideAction]       → 输出：success / retry / replan / abort
```

Reflection Prompt：
```
Goal: {goal}
Current Step: {step_name}
Tool Used: {tool_name}
Tool Result: {result_summary}
Previous Failures: {failure_history}
Retry Count: {retry_count}

Evaluate this step result.
Return ONLY JSON:
{
  "success": bool,
  "action": "continue" | "retry" | "replan" | "abort",
  "reason": "...",
  "suggested_fix": "..."  // 仅 replan 时填写
}
```

---

### 3.8 Memory 系统

**职责**：Agent 的记忆系统，分层存储，按需检索，防止上下文污染。

#### Memory 分层架构

```
Memory System
├── Working Memory    （当前任务上下文，存于 AgentContext 内存）
├── Tool Memory       （工具结果摘要，存于 PostgreSQL + Qdrant）
├── Episodic Memory   （历史任务记录，存于 PostgreSQL）
├── Knowledge Memory  （长期知识库 RAG，存于 Qdrant）
└── Conversation Memory（对话历史，存于 Redis，TTL 24h）
```

#### Memory 接口

```go
type MemoryManager interface {
    // Tool Memory
    SaveToolMemory(ctx context.Context, mem *ToolMemory) error
    SearchToolMemory(ctx context.Context, query string, sessionID string, topK int) ([]*ToolMemory, error)

    // Episodic Memory
    SaveEpisode(ctx context.Context, ep *Episode) error
    SearchEpisodes(ctx context.Context, query string, topK int) ([]*Episode, error)

    // Knowledge Memory（RAG）
    IndexDocument(ctx context.Context, doc *Document) error
    SearchKnowledge(ctx context.Context, query string, topK int) ([]*KnowledgeChunk, error)

    // Conversation
    AppendMessage(ctx context.Context, sessionID string, msg *Message) error
    GetConversation(ctx context.Context, sessionID string, limit int) ([]*Message, error)

    // 清理
    ClearSession(ctx context.Context, sessionID string) error
}
```

#### VectorDB 检索策略

```
检索 Query 构建：
query = goal + " " + current_step + " " + last_tool_result_summary

检索流程：
1. Vector Search Top-20（Qdrant cosine similarity）
2. bge-reranker 精排 → Top-5
3. Token 预算截断
```

---

### 3.9 Context Builder（上下文构建器）

**职责**：执行 Context Builder Pipeline，为每次 LLM 调用构建最优 Prompt。

```go
type ContextBuilder interface {
    Build(ctx context.Context, session *AgentSession) (*Prompt, error)
}

type Prompt struct {
    System    string
    Messages  []Message
    Tools     []ToolSchema // function calling 工具列表
    TokensUsed int
}
```

Context Builder 是 Agent 智能程度的关键因素，见 Pipeline 2 设计。

---

### 3.10 Reflection（反思模块）

**职责**：执行 Reflection Pipeline，防止死循环，触发重规划。

内置保险丝：

```go
type ReflectionConfig struct {
    MaxRetryPerStep int           // 单步最大重试次数，默认 3
    MaxReplanCount  int           // 最大重规划次数，默认 5
    MaxStepCount    int           // 最大总步骤数，默认 30
    MaxLLMCalls     int           // 最大 LLM 调用次数，默认 30
    MaxToolCalls    int           // 最大工具调用次数，默认 20
    MaxRuntime      time.Duration // 最大运行时间，默认 10min
    MaxTokenBudget  int           // 最大 Token 消耗，默认 200k
}
```

---

### 3.11 LLM Adapter（模型适配层）

**职责**：统一多种 LLM 的调用接口，支持流式输出和 Function Calling。

```go
type LLMClient interface {
    // 标准聊天完成
    ChatComplete(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    // 流式输出
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
    // 带工具调用
    ChatWithTools(ctx context.Context, req *ChatRequest, tools []ToolSchema) (*ChatResponse, error)
    // Embedding
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

#### 支持的 LLM 后端

| 后端 | 用途 | 配置 |
|------|------|------|
| OpenAI GPT-4o | 主力模型（Cloud） | API Key |
| Anthropic Claude 3.5 Sonnet | 主力模型（Cloud） | API Key |
| Ollama (qwen2.5/llama3) | 本地模型 | 本地 URL |
| OpenAI compatible API | 通用接入 | Base URL + Key |

#### 分模型策略（Cost Optimization）

| 场景 | 推荐模型 | 原因 |
|------|---------|------|
| Planner（Global） | GPT-4o / Claude Sonnet | 需要强推理 |
| Executor（ReAct） | GPT-4o / Claude Sonnet | 需要工具调用 |
| Tool Summarizer | GPT-4o-mini / Ollama | 低成本任务 |
| Reranker | bge-reranker（本地） | 速度快无成本 |
| Embedding | bge-small（本地） | 本地运行 |
| Reflection Judge | GPT-4o-mini | 成本优化 |

#### LLM Router（多模型路由）

不同任务场景自动路由到最优模型，平衡能力与成本：

```go
type LLMRouter struct {
    clients map[string]LLMClient // model_name → client
    config  *LLMRoutingConfig
}

// 按场景路由
func (r *LLMRouter) Route(scene LLMScene) LLMClient {
    switch scene {
    case ScenePlanning:
        return r.clients[r.config.PlannerModel]   // gpt-4o
    case SceneExecute:
        return r.clients[r.config.ExecutorModel]  // gpt-4o
    case SceneSummarize:
        return r.clients[r.config.SummaryModel]   // gpt-4o-mini（省钱）
    case SceneReflect:
        return r.clients[r.config.ReflectModel]   // gpt-4o-mini（省钱）
    default:
        return r.clients[r.config.DefaultModel]
    }
}

type LLMScene string
const (
    ScenePlanning   LLMScene = "planning"
    SceneExecute    LLMScene = "execute"
    SceneSummarize  LLMScene = "summarize"
    SceneReflect    LLMScene = "reflect"
    SceneEmbed      LLMScene = "embed"     // 始终用本地 bge-small
    SceneRerank     LLMScene = "rerank"    // 始终用本地 bge-reranker
)
```

#### Scratchpad（ReAct 推理链）管理

Scratchpad 记录当前 Step 内的完整 ReAct 推理链，是 Context Builder 的重要输入：

```go
type Scratchpad struct {
    Entries []ScratchpadEntry
    MaxTokens int
}

type ScratchpadEntry struct {
    Turn      int    `json:"turn"`
    Thought   string `json:"thought"`
    Action    string `json:"action"`     // tool_name + input JSON
    Observation string `json:"observation"` // tool result summary
}

// Scratchpad 管理规范：
// 1. 每个 Step 开始时 Reset Scratchpad
// 2. 每次 ReAct 循环后 Append 新 Entry
// 3. 超过 MaxTokens 时，保留最新 N 条 Entry（FIFO 压缩）
// 4. Step 完成后，将 Scratchpad 压缩摘要存入 Tool Memory
// 5. ReAct 最大轮数：默认 10 轮（防止单步内无限循环）
```

---

**职责**：并发管理 Agent Session，支持多任务并行、超时控制。

```go
type Scheduler interface {
    // 提交新 Session 到调度队列
    Submit(session *AgentSession) error
    // 取消 Session
    Cancel(sessionID string) error
    // 查询 Session 状态
    Status(sessionID string) (*SessionStatus, error)
    // 获取当前资源使用情况
    Metrics() *SchedulerMetrics
}
```

配置参数：
```yaml
scheduler:
  max_concurrent_sessions: 10    # 最大并发 Session 数
  session_timeout: 10m           # Session 超时
  worker_pool_size: 20           # Goroutine 池大小
  queue_size: 100                # 待处理队列大小
```

---

### 3.13 AgentConfig（运行时配置）

每个 Session 可携带独立配置，覆盖全局默认值：

```go
type AgentConfig struct {
    // 执行限制
    MaxSteps       int           `json:"max_steps" default:"30"`
    MaxRuntime     time.Duration `json:"max_runtime" default:"10m"`
    MaxLLMCalls    int           `json:"max_llm_calls" default:"30"`
    MaxToolCalls   int           `json:"max_tool_calls" default:"20"`
    MaxReplanCount int           `json:"max_replan_count" default:"5"`
    MaxTokenBudget int           `json:"max_token_budget" default:"200000"`

    // 模型选择
    PlannerModel   string `json:"planner_model" default:"gpt-4o"`
    ExecutorModel  string `json:"executor_model" default:"gpt-4o"`
    SummaryModel   string `json:"summary_model" default:"gpt-4o-mini"`
    ReflectModel   string `json:"reflect_model" default:"gpt-4o-mini"`

    // 工具权限（白名单，空表示使用全局默认）
    AllowedTools   []string `json:"allowed_tools"`
    EnableBrowser  bool     `json:"enable_browser" default:"false"`
    EnableSandbox  bool     `json:"enable_sandbox" default:"true"`

    // 用户审批（WAIT_USER_INPUT 状态触发条件）
    RequireApprovalFor []string `json:"require_approval_for"` // e.g. ["shell.exec", "file.write"]

    // Scratchpad 配置
    ScratchpadMaxTokens int `json:"scratchpad_max_tokens" default:"20000"`
    ReActMaxTurns       int `json:"react_max_turns" default:"10"` // 单 Step 内最大 ReAct 轮数
}
```

### 3.14 Tool Registry（工具注册中心）

**职责**：统一管理所有工具的注册、发现和权限控制。

```go
type ToolRegistry interface {
    // 注册工具
    Register(tool Tool) error
    // 注销工具
    Unregister(name string) error
    // 获取工具
    Get(name string) (Tool, error)
    // 列出所有工具（含 schema，用于 function calling）
    List() []Tool
    // 按权限过滤
    ListAllowed(allowedNames []string) []Tool
    // 生成 Function Calling Schema 列表
    Schemas(allowedNames []string) []ToolSchema
}
```

工具注册在 `wire.go` 启动时完成：

```go
// 启动时注册所有工具
registry.Register(shell.NewExecTool(sandboxClient))
registry.Register(file.NewReadTool())
registry.Register(file.NewWriteTool())
registry.Register(file.NewListTool())
registry.Register(file.NewSearchTool())
registry.Register(file.NewPatchTool())
registry.Register(browser.NewOpenTool(playwright))
registry.Register(search.NewTavilyTool(tavilyAPIKey))
registry.Register(rag.NewSearchTool(memoryMgr))
registry.Register(git.NewCloneTool())
registry.Register(http.NewRequestTool())

// MCP 动态注册（运行时）
for _, server := range config.MCPServers {
    tools, _ := mcpAdapter.LoadTools(server.URL)
    for _, t := range tools {
        registry.Register(t)
    }
}
```

### 3.15 错误码规范

```go
// Agent 系统统一错误码
const (
    // 系统错误 (1xxx)
    ErrInternal        = "AGT-1001" // 内部错误
    ErrTimeout         = "AGT-1002" // 执行超时
    ErrLoopDetected    = "AGT-1003" // 死循环检测

    // Planner 错误 (2xxx)
    ErrPlanFailed      = "AGT-2001" // 规划失败（LLM 返回无效 JSON）
    ErrPlanTooLarge    = "AGT-2002" // 步骤数超限
    ErrReplanExhausted = "AGT-2003" // 重规划次数耗尽

    // Tool 错误 (3xxx)
    ErrToolNotFound    = "AGT-3001" // 工具不存在
    ErrToolForbidden   = "AGT-3002" // 工具无权限
    ErrToolTimeout     = "AGT-3003" // 工具执行超时
    ErrSandboxFailed   = "AGT-3004" // Sandbox 启动失败
    ErrToolExhausted   = "AGT-3005" // 工具调用次数耗尽

    // LLM 错误 (4xxx)
    ErrLLMTimeout      = "AGT-4001" // LLM 调用超时
    ErrLLMRateLimit    = "AGT-4002" // LLM 限流
    ErrLLMBudget       = "AGT-4003" // Token 预算耗尽
    ErrLLMCallsLimit   = "AGT-4004" // LLM 调用次数耗尽

    // Memory 错误 (5xxx)
    ErrMemoryStore     = "AGT-5001" // 记忆存储失败
    ErrMemorySearch    = "AGT-5002" // 记忆检索失败

    // Session 错误 (6xxx)
    ErrSessionNotFound = "AGT-6001" // Session 不存在
    ErrSessionFull     = "AGT-6002" // 并发 Session 超限
    ErrSessionCancelled= "AGT-6003" // Session 已取消
)

// 统一 API 错误响应
type APIError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Detail  any    `json:"detail,omitempty"`
}
```

---

## 4. 数据结构设计

### 4.1 AgentSession（核心运行时对象）

```go
type AgentSession struct {
    ID          string            `json:"id"`
    Goal        string            `json:"goal"`
    State       AgentState        `json:"state"`
    Plan        *Plan             `json:"plan"`
    WorkingMem  *WorkingMemory    `json:"working_memory"`
    Metrics     *SessionMetrics   `json:"metrics"`
    Config      *AgentConfig      `json:"config"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    FinishedAt  *time.Time        `json:"finished_at,omitempty"`

    // 运行时控制（不序列化）
    cancel    context.CancelFunc
    eventChan chan Event
}

type WorkingMemory struct {
    Goal           string
    CurrentPlan    *Plan
    RecentMessages []Message
    RecentToolResults []ToolMemory
    Scratchpad     string         // ReAct 推理链
    TokensUsed     int
}

type SessionMetrics struct {
    StepCount      int
    LLMCallCount   int
    ToolCallCount  int
    ReplanCount    int
    TokensConsumed int
    TotalCost      float64
    StartTime      time.Time
}
```

### 4.2 ToolMemory（工具记忆结构化存储）

```go
type ToolMemory struct {
    ID        string         `json:"id" db:"id"`
    ToolName  string         `json:"tool_name" db:"tool_name"`
    TaskID    string         `json:"task_id" db:"task_id"`
    StepID    int            `json:"step_id" db:"step_id"`
    RawOutput string         `json:"raw_output" db:"raw_output"`      // 原始输出（可截断）
    Summary   string         `json:"summary" db:"summary"`            // 摘要
    KeyPoints []string       `json:"key_points" db:"key_points"`     // 要点
    Entities  []Entity       `json:"entities" db:"entities"`         // 实体
    Numbers   []string       `json:"numbers" db:"numbers"`           // 数字/指标
    Embedding []float32      `json:"embedding" db:"embedding"`       // 向量（Qdrant）
    CreatedAt time.Time      `json:"created_at" db:"created_at"`
}

type Entity struct {
    Name string `json:"name"`
    Type string `json:"type"` // file/function/error/company/etc.
}
```

### 4.3 Episode（情节记忆）

```go
type Episode struct {
    ID        string    `json:"id" db:"id"`
    Goal      string    `json:"goal" db:"goal"`
    Plan      string    `json:"plan" db:"plan"`           // JSON
    Result    string    `json:"result" db:"result"`
    Success   bool      `json:"success" db:"success"`
    Summary   string    `json:"summary" db:"summary"`
    Embedding []float32 `json:"embedding" db:"embedding"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

---

## 5. 接口规范

所有核心模块均面向接口编程（依赖注入），接口定义汇总：

```go
// 六大核心接口
type (
    Planner        interface { CreatePlan(...); Replan(...); ValidatePlan(...) }
    Executor       interface { ExecuteStep(...); ParseToolCall(...) }
    Tool           interface { Name(); Description(); Schema(); Execute(...); Validate(...) }
    MemoryManager  interface { SaveToolMemory(...); SearchToolMemory(...); ... }
    ContextBuilder interface { Build(...) (*Prompt, error) }
    LLMClient      interface { ChatComplete(...); ChatStream(...); ChatWithTools(...); Embed(...) }
)

// 基础设施接口
type (
    EventBus   interface { Emit(...); Subscribe(...); SubscribeSession(...) }
    FSMEngine  interface { Transition(...); RegisterHandler(...); CurrentState(...) }
    Scheduler  interface { Submit(...); Cancel(...); Status(...) }
)
```

---

## 6. 技术栈选型

| 层次 | 技术 | 版本 | 用途 |
|------|------|------|------|
| **Web 框架** | Gin | v1.9+ | HTTP Server, SSE |
| **LLM（云）** | OpenAI SDK / Anthropic SDK | latest | 主力模型 |
| **LLM（本地）** | Ollama | latest | 本地模型 |
| **Embedding** | bge-small-zh / bge-small-en | 本地 | 向量化 |
| **Reranker** | bge-reranker-base | 本地 | 精排 |
| **向量数据库** | Qdrant | v1.9+ | 向量存储检索 |
| **关系数据库** | PostgreSQL | v16 | Session/Memory 持久化 |
| **缓存** | Redis | v7 | Session/对话缓存 |
| **浏览器** | Playwright (go-playwright) | latest | 浏览器工具 |
| **文档解析** | unstructured-go / pdfcpu | - | RAG 文档处理 |
| **配置** | Viper | v1.18+ | 配置管理 |
| **日志** | Zap | v1.27+ | 结构化日志 |
| **Tracing** | OpenTelemetry | latest | 分布式追踪 |
| **指标** | Prometheus client-go | latest | 性能指标 |
| **迁移** | golang-migrate | latest | DB Schema 迁移 |
| **依赖注入** | Wire | latest | DI 代码生成 |
| **测试** | testify + mockery | latest | 单测/Mock |
| **容器** | Docker SDK | latest | Sandbox 管理 |
| **ORM** | sqlx | latest | SQL 查询封装 |

---

## 7. 工程目录结构

```
goagent/
│
├── cmd/
│   └── server/
│       └── main.go              # 程序入口，Wire 注入
│
├── internal/
│   ├── api/                     # Web API 层
│   │   ├── handler/
│   │   │   ├── agent.go         # Agent 相关 Handler
│   │   │   ├── memory.go        # Memory 相关 Handler
│   │   │   └── tool.go          # Tool 调试 Handler
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── ratelimit.go
│   │   │   ├── logger.go
│   │   │   └── recovery.go
│   │   ├── sse/
│   │   │   └── streamer.go      # SSE 推送管理
│   │   └── router.go
│   │
│   ├── agent/                   # Agent Runtime 核心
│   │   ├── session.go           # AgentSession 定义
│   │   ├── runner.go            # Session 运行主循环
│   │   └── config.go            # Agent 配置（Reflection 参数等）
│   │
│   ├── fsm/                     # FSM 状态机引擎
│   │   ├── engine.go            # FSM 核心逻辑
│   │   ├── state.go             # 状态/事件定义
│   │   ├── transition.go        # 状态转移表
│   │   └── handlers/
│   │       ├── planning.go      # PLANNING 状态处理器
│   │       ├── executing.go     # EXECUTING 状态处理器
│   │       ├── wait_tool.go     # WAIT_TOOL 状态处理器
│   │       ├── processing.go    # PROCESS_RESULT 状态处理器
│   │       ├── reflecting.go    # REFLECTING 状态处理器
│   │       └── replanning.go    # REPLANNING 状态处理器
│   │
│   ├── eventbus/                # Event Bus
│   │   ├── bus.go               # 核心实现（优先队列）
│   │   ├── event.go             # Event 结构定义
│   │   ├── dedup.go             # 去重逻辑
│   │   ├── ratelimit.go         # 事件速率限制
│   │   ├── ttl.go               # TTL 管理
│   │   ├── debounce.go          # 防抖/节流
│   │   └── loop_detector.go     # 死循环检测
│   │
│   ├── planner/                 # 规划器
│   │   ├── planner.go           # Planner 接口实现
│   │   ├── prompt.go            # Prompt 模板
│   │   └── validator.go         # Plan 校验
│   │
│   ├── executor/                # 执行器
│   │   ├── executor.go
│   │   └── react.go             # ReAct 推理解析
│   │
│   ├── pipeline/                # Pipeline 系统
│   │   ├── pipeline.go          # Pipeline 引擎（中间件链）
│   │   ├── tool_pipeline.go     # Tool Result Pipeline 组装
│   │   ├── context_pipeline.go  # Context Builder Pipeline 组装
│   │   ├── reflection_pipeline.go
│   │   └── middleware/
│   │       ├── filter.go        # 内容过滤
│   │       ├── extractor.go     # 信息提取
│   │       ├── summarizer.go    # 摘要生成
│   │       ├── embedder.go      # 向量化
│   │       ├── storage.go       # 存储
│   │       ├── retriever.go     # 向量检索
│   │       ├── reranker.go      # 重排
│   │       ├── compressor.go    # Token 压缩
│   │       ├── token_budget.go  # Token 预算控制
│   │       └── prompt_builder.go# Prompt 拼装
│   │
│   ├── tools/                   # Tool 系统
│   │   ├── registry.go          # Tool 注册中心
│   │   ├── mcp_adapter.go       # MCP 协议适配器
│   │   ├── shell/
│   │   │   ├── shell.go         # shell.exec 实现
│   │   │   └── sandbox.go       # Docker sandbox 管理
│   │   ├── file/
│   │   │   ├── read.go
│   │   │   ├── write.go
│   │   │   ├── list.go
│   │   │   ├── search.go
│   │   │   └── patch.go
│   │   ├── browser/
│   │   │   ├── open.go
│   │   │   ├── click.go
│   │   │   └── find.go
│   │   ├── search/
│   │   │   ├── tavily.go
│   │   │   └── serper.go
│   │   ├── git/
│   │   │   ├── clone.go
│   │   │   └── commit.go
│   │   ├── http/
│   │   │   └── request.go
│   │   └── rag/
│   │       └── search.go
│   │
│   ├── memory/                  # Memory 系统
│   │   ├── manager.go           # MemoryManager 接口实现
│   │   ├── tool_memory.go       # Tool Memory CRUD
│   │   ├── episodic.go          # Episodic Memory CRUD
│   │   ├── knowledge.go         # Knowledge/RAG 操作
│   │   └── conversation.go      # 对话历史（Redis）
│   │
│   ├── llm/                     # LLM Adapter 层
│   │   ├── client.go            # LLMClient 接口
│   │   ├── openai.go            # OpenAI 实现
│   │   ├── anthropic.go         # Anthropic 实现
│   │   ├── ollama.go            # Ollama 实现
│   │   └── prompt/
│   │       └── templates.go     # Prompt 模板管理
│   │
│   ├── scheduler/               # 调度器
│   │   ├── scheduler.go
│   │   └── worker_pool.go
│   │
│   └── infra/                   # 基础设施
│       ├── db/
│       │   ├── postgres.go      # PostgreSQL 连接
│       │   └── migrations/      # DB Migration 文件
│       ├── redis/
│       │   └── client.go
│       ├── qdrant/
│       │   └── client.go        # Qdrant 向量数据库
│       ├── docker/
│       │   └── client.go        # Docker SDK 封装
│       └── config/
│           └── config.go        # Viper 配置加载
│
├── pkg/                         # 公共工具包
│   ├── logger/                  # Zap 封装
│   ├── tracer/                  # OpenTelemetry
│   ├── token/                   # Token 计算工具
│   ├── hash/                    # Hash 工具（事件去重）
│   └── retry/                   # 重试工具
│
├── configs/
│   ├── config.yaml              # 默认配置
│   └── config.prod.yaml         # 生产配置
│
├── scripts/
│   ├── migrate.sh
│   └── build_sandbox.sh         # 构建 sandbox Docker 镜像
│
├── deployments/
│   ├── docker-compose.yml       # 本地开发环境
│   └── k8s/                     # Kubernetes 配置
│
├── docs/
│   └── api.yaml                 # OpenAPI 3.0 规范
│
├── wire.go                      # Wire DI 定义
├── wire_gen.go                  # Wire 生成代码
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 8. 数据库 Schema

### 8.1 PostgreSQL

```sql
-- Agent Session 表
CREATE TABLE agent_sessions (
    id            TEXT PRIMARY KEY,
    goal          TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'IDLE',
    plan          JSONB,
    metrics       JSONB,
    config        JSONB,
    result        TEXT,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ
);
CREATE INDEX idx_agent_sessions_state ON agent_sessions(state);
CREATE INDEX idx_agent_sessions_created_at ON agent_sessions(created_at);

-- Tool Memory 表
CREATE TABLE tool_memories (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    step_id       INT NOT NULL,
    tool_name     TEXT NOT NULL,
    raw_output    TEXT,
    summary       TEXT,
    key_points    JSONB,              -- []string
    entities      JSONB,              -- []Entity
    numbers       JSONB,              -- []string
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tool_memories_session ON tool_memories(session_id);
-- Embedding 向量存储在 Qdrant，通过 id 关联

-- Episodic Memory 表
CREATE TABLE episodic_memories (
    id            TEXT PRIMARY KEY,
    goal          TEXT NOT NULL,
    plan          JSONB,
    result        TEXT,
    success       BOOLEAN NOT NULL DEFAULT FALSE,
    summary       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Embedding 存储在 Qdrant

-- Knowledge Documents 表（RAG 文档元数据）
CREATE TABLE knowledge_documents (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    source        TEXT,              -- 来源（文件路径/URL）
    doc_type      TEXT,              -- pdf/md/txt/url
    chunk_count   INT DEFAULT 0,
    indexed_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Event Log 表（审计/调试）
CREATE TABLE event_logs (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    payload       JSONB,
    state_before  TEXT,
    state_after   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_event_logs_session ON event_logs(session_id);
CREATE INDEX idx_event_logs_created ON event_logs(created_at);
-- 保留策略：超过 30 天自动清理
```

### 8.2 Qdrant Collections

```json
// tool-memory collection
{
  "name": "tool_memory",
  "vectors": { "size": 512, "distance": "Cosine" },
  "payload_schema": {
    "memory_id": "keyword",
    "session_id": "keyword",
    "tool_name": "keyword",
    "created_at": "datetime"
  }
}

// episodic-memory collection
{
  "name": "episodic_memory",
  "vectors": { "size": 512, "distance": "Cosine" },
  "payload_schema": {
    "episode_id": "keyword",
    "goal": "text",
    "success": "bool"
  }
}

// knowledge collection
{
  "name": "knowledge",
  "vectors": { "size": 512, "distance": "Cosine" },
  "payload_schema": {
    "doc_id": "keyword",
    "chunk_index": "integer",
    "source": "keyword",
    "content": "text"
  }
}
```

### 8.3 Redis Key 设计

```
session:{session_id}:messages     → List, 对话消息（TTL 24h）
session:{session_id}:state        → String, 当前状态（TTL 1h）
session:{session_id}:events       → List, 事件历史（TTL 1h）
ratelimit:event:{type}:{session}  → Counter，事件速率计数（TTL 1min）
dedup:event:{hash}                → String，去重缓存（TTL 30s）
```

---

## 9. API 接口文档

### 9.1 启动 Agent 任务

```
POST /api/v1/agent/run
Content-Type: application/json

Request:
{
  "goal": "帮我修复 main.go 中的 nil pointer 错误",
  "config": {
    "max_steps": 20,
    "max_runtime": "5m",
    "llm_model": "gpt-4o",
    "tools": ["shell.exec", "file.read", "file.write"],
    "enable_browser": false
  }
}

Response 200:
{
  "session_id": "sess_abc123",
  "state": "IDLE",
  "stream_url": "/api/v1/agent/sess_abc123/stream",
  "created_at": "2024-01-01T00:00:00Z"
}
```

### 9.2 SSE 流式接收

```
GET /api/v1/agent/{session_id}/stream
Accept: text/event-stream

Stream Events:
data: {"type":"state_change","from":"IDLE","to":"PLANNING","ts":"..."}
data: {"type":"thought","content":"分析任务...","step":0}
data: {"type":"plan_created","steps":[{"id":1,"name":"读取文件","tool":"file.read"},...],"ts":"..."}
data: {"type":"step_start","step":1,"name":"读取文件","ts":"..."}
data: {"type":"tool_call","tool":"file.read","input":{"path":"main.go"},"ts":"..."}
data: {"type":"tool_result","success":true,"summary":"文件包含nil pointer at line 42","ts":"..."}
data: {"type":"step_done","step":1,"ts":"..."}
data: {"type":"step_start","step":2,"name":"修复代码","ts":"..."}
data: {"type":"done","result":"已成功修复 main.go 第42行的nil pointer错误","ts":"..."}
```

### 9.3 查询 Session 状态

```
GET /api/v1/agent/{session_id}/status

Response:
{
  "session_id": "sess_abc123",
  "state": "EXECUTING",
  "goal": "...",
  "plan": { "steps": [...], "current_step": 2 },
  "metrics": {
    "step_count": 2,
    "llm_call_count": 3,
    "tool_call_count": 2,
    "tokens_consumed": 15420,
    "elapsed": "45s"
  }
}
```

### 9.4 取消 Agent 任务

```
DELETE /api/v1/agent/{session_id}

Response 200: {"status": "cancelled"}
```

### 9.5 检索记忆

```
GET /api/v1/memory/search?q=nil+pointer&session_id=sess_abc123&top_k=5

Response:
{
  "results": [
    {
      "id": "mem_123",
      "tool_name": "file.read",
      "summary": "...",
      "key_points": ["..."],
      "score": 0.92
    }
  ]
}
```

### 9.6 用户审批（WAIT_USER_INPUT）

当 Agent 执行高风险工具前，触发 `WAIT_USER_INPUT` 状态，暂停执行等待用户确认。

```
# SSE 推送审批请求
data: {
  "type": "approval_required",
  "session_id": "sess_abc123",
  "tool": "shell.exec",
  "input": {"cmd": "rm -rf /workspace/old"},
  "reason": "删除文件操作需要用户确认",
  "timeout": 300
}

# 用户审批
POST /api/v1/agent/{session_id}/approve
{
  "approved": true,       // true=通过, false=拒绝
  "comment": "已确认安全"  // 可选
}

Response 200: {"status": "resumed"}

# 拒绝后 Agent 会触发 reflect，尝试寻找替代方案
```



```yaml
# config.yaml

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 60s

agent:
  max_concurrent_sessions: 10
  default_max_steps: 30
  default_max_runtime: 10m
  default_max_llm_calls: 30
  default_max_tool_calls: 20
  default_max_replan: 5
  default_max_tokens: 200000
  worker_pool_size: 20

llm:
  default_model: "gpt-4o"
  summarizer_model: "gpt-4o-mini"    # 用于 Tool Summarizer
  reflection_model: "gpt-4o-mini"    # 用于 Reflection
  openai:
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    timeout: 120s
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
  ollama:
    base_url: "http://localhost:11434"
    model: "qwen2.5:32b"

embedding:
  model: "bge-small-en-v1.5"
  endpoint: "http://localhost:8001"   # 本地 embedding 服务
  dimensions: 512
  batch_size: 32

reranker:
  model: "bge-reranker-base"
  endpoint: "http://localhost:8002"
  top_k_input: 20
  top_k_output: 5

memory:
  tool_memory_max_raw_size: 50000    # 原始输出最大字节数
  conversation_ttl: 24h
  session_cache_ttl: 1h

event_bus:
  queue_capacity: 1000
  max_replan_per_min: 3
  max_tool_retry_per_min: 2
  max_llm_calls_per_min: 20
  debounce_memory_update: 2s
  debounce_context_build: 5s
  dedup_window: 30s
  loop_detection_threshold: 3        # 相同路径重复3次触发

tools:
  shell:
    sandbox_image: "goagent-sandbox:latest"
    cpu_limit: "1.0"
    memory_limit: "512m"
    command_timeout: 60s
    session_timeout: 10m
    workspace_root: "/tmp/goagent/workspaces"
  browser:
    headless: true
    timeout: 30s
    user_agent: "GoAgent/1.0"
  search:
    provider: "tavily"               # tavily / serper
    tavily_api_key: "${TAVILY_API_KEY}"
    max_results: 5

database:
  postgres:
    dsn: "${POSTGRES_DSN}"
    max_open_conns: 20
    max_idle_conns: 5
    conn_max_lifetime: 5m
  redis:
    addr: "${REDIS_ADDR}"
    password: "${REDIS_PASSWORD}"
    db: 0
  qdrant:
    addr: "${QDRANT_ADDR}"
    port: 6334

auth:
  enabled: true
  jwt_secret: "${JWT_SECRET}"
  token_ttl: 24h

log:
  level: "info"                      # debug / info / warn / error
  format: "json"
  output: "stdout"

telemetry:
  enabled: true
  otel_endpoint: "http://jaeger:4317"
  prometheus_port: 9090
```

---

## 11. 安全与限流

### 11.1 API 限流

```
全局：1000 req/min（IP 级别）
单 Session：100 req/min
SSE 连接数：每 IP 最多 5 个
```

### 11.2 Agent 执行安全

| 风险 | 控制措施 |
|------|---------|
| Shell 代码注入 | Docker Sandbox 隔离，只执行白名单命令 |
| 文件越权访问 | 限制文件操作路径为 /workspace |
| 网络滥用 | Sandbox 网络可配置为隔离 |
| LLM Prompt 注入 | 系统 Prompt 优先级最高，用户输入 sanitize |
| 成本超支 | MaxToken / MaxLLMCall 硬限制 |
| 死循环 | FSM 死循环检测 + Max Step |
| 资源耗尽 | Scheduler 并发限制 + Docker 资源限制 |

### 11.3 Tool 权限分级

```
Level 1（低风险）：file.read / file.list / search.web / rag.search
Level 2（中风险）：file.write / file.patch / browser.* / http.request / git.*
Level 3（高风险）：shell.exec / python.exec（强制 Sandbox）
```

---

## 12. 可观测性

### 12.1 结构化日志（Zap）

每个关键操作记录：

```json
{
  "level": "info",
  "ts": "2024-01-01T00:00:00Z",
  "logger": "fsm",
  "msg": "state_transition",
  "session_id": "sess_abc123",
  "from": "EXECUTING",
  "to": "WAIT_TOOL",
  "event": "tool_call",
  "step_id": 2,
  "duration_ms": 12
}
```

### 12.2 Prometheus 指标

```
# Session 指标
goagent_sessions_total{state="done|error|cancelled"}
goagent_sessions_active
goagent_session_duration_seconds{quantile="0.5|0.9|0.99"}

# FSM 指标
goagent_state_transitions_total{from, to, event}
goagent_state_duration_seconds{state}

# LLM 指标
goagent_llm_calls_total{model, status}
goagent_llm_tokens_total{model, type="prompt|completion"}
goagent_llm_latency_seconds{model, quantile}

# Tool 指标
goagent_tool_calls_total{tool, status}
goagent_tool_latency_seconds{tool, quantile}

# Memory 指标
goagent_memory_ops_total{op="save|search", type}
goagent_memory_search_latency_seconds{type}

# EventBus 指标
goagent_events_total{type, action="processed|dropped|deduped"}
goagent_event_queue_depth
```

### 12.3 Tracing（OpenTelemetry）

每个 Agent Session 创建一个 Root Span，关键操作创建子 Span：
- `agent.session.run`
- `planner.create_plan`
- `executor.execute_step`
- `tool.{tool_name}.execute`
- `pipeline.tool_result`
- `pipeline.context_build`
- `llm.{model}.chat`
- `memory.search`

---

## 13. 部署架构

### 13.1 本地开发（docker-compose）

```yaml
version: "3.9"
services:
  goagent:
    build: .
    ports: ["8080:8080"]
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - POSTGRES_DSN=postgres://goagent:goagent@postgres:5432/goagent
      - REDIS_ADDR=redis:6379
      - QDRANT_ADDR=qdrant:6334
    depends_on: [postgres, redis, qdrant]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock  # 用于 sandbox

  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: goagent
      POSTGRES_USER: goagent
      POSTGRES_PASSWORD: goagent
    volumes: [pg_data:/var/lib/postgresql/data]

  redis:
    image: redis:7-alpine
    volumes: [redis_data:/data]

  qdrant:
    image: qdrant/qdrant:v1.9.0
    ports: ["6333:6333", "6334:6334"]
    volumes: [qdrant_data:/qdrant/storage]

  # 可选：本地 Ollama
  ollama:
    image: ollama/ollama:latest
    ports: ["11434:11434"]
    volumes: [ollama_data:/root/.ollama]

volumes:
  pg_data:
  redis_data:
  qdrant_data:
  ollama_data:
```

### 13.2 生产部署（Kubernetes 要点）

```yaml
# goagent deployment 关键配置

resources:
  requests: { cpu: "500m", memory: "512Mi" }
  limits:   { cpu: "2000m", memory: "2Gi" }

# 关键：需要 Docker-in-Docker 或 Docker Socket 权限（Sandbox）
volumes:
  - name: docker-sock
    hostPath: { path: /var/run/docker.sock }

# HPA 配置
autoscaling:
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80
```

---

## 14. 开发实施路线

### Phase 1 — 核心骨架（3-4 周）

目标：单步工具调用可用

```
✅ 项目骨架 + DI（Wire）
✅ Gin Web 服务 + SSE
✅ Event Bus（基础版：无去重/限流）
✅ FSM 状态机（核心状态）
✅ Planner（LLM 调用，JSON 解析）
✅ Executor（ReAct 单步）
✅ Tool System（shell.exec + file.read/write）
✅ LLM Adapter（OpenAI）
✅ AgentSession 管理
✅ PostgreSQL 基础表
```

### Phase 2 — Memory 系统（2-3 周）

目标：Agent 有记忆，不忘事

```
✅ Tool Result Pipeline（Filter→Extract→Summarize→Store）
✅ Embedding 集成（bge-small 本地）
✅ Qdrant 集成
✅ MemoryManager 完整实现
✅ Context Builder Pipeline（基础版）
✅ Redis 对话历史
```

### Phase 3 — 智能反思（2 周）

目标：Agent 会自我纠错

```
✅ Reflection Pipeline
✅ Event Bus 防爆机制（TTL/限流/去重/防抖）
✅ 死循环检测
✅ Replan 机制
✅ Reranker 集成
✅ Token Budget 控制
```

### Phase 4 — 工具生态（2-3 周）

目标：工具丰富可用

```
✅ Docker Sandbox 完整实现
✅ Browser 工具（Playwright）
✅ Git 工具
✅ Search 工具（Tavily）
✅ RAG 工具
✅ MCP 协议适配器
✅ HTTP 工具
```

### Phase 5 — 生产就绪（2 周）

目标：可上生产

```
✅ 完整 Prometheus 指标
✅ OpenTelemetry Tracing
✅ Auth 中间件（JWT）
✅ 完整限流体系
✅ DB Migration 完整
✅ Sandbox 安全加固
✅ 压力测试
✅ 部署文档
```

---

## 15. 附录 A：关键工程文件规范

### A.1 go.mod 核心依赖

```
module github.com/coohu/goagent

go 1.22

require (
    github.com/gin-gonic/gin v1.9.1
    github.com/sashabaranov/go-openai v1.20.0          // OpenAI SDK
    go.uber.org/zap v1.27.0                             // 日志
    github.com/spf13/viper v1.18.2                      // 配置
    github.com/jmoiron/sqlx v1.3.5                      // SQL 封装
    github.com/lib/pq v1.10.9                           // PostgreSQL 驱动
    github.com/redis/go-redis/v9 v9.5.1                 // Redis 客户端
    github.com/qdrant/go-client v1.9.0                  // Qdrant 客户端
    github.com/docker/docker v26.1.0+incompatible       // Docker SDK
    github.com/playwright-community/playwright-go v0.4201.1 // Playwright
    github.com/golang-migrate/migrate/v4 v4.17.0        // DB Migration
    go.opentelemetry.io/otel v1.24.0                    // Tracing
    github.com/prometheus/client_golang v1.19.0         // Metrics
    github.com/google/wire v0.6.0                       // DI
    github.com/stretchr/testify v1.9.0                  // 测试断言
    github.com/stretchr/mockery/v2 v2.42.0              // Mock 生成
    github.com/google/uuid v1.6.0                       // UUID
    golang.org/x/time v0.5.0                            // Rate Limiter
    github.com/gorilla/websocket v1.5.1                 // WebSocket
    github.com/tiktoken-go/tokenizer v0.1.7             // Token 计数
    github.com/cenkalti/backoff/v4 v4.3.0               // 重试
)
```

### A.2 Makefile 规范

```makefile
.PHONY: build run test lint migrate wire docker-build

# 构建
build:
	go build -o bin/goagent ./cmd/server

# 本地运行（依赖 docker-compose 启动基础设施）
run:
	go run ./cmd/server

# 测试
test:
	go test ./... -v -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# 集成测试
test-integration:
	go test ./... -v -tags=integration

# 代码检查
lint:
	golangci-lint run ./...

# Wire DI 代码生成
wire:
	wire ./...

# Mock 生成（mockery）
mock:
	go generate ./...

# DB 迁移（up）
migrate-up:
	migrate -path internal/infra/db/migrations -database "${POSTGRES_DSN}" up

# DB 迁移（down）
migrate-down:
	migrate -path internal/infra/db/migrations -database "${POSTGRES_DSN}" down 1

# 构建 Sandbox Docker 镜像
build-sandbox:
	docker build -f deployments/sandbox.Dockerfile -t goagent-sandbox:latest .

# 启动本地开发环境
dev:
	docker-compose -f deployments/docker-compose.yml up -d
	$(MAKE) migrate-up
	$(MAKE) run

# 构建服务镜像
docker-build:
	docker build -t goagent:latest .

# 代码格式化
fmt:
	gofmt -w .
	goimports -w .

# 生成 OpenAPI 文档
swagger:
	swag init -g cmd/server/main.go -o docs
```

### A.3 Wire DI 设计

`wire.go`（手写 Provider 集合）：

```go
//go:build wireinject

package main

import (
    "github.com/google/wire"
    "github.com/coohu/goagent/internal/..."
)

// ProviderSet 各模块 Provider 集合
var InfraSet = wire.NewSet(
    infra.NewPostgresDB,
    infra.NewRedisClient,
    infra.NewQdrantClient,
    infra.NewDockerClient,
)

var LLMSet = wire.NewSet(
    llm.NewOpenAIClient,
    llm.NewAnthropicClient,
    llm.NewOllamaClient,
    llm.NewRouter,          // 按场景路由到不同模型
)

var ToolSet = wire.NewSet(
    tools.NewRegistry,
    tools.NewSandboxClient,
    // 各工具 Provider...
)

var MemorySet = wire.NewSet(
    memory.NewManager,
)

var AgentSet = wire.NewSet(
    planner.New,
    executor.New,
    eventbus.New,
    fsm.NewEngine,
    pipeline.NewToolPipeline,
    pipeline.NewContextPipeline,
    pipeline.NewReflectionPipeline,
    contextbuilder.New,
    scheduler.New,
)

var APISet = wire.NewSet(
    api.NewRouter,
    api.NewAgentHandler,
    api.NewMemoryHandler,
    api.NewSSEStreamer,
)

func InitializeApp(cfg *config.Config) (*App, error) {
    wire.Build(
        InfraSet,
        LLMSet,
        ToolSet,
        MemorySet,
        AgentSet,
        APISet,
        NewApp,
    )
    return nil, nil
}
```

### A.4 测试策略

**测试分层**：

| 层次 | 类型 | 覆盖目标 | 工具 |
|------|------|---------|------|
| 单元测试 | Unit | 每个模块独立逻辑 | testify + mockery |
| 集成测试 | Integration | 模块间交互（含 DB） | testcontainers-go |
| 端到端测试 | E2E | 完整 Agent 任务流程 | httptest |
| 性能测试 | Benchmark | 关键路径性能 | go benchmark |

**Mock 生成（在接口文件中添加 `//go:generate`）**：

```go
// internal/llm/client.go
//go:generate mockery --name=LLMClient --output=./mocks

// internal/memory/manager.go
//go:generate mockery --name=MemoryManager --output=./mocks

// internal/tools/tool.go
//go:generate mockery --name=Tool --output=./mocks
```

**关键测试用例列表**：

```
fsm/engine_test.go
  - TestStateTransition_PlanningToExecuting
  - TestStateTransition_LoopDetection
  - TestStateTransition_MaxStepExceeded
  - TestStateTransition_CancelFromAnyState

eventbus/bus_test.go
  - TestEventDeduplication
  - TestEventTTLExpiry
  - TestEventRateLimit
  - TestEventPriorityOrder
  - TestDebounce

pipeline/tool_pipeline_test.go
  - TestToolResultFilter
  - TestToolResultSummarize
  - TestTokenBudgetControl

planner/planner_test.go
  - TestCreatePlanParseJSON
  - TestReplanPreservesCompletedSteps

memory/manager_test.go
  - TestSaveAndSearchToolMemory
  - TestContextBuilderTokenBudget
```

---

## 16. 附录：设计决策说明

### A. 为什么选 Event-driven FSM 而非简单 Loop

| 维度 | 简单 while Loop | Event-driven FSM |
|------|----------------|-----------------|
| 异步工具 | 阻塞 | 天然支持 |
| 状态可见性 | 无 | 完整 |
| 死循环控制 | 困难 | 内置 |
| 多 Agent | 很难 | 共享 EventBus |
| 调试 | 困难 | 事件日志完整 |
| 扩展 | 耦合 | 解耦 |

### B. 为什么 Tool 结果不直接注入 Prompt

直接注入会导致：Token 爆炸、推理能力下降、成本极高。  
正确路径：`Tool Output → 摘要/结构化 → Memory → 按需检索注入`

### C. 为什么不用 LangChain/LangGraph（Go 生态）

- Go 生态中无成熟 LangGraph 等价物
- 自研更易于控制 token 成本、sandbox 安全
- 本 PRD 设计已覆盖 LangGraph 全部核心概念（Graph FSM = Node + Edge）
- 业务定制化更灵活

### D. Sandbox 设计原则

- Agent 绝不直接执行 shell，所有 shell 在 Docker 容器内执行
- 每个 Session 独立容器，Session 结束后销毁
- 工作目录 /workspace 持久化到宿主机，Agent 重启后可恢复

### E. bge 系列模型选型原因

- bge-small 在 512 维度下性能优秀，本地运行无 API 成本
- bge-reranker 精排效果接近 cohere，完全开源免费
- 适合构建完全本地化的 Agent 系统（隐私敏感场景）

### E. 实施步骤

Step 1  项目骨架 + 核心类型定义（最稳定，所有人依赖它）
Step 2  配置加载（Viper，基本不变）
Step 3  LLM Adapter（纯 I/O，接口清晰，可立即测试）
Step 4  Tool System（接口固定，各工具独立可测）
Step 5  Event Bus（纯 Go channel + 优先队列，不依赖外部）
Step 6  FSM 引擎（状态机，逻辑封闭可单测）
Step 7  Planner（依赖 LLM，接口固定）
Step 8  Executor + ReAct（依赖 Planner + Tool）
Step 9  Pipeline 系统（数据流处理，组合以上模块）
Step 10 Memory + Context Builder（依赖 Pipeline + DB）
Step 11 Agent Runner（组装主循环）
Step 12 Web API + SSE（最外层，依赖一切）
Step 13 DB Schema + Migration（配合 Memory 层落地）
Step 14 Scheduler + docker-compose（部署层，最后调整）
---

*文档版本: v1.0 | 最后更新: 2024 | GoAgent PRD*
