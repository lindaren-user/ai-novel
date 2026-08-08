package ai

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// StreamEventType 是流式事件的类型标签，一种类型对应一种数据结构。
type StreamEventType string

const (
	StreamEventDelta      StreamEventType = "delta"
	StreamEventSync       StreamEventType = "sync"
	StreamEventDone       StreamEventType = "done"
	StreamEventError      StreamEventType = "error"
	StreamEventA2UI       StreamEventType = "a2ui"
	StreamEventToolCall   StreamEventType = "tool_call"
	StreamEventToolResult StreamEventType = "tool_result"
)

type StreamMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Protected bool   `json:"protected,omitempty"`
}

// AgentDefinition 描述一次 AI 调用要使用的通用 Agent，不包含小说、卷、章等业务身份。
type AgentDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	Instruction string                         `json:"instruction"`
	CacheKey    string                         `json:"cacheKey,omitempty"`
	Tools       []tool.BaseTool                `json:"-"`
	Middlewares []adk.ChatModelAgentMiddleware `json:"-"`
}

type AgentStreamRequest struct {
	UserID   int64              `json:"userId"`
	ModelKey string             `json:"modelKey"`
	Model    ModelRuntimeConfig `json:"model"`
	Messages []StreamMessage    `json:"messages"`
	Agent    AgentDefinition    `json:"agent"`
}

type ChatGenerateRequest struct {
	UserID   int64              `json:"userId"`
	ModelKey string             `json:"modelKey"`
	Model    ModelRuntimeConfig `json:"model"`
	Messages []StreamMessage    `json:"messages"`
}

type AgentGenerateRequest struct {
	ChatGenerateRequest
	Agent AgentDefinition `json:"agent"`
}

// GenerateResult 是非流式模型调用的正文和结束元数据。
type GenerateResult struct {
	Content      string `json:"content"`
	TokenCount   int64  `json:"tokenCount,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
}

type ModelRuntimeConfig struct {
	Provider            string   `json:"provider"`
	ModelID             string   `json:"modelId"`
	APIURL              string   `json:"apiUrl"`
	APIKey              string   `json:"apiKey"`
	Temperature         float64  `json:"temperature"`
	TopP                float64  `json:"topP"`
	MaxTokens           int      `json:"maxTokens,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	Stop                []string `json:"stop,omitempty"`
}

// StreamEvent 是单次流式事件。Type 决定 Data 的载荷类型。
type StreamEvent struct {
	Type StreamEventType `json:"type"`
	Data any             `json:"data,omitempty"`
}

// StreamDelta 是逐字增量文本。
type StreamDelta struct {
	Text string `json:"text"`
}

// StreamSync 是重建流时的完整快照。
type StreamSync struct {
	Content    string         `json:"content"`
	RenderData map[string]any `json:"renderData,omitempty"`
}

// StreamDone 是流结束信号，顶层只放通用结束统计。
type StreamDone struct {
	TokenCount   int64             `json:"tokenCount,omitempty"`
	FinishReason string            `json:"finishReason,omitempty"`
	Params       *StreamDoneParams `json:"params,omitempty"`
}

// StreamDoneParams 是业务侧附加参数，不参与通用流结束语义。
type StreamDoneParams struct {
	// DraftID 是本次回复落库后关联的章节正文草稿 ID，前端用于绑定草稿操作入口。
	DraftID int64 `json:"draftId,omitempty"`
}

func (p StreamDoneParams) empty() bool {
	return p.DraftID <= 0
}

// StreamError 是流式错误。
type StreamError struct {
	Message      string `json:"message"`
	TokenCount   int64  `json:"tokenCount,omitempty"`   // 这个字段只有 stream 才有
	FinishReason string `json:"finishReason,omitempty"` // 同理
}

// StreamA2UI 是前端 UI 增量渲染指令。
type StreamA2UI struct {
	Data map[string]any `json:"data"`
}

// StreamToolCall 是 AI 即将调用某个工具的通知。
type StreamToolCall struct {
	Name string `json:"name"`
}

