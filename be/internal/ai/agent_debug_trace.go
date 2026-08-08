package ai

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// agentDebugTrace 负责在 debug 模式下输出简洁的 agent 执行轨迹。
type agentDebugTrace struct {
	enabled     bool
	kind        string
	userID      int64
	modelKey    string
	agent       string
	tools       int
	middlewares int
}

// newAgentDebugTrace 根据全局日志级别创建调试轨迹；非 debug 级别时所有方法都是空操作。
func newAgentDebugTrace(kind string, userID int64, modelKey string, definition AgentDefinition) agentDebugTrace {
	return agentDebugTrace{
		enabled:     zap.L().Core().Enabled(zap.DebugLevel),
		kind:        kind,
		userID:      userID,
		modelKey:    modelKey,
		agent:       firstNonEmpty(definition.Name, "unknown"),
		tools:       len(definition.Tools),
		middlewares: len(definition.Middlewares),
	}
}

// start 输出 agent 本轮执行的入口信息。
func (t agentDebugTrace) start(messageCount int) {
	t.event("start",
		zap.Int("messages", messageCount),
		zap.Int("tools", t.tools),
		zap.Int("middlewares", t.middlewares),
	)
}

// done 输出 agent 本轮执行的结束信息。
func (t agentDebugTrace) done(tokenCount int64, finishReason string) {
	t.event("done",
		zap.Int64("tokens", tokenCount),
		zap.String("finish", finishReason),
	)
}

// error 输出 agent 本轮执行的阶段错误。
func (t agentDebugTrace) error(stage string, err error) {
	if err == nil {
		return
	}
	t.event("error", zap.String("stage", stage), zap.String("error", err.Error()))
}

// errorStep 输出 agent 事件循环中某一步的错误。
func (t agentDebugTrace) errorStep(step int, stage string, err error) {
	if err == nil {
		return
	}
	t.event("error",
		zap.Int("step", step),
		zap.String("stage", stage),
		zap.String("error", err.Error()),
	)
}

// panic 输出 agent goroutine 中恢复到的 panic。
func (t agentDebugTrace) panic(panicErr any) {
	t.event("panic", zap.Any("panic", panicErr))
}

// emptyStep 输出没有消息输出的 ADK 事件。
func (t agentDebugTrace) emptyStep(step int) {
}

// messageStep 输出 ADK 事件中的中间过程，不记录最终正文内容。
func (t agentDebugTrace) messageStep(step int, output *adk.MessageVariant) {
	if !t.enabled {
		return
	}
	if output == nil {
		return
	}
	if output.Message == nil {
		return
	}

	msg := output.Message
	if !output.IsStreaming {
		if reasoning := reasoningContentFromMessage(msg); reasoning != "" {
			t.reasoning("reasoning", reasoning)
		}
	}
	if len(msg.ToolCalls) > 0 {
		t.toolCall(step, msg)
	}
	if msg.Role == schema.Tool {
		t.toolResult(step, msg)
	}
}

// message 输出非流式消息里的思考过程或工具过程，不记录最终正文内容。
func (t agentDebugTrace) message(label string, msg *schema.Message) {
	if msg == nil {
		return
	}
	if reasoning := reasoningContentFromMessage(msg); reasoning != "" {
		t.reasoning(label+"_reasoning", reasoning)
	}
	if len(msg.ToolCalls) > 0 {
		t.toolCall(0, msg)
	}
	if msg.Role == schema.Tool {
		t.toolResult(0, msg)
	}
}

// reasoning 输出模型深度思考内容摘要，只在模型实际返回时出现。
func (t agentDebugTrace) reasoning(event string, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	t.event(event,
		zap.Int("reasoning_chars", len([]rune(content))),
		zap.String("reasoning", compactSnippet(content, 0)),
	)
}

// toolCall 输出模型准备调用的工具名，方便观察 agent 中间动作。
func (t agentDebugTrace) toolCall(step int, msg *schema.Message) {
	t.event("tool_call",
		zap.Int("step", step),
		zap.Strings("tools", toolCallNames(msg)),
	)
}

// toolResult 输出工具执行结果摘要，避免完整大 JSON 或正文刷屏。
func (t agentDebugTrace) toolResult(step int, msg *schema.Message) {
	t.event("tool_result",
		zap.Int("step", step),
		zap.String("tool", msg.ToolName),
		zap.Int("result_chars", len([]rune(msg.Content))),
		zap.String("result", compactSnippet(msg.Content, 220)),
	)
}

// event 按统一字段输出一条 agent 调试日志。
func (t agentDebugTrace) event(event string, fields ...zap.Field) {
	if !t.enabled {
		return
	}
	base := []zap.Field{
		zap.String("event", event),
		zap.String("kind", t.kind),
		zap.String("agent", t.agent),
		// zap.Int64("user_id", t.userID),
		// zap.String("model_key", t.modelKey),
	}
	zap.L().Debug("agent trace", append(base, fields...)...)
}

// reasoningContentFromMessage 从标准字段或模型适配 Extra 中提取深度思考内容。
func reasoningContentFromMessage(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.ReasoningContent) != "" {
		return msg.ReasoningContent
	}
	for key, value := range msg.Extra {
		if !strings.Contains(strings.ToLower(key), "reasoning") {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case []byte:
			text := string(v)
			if strings.TrimSpace(text) != "" {
				return text
			}
		default:
			text := fmt.Sprint(v)
			if strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

// toolCallNames 返回消息中的工具调用名称列表。
func toolCallNames(msg *schema.Message) []string {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	names := make([]string, 0, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		if call.Function.Name != "" {
			names = append(names, call.Function.Name)
		}
	}
	return names
}

// compactSnippet 压缩多余空白并截断长文本，保证 debug 日志可读且不刷屏。
func compactSnippet(content string, limit int) string {
	text := strings.Join(strings.Fields(content), " ")
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}
