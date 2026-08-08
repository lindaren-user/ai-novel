package ai

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

var (
	ErrInvalidAgentDefinition = errors.New("invalid agent definition")
)

// Agents 管理通用 ChatModelAgent 的懒加载缓存，业务层负责传入具体 Agent 定义。
type Agents struct {
	mu     sync.RWMutex
	agents map[string]*adk.ChatModelAgent
}

// UserAgentKey 生成用户模型配置维度的助手缓存键。
func UserAgentKey(userID int64, modelKey string) string {
	if userID <= 0 || modelKey == "" {
		return defaultAgentKey
	}
	return fmt.Sprintf("user:%d:model:%s", userID, modelKey)
}

const defaultAgentKey = "default"

// agentForDefinition 读取或懒创建指定模型和 Agent 定义对应的通用助手。
func (ag *Agents) agentForDefinition(ctx context.Context, key string, chatModel model.BaseChatModel, definition AgentDefinition) (*adk.ChatModelAgent, error) {
	if chatModel == nil {
		return nil, errors.New("chat model is required")
	}
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Instruction) == "" {
		return nil, ErrInvalidAgentDefinition
	}

	cacheKey := key + ":agent:" + agentDefinitionHash(definition)
	ag.mu.RLock()
	agent := ag.agents[cacheKey]
	ag.mu.RUnlock()
	if agent != nil {
		return agent, nil
	}

	config := &adk.ChatModelAgentConfig{
		Name:        definition.Name,
		Description: definition.Description,
		Instruction: definition.Instruction,
		Model:       chatModel,
		Handlers:    definition.Middlewares,
	}
	if len(definition.Tools) > 0 {
		config.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: definition.Tools},
		}
	}
	created, err := adk.NewChatModelAgent(ctx, config)
	if err != nil {
		return nil, err
	}

	ag.mu.Lock()
	defer ag.mu.Unlock()
	if agent = ag.agents[cacheKey]; agent != nil {
		return agent, nil
	}
	ag.agents[cacheKey] = created
	return created, nil
}

