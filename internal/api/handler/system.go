package handler

import (
	"net/http"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/tools/registry"
	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	registry     *registry.Registry
	knownModels  []ModelInfo
}

type ModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Default  bool   `json:"default"`
}

func NewSystemHandler(reg *registry.Registry, defaultModel string, models []ModelInfo) *SystemHandler {
	for i := range models {
		if models[i].ID == defaultModel {
			models[i].Default = true
		}
	}
	return &SystemHandler{registry: reg, knownModels: models}
}

func (h *SystemHandler) ListModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"models": h.knownModels})
}

func (h *SystemHandler) ListTools(c *gin.Context) {
	tools := h.registry.List()
	type toolInfo struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      core.ToolSchema `json:"schema"`
	}
	result := make([]toolInfo, len(tools))
	for i, t := range tools {
		result[i] = toolInfo{Name: t.Name(), Description: t.Description(), Schema: t.Schema()}
	}
	c.JSON(http.StatusOK, gin.H{"tools": result})
}