// StreamToolResult 是工具执行完毕后返回的原始结果。
type StreamToolResult struct {
	Name   string `json:"name"`
	Result string `json:"result"`
}

func NewStreamDelta(text string) StreamEvent {
	return StreamEvent{Type: StreamEventDelta, Data: StreamDelta{Text: text}}
}

func NewStreamSync(content string, renderData map[string]any) StreamEvent {
	return StreamEvent{Type: StreamEventSync, Data: StreamSync{Content: content, RenderData: renderData}}
}

func NewStreamDone(tokenCount int64, finishReason string, draftID int64) StreamEvent {
	return NewStreamDoneWithParams(tokenCount, finishReason, StreamDoneParams{DraftID: draftID})
}

func NewStreamDoneWithParams(tokenCount int64, finishReason string, params StreamDoneParams) StreamEvent {
	done := StreamDone{TokenCount: tokenCount, FinishReason: finishReason}
	if !params.empty() {
		done.Params = &params
	}
	return StreamEvent{Type: StreamEventDone, Data: done}
}

func NewStreamError(message string) StreamEvent {
	return StreamEvent{Type: StreamEventError, Data: StreamError{Message: message}}
}

func NewStreamErrorWithMeta(message string, tokenCount int64, finishReason string) StreamEvent {
	return StreamEvent{Type: StreamEventError, Data: StreamError{
		Message:      message,
		TokenCount:   tokenCount,
		FinishReason: finishReason,
	}}
}

func NewStreamA2UI(kind string, data map[string]any) StreamEvent {
	payload := make(map[string]any, len(data)+1)
	for key, value := range data {
		payload[key] = value
	}
	if _, ok := payload["kind"]; !ok {
		payload["kind"] = kind
	}
	return StreamEvent{Type: StreamEventA2UI, Data: StreamA2UI{Data: payload}}
}

func NewStreamToolCall(name string) StreamEvent {
	return StreamEvent{Type: StreamEventToolCall, Data: StreamToolCall{Name: name}}
}

func NewStreamToolResult(name string, result string) StreamEvent {
	return StreamEvent{Type: StreamEventToolResult, Data: StreamToolResult{Name: name, Result: result}}
}

func (e StreamEvent) Delta() StreamDelta {
	if data, ok := e.Data.(StreamDelta); ok {
		return data
	}
	return StreamDelta{}
}

func (e StreamEvent) Sync() StreamSync {
	if data, ok := e.Data.(StreamSync); ok {
		return data
	}
	return StreamSync{}
}

func (e StreamEvent) Done() StreamDone {
	if data, ok := e.Data.(StreamDone); ok {
		return data
	}
	return StreamDone{}
}

func (e StreamEvent) Error() StreamError {
	if data, ok := e.Data.(StreamError); ok {
		return data
	}
	return StreamError{}
}

func (e StreamEvent) A2UI() StreamA2UI {
	if data, ok := e.Data.(StreamA2UI); ok {
		return data
	}
	return StreamA2UI{}
}

func (e StreamEvent) ToolCall() StreamToolCall {
	if data, ok := e.Data.(StreamToolCall); ok {
		return data
	}
	return StreamToolCall{}
}

func (e StreamEvent) ToolResult() StreamToolResult {
	if data, ok := e.Data.(StreamToolResult); ok {
		return data
	}
	return StreamToolResult{}
}

type Client interface {
	StreamAgent(ctx context.Context, req AgentStreamRequest) (<-chan StreamEvent, error)
	StreamChat(ctx context.Context, req ChatGenerateRequest) (<-chan StreamEvent, error)
	GenerateAgent(ctx context.Context, req AgentGenerateRequest) (GenerateResult, error)
	GenerateChat(ctx context.Context, req ChatGenerateRequest) (GenerateResult, error)
}

func NewClient() Client {
	return &Agents{
		agents: make(map[string]*adk.ChatModelAgent),
	}
}