// agentDefinitionHash 生成 Agent 定义缓存摘要，避免不同提示词或工具集合共用同一实例。
func agentDefinitionHash(definition AgentDefinition) string {
	h := sha1.New()
	_, _ = h.Write([]byte(definition.Name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(definition.Description))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(definition.CacheKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(definition.Instruction))
	_, _ = h.Write([]byte(fmt.Sprintf(":tools:%d:middlewares:%d", len(definition.Tools), len(definition.Middlewares))))
	return hex.EncodeToString(h.Sum(nil))
}

// StreamAgent 使用业务层传入的通用 Agent 定义执行流式对话，并只转发最终可见助手文本。
func (ag *Agents) StreamAgent(ctx context.Context, req AgentStreamRequest) (<-chan StreamEvent, error) {
	trace := newAgentDebugTrace("stream", req.UserID, req.ModelKey, req.Agent)
	trace.start(len(req.Messages))
	runtimeModel, err := runtimeModelConfig(ctx, req.Model)
	if err != nil {
		trace.error("model_config", err)
		return nil, fmt.Errorf("构建运行时模型失败: %w", err)
	}
	var runOpts []adk.AgentRunOption
	if modelOpts := runtimeModel.ModelOptions(); len(modelOpts) > 0 {
		runOpts = append(runOpts, adk.WithChatModelOptions(modelOpts))
	}

	agentDefinition := req.Agent
	if summaryMiddleware, err := newSummarizationMiddleware(ctx, runtimeModel, req.Model); err != nil {
		trace.error("summarization", err)
		return nil, fmt.Errorf("创建上下文压缩中间件失败: %w", err)
	} else if summaryMiddleware != nil {
		agentDefinition.Middlewares = append([]adk.ChatModelAgentMiddleware{summaryMiddleware}, agentDefinition.Middlewares...)
	}

	agent, err := ag.agentForDefinition(ctx, UserAgentKey(req.UserID, req.ModelKey), runtimeModel.BaseModel, agentDefinition)
	if err != nil {
		trace.error("agent_create", err)
		return nil, fmt.Errorf("创建 agent 失败: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer func() {
			if panicErr := recover(); panicErr != nil {
				trace.panic(panicErr)
				zap.L().Error("agent stream panic",
					zap.Int64("user_id", req.UserID),
					zap.String("model_key", req.ModelKey),
					zap.String("agent", req.Agent.Name),
					zap.Any("panic", panicErr),
				)
				ch <- NewStreamError(fmt.Sprintf("agent stream panic: %v", panicErr))
			}
		}()

		iter := agent.Run(ctx, &adk.AgentInput{
			Messages:        streamMessagesToSchema(req.Messages),
			EnableStreaming: true,
		}, runOpts...)

		var totalTokenCount int64
		var finishReason string
		step := 0

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			step++

			if event.Err != nil {
				trace.errorStep(step, "event_error", event.Err)
				zap.L().Error("agent stream event error",
					zap.Int64("user_id", req.UserID),
					zap.String("model_key", req.ModelKey),
					zap.String("agent", req.Agent.Name),
					zap.Int("step", step),
					zap.Error(event.Err),
				)
				ch <- NewStreamError(event.Err.Error())
				return
			}

			if event.Output == nil || event.Output.MessageOutput == nil {
				trace.emptyStep(step)
				continue
			}

			messageOutput := event.Output.MessageOutput
			trace.messageStep(step, messageOutput)
			if toolEvent, ok := toolResultStreamEvent(messageOutput.Message); ok {
				ch <- toolEvent
				continue
			}
			if toolEvent, ok := toolCallStreamEvent(messageOutput.Message); ok {
				ch <- toolEvent
				if messageOutput.Message != nil && messageOutput.Message.Content == "" {
					continue
				}
			}
			if !isAssistantMessageOutput(messageOutput) {
				continue
			}
			if messageOutput.IsStreaming {
				tokenCount, streamFinishReason, err := streamMessageDeltas(ctx, ch, messageOutput.MessageStream, trace)
				if err != nil {
					totalTokenCount += tokenCount
					finishReason = firstNonEmpty(streamFinishReason, finishReason)
					trace.errorStep(step, "stream_recv", err)
					if ctx.Err() != nil {
						zap.L().Debug("agent stream receive canceled",
							zap.Int64("user_id", req.UserID),
							zap.String("model_key", req.ModelKey),
							zap.String("agent", req.Agent.Name),
							zap.Int("step", step),
							zap.Error(err),
						)
					} else {
						zap.L().Error("agent stream receive error",
							zap.Int64("user_id", req.UserID),
							zap.String("model_key", req.ModelKey),
							zap.String("agent", req.Agent.Name),
							zap.Int("step", step),
							zap.Error(err),
						)
					}
					ch <- NewStreamErrorWithMeta(err.Error(), totalTokenCount, finishReason)
					return
				}
				totalTokenCount += tokenCount
				finishReason = firstNonEmpty(streamFinishReason, finishReason)
				continue
			}

			totalTokenCount += tokenCountFromMessage(messageOutput.Message)
			finishReason = firstNonEmpty(finishReasonFromMessage(messageOutput.Message), finishReason)
			if messageOutput.Message != nil && messageOutput.Message.Content != "" {
				content := sanitizeVisibleAgentContent(messageOutput.Message.Content)
				if content != "" {
					emitVisibleContentDeltas(ctx, ch, content)
				}
			}
		}

		trace.done(totalTokenCount, finishReason)
		ch <- NewStreamDone(totalTokenCount, finishReason, 0)
	}()

	return ch, nil
}

