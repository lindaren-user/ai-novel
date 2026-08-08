// OpenAI Responses 适配器说明：
//  1. OpenAI 官方 Responses API 可以承载工具调用/skill 续写流程，模型先返回 function_call，
//     工具执行后再把 function_call_output 回传给模型继续生成。
//  2. 官方 function_call_output 的工具结果字段是 output，不是 content；call_id 必须对应前一次
//     function_call 的 call_id。当前适配器仍是最小可用实现，重新开放前需要重点校准这部分协议。
//  3. 简单问答不走工具调用，通常可以成功；正文生成会先调用 skill，再把 skill 结果回传模型，
//     因此更容易暴露 function_call_output、previous_response_id、流式事件解析等适配问题。
//  4. OpenAI 官方协议不等同于各类中转站协议；中转站可能额外要求 item_reference、WebSocket v2
//     或其他私有续写状态，不能用官方可行性直接推断中转站一定可用。
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	openAIResponsesEndpoint = "/responses"

	responsesEventTextDelta          = "response.output_text.delta"
	responsesEventReasoningTextDelta = "response.reasoning_text.delta"
	responsesEventOutputItemAdded    = "response.output_item.added"
	responsesEventArgumentsDelta     = "response.function_call_arguments.delta"
	responsesEventCompleted          = "response.completed"
	responsesEventIncomplete         = "response.incomplete"
	responsesEventFailed             = "response.failed"
	responsesEventError              = "error"
)

type openAIResponsesChatModel struct {
	model       string
	baseURL     string
	apiKey      string
	maxTokens   *int
	temperature *float32
	topP        *float32
	tools       []*schema.ToolInfo
	httpClient  *http.Client
}

type responsesRequest struct {
	Model           string                 `json:"model"`
	Input           []responsesInputItem   `json:"input"`
	Stream          bool                   `json:"stream,omitempty"`
	Tools           []responsesTool        `json:"tools,omitempty"`
	ToolChoice      any                    `json:"tool_choice,omitempty"`
	MaxOutputTokens *int                   `json:"max_output_tokens,omitempty"`
	Temperature     *float32               `json:"temperature,omitempty"`
	TopP            *float32               `json:"top_p,omitempty"`
	Extra           map[string]interface{} `json:"-"`
}

type responsesInputItem struct {
	Type      string                  `json:"type,omitempty"`
	Role      string                  `json:"role,omitempty"`
	Content   any                     `json:"content,omitempty"`
	CallID    string                  `json:"call_id,omitempty"`
	Name      string                  `json:"name,omitempty"`
	Arguments string                  `json:"arguments,omitempty"`
	ToolCalls []responsesFunctionCall `json:"-"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type responsesObject struct {
	ID     string                 `json:"id"`
	Model  string                 `json:"model"`
	Status string                 `json:"status"`
	Output []responsesOutputItem  `json:"output"`
	Usage  *responsesUsage        `json:"usage"`
	Error  *responsesErrorPayload `json:"error"`
}

type responsesOutputItem struct {
	Type      string                   `json:"type"`
	ID        string                   `json:"id"`
	CallID    string                   `json:"call_id"`
	Name      string                   `json:"name"`
	Arguments string                   `json:"arguments"`
	Content   []responsesOutputContent `json:"content"`
}

type responsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	PromptTokens       int `json:"prompt_tokens"`
	CompletionTokens   int `json:"completion_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type responsesErrorPayload struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type responsesStreamEvent struct {
	Type        string                 `json:"type"`
	Delta       string                 `json:"delta"`
	Item        *responsesOutputItem   `json:"item"`
	Response    *responsesObject       `json:"response"`
	Error       *responsesErrorPayload `json:"error"`
	OutputIndex *int                   `json:"output_index"`
	ItemID      string                 `json:"item_id"`
	Raw         map[string]interface{} `json:"-"`
}

type responsesStreamToolCall struct {
	index     int
	id        string
	callID    string
	name      string
	arguments strings.Builder
}

// NewOpenAIResponsesChatModel 创建 OpenAI Responses API 的 ChatModel 适配器。
// 对外仍实现 Eino BaseChatModel/ToolCallingChatModel，方便复用现有 ChatModelAgent。
func NewOpenAIResponsesChatModel(_ context.Context, cfg ChatModelConfig) (einomodel.BaseChatModel, error) {
	m := &openAIResponsesChatModel{
		model:      strings.TrimSpace(cfg.Model),
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: httpClientWithTimeout(cfg.Timeout),
	}
	if m.httpClient == nil {
		m.httpClient = &http.Client{Timeout: 10 * time.Minute}
	}
	if cfg.MaxTokens > 0 {
		m.maxTokens = &cfg.MaxTokens
	}
	if cfg.Temperature > 0 {
		temperature := float32(cfg.Temperature)
		m.temperature = &temperature
	}
	if cfg.TopP > 0 {
		topP := float32(cfg.TopP)
		m.topP = &topP
	}
	return m, nil
}

