package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig
	Agent     AgentDefaults
	LLM       LLMConfig
	Embedding EmbeddingConfig
	Reranker  RerankerConfig
	Memory    MemoryConfig
	EventBus  EventBusConfig
	Tools     ToolsConfig
	Database  DatabaseConfig
	Log       LogConfig
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type AgentDefaults struct {
	MaxConcurrentSessions int           `mapstructure:"max_concurrent_sessions"`
	DefaultMaxSteps       int           `mapstructure:"default_max_steps"`
	DefaultMaxRuntime     time.Duration `mapstructure:"default_max_runtime"`
	DefaultMaxLLMCalls    int           `mapstructure:"default_max_llm_calls"`
	DefaultMaxToolCalls   int           `mapstructure:"default_max_tool_calls"`
	DefaultMaxReplan      int           `mapstructure:"default_max_replan"`
	DefaultMaxTokens      int           `mapstructure:"default_max_tokens"`
	WorkerPoolSize        int           `mapstructure:"worker_pool_size"`
}

type LLMConfig struct {
	DefaultModel   string        `mapstructure:"default_model"`
	SummarizerModel string       `mapstructure:"summarizer_model"`
	ReflectionModel string       `mapstructure:"reflection_model"`
	OpenAI         OpenAIConfig  `mapstructure:"openai"`
	Anthropic      AnthropicConfig `mapstructure:"anthropic"`
	Ollama         OllamaConfig  `mapstructure:"ollama"`
}

type OpenAIConfig struct {
	APIKey  string        `mapstructure:"api_key"`
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type AnthropicConfig struct {
	APIKey string `mapstructure:"api_key"`
}

type OllamaConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

type EmbeddingConfig struct {
	Model      string `mapstructure:"model"`
	Endpoint   string `mapstructure:"endpoint"`
	Dimensions int    `mapstructure:"dimensions"`
	BatchSize  int    `mapstructure:"batch_size"`
}

type RerankerConfig struct {
	Endpoint    string `mapstructure:"endpoint"`
	TopKInput   int    `mapstructure:"top_k_input"`
	TopKOutput  int    `mapstructure:"top_k_output"`
}

type MemoryConfig struct {
	ToolMemoryMaxRawSize int           `mapstructure:"tool_memory_max_raw_size"`
	ConversationTTL      time.Duration `mapstructure:"conversation_ttl"`
	SessionCacheTTL      time.Duration `mapstructure:"session_cache_ttl"`
}

type EventBusConfig struct {
	QueueCapacity         int           `mapstructure:"queue_capacity"`
	MaxReplanPerMin       int           `mapstructure:"max_replan_per_min"`
	MaxToolRetryPerMin    int           `mapstructure:"max_tool_retry_per_min"`
	MaxLLMCallsPerMin     int           `mapstructure:"max_llm_calls_per_min"`
	DebounceMemoryUpdate  time.Duration `mapstructure:"debounce_memory_update"`
	DebounceContextBuild  time.Duration `mapstructure:"debounce_context_build"`
	DedupWindow           time.Duration `mapstructure:"dedup_window"`
	LoopDetectThreshold   int           `mapstructure:"loop_detection_threshold"`
}

type ToolsConfig struct {
	Shell  ShellConfig  `mapstructure:"shell"`
	Search SearchConfig `mapstructure:"search"`
}

type ShellConfig struct {
	SandboxImage    string        `mapstructure:"sandbox_image"`
	CPULimit        string        `mapstructure:"cpu_limit"`
	MemoryLimit     string        `mapstructure:"memory_limit"`
	CommandTimeout  time.Duration `mapstructure:"command_timeout"`
	SessionTimeout  time.Duration `mapstructure:"session_timeout"`
	WorkspaceRoot   string        `mapstructure:"workspace_root"`
}

type SearchConfig struct {
	Provider    string `mapstructure:"provider"`
	TavilyKey   string `mapstructure:"tavily_api_key"`
	MaxResults  int    `mapstructure:"max_results"`
}

type DatabaseConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Qdrant   QdrantConfig   `mapstructure:"qdrant"`
}

type PostgresConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type QdrantConfig struct {
	Addr string `mapstructure:"addr"`
	Port int    `mapstructure:"port"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "60s")

	v.SetDefault("agent.max_concurrent_sessions", 10)
	v.SetDefault("agent.default_max_steps", 30)
	v.SetDefault("agent.default_max_runtime", "10m")
	v.SetDefault("agent.default_max_llm_calls", 30)
	v.SetDefault("agent.default_max_tool_calls", 20)
	v.SetDefault("agent.default_max_replan", 5)
	v.SetDefault("agent.default_max_tokens", 200000)
	v.SetDefault("agent.worker_pool_size", 20)

	v.SetDefault("llm.default_model", "gpt-4o")
	v.SetDefault("llm.summarizer_model", "gpt-4o-mini")
	v.SetDefault("llm.reflection_model", "gpt-4o-mini")
	v.SetDefault("llm.openai.base_url", "https://api.openai.com/v1")
	v.SetDefault("llm.openai.timeout", "120s")
	v.SetDefault("llm.ollama.base_url", "http://localhost:11434")

	v.SetDefault("embedding.dimensions", 512)
	v.SetDefault("embedding.batch_size", 32)

	v.SetDefault("reranker.top_k_input", 20)
	v.SetDefault("reranker.top_k_output", 5)

	v.SetDefault("memory.tool_memory_max_raw_size", 50000)
	v.SetDefault("memory.conversation_ttl", "24h")
	v.SetDefault("memory.session_cache_ttl", "1h")

	v.SetDefault("event_bus.queue_capacity", 1000)
	v.SetDefault("event_bus.max_replan_per_min", 3)
	v.SetDefault("event_bus.max_tool_retry_per_min", 2)
	v.SetDefault("event_bus.debounce_memory_update", "2s")
	v.SetDefault("event_bus.debounce_context_build", "5s")
	v.SetDefault("event_bus.dedup_window", "30s")
	v.SetDefault("event_bus.loop_detection_threshold", 3)

	v.SetDefault("tools.shell.sandbox_image", "goagent-sandbox:latest")
	v.SetDefault("tools.shell.cpu_limit", "1.0")
	v.SetDefault("tools.shell.memory_limit", "512m")
	v.SetDefault("tools.shell.command_timeout", "60s")
	v.SetDefault("tools.shell.session_timeout", "10m")
	v.SetDefault("tools.shell.workspace_root", "/tmp/goagent/workspaces")

	v.SetDefault("database.postgres.max_open_conns", 20)
	v.SetDefault("database.postgres.max_idle_conns", 5)
	v.SetDefault("database.postgres.conn_max_lifetime", "5m")
	v.SetDefault("database.qdrant.port", 6334)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
}