// StreamChat 直接调用 ChatModel 的原生流式接口，不经过 Agent 和工具编排。
func (ag *Agents) StreamChat(ctx context.Context, req ChatGenerateRequest) (<-chan StreamEvent, error) {
	trace := newAgentDebugTrace("chat_stream", req.UserID, req.ModelKey, AgentDefinition{Name: "chat_model"})
	trace.start(len(req.Messages))
	runtimeModel, err := runtimeModelConfig(ctx, req.Model)
	if err != nil {
		trace.error("model_config", err)
		return nil, fmt.Errorf("构建运行时模型失败: %w", err)
	}
	stream, err := runtimeModel.BaseModel.Stream(ctx, streamMessagesToSchema(req.Messages), runtimeModel.ModelOptions()...)
	if err != nil {
		trace.error("stream", err)
		return nil, fmt.Errorf("创建聊天流失败: %w", err)
	}
	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer func() {
			if panicErr := recover(); panicErr != nil {
				trace.panic(panicErr)
				zap.L().Error("chat stream panic",
					zap.Int64("user_id", req.UserID),
					zap.String("model_key", req.ModelKey),
					zap.Any("panic", panicErr),
				)
				ch <- NewStreamError(fmt.Sprintf("chat stream panic: %v", panicErr))
			}
		}()
		tokenCount, finishReason, err := streamMessageDeltas(ctx, ch, stream, trace)
		if err != nil {
			trace.error("stream_recv", err)
			if ctx.Err() != nil {
				zap.L().Debug("chat stream receive canceled",
					zap.Int64("user_id", req.UserID),
					zap.String("model_key", req.ModelKey),
					zap.Error(err),
				)
			} else {
				zap.L().Error("chat stream receive error",
					zap.Int64("user_id", req.UserID),
					zap.String("model_key", req.ModelKey),
					zap.Error(err),
				)
			}
			ch <- NewStreamErrorWithMeta(err.Error(), tokenCount, finishReason)
			return
		}
		trace.done(tokenCount, finishReason)
		ch <- NewStreamDone(tokenCount, finishReason, 0)
	}()
	return ch, nil
}

// toolCallStreamEvent 将 Eino 工具调用消息转为通用流事件，业务层可据此提前展示加载态。
func toolCallStreamEvent(msg *schema.Message) (StreamEvent, bool) {
	if msg == nil || msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
		return StreamEvent{}, false
	}
	for _, call := range msg.ToolCalls {
		if call.Function.Name != "" {
			return NewStreamToolCall(call.Function.Name), true
		}
	}
	return StreamEvent{}, false
}

// toolResultStreamEvent 将 Eino 工具结果消息转为通用流事件，交给业务层决定是否展示。
func toolResultStreamEvent(msg *schema.Message) (StreamEvent, bool) {
	if msg == nil || msg.Role != schema.Tool {
		return StreamEvent{}, false
	}
	return NewStreamToolResult(msg.ToolName, msg.Content), true
}

// GenerateChat 直接调用 ChatModel 生成一次非流式回复，适合后台总结等不需要 Agent 的任务。
func (ag *Agents) GenerateChat(ctx context.Context, req ChatGenerateRequest) (GenerateResult, error) {
	trace := newAgentDebugTrace("chat", req.UserID, req.ModelKey, AgentDefinition{Name: "chat_model"})
	trace.start(len(req.Messages))
	runtimeModel, err := runtimeModelConfig(ctx, req.Model)
	if err != nil {
		trace.error("model_config", err)
		return GenerateResult{}, fmt.Errorf("构建运行时模型失败: %w", err)
	}

	response, err := runtimeModel.BaseModel.Generate(ctx, streamMessagesToSchema(req.Messages), runtimeModel.ModelOptions()...)
	if err != nil {
		trace.error("generate", err)
		return GenerateResult{}, fmt.Errorf("生成聊天回复失败: %w", err)
	}
	if response == nil {
		trace.done(0, "")
		return GenerateResult{}, nil
	}
	tokenCount := tokenCountFromMessage(response)
	finishReason := finishReasonFromMessage(response)
	trace.message("chat_response", response)
	trace.done(tokenCount, finishReason)
	return GenerateResult{
		Content:      sanitizeVisibleAgentContent(response.Content),
		TokenCount:   tokenCount,
		FinishReason: finishReason,
	}, nil
}