func (m *openAIResponsesChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	reqBody, err := m.buildRequest(input, false, opts...)
	if err != nil {
		return nil, err
	}
	var resp responsesObject
	if err := m.doJSON(ctx, reqBody, &resp); err != nil {
		return nil, err
	}
	return responseObjectToMessage(&resp), nil
}

func (m *openAIResponsesChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	reqBody, err := m.buildRequest(input, true, opts...)
	if err != nil {
		return nil, err
	}
	httpResp, err := m.do(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	sr, sw := schema.Pipe[*schema.Message](16)
	go func() {
		defer sw.Close()
		defer httpResp.Body.Close()
		readResponsesSSE(httpResp.Body, sw)
	}()
	return sr, nil
}

func (m *openAIResponsesChatModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	next := *m
	next.tools = append([]*schema.ToolInfo(nil), tools...)
	return &next, nil
}

func (m *openAIResponsesChatModel) buildRequest(input []*schema.Message, stream bool, opts ...einomodel.Option) (*responsesRequest, error) {
	base := &einomodel.Options{
		MaxTokens:   m.maxTokens,
		Temperature: m.temperature,
		TopP:        m.topP,
		Tools:       m.tools,
	}
	options := einomodel.GetCommonOptions(base, opts...)
	modelID := m.model
	if options.Model != nil && strings.TrimSpace(*options.Model) != "" {
		modelID = strings.TrimSpace(*options.Model)
	}
	items, err := responsesInputFromMessages(input)
	if err != nil {
		return nil, err
	}
	tools, err := responsesToolsFromToolInfos(options.Tools)
	if err != nil {
		return nil, err
	}
	req := &responsesRequest{
		Model:           modelID,
		Input:           items,
		Stream:          stream,
		Tools:           tools,
		MaxOutputTokens: options.MaxTokens,
		Temperature:     options.Temperature,
		TopP:            options.TopP,
	}
	if len(tools) > 0 {
		req.ToolChoice = responsesToolChoice(options.ToolChoice)
	}
	return req, nil
}

func (m *openAIResponsesChatModel) doJSON(ctx context.Context, body *responsesRequest, out any) error {
	resp, err := m.do(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode responses body failed: %w", err)
	}
	return nil
}

func (m *openAIResponsesChatModel) do(ctx context.Context, body *responsesRequest) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal responses request failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+openAIResponsesEndpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	if body.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return nil, fmt.Errorf("responses api request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
}

func responsesInputFromMessages(messages []*schema.Message) ([]responsesInputItem, error) {
	items := make([]responsesInputItem, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Tool {
			items = append(items, responsesInputItem{
				Type:    "function_call_output",
				CallID:  msg.ToolCallID,
				Content: msg.Content,
			})
			continue
		}
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			for _, call := range msg.ToolCalls {
				items = append(items, responsesInputItem{
					Type:      "function_call",
					CallID:    firstNonEmpty(call.ID, call.Function.Name),
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				})
			}
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" && len(msg.UserInputMultiContent) == 0 {
			continue
		}
		items = append(items, responsesInputItem{
			Type:    "message",
			Role:    string(msg.Role),
			Content: responsesContentFromMessage(msg),
		})
	}
	return items, nil
}

func responsesContentFromMessage(msg *schema.Message) any {
	if len(msg.UserInputMultiContent) == 0 {
		return []responsesContentItem{{
			Type: responsesInputTextType(msg.Role),
			Text: msg.Content,
		}}
	}
	parts := make([]responsesContentItem, 0, len(msg.UserInputMultiContent))
	for _, part := range msg.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			parts = append(parts, responsesContentItem{Type: "input_text", Text: part.Text})
		}
	}
	if len(parts) == 0 {
		return msg.Content
	}
	return parts
}

func responsesInputTextType(role schema.RoleType) string {
	if role == schema.Assistant {
		return "output_text"
	}
	return "input_text"
}

func responsesToolsFromToolInfos(infos []*schema.ToolInfo) ([]responsesTool, error) {
	if len(infos) == 0 {
		return nil, nil
	}
	tools := make([]responsesTool, 0, len(infos))
	for _, info := range infos {
		if info == nil || strings.TrimSpace(info.Name) == "" {
			continue
		}
		var params json.RawMessage
		if info.ParamsOneOf != nil {
			js, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("convert tool schema %s failed: %w", info.Name, err)
			}
			if js != nil {
				raw, err := json.Marshal(js)
				if err != nil {
					return nil, fmt.Errorf("marshal tool schema %s failed: %w", info.Name, err)
				}
				params = raw
			}
		}
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        info.Name,
			Description: info.Desc,
			Parameters:  params,
		})
	}
	return tools, nil
}

func responsesToolChoice(choice *schema.ToolChoice) any {
	if choice == nil {
		return "auto"
	}
	switch *choice {
	case schema.ToolChoiceForbidden:
		return "none"
	case schema.ToolChoiceForced:
		return "required"
	default:
		return "auto"
	}
}

