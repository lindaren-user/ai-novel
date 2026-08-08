package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	einomodel "github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

var ErrInvalidChatModelConfig = errors.New("invalid chat model config")

const defaultClaudeMaxTokens = 4096

type ChatModelConfig struct {
	Provider    string
	Model       string
	APIURL      string
	APIKey      string
	MaxTokens   int
	Temperature float64
	TopP        float64
	Timeout     time.Duration
}

// NewChatModel 根据用户保存的模型配置创建聊天模型，并按厂商选择对应原生或兼容客户端。
//
// temperature、topP 不在这里传递，而是在运行时传递。
func NewChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.APIURL = strings.TrimSpace(cfg.APIURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.Provider == "" || cfg.Model == "" || cfg.APIURL == "" || cfg.APIKey == "" {
		return nil, ErrInvalidChatModelConfig
	}

	switch cfg.Provider {
	case "deepseek":
		return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			Model:   cfg.Model,
			BaseURL: cfg.APIURL,
			APIKey:  cfg.APIKey,
			Timeout: cfg.Timeout,
		})
	case "gemini":
		return NewGeminiChatModel(ctx, cfg)
	case "claude":
		return NewClaudeChatModel(ctx, cfg)
	case "doubao", "ark":
		return NewArkChatModel(ctx, cfg)
	case "qianwen", "qwen":
		return NewQwenChatModel(ctx, cfg)
	case "gpt", "kimi", "grok", "custom_openai_completions":
		return NewOpenAICompatibleChatModel(ctx, cfg)
	// case "custom_openai_responses":
	// 	return NewOpenAIResponsesChatModel(ctx, cfg)
	default:
		return nil, ErrInvalidChatModelConfig
	}
}

// NewOpenAICompatibleChatModel 使用 OpenAI Chat Completions API 协议创建聊天模型。
func NewOpenAICompatibleChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	config := &openai.ChatModelConfig{
		Model:   cfg.Model,
		BaseURL: cfg.APIURL,
		APIKey:  cfg.APIKey,
		Timeout: cfg.Timeout,
	}
	if cfg.MaxTokens > 0 {
		config.MaxTokens = &cfg.MaxTokens
	}
	return openai.NewChatModel(ctx, config)
}

// NewClaudeChatModel 使用 Anthropic Claude 原生 Messages 协议创建聊天模型。
func NewClaudeChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	baseURL := cfg.APIURL
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultClaudeMaxTokens
	}
	return claude.NewChatModel(ctx, &claude.Config{
		APIKey:     cfg.APIKey,
		BaseURL:    &baseURL,
		Model:      cfg.Model,
		MaxTokens:  maxTokens,
		HTTPClient: httpClientWithTimeout(cfg.Timeout),
	})
}

// NewGeminiChatModel 使用 Google Gemini 原生协议创建聊天模型。
func NewGeminiChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	clientConfig := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClientWithTimeout(cfg.Timeout),
	}
	if cfg.APIURL != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{BaseURL: cfg.APIURL}
	}
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, err
	}
	var maxTokens *int
	if cfg.MaxTokens > 0 {
		maxTokens = &cfg.MaxTokens
	}
	return gemini.NewChatModel(ctx, &gemini.Config{
		Client:    client,
		Model:     cfg.Model,
		MaxTokens: maxTokens,
	})
}

// NewArkChatModel 使用火山方舟 Ark 原生组件创建聊天模型。
func NewArkChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	config := &ark.ChatModelConfig{
		Model:   cfg.Model,
		BaseURL: cfg.APIURL,
		APIKey:  cfg.APIKey,
	}
	if cfg.Timeout > 0 {
		config.Timeout = &cfg.Timeout
	}
	if cfg.MaxTokens > 0 {
		config.MaxTokens = &cfg.MaxTokens
	}
	return ark.NewChatModel(ctx, config)
}

// NewQwenChatModel 使用通义千问 Qwen 组件创建聊天模型。
func NewQwenChatModel(ctx context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	config := &qwen.ChatModelConfig{
		Model:   cfg.Model,
		BaseURL: cfg.APIURL,
		APIKey:  cfg.APIKey,
		Timeout: cfg.Timeout,
	}
	if cfg.MaxTokens > 0 {
		config.MaxTokens = &cfg.MaxTokens
	}
	return qwen.NewChatModel(ctx, config)
}

func httpClientWithTimeout(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		return nil
	}
	return &http.Client{Timeout: timeout}
}

// RuntimeModelConfig 是一次 agent 运行的模型配置。
//
// BaseModel 用于真正替换本次运行使用的模型实例，例如从 DeepSeek 切到 Claude、
// Gemini，或切换 apiURL/apiKey 不同的用户自定义模型。
// 其余字段是传给该模型实例的调用参数。
type RuntimeModelConfig struct {
	BaseModel   model.BaseChatModel
	Temperature *float32
	TopP        *float32
	MaxTokens   *int
	Stop        []string
}

func (c RuntimeModelConfig) ModelOptions() []model.Option {
	opts := make([]model.Option, 0, 4)
	if c.Temperature != nil {
		opts = append(opts, model.WithTemperature(*c.Temperature))
	}
	if c.TopP != nil {
		opts = append(opts, model.WithTopP(*c.TopP))
	}
	if c.MaxTokens != nil {
		opts = append(opts, model.WithMaxTokens(*c.MaxTokens))
	}
	if len(c.Stop) > 0 {
		opts = append(opts, model.WithStop(c.Stop))
	}
	return opts
}