// GenerateAgent 使用业务层传入的单个 Agent 定义执行一次非流式任务。
func (ag *Agents) GenerateAgent(ctx context.Context, req AgentGenerateRequest) (GenerateResult, error) {
	trace := newAgentDebugTrace("agent", req.UserID, req.ModelKey, req.Agent)
	trace.start(len(req.Messages))
	runtimeModel, err := runtimeModelConfig(ctx, req.Model)
	if err != nil {
		trace.error("model_config", err)
		return GenerateResult{}, fmt.Errorf("构建运行时模型失败: %w", err)
	}
	agentDefinition := req.Agent
	agent, err := ag.agentForDefinition(ctx, UserAgentKey(req.UserID, req.ModelKey), runtimeModel.BaseModel, agentDefinition)
	if err != nil {
		trace.error("agent_create", err)
		return GenerateResult{}, fmt.Errorf("创建 agent 失败: %w", err)
	}
	var runOpts []adk.AgentRunOption
	if modelOpts := runtimeModel.ModelOptions(); len(modelOpts) > 0 {
		runOpts = append(runOpts, adk.WithChatModelOptions(modelOpts))
	}
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:        streamMessagesToSchema(req.Messages),
		EnableStreaming: false,
	}, runOpts...)

	var final strings.Builder
	var tokenCount int64
	var finishReason string
	step := 0
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		step++
		if event.Err != nil {
			trace.errorStep(step, "event_error", event.Err)
			return GenerateResult{}, fmt.Errorf("agent 执行失败: %w", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			trace.emptyStep(step)
			continue
		}
		messageOutput := event.Output.MessageOutput
		trace.messageStep(step, messageOutput)
		if !isAssistantMessageOutput(messageOutput) || messageOutput.Message == nil {
			continue
		}
		tokenCount += tokenCountFromMessage(messageOutput.Message)
		finishReason = firstNonEmpty(finishReasonFromMessage(messageOutput.Message), finishReason)
		if content := sanitizeVisibleAgentContent(messageOutput.Message.Content); content != "" {
			final.Reset()
			final.WriteString(content)
		}
	}
	trace.done(tokenCount, finishReason)
	return GenerateResult{
		Content:      final.String(),
		TokenCount:   tokenCount,
		FinishReason: finishReason,
	}, nil
}

// streamMessagesToSchema 将内部消息结构转换为 Eino schema 消息。
func streamMessagesToSchema(messages []StreamMessage) []*schema.Message {
	result := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		schemaMsg := &schema.Message{
			Role:    schemaRole(msg.Role),
			Content: msg.Content,
		}
		if msg.Protected {
			markSummaryProtected(schemaMsg)
		}
		result = append(result, schemaMsg)
	}
	return result
}

// schemaRole 将内部字符串角色映射为 Eino schema 角色。
func schemaRole(role string) schema.RoleType {
	switch role {
	case "assistant":
		return schema.Assistant
	case "system":
		return schema.System
	default:
		return schema.User
	}
}

