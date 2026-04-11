package handler

import (
	"net/http"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
	"github.com/coohu/goagent/internal/tools/registry"
	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	registry *registry.Registry
	router   *llm.Router
}

func NewSystemHandler(reg *registry.Registry, router *llm.Router) *SystemHandler {
	return &SystemHandler{registry: reg, router: router}
}

type ModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

func (h *SystemHandler) ListModels(c *gin.Context) {
	known := h.router.KnownModels()
	global := h.router.GlobalConfig()

	models := make([]ModelInfo, 0, len(known))
	for _, id := range known {
		models = append(models, ModelInfo{ID: id, Provider: providerOf(id)})
	}

	c.JSON(http.StatusOK, gin.H{
		"models":       models,
		"scene_config": global,
	})
}

func (h *SystemHandler) ListTools(c *gin.Context) {
	tools := h.registry.List()
	type toolInfo struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      core.ToolSchema `json:"schema"`
	}
	result := make([]toolInfo, len(tools))
	for i, t := range tools {
		result[i] = toolInfo{Name: t.Name(), Description: t.Description(), Schema: t.Schema()}
	}
	c.JSON(http.StatusOK, gin.H{"tools": result})
}

func providerOf(modelID string) string {
	switch {
	case len(modelID) >= 4 && modelID[:4] == "gpt-":
		return "openai"
	case len(modelID) >= 6 && modelID[:6] == "claude":
		return "anthropic"
	case len(modelID) >= 5 && modelID[:5] == "qwen-":
		return "alibaba"
	default:
		return "unknown"
	}
}
