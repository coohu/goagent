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

func (h *SystemHandler) ListModels(c *gin.Context) {
	models := h.router.KnownModels()
	global := h.router.GlobalConfig()

	type modelInfo struct {
		ID            string           `json:"id"`
		DisplayName   string           `json:"display_name"`
		ProviderID    string           `json:"provider_id"`
		Endpoints     []llm.Endpoint   `json:"endpoints"`
		Capabilities  []llm.Capability `json:"capabilities"`
		ContextWindow int              `json:"context_window,omitempty"`
	}

	result := make([]modelInfo, len(models))
	for i, m := range models {
		result[i] = modelInfo{
			ID:            m.ID,
			DisplayName:   m.Display(),
			ProviderID:    m.ProviderID,
			Endpoints:     m.Endpoints,
			Capabilities:  m.Capabilities,
			ContextWindow: m.ContextWindow,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"models":       result,
		"scene_config": global,
	})
}

func (h *SystemHandler) ListProviders(c *gin.Context) {
	providers := h.router.Providers()

	type providerInfo struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		BaseURL     string `json:"base_url"`
		ModelCount  int    `json:"model_count"`
	}

	result := make([]providerInfo, len(providers))
	for i, p := range providers {
		result[i] = providerInfo{
			ID:          p.ID,
			DisplayName: p.DisplayName,
			BaseURL:     p.BaseURL,
			ModelCount:  len(p.Models),
		}
	}
	c.JSON(http.StatusOK, gin.H{"providers": result})
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