// streamMessageDeltas 消费一段 Eino 消息流，把用户可见文本转成内部 delta 事件。
//
// 流消息可能夹带 reasoning、tool_call、token 用量和结束原因；这里只转发用户可见文本，
// 并对重复前缀做防护，避免上游或中间层重复吐出已发送内容。
func streamMessageDeltas(ctx context.Context, ch chan<- StreamEvent, stream adk.MessageStream, trace agentDebugTrace) (int64, string, error) {
	if stream == nil {
		return 0, "", nil
	}
	defer stream.Close()

	// sent 记录已发送的可见文本，用于过滤重复前缀。
	var sent strings.Builder
	// reasoning 只进调试日志，不进入用户可见流。
	var sentReasoning strings.Builder
	var pendingReasoning strings.Builder
	var tokenCount int64
	var finishReason string
	// 同一段流里工具调用消息可能重复出现，只通知业务层一次。
	sentToolCalls := map[string]struct{}{}
	flushReasoning := func(force bool) {
		if pendingReasoning.Len() == 0 {
			return
		}
		complete, rest := completeReasoningSegments(pendingReasoning.String())
		if force && strings.TrimSpace(rest) != "" {
			complete += rest
			rest = ""
		}
		if strings.TrimSpace(complete) != "" {
			trace.reasoning("reasoning", complete)
		}
		pendingReasoning.Reset()
		pendingReasoning.WriteString(rest)
	}
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// 流正常结束前，把剩余 reasoning 写入调试日志。
			flushReasoning(true)
			return tokenCount, finishReason, nil
		}
		if err != nil {
			flushReasoning(true)
			return tokenCount, finishReason, err
		}
		// 部分模型适配器会在后续或末尾的空消息里返回 usage / finishReason，逐帧更新以保留最终元数据。
		if usage := tokenCountFromMessage(msg); usage > tokenCount {
			tokenCount = usage
		}
		finishReason = firstNonEmpty(finishReasonFromMessage(msg), finishReason)
		if msg != nil {
			if reasoning := reasoningContentFromMessage(msg); reasoning != "" {
				reasoningDelta := reasoning
				currentReasoning := sentReasoning.String()
				if strings.HasPrefix(reasoning, currentReasoning) {
					reasoningDelta = strings.TrimPrefix(reasoning, currentReasoning)
				}
				if strings.TrimSpace(reasoningDelta) != "" {
					sentReasoning.WriteString(reasoningDelta)
					pendingReasoning.WriteString(reasoningDelta)
					flushReasoning(false)
				}
			}
			// 工具调用事件要尽早透出，业务层用它展示占位卡片。
			for _, call := range msg.ToolCalls {
				name := call.Function.Name
				if name == "" {
					continue
				}
				if _, ok := sentToolCalls[name]; ok {
					continue
				}
				sentToolCalls[name] = struct{}{}
				trace.toolCall(0, msg)
				ch <- NewStreamToolCall(name)
			}
		}
		if msg == nil || msg.Content == "" {
			continue
		}

		content := sanitizeVisibleAgentStreamContent(msg.Content)
		if content == "" {
			continue
		}
		delta := content
		current := sent.String()
		// 过滤已发送前缀，避免重复内容进入前端消息。
		if strings.HasPrefix(content, current) {
			delta = strings.TrimPrefix(content, current)
		}
		if delta == "" {
			continue
		}
		sent.WriteString(delta)
		emitVisibleContentDeltas(ctx, ch, delta)
	}
}

const minReasoningDebugRunes = 200

// completeReasoningSegments 从深度思考缓冲区中切出较完整的思考段落，避免逐 token 刷调试日志。
func completeReasoningSegments(content string) (string, string) {
	lastEnd := -1
	runeCount := 0
	for index, ch := range content {
		runeCount++
		if ch == '\n' || (runeCount >= minReasoningDebugRunes && isReasoningSegmentEnd(ch)) {
			lastEnd = index + len(string(ch))
			runeCount = 0
		}
	}
	if lastEnd < 0 {
		return "", content
	}
	return content[:lastEnd], content[lastEnd:]
}

// isReasoningSegmentEnd 判断字符是否可以作为一段调试思考的自然结束点。
func isReasoningSegmentEnd(ch rune) bool {
	switch ch {
	case '。', '！', '？', '；', '!', '?', ';', '\n':
		return true
	default:
		return false
	}
}

// emitVisibleContentDeltas 发送可见助手文本，普通问答首段内容会立即到达前端。
func emitVisibleContentDeltas(ctx context.Context, ch chan<- StreamEvent, content string) {
	if content == "" {
		return
	}
	select {
	case <-ctx.Done():
		return
	case ch <- NewStreamDelta(content):
	}
}

