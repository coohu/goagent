package api

import (
	"github.com/coohu/goagent/internal/api/handler"
	"github.com/coohu/goagent/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func NewRouter(agentHandler *handler.AgentHandler) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery(), middleware.RequestID(), middleware.Logger())

	v1 := r.Group("/api/v1")
	{
		ag := v1.Group("/agent")
		ag.POST("/run", agentHandler.Run)
		ag.GET("/:session_id/stream", agentHandler.Stream)
		ag.GET("/:session_id/status", agentHandler.Status)
		ag.DELETE("/:session_id", agentHandler.Cancel)
		ag.POST("/:session_id/approve", agentHandler.Approve)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}
