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
	"github.com/coohu/goagent/internal/memory"
	"github.com/coohu/goagent/internal/planner"
	"github.com/coohu/goagent/internal/llm"
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
	workspaceRoot := envOr("WORKSPACE_ROOT", "/tmp/goagent/workspaces")
	provReg := llm.NewRegistry()

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		provReg.RegisterProvider(llm.OpenAIProvider(key))
		slog.Info("provider registered", "id", "openai")
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		provReg.RegisterProvider(llm.AnthropicProvider(key))
		slog.Info("provider registered", "id", "anthropic")
	}

	llmKey := envOr("LLM_API_KEY", os.Getenv("OPENAI_API_KEY"))
	llmBase := os.Getenv("LLM_BASE_URL")
	fallbackProvider := ""
	if llmKey != "" && llmBase != "" {
		openrouterCfg := llm.ProviderConfig{
			ID:          "openrouter",
			DisplayName: "OpenRouter / Custom",
			BaseURL:     llmBase,
			APIKey:      llmKey,
			Models:      []llm.ModelDef{}, // models resolved dynamically
		}
		provReg.RegisterProvider(openrouterCfg)
		fallbackProvider = "openrouter"
		slog.Info("provider registered", "id", "openrouter", "base_url", llmBase)
	}

	if ollamaModel := os.Getenv("OLLAMA_MODEL"); ollamaModel != "" {
		ollamaURL := envOr("OLLAMA_URL", "http://localhost:11434")
		provReg.RegisterProvider(llm.OllamaProvider(ollamaURL, ollamaModel))
		slog.Info("provider registered", "id", "ollama", "model", ollamaModel)
	}

	if len(provReg.KnownModels()) == 0 && fallbackProvider == "" {
		return fmt.Errorf("no LLM providers configured — set OPENAI_API_KEY, ANTHROPIC_API_KEY, or LLM_API_KEY+LLM_BASE_URL")
	}

	defaultModel := envOr("DEFAULT_MODEL", firstKnownModel(provReg, "gpt-4o"))
	miniModel := envOr("DEFAULT_MINI_MODEL", firstKnownModel(provReg, "gpt-4o-mini"))

	globalModels := core.SceneModels{
		Planning:  defaultModel,
		Execute:   defaultModel,
		Summarize: miniModel,
		Reflect:   miniModel,
	}
	slog.Info("default models", "planning", globalModels.Planning, "execute", globalModels.Execute,
		"summarize", globalModels.Summarize, "reflect", globalModels.Reflect)

	llmRouter := llm.NewRouter(provReg, globalModels, fallbackProvider)
	toolReg := registry.New(workspaceRoot)
	mem := memory.NewInMemoryManager()
	bus := eventbus.New(eventbus.DefaultConfig())
	fsmEngine := fsm.NewEngine()

	pl := planner.New(llmRouter)
	ex := executor.New(llmRouter, toolReg)
	toolRunner := agent.NewDefaultToolRunner(toolReg)
	runner := agent.NewRunner(fsmEngine, bus, pl, ex, mem, toolRunner)

	sessionMgr := agent.NewSessionManager(10)
	hub := sse.NewHub()

	agentHandler := handler.NewAgentHandler(sessionMgr, runner, hub, llmRouter, provReg)
	fileHandler := handler.NewFileHandler(sessionMgr, workspaceRoot)
	sysHandler := handler.NewSystemHandler(toolReg, llmRouter)
	r := api.NewRouter(agentHandler, fileHandler, sysHandler)

	srv := &http.Server{
		Addr:         ":" + envOr("PORT", "8080"),
		Handler:      r,
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstKnownModel(reg *llm.Registry, preferred string) string {
	for _, m := range reg.KnownModels() {
		if m.ID == preferred {
			return preferred
		}
	}
	models := reg.KnownModels()
	if len(models) > 0 {
		return models[0].ID
	}
	return preferred
}