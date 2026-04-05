package handler

import (
	"net/http"

	"github.com/coohu/goagent/internal/agent"
	"github.com/coohu/goagent/internal/api/sse"
	"github.com/coohu/goagent/internal/core"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	sessions *agent.SessionManager
	runner   *agent.Runner
	hub      *sse.Hub
}

func NewAgentHandler(sessions *agent.SessionManager, runner *agent.Runner, hub *sse.Hub) *AgentHandler {
	return &AgentHandler{sessions: sessions, runner: runner, hub: hub}
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

	go func() {
		ctx := c.Request.Context()
		_ = h.runner.Run(ctx, session)
	}()

	c.JSON(http.StatusOK, RunResponse{
		SessionID: session.ID,
		State:     string(session.GetState()),
		StreamURL: "/api/v1/agent/" + session.ID + "/stream",
	})
}

func (h *AgentHandler) Stream(c *gin.Context) {
	sessionID := c.Param("session_id")
	if _, err := h.sessions.Get(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	h.hub.ServeClient(c.Writer, c.Request, sessionID)
}

func (h *AgentHandler) Status(c *gin.Context) {
	session, err := h.sessions.Get(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var planInfo any
	if session.Plan != nil {
		planInfo = session.Plan
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"state":      session.GetState(),
		"goal":       session.Goal,
		"plan":       planInfo,
		"metrics":    session.Metrics,
		"created_at": session.CreatedAt,
		"updated_at": session.UpdatedAt,
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

	if body.Approved {
		session.EventChan <- core.Event{
			Type:      core.EventApproved,
			SessionID: session.ID,
			Payload:   map[string]any{"comment": body.Comment},
		}
		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
	} else {
		session.EventChan <- core.Event{
			Type:      core.EventCancel,
			SessionID: session.ID,
		}
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
	}
}
