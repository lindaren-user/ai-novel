package ai

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const summaryProtectedExtraKey = "ai_novel_summary_protected"

// newSummarizationMiddleware 创建上下文压缩中间件。
//
// Eino 的 summarization middleware 挂在 Agent 的 ChatModel 调用前，内部流程是：
// 1. BeforeModelRewriteState 拿到本次即将发给模型的 state.Messages 和 ToolInfos。
// 2. 通过 TokenCounter 估算上下文长度；超过 Trigger.ContextTokens 时才触发压缩。
// 3. 触发后调用 GenModelInput 构造“摘要模型”的输入，再用 Model.Generate 非流式生成一条摘要消息。
// 4. 最后调用 Finalize 用“系统消息/受保护消息 + 摘要消息”替换本次运行的消息列表。
//
// 这个中间件只改写当前这一次 Agent run 的内存态消息，不会写回 t_chat_messages，也不是长期记忆系统。
// 系统消息和业务层通过 markSummaryProtected 标记的消息不会进入摘要模型，但会原样保留给最终业务模型。
func newSummarizationMiddleware(ctx context.Context, runtime RuntimeModelConfig, cfg ModelRuntimeConfig) (adk.ChatModelAgentMiddleware, error) {
	if runtime.BaseModel == nil {
		return nil, nil
	}
	triggerTokens := SummarizationTriggerTokens(cfg.Provider, cfg.ModelID, cfg.ContextWindowTokens)
	if triggerTokens <= 0 {
		return nil, nil
	}
	return summarization.New(ctx, &summarization.Config{
		Model:        runtime.BaseModel, // 复用用户当前选择的模型
		ModelOptions: summaryModelOptions(runtime),
		TokenCounter: estimatedSummaryTokenCounter,
		Trigger: &summarization.TriggerCondition{ // 摘要触发条件
			ContextTokens: triggerTokens,
		},
		EmitInternalEvents: false,                         // 关闭 Eino 内部摘要事件，避免污染业务 SSE。
		UserInstruction:    summaryUserInstruction(),      // 替换 Eino 默认摘要任务说明；系统提示词仍使用 Eino 内部摘要系统提示词。
		GenModelInput:      compressibleSummaryModelInput, // 控制哪些历史交给摘要模型：只压缩普通聊天，跳过 system 和 protected 消息。
		Finalize:           protectedSummaryFinalize,
	})
}

// summaryModelOptions 返回摘要模型参数；摘要任务使用当前用户模型，但不继承正文输出上限。
func summaryModelOptions(runtime RuntimeModelConfig) []einomodel.Option {
	opts := make([]einomodel.Option, 0, 3)
	if runtime.Temperature != nil {
		opts = append(opts, einomodel.WithTemperature(*runtime.Temperature))
	}
	if runtime.TopP != nil {
		opts = append(opts, einomodel.WithTopP(*runtime.TopP))
	}
	if len(runtime.Stop) > 0 {
		opts = append(opts, einomodel.WithStop(runtime.Stop))
	}
	return opts
}

// summaryUserInstruction 约束摘要只压缩聊天讨论，不改写小说规划和系统约束。
func summaryUserInstruction() string {
	return `请把以上可压缩的用户与助手聊天历史整理为一份简洁但完整的中文上下文摘要。

要求：
1. 保留用户明确确认过的创作偏好、修改意见、约束和待办。
2. 保留最近问题与回答中的关键结论，方便后续继续对话。
3. 不要编造小说设定、卷规划、章节规划或正文内容。
4. 系统指令和业务层标记为受保护的上下文不在本次输入中，不能臆测补写。
5. 输出只写摘要正文，不使用 Markdown 表格。`
}

// compressibleSummaryModelInput 只把未受保护的普通聊天交给摘要模型。
func compressibleSummaryModelInput(ctx context.Context, sysInstruction, userInstruction adk.Message, originalMsgs []adk.Message) ([]adk.Message, error) {
	input := make([]adk.Message, 0, len(originalMsgs)+2)
	input = append(input, sysInstruction)
	for _, msg := range originalMsgs {
		if isSummaryProtectedMessage(msg) || msg.Role == schema.System {
			continue
		}
		input = append(input, msg)
	}
	input = append(input, userInstruction)
	return input, nil
}

// protectedSummaryFinalize 将系统消息和受保护业务消息原样保留，再追加普通聊天摘要。
func protectedSummaryFinalize(ctx context.Context, originalMessages []adk.Message, summary adk.Message) ([]adk.Message, error) {
	finalMessages := make([]adk.Message, 0, len(originalMessages)+1)
	for _, msg := range originalMessages {
		if msg.Role == schema.System || isSummaryProtectedMessage(msg) {
			finalMessages = append(finalMessages, msg)
		}
	}
	if summary != nil {
		finalMessages = append(finalMessages, summary)
	}
	return finalMessages, nil
}

// estimatedSummaryTokenCounter 使用轻量字符估算 token 数，并把工具定义纳入触发判断。
func estimatedSummaryTokenCounter(ctx context.Context, input *summarization.TokenCounterInput) (int, error) {
	if input == nil {
		return 0, nil
	}
	total := 0
	for _, msg := range input.Messages {
		total += estimateMessageTokens(msg)
	}
	for _, tool := range input.Tools {
		raw, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		total += estimateTextTokens(string(raw))
	}
	return total, nil
}

// estimateMessageTokens 估算单条消息的 token 数，用于摘要触发前的快速判断。
func estimateMessageTokens(msg adk.Message) int {
	if msg == nil {
		return 0
	}
	total := estimateTextTokens(msg.Content) + estimateTextTokens(msg.ReasoningContent)
	for _, part := range msg.UserInputMultiContent {
		total += estimateTextTokens(part.Text)
	}
	for _, part := range msg.AssistantGenMultiContent {
		total += estimateTextTokens(part.Text)
	}
	return total + 4
}

// estimateTextTokens 按中英文混合文本的保守比例估算 token 数。
func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	return runes/2 + 1
}

// markSummaryProtected 标记消息在上下文压缩中必须原样保留。
func markSummaryProtected(msg *schema.Message) {
	if msg == nil {
		return
	}
	if msg.Extra == nil {
		msg.Extra = map[string]any{}
	}
	msg.Extra[summaryProtectedExtraKey] = true
}

// isSummaryProtectedMessage 判断消息是否需要避开摘要压缩。
func isSummaryProtectedMessage(msg adk.Message) bool {
	if msg == nil || msg.Extra == nil {
		return false
	}
	protected, _ := msg.Extra[summaryProtectedExtraKey].(bool)
	return protected
}
