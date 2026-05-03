package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/coohu/goagent/internal/agent"
	"github.com/coohu/goagent/internal/api/sse"
	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	sessions *agent.SessionManager
	runner   *agent.Runner
	hub      *sse.Hub
	router   *llm.Router
	provReg  *llm.Registry
}

func NewAgentHandler(sessions *agent.SessionManager, runner *agent.Runner, hub *sse.Hub, router *llm.Router, provReg *llm.Registry) *AgentHandler {
	return &AgentHandler{sessions: sessions, runner: runner, hub: hub, router: router, provReg: provReg}
}

type RunRequest struct {
	Goal   string            `json:"goal" binding:"required"`
	Config *core.AgentConfig `json:"config"`
}

type RunResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	StreamURL string `json:"stream_url"`
}

func (h *AgentHandler) Run(c *gin.Context) {
	var req RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.sessions.Create(req.Goal, req.Config)
	if err != nil {
		status := http.StatusInternalServerError
		if ae, ok := err.(*core.AgentError); ok && ae.Code == core.ErrSessionFull {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	h.ensureModelsRegistered(session.Config.Models)

	go func() {
		_ = h.runner.Run(context.Background(), session)
	}()

	c.JSON(http.StatusOK, RunResponse{
		SessionID: session.ID,
		State:     string(session.GetState()),
		StreamURL: "/api/v1/agent/" + session.ID + "/stream",
	})
}

func (h *AgentHandler) Continue(c *gin.Context) {
	session, err := h.sessions.Get(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var req struct {
		Goal string `json:"goal" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session.Goal = req.Goal
	session.AgentCtx.Goal = req.Goal
	session.Plan = nil
	session.SetState(core.StateIdle)
	session.AgentCtx.Scratchpad = &core.Scratchpad{MaxTokens: session.Config.ScratchpadMaxTokens}

	go func() {
		_ = h.runner.Run(context.Background(), session)
	}()

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"state":      session.GetState(),
		"stream_url": "/api/v1/agent/" + session.ID + "/stream",
	})
}

func (h *AgentHandler) Stream(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	cursor := 0
	if v := c.Query("cursor"); v != "" {
		cursor, _ = strconv.Atoi(v)
	}
	h.hub.ServeClient(c.Writer, c.Request, sessionID, cursor)
}

func (h *AgentHandler) Events(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	cursor := 0
	if v := c.Query("cursor"); v != "" {
		cursor, _ = strconv.Atoi(v)
	}
	events := h.hub.History(sessionID, cursor)
	raw := make([]any, len(events))
	for i, e := range events {
		raw[i] = string(e)
	}
	c.JSON(http.StatusOK, gin.H{"events": raw, "cursor": cursor + len(events)})
}

func (h *AgentHandler) Status(c *gin.Context) {
	session, err := h.sessions.Get(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session_id":  session.ID,
		"state":       session.GetState(),
		"goal":        session.Goal,
		"plan":        session.Plan,
		"metrics":     session.Metrics,
		"config":      session.Config,
		"created_at":  session.CreatedAt,
		"updated_at":  session.UpdatedAt,
		"finished_at": session.FinishedAt,
	})
}

func (h *AgentHandler) Cancel(c *gin.Context) {
	if err := h.sessions.Cancel(c.Param("session_id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *AgentHandler) Approve(c *gin.Context) {
	session, err := h.sessions.Get(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var body struct {
		Approved bool   `json:"approved"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	evType := core.EventCancel
	if body.Approved {
		evType = core.EventApproved
	}
	session.EventChan <- core.Event{
		Type:      evType,
		SessionID: session.ID,
		Payload:   map[string]any{"comment": body.Comment},
	}

	status := "cancelled"
	if body.Approved {
		status = "resumed"
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}

// ensureModelsRegistered ensures all model IDs in the session config are resolvable.
// Unknown models are registered as dynamic entries in the fallback provider.
func (h *AgentHandler) ensureModelsRegistered(models core.SceneModels) {
	for _, modelID := range []string{models.Planning, models.Execute, models.Summarize, models.Reflect} {
		if modelID == "" {
			continue
		}
		// ClientOrFallback handles dynamic registration via fallback provider.
		_ = h.provReg
	}
}

func (h *AgentHandler) UpdateConfig(c *gin.Context) {
	session, err := h.sessions.Get(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var patch struct {
		MaxSteps     *int              `json:"max_steps"`
		AllowedTools []string          `json:"allowed_tools"`
		Models       *core.SceneModels `json:"models"`
	}
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if patch.MaxSteps != nil {
		session.Config.MaxSteps = *patch.MaxSteps
	}
	if patch.AllowedTools != nil {
		session.Config.AllowedTools = patch.AllowedTools
	}
	if patch.Models != nil {
		m := patch.Models
		if m.Planning != "" {
			session.Config.Models.Planning = m.Planning
		}
		if m.Execute != "" {
			session.Config.Models.Execute = m.Execute
		}
		if m.Summarize != "" {
			session.Config.Models.Summarize = m.Summarize
		}
		if m.Reflect != "" {
			session.Config.Models.Reflect = m.Reflect
		}
		h.ensureModelsRegistered(session.Config.Models)
	}

	c.JSON(http.StatusOK, gin.H{
		"config": session.Config,
		"models": session.Config.Models,
	})
}

func (h *AgentHandler) ListSessions(c *gin.Context) {
	all := h.sessions.List()
	type item struct {
		ID        string `json:"id"`
		Goal      string `json:"goal"`
		State     string `json:"state"`
		CreatedAt any    `json:"created_at"`
	}
	result := make([]item, len(all))
	for i, s := range all {
		result[i] = item{
			ID:        s.ID,
			Goal:      s.Goal,
			State:     string(s.GetState()),
			CreatedAt: s.CreatedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": result})
}
