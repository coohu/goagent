module github.com/coohu/goagent

go 1.22

require (
	// Server deps
	github.com/gin-gonic/gin v1.9.1
	github.com/google/uuid v1.6.0
	github.com/sashabaranov/go-openai v1.20.4
	github.com/spf13/viper v1.18.2
	github.com/lib/pq v1.10.9

	// CLI deps
	github.com/charmbracelet/bubbletea v0.26.4
	github.com/charmbracelet/bubbles v0.18.0
	github.com/charmbracelet/lipgloss v0.11.0
	github.com/spf13/cobra v1.8.0
)
