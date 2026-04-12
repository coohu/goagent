package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coohu/goagent/internal/agent"
	"github.com/coohu/goagent/internal/api"
	"github.com/coohu/goagent/internal/api/handler"
	"github.com/coohu/goagent/internal/api/sse"
	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/eventbus"
	"github.com/coohu/goagent/internal/executor"
	"github.com/coohu/goagent/internal/fsm"
	"github.com/coohu/goagent/internal/llm"
	"github.com/coohu/goagent/internal/memory"
	"github.com/coohu/goagent/internal/planner"
	"github.com/coohu/goagent/internal/tools/builtin/file"
	fileshell "github.com/coohu/goagent/internal/tools/builtin/shell"
	"github.com/coohu/goagent/internal/tools/registry"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("Warning: 没有找到 .env 文件")
	}
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY not set")
	}

	workspaceRoot := envOr("WORKSPACE_ROOT", "/tmp/goagent/workspaces")
	baseUrl := envOr("BASE_URL", "https://openrouter.ai/api/v1")
	planModel := envOr("PLAN_MODEL", "gpt-4o")
	execModel := envOr("EXEC_MODEL", "gpt-4o")
	summarizeModel := envOr("SUMMARIZE_MODEL", "gpt-4o-mini")
	reflectModel := envOr("REFLECT_MODEL", "gpt-4o-mini")
	llmClients := map[string]core.LLMClient{
		planModel:      llm.NewOpenAIClient(apiKey, baseUrl, planModel),
		execModel:      llm.NewOpenAIClient(apiKey, baseUrl, execModel),
		summarizeModel: llm.NewOpenAIClient(apiKey, baseUrl, summarizeModel),
		reflectModel: llm.NewOpenAIClient(apiKey, baseUrl, reflectModel),
	}
	globalModels := core.SceneModels{
		Planning:  planModel,
		Execute:   execModel,
		Summarize: summarizeModel,
		Reflect:   reflectModel,
	}
	llmRouter := llm.NewRouter(llmClients, globalModels)

	reg := registry.New()
	reg.Register(file.NewReadTool())
	reg.Register(file.NewWriteTool())
	reg.Register(file.NewListTool())
	reg.Register(file.NewSearchTool())
	reg.Register(fileshell.NewExecTool(60*time.Second, workspaceRoot))

	mem := memory.NewInMemoryManager()
	bus := eventbus.New(eventbus.DefaultConfig())
	fsmEngine := fsm.NewEngine()

	pl := planner.New(llmRouter)
	ex := executor.New(llmRouter, reg)
	toolRunner := agent.NewDefaultToolRunner(reg)
	runner := agent.NewRunner(fsmEngine, bus, pl, ex, mem, toolRunner)

	sessionMgr := agent.NewSessionManager(10)
	hub := sse.NewHub()

	agentHandler := handler.NewAgentHandler(sessionMgr, runner, hub, llmRouter, apiKey, baseUrl)
	fileHandler := handler.NewFileHandler(sessionMgr, workspaceRoot)
	sysHandler := handler.NewSystemHandler(reg, llmRouter)

	router := api.NewRouter(agentHandler, fileHandler, sysHandler)

	srv := &http.Server{
		Addr:         ":" + envOr("PORT", "8080"),
		Handler:      router,
		ReadTimeout:  3000 * time.Second,
		WriteTimeout: 6000 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	<-quit
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = bus.Shutdown(ctx)
	return srv.Shutdown(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