// isAssistantMessageOutput 判断 ADK 事件是否是最终可展示的助手输出，避免工具结果泄露到 SSE。
func isAssistantMessageOutput(messageOutput *adk.MessageVariant) bool {
	if messageOutput == nil {
		return false
	}
	if messageOutput.Role != "" {
		return messageOutput.Role == schema.Assistant
	}
	if messageOutput.Message != nil {
		return messageOutput.Message.Role == schema.Assistant
	}
	return true
}

// tokenCountFromMessage 从 Eino 消息元信息中读取模型返回的总 token 数。
func tokenCountFromMessage(msg *schema.Message) int64 {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return 0
	}
	return int64(msg.ResponseMeta.Usage.TotalTokens)
}

// finishReasonFromMessage 从 Eino 消息元信息中读取模型结束原因。
func finishReasonFromMessage(msg *schema.Message) string {
	if msg == nil || msg.ResponseMeta == nil {
		return ""
	}
	return strings.TrimSpace(msg.ResponseMeta.FinishReason)
}

// firstNonEmpty 返回第一个非空字符串，用于保留最新可用的模型结束原因。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// sanitizeVisibleAgentContent 清理不应展示给用户的工具结果残留。
func sanitizeVisibleAgentContent(content string) string {
	return strings.TrimSpace(stripToolResultJSON(content))
}

// sanitizeVisibleAgentStreamContent 清理流式片段中的工具结果残留，并保留正文换行和空白。
func sanitizeVisibleAgentStreamContent(content string) string {
	return stripToolResultJSON(content)
}

// stripToolResultJSON 删除模型误输出的结构化工具保存结果 JSON。
func stripToolResultJSON(content string) string {
	var out strings.Builder
	for i := 0; i < len(content); {
		if content[i] != '{' {
			out.WriteByte(content[i])
			i++
			continue
		}

		end := matchingJSONObjectEnd(content, i)
		if end < 0 {
			out.WriteByte(content[i])
			i++
			continue
		}

		candidate := content[i : end+1]
		if looksLikePersistenceToolResult(candidate) || looksLikePresentationToolResult(candidate) {
			i = end + 1
			continue
		}

		out.WriteString(candidate)
		i = end + 1
	}
	return out.String()
}

// matchingJSONObjectEnd 返回从 start 开始的完整 JSON 对象结束位置。
func matchingJSONObjectEnd(content string, start int) int {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// looksLikePersistenceToolResult 判断 JSON 是否像业务保存工具结果，避免工具回执污染用户可见回复。
func looksLikePersistenceToolResult(content string) bool {
	return strings.Contains(content, `"saved_count"`) &&
		strings.Contains(content, `"message"`)
}

// looksLikePresentationToolResult 判断 JSON 是否像前端展示工具结果，避免模型复述 UI 数据。
func looksLikePresentationToolResult(content string) bool {
	return strings.Contains(content, `"kind"`) &&
		(strings.Contains(content, `"plan_options"`) || strings.Contains(content, `"chapter_draft"`))
}

// runtimeModelConfig 根据用户运行时模型配置创建 Eino 模型和调用参数。
func runtimeModelConfig(ctx context.Context, cfg ModelRuntimeConfig) (RuntimeModelConfig, error) {
	baseModel, err := NewChatModel(ctx, ChatModelConfig{
		Provider:  cfg.Provider,
		Model:     cfg.ModelID,
		APIURL:    cfg.APIURL,
		APIKey:    cfg.APIKey,
		MaxTokens: cfg.MaxTokens,
	})
	if err != nil {
		return RuntimeModelConfig{}, err
	}

	runtime := RuntimeModelConfig{
		BaseModel: baseModel,
		Stop:      cfg.Stop,
	}
	if cfg.Temperature > 0 {
		temperature := float32(cfg.Temperature)
		runtime.Temperature = &temperature
	}
	if cfg.TopP > 0 {
		topP := float32(cfg.TopP)
		runtime.TopP = &topP
	}
	if cfg.MaxTokens > 0 {
		runtime.MaxTokens = &cfg.MaxTokens
	}
	return runtime, nil
}