func responseObjectToMessage(resp *responsesObject) *schema.Message {
	msg := &schema.Message{
		Role:         schema.Assistant,
		ResponseMeta: responseMetaFromResponses(resp),
	}
	if resp == nil {
		return msg
	}
	var content strings.Builder
	for i, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Text != "" {
					content.WriteString(part.Text)
				}
			}
		case "function_call":
			index := i
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				Index: &index,
				ID:    firstNonEmpty(item.CallID, item.ID, item.Name),
				Type:  "function",
				Function: schema.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}
	msg.Content = content.String()
	return msg
}

func responseMetaFromResponses(resp *responsesObject) *schema.ResponseMeta {
	if resp == nil {
		return nil
	}
	finishReason := strings.TrimSpace(resp.Status)
	if resp.Error != nil && resp.Error.Message != "" {
		finishReason = resp.Error.Message
	}
	return &schema.ResponseMeta{
		FinishReason: finishReason,
		Usage:        tokenUsageFromResponses(resp.Usage),
	}
}

func tokenUsageFromResponses(usage *responsesUsage) *schema.TokenUsage {
	if usage == nil {
		return nil
	}
	promptTokens := firstPositive(usage.InputTokens, usage.PromptTokens)
	completionTokens := firstPositive(usage.OutputTokens, usage.CompletionTokens)
	totalTokens := usage.TotalTokens
	if totalTokens <= 0 {
		totalTokens = promptTokens + completionTokens
	}
	out := &schema.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	if usage.InputTokensDetails != nil {
		out.PromptTokenDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
	}
	if usage.OutputTokensDetails != nil {
		out.CompletionTokensDetails.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}
	return out
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func readResponsesSSE(body io.Reader, sw *schema.StreamWriter[*schema.Message]) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventName string
	var data strings.Builder
	toolCalls := map[string]*responsesStreamToolCall{}

	flush := func() bool {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		if raw == "" || raw == "[DONE]" {
			eventName = ""
			return false
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return sw.Send(nil, fmt.Errorf("decode responses stream event failed: %w", err))
		}
		if event.Type == "" {
			event.Type = eventName
		}
		eventName = ""
		return handleResponsesStreamEvent(event, toolCalls, sw)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if flush() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data.Len() > 0 && flush() {
		return
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		sw.Send(nil, err)
	}
}

func handleResponsesStreamEvent(event responsesStreamEvent, toolCalls map[string]*responsesStreamToolCall, sw *schema.StreamWriter[*schema.Message]) bool {
	switch event.Type {
	case responsesEventTextDelta:
		if event.Delta == "" {
			return false
		}
		return sw.Send(&schema.Message{Role: schema.Assistant, Content: event.Delta}, nil)
	case responsesEventReasoningTextDelta:
		if event.Delta == "" {
			return false
		}
		return sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: event.Delta}, nil)
	case responsesEventOutputItemAdded:
		if event.Item == nil || event.Item.Type != "function_call" {
			return false
		}
		index := 0
		if event.OutputIndex != nil {
			index = *event.OutputIndex
		}
		key := firstNonEmpty(event.Item.ID, event.Item.CallID, fmt.Sprintf("%d:%s", index, event.Item.Name))
		toolCalls[key] = &responsesStreamToolCall{
			index:  index,
			id:     event.Item.ID,
			callID: event.Item.CallID,
			name:   event.Item.Name,
		}
	case responsesEventArgumentsDelta:
		call := responsesStreamCall(event, toolCalls)
		if call == nil {
			return false
		}
		call.arguments.WriteString(event.Delta)
		if event.Delta == "" {
			return false
		}
		index := call.index
		return sw.Send(&schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				Index: &index,
				ID:    firstNonEmpty(call.callID, call.id, call.name),
				Type:  "function",
				Function: schema.FunctionCall{
					Name:      call.name,
					Arguments: event.Delta,
				},
			}},
		}, nil)
	case responsesEventCompleted, responsesEventIncomplete, responsesEventFailed:
		if event.Response == nil {
			return false
		}
		return sw.Send(&schema.Message{
			Role:         schema.Assistant,
			ResponseMeta: responseMetaFromResponses(event.Response),
		}, nil)
	case responsesEventError:
		if event.Error == nil {
			return false
		}
		return sw.Send(nil, fmt.Errorf("responses stream error: %s", event.Error.Message))
	}
	return false
}

func responsesStreamCall(event responsesStreamEvent, toolCalls map[string]*responsesStreamToolCall) *responsesStreamToolCall {
	if event.ItemID != "" {
		if call := toolCalls[event.ItemID]; call != nil {
			return call
		}
	}
	if event.OutputIndex != nil {
		for _, call := range toolCalls {
			if call.index == *event.OutputIndex {
				return call
			}
		}
	}
	if len(toolCalls) == 1 {
		for _, call := range toolCalls {
			return call
		}
	}
	return nil
}
