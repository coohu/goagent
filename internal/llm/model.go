package llm
import "fmt"

type Endpoint string

const (
	EndpointOpenAIChat      Endpoint = "/v1/chat/completions"
	EndpointOpenAIResponses Endpoint = "/v1/responses"
	EndpointAnthropicMsg    Endpoint = "/v1/messages"
	EndpointOllamaChat      Endpoint = "/api/chat"
	EndpointOllamaGenerate  Endpoint = "/api/generate"
)

type Capability string

const (
	CapabilityTools     Capability = "tools"
	CapabilityVision    Capability = "vision"
	CapabilityStreaming Capability = "streaming"
	CapabilityEmbedding Capability = "embedding"
)

type ModelDef struct {
	ID string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	ProviderID string `json:"provider_id"`
	Endpoints []Endpoint `json:"endpoints"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	ContextWindow int `json:"context_window,omitempty"`
}

func (m ModelDef) Display() string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.ID
}

func (m ModelDef) HasEndpoint(e Endpoint) bool {
	for _, ep := range m.Endpoints {
		if ep == e {
			return true
		}
	}
	return false
}

func (m ModelDef) HasCapability(c Capability) bool {
	for _, cap := range m.Capabilities {
		if cap == c {
			return true
		}
	}
	return false
}

func (m ModelDef) PreferredEndpoint(candidates []Endpoint) Endpoint {
	for _, c := range candidates {
		for _, ep := range m.Endpoints {
			if ep == c {
				return ep
			}
		}
	}
	return ""
}

type ProviderConfig struct {
	ID string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	BaseURL string `json:"base_url"`
	APIKey string `json:"api_key"`
	DefaultHeaders map[string]string `json:"default_headers,omitempty"`
	Models []ModelDef `json:"models"`
}

func (p ProviderConfig) FindModel(modelID string) (ModelDef, error) {
	for _, m := range p.Models {
		if m.ID == modelID {
			return m, nil
		}
	}
	return ModelDef{}, fmt.Errorf("model %q not found in provider %q", modelID, p.ID)
}

func OpenAIProvider(apiKey string) ProviderConfig {
	return ProviderConfig{
		ID:          "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com",
		APIKey:      apiKey,
		Models: []ModelDef{
			{
				ID: "gpt-4o", ProviderID: "openai",
				Endpoints:     []Endpoint{EndpointOpenAIChat, EndpointOpenAIResponses},
				Capabilities:  []Capability{CapabilityTools, CapabilityVision, CapabilityStreaming},
				ContextWindow: 128000,
			},
			{
				ID: "gpt-4o-mini", ProviderID: "openai",
				Endpoints:     []Endpoint{EndpointOpenAIChat, EndpointOpenAIResponses},
				Capabilities:  []Capability{CapabilityTools, CapabilityStreaming},
				ContextWindow: 128000,
			},
			{
				ID: "text-embedding-3-small", ProviderID: "openai",
				Endpoints:    []Endpoint{"/v1/embeddings"},
				Capabilities: []Capability{CapabilityEmbedding},
			},
		},
	}
}

func AnthropicProvider(apiKey string) ProviderConfig {
	return ProviderConfig{
		ID:          "anthropic",
		DisplayName: "Anthropic",
		BaseURL:     "https://api.anthropic.com",
		APIKey:      apiKey,
		DefaultHeaders: map[string]string{
			"anthropic-version": "2023-06-01",
		},
		Models: []ModelDef{
			{
				ID: "claude-opus-4-5", ProviderID: "anthropic",
				Endpoints:     []Endpoint{EndpointAnthropicMsg},
				Capabilities:  []Capability{CapabilityTools, CapabilityVision, CapabilityStreaming},
				ContextWindow: 200000,
			},
			{
				ID: "claude-sonnet-4-5", ProviderID: "anthropic",
				Endpoints:     []Endpoint{EndpointAnthropicMsg},
				Capabilities:  []Capability{CapabilityTools, CapabilityVision, CapabilityStreaming},
				ContextWindow: 200000,
			},
			{
				ID: "claude-haiku-4-5", ProviderID: "anthropic",
				Endpoints:     []Endpoint{EndpointAnthropicMsg},
				Capabilities:  []Capability{CapabilityTools, CapabilityStreaming},
				ContextWindow: 200000,
			},
		},
	}
}

func OpenRouterProvider(apiKey string) ProviderConfig {
	return ProviderConfig{
		ID:          "openrouter",
		DisplayName: "OpenRouter",
		BaseURL:     "https://openrouter.ai/api",
		APIKey:      apiKey,
		DefaultHeaders: map[string]string{
			"HTTP-Referer": "https://github.com/coohu/goagent",
		},
		// OpenRouter supports any model via /v1/chat/completions.
		// Models are added dynamically; a wildcard entry handles all of them.
		Models: []ModelDef{},
	}
}

func OllamaProvider(baseURL, model string) ProviderConfig {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return ProviderConfig{
		ID:          "ollama",
		DisplayName: "Ollama (local)",
		BaseURL:     baseURL,
		Models: []ModelDef{
			{
				ID: model, ProviderID: "ollama",
				Endpoints:    []Endpoint{EndpointOllamaChat},
				Capabilities: []Capability{CapabilityTools, CapabilityStreaming},
			},
		},
	}
}

func VercelAIGatewayProvider(apiKey, baseURL string) ProviderConfig {
	return ProviderConfig{
		ID:          "vercel",
		DisplayName: "Vercel AI Gateway",
		BaseURL:     baseURL,
		APIKey:      apiKey,
		// Vercel gateway speaks OpenAI Chat protocol.
		Models: []ModelDef{},
	}
}
