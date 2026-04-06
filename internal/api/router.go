package api

import (
	"github.com/coohu/goagent/internal/api/handler"
	"github.com/coohu/goagent/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func NewRouter(
	agentHandler *handler.AgentHandler,
	fileHandler *handler.FileHandler,
	sysHandler *handler.SystemHandler,
) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery(), middleware.RequestID(), middleware.Logger())

	v1 := r.Group("/api/v1")

	ag := v1.Group("/agent")
	ag.POST("/run", agentHandler.Run)
	ag.GET("/:session_id/stream", agentHandler.Stream)
	ag.GET("/:session_id/status", agentHandler.Status)
	ag.GET("/:session_id/events", agentHandler.Events)
	ag.POST("/:session_id/continue", agentHandler.Continue)
	ag.DELETE("/:session_id", agentHandler.Cancel)
	ag.POST("/:session_id/approve", agentHandler.Approve)
	ag.PUT("/:session_id/config", agentHandler.UpdateConfig)

	v1.GET("/sessions", agentHandler.ListSessions)
	v1.GET("/models", sysHandler.ListModels)
	v1.GET("/tools", sysHandler.ListTools)

	files := v1.Group("/files")
	files.POST("/:session_id/upload", fileHandler.Upload)
	files.GET("/:session_id/download", fileHandler.Download)
	files.GET("/:session_id/list", fileHandler.List)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}
