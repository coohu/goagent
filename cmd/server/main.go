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
	// fileshell "github.com/coohu/goagent/internal/tools/builtin/shell"
	"github.com/coohu/goagent/internal/tools/registry"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
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
	baseURL := os.Getenv("BASE_URL")
	planModel := os.Getenv("PLAN_MODEL")
	if planModel == "" {
		planModel = "gpt-4o"
	}
	execModel := os.Getenv("EXEC_MODEL")
	if execModel == "" {
		execModel = "gpt-4o"
	}
	summaryModel := os.Getenv("SUMMARY_MODEL")
	if summaryModel == "" {
		summaryModel = "gpt-4o-mini"
	}
	reflectModel := os.Getenv("REFLECT_MODEL")
	if reflectModel == "" {
		reflectModel = "gpt-4o-mini"
	}
	if baseURL == "" {
		return fmt.Errorf("BASE_URL not set")
	}
	llmClients := map[string]core.LLMClient{
		planModel:      llm.NewOpenAIClient(apiKey, baseURL, planModel),
		execModel:      llm.NewOpenAIClient(apiKey, baseURL, execModel),
		summaryModel:   llm.NewOpenAIClient(apiKey, baseURL, summaryModel),
		reflectModel:   llm.NewOpenAIClient(apiKey, baseURL, reflectModel),
	}
	scenes := map[llm.Scene]string{
		llm.ScenePlanning:  planModel,
		llm.SceneExecute:   execModel,
		llm.SceneSummarize: summaryModel,
		llm.SceneReflect:   reflectModel,
	}
	llmRouter := llm.NewRouter(llmClients, scenes)

	reg := registry.New()
	reg.Register(file.NewReadTool())
	reg.Register(file.NewWriteTool())
	reg.Register(file.NewListTool())
	// reg.Register(file.NewSearchTool())
	// reg.Register(fileshell.NewExecTool(60*time.Second, "/tmp/goagent"))

	mem := memory.NewInMemoryManager()
	bus := eventbus.New(eventbus.DefaultConfig())
	fsmEngine := fsm.NewEngine()

	pl := planner.New(llmRouter)
	ex := executor.New(llmRouter, reg)
	toolRunner := agent.NewDefaultToolRunner(reg)
	runner := agent.NewRunner(fsmEngine, bus, pl, ex, mem, toolRunner)

	sessionMgr := agent.NewSessionManager(10)
	hub := sse.NewHub()
	agentHandler := handler.NewAgentHandler(sessionMgr, runner, hub)
	router := api.NewRouter(agentHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
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
