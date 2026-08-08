package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/crypto"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/cloudwego/eino/adk"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const chatRoleUser int16 = model.ChatRoleUser
const chatRoleAssistant int16 = model.ChatRoleAssistant
const streamingReplyCacheTTL = 5 * time.Minute // 不能太短，否则长思考会把这个快照清空
const membershipFeatureTTL = 24 * time.Hour
const defaultMaxConcurrentStreams = 3
const streamFinishReasonCanceled = "canceled"
const streamFinishReasonServerShutdown = "server_shutdown"

type effectiveModelConfig struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"userId"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ModelID     string  `json:"modelId"`
	APIURL      string  `json:"apiUrl"`
	APIKey      string  `json:"apiKey"`
	TopP        float64 `json:"topP"`
	Temperature float64 `json:"temperature"`
	UpdatedAt   int64   `json:"updatedAt"`
}

type streamingReplySnapshot struct {
	SessionID        int64         `json:"sessionId"`
	ScopeType        int16         `json:"scopeType"`
	ScopeID          int64         `json:"scopeId"`
	ModelRunID       int64         `json:"modelRunId,omitempty"`
	AssistantContent string        `json:"assistantContent"`
	RenderData       model.JSONMap `json:"renderData"`
	StartedAt        time.Time     `json:"startedAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
}

type aiStreamSupport struct {
	repositories   repo.Repositories
	redisClient    *redis.Client
	aiClient       ai.Client
	streamHub      *streamHub
	chapterSkill   adk.ChatModelAgentMiddleware
	storyEditSkill adk.ChatModelAgentMiddleware
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

type streamScopeParams struct {
	UserID                   int64
	ScopeID                  int64
	ScopeType                model.ScopeType
	SessionTitle             string
	ModelConfigID            int64
	Content                  string
	SystemContext            string
	InitialAssistantMessage  string
	ChapterSkill             adk.ChatModelAgentMiddleware
	ChapterWritingMode       chapterWritingPromptMode
	ChapterDraftToolDisabled bool
	SaveChapterDrafts        func(ctx context.Context, repositories repo.Repositories, sessionID int64, assistantMessageID int64, renderData model.JSONMap) (int64, error)
	HistoryFilter            func([]model.ChatMessage) []model.ChatMessage
}

type modelRunMeta struct {
	ID           int64
	UserID       int64
	ScopeType    int16
	ScopeID      *int64
	ModelID      int64
	Status       int16
	TokenCount   int64
	FinishReason string
	ErrorMessage string
	StartTime    time.Time
	EndTime      *time.Time
}

// newAIStreamSupport 创建 AI 流共享依赖，供小说、卷、章服务复用。
func newAIStreamSupport(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client) *aiStreamSupport {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	chapterSkill, err := newChapterSkillsMiddleware(context.Background())
	if err != nil {
		// 其实最好直接 panic
		zap.L().Error("init chapter skills middleware failed", zap.Error(err))
	}
	storyEditSkill, err := newStoryEditSkillsMiddleware(context.Background())
	if err != nil {
		zap.L().Error("init story edit skills middleware failed", zap.Error(err))
	}
	return &aiStreamSupport{
		repositories:   repositories,
		redisClient:    redisClient,
		aiClient:       aiClient,
		streamHub:      newStreamHub(),
		chapterSkill:   chapterSkill,
		storyEditSkill: storyEditSkill,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}
}

// streamScoped 发起指定层级 AI 流，并在调用模型前先持久化用户消息。
func (s *aiStreamSupport) streamScoped(ctx context.Context, params streamScopeParams) (<-chan ai.StreamEvent, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" {
		return nil, ErrInvalidMessage
	}
	if s.aiClient == nil {
		return nil, ErrAIUnavailable
	}

	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, params.UserID, int16(params.ScopeType), params.ScopeID)
	sessionID := session.ID
	if errors.Is(err, repo.ErrChatSessionNotFound) {
		sessionID, err = s.repositories.ChatSessions.Create(ctx, params.UserID, int16(params.ScopeType), params.ScopeID, params.SessionTitle)
		if err == nil && strings.TrimSpace(params.InitialAssistantMessage) != "" {
			_, err = createChatMessage(ctx, s.repositories, sessionID, chatRoleAssistant, params.InitialAssistantMessage)
		}
	}
	if err != nil {
		return nil, wrapError("准备对话会话失败", err)
	}
	if stream, ok := s.streamHub.subscribe(ctx, sessionID); ok {
		return stream, nil
	}

	history, err := s.repositories.ChatMessages.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, wrapError("查询对话历史失败", err)
	}
	if params.HistoryFilter != nil {
		history = params.HistoryFilter(history)
	}
	params.ChapterSkill = s.chapterSkill

	messages := make([]ai.StreamMessage, 0, len(history))
	if strings.TrimSpace(params.SystemContext) != "" {
		messages = append(messages, ai.StreamMessage{
			Role:      "system",
			Content:   params.SystemContext,
			Protected: true,
		})
	}
	for i, msg := range history {
		historyContent := strings.TrimSpace(msg.Content)
		if historyContent == "" {
			continue
		}
		messages = append(messages, ai.StreamMessage{
			Role:      msg.Role,
			Content:   historyContent,
			Protected: i == 0 || isProtectedHistoryMessage(msg),
		})
	}
	messages = append(messages, ai.StreamMessage{
		Role:    "user",
		Content: content,
	})

	modelConfig, err := s.resolveUserModelConfig(ctx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("解析用户模型配置失败: %w", err)
	}
	params.ModelConfigID = modelConfig.ID
	agentDefinition, err := buildStreamAgentDefinition(params)
	if err != nil {
		return nil, fmt.Errorf("构建 AI 助手定义失败: %w", err)
	}

	maxConcurrentStreams, err := s.resolveMaxConcurrentStreams(ctx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("解析用户并发配置失败: %w", err)
	}
	jobCtx, started, err := s.streamHub.startJob(sessionID, params.UserID, maxConcurrentStreams)
	if err != nil {
		return nil, wrapError("启动 AI 任务失败", err)
	}
	if !started {
		if stream, ok := s.streamHub.subscribe(ctx, sessionID); ok {
			return stream, nil
		}
	}
	if _, err := createChatMessage(ctx, s.repositories, sessionID, chatRoleUser, content); err != nil {
		s.streamHub.finishJob(sessionID)
		return nil, wrapError("保存用户消息失败", err)
	}
	runStartedAt := time.Now()
	runID := s.createModelRun(context.Background(), modelRunMeta{
		UserID:    params.UserID,
		ScopeType: model.ModelRunScopeMessage,
		ModelID:   modelConfig.ID,
		Status:    model.ModelRunStatusRunning,
		StartTime: runStartedAt,
	})
	if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
		SessionID:  sessionID,
		ScopeType:  int16(params.ScopeType),
		ScopeID:    params.ScopeID,
		ModelRunID: runID,
		StartedAt:  runStartedAt,
		UpdatedAt:  runStartedAt,
	}); err != nil {
		zap.L().Error("cache initial streaming reply failed",
			zap.Int64("user_id", params.UserID),
			zap.Int64("session_id", sessionID),
			zap.Error(err),
		)
	}
	aiStream, err := s.aiClient.StreamAgent(jobCtx, ai.AgentStreamRequest{ // 注意是 jobCtx
		UserID:   params.UserID,
		ModelKey: modelConfig.AgentKey(),
		Model:    modelConfig.RuntimeConfig(),
		Messages: messages,
		Agent:    agentDefinition,
	})
	if err != nil {
		s.streamHub.finishJob(sessionID)
		s.deleteStreamingReply(context.Background(), sessionID)
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusFailed,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})

		if errors.Is(err, ai.ErrInvalidChatModelConfig) {
			return nil, ErrInvalidModel
		}
		return nil, fmt.Errorf("启动 AI 流失败: %w", err)
	}

	stream, _ := s.streamHub.subscribe(ctx, sessionID)       // 这里传递 ctx，前端刷新会导致 stream ch 被 close，但是对应的 job 还存在。
	go s.forwardAIStream(aiStream, params, sessionID, runID) // 这里是没有传递 ctx，所以 AI 回复的时候前端页面刷新是不会影响 AI 回复的，但是不会再写入 steam ch（nil），只是单独写入 redis 快照。
	return stream, nil
}

// forwardAIStream 在后台消费 AI 流，写 Redis、广播订阅者（将消息写入 stream chan），并在结束后持久化助手完整回复。
func (s *aiStreamSupport) forwardAIStream(aiStream <-chan ai.StreamEvent, params streamScopeParams, sessionID int64, runID int64) {
	defer s.streamHub.finishJob(sessionID)

	// visibleContent 是普通文本气泡内容；renderData 是 A2UI 卡片状态。
	// 两者分开累积，避免正文卡片或规划卡片把气泡文本覆盖掉。
	var visibleContent strings.Builder
	renderData := model.JSONMap{}
	sawDone := false

	// 工具展示开始后，后续模型 delta 通常是工具调用后的说明或重复文本，
	// 前端只展示卡片即可，所以停止把这些 delta 追加到普通气泡。
	suppressVisibleAfterPresentation := false
	// 章节正文草稿的正文内容在 present_chapter_draft 工具结果之后继续流式输出，
	// 它应该进入 renderData.draft.content，而不是进入普通文本气泡。
	draftStreaming := false
	var draftContent strings.Builder

	// done 事件可能早于通道关闭到达，先记住 token 和结束原因，等最终落库时使用。
	var tokenCount int64
	finishReason := ""

	// Redis 快照用于刷新页面和断线重连恢复；用节流避免每个 token 都写 Redis。
	startedAt := time.Now()
	lastCacheAt := time.Time{}
	lastCacheLen := 0

	rememberStreamMeta := func(nextTokenCount int64, nextFinishReason string) {
		if nextTokenCount > tokenCount {
			tokenCount = nextTokenCount
		}
		if nextFinishReason != "" {
			finishReason = nextFinishReason
		}
	}

	cacheStreaming := func(content string, renderData model.JSONMap, currentLen int, logMessage string) {
		// 距离上次缓存已经超过 500ms 或者 正文草稿内容比上次缓存时多了至少 300 个字节/字符长度，就保存一次快照。
		// 由于两次快照存储有时间间隔，所以很有可能出现 些许 token 丢失的情况（前端重连的时候，恰好在这个间隔中）。
		if !(time.Since(lastCacheAt) >= 500*time.Millisecond || currentLen-lastCacheLen >= 300) {
			return
		}
		if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
			SessionID:        sessionID,
			ScopeType:        int16(params.ScopeType),
			ScopeID:          params.ScopeID,
			AssistantContent: content,
			RenderData:       renderData,
			StartedAt:        startedAt,
			UpdatedAt:        time.Now(),
		}); err != nil {
			zap.L().Error(logMessage,
				zap.Int64("user_id", params.UserID),
				zap.Int64("session_id", sessionID),
				zap.Error(err),
			)
		}
		lastCacheAt = time.Now()
		lastCacheLen = currentLen
	}

streamLoop:
	for event := range aiStream {
		switch event.Type {
		case ai.StreamEventDone:
			// done 只记录元信息，不直接结束；真正结束以 aiStream 通道关闭为准，
			// 这样可以保证 done 前后残留的工具事件或 delta 都被消费完。
			done := event.Done()
			rememberStreamMeta(done.TokenCount, done.FinishReason)
			sawDone = true
			continue

		case ai.StreamEventDelta:
			delta := event.Delta().Text
			if draftStreaming {
				// 正文草稿模式：delta 属于卡片正文，后端只推增量给前端；
				// 完整正文继续留在 renderData 和 Redis 快照里。
				draftContent.WriteString(delta)
				renderData = setChapterDraftStreamContent(renderData, draftContent.String())
				content := visibleContent.String()
				cacheStreaming(content, renderData, draftContent.Len(), "cache streaming draft failed")
				if delta != "" {
					uiEvent := ai.NewStreamA2UI(renderKindChapterDraft, model.JSONMap{
						"draft": map[string]any{
							"content": delta,
						},
					})
					s.streamHub.push(sessionID, uiEvent)
				}
				continue
			}
			if suppressVisibleAfterPresentation {
				// 工具卡片已经接管展示，丢弃后续普通 delta，避免出现
				// “气泡里一份、卡片里一份”的重复内容。
				continue
			}

			// 普通文本模式：累积为气泡内容，写入 Redis 快照并广播 delta。
			visibleContent.WriteString(delta)
			content := visibleContent.String()
			cacheStreaming(content, renderData, len(content), "cache streaming reply failed")
			if delta != "" {
				s.streamHub.push(sessionID, ai.NewStreamDelta(delta))
			}
			continue

		case ai.StreamEventToolCall:
			toolCall := event.ToolCall()
			if isPresentationTool(toolCall.Name) {
				// 展示工具开始后先推空卡片，后续普通 delta 不再进气泡。
				suppressVisibleAfterPresentation = true
				if nextRenderData, ok := loadingRenderDataForPresentationTool(toolCall.Name); ok {
					// 后端 renderData 保持全量，供 Redis 快照和最终落库使用。
					renderData = mergeRenderData(renderData, nextRenderData)
					content := visibleContent.String()
					if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
						SessionID:        sessionID,
						ScopeType:        int16(params.ScopeType),
						ScopeID:          params.ScopeID,
						AssistantContent: content,
						RenderData:       renderData,
						StartedAt:        startedAt,
						UpdatedAt:        time.Now(),
					}); err != nil {
						zap.L().Error("cache presentation placeholder failed", zap.Int64("session_id", sessionID), zap.Error(err))
					}
					// 推给前端的 a2ui 使用本次 tool_call 的增量占位。
					if uiEvent, ok := a2uiRenderEvent(nextRenderData); ok {
						s.streamHub.push(sessionID, uiEvent)
					}
				}
			}
			continue

		case ai.StreamEventToolResult:
			// 工具返回后，把工具结果转换成前端 renderData。转换失败说明模型
			// 或工具输出结构不合法，此时中断并提示用户重试。
			toolResult := event.ToolResult()
			nextRenderData, ok, err := renderDataFromToolResult(toolResult.Name, toolResult.Result)
			if err != nil {
				s.deleteStreamingReply(context.Background(), sessionID)
				s.streamHub.push(sessionID, ai.NewStreamError("AI 展示工具返回数据不合法，请重试"))
				return
			}
			if !ok {
				continue
			}
			// 工具结果推给前端时保持本批增量；后端 renderData 合并为全量，
			// 供 Redis 快照、最终落库和后续保存规划使用。
			renderData = mergeRenderData(renderData, nextRenderData)
			if toolResult.Name == "present_chapter_draft" {
				// present_chapter_draft 只先给卡片骨架，正文会继续以 delta 形式到达。
				draftStreaming = true
			}
			suppressVisibleAfterPresentation = true
			content := visibleContent.String()
			if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
				SessionID:        sessionID,
				ScopeType:        int16(params.ScopeType),
				ScopeID:          params.ScopeID,
				AssistantContent: content,
				RenderData:       renderData,
				StartedAt:        startedAt,
				UpdatedAt:        time.Now(),
			}); err != nil {
				zap.L().Error("cache streaming render data failed", zap.Int64("session_id", sessionID), zap.Error(err))
			}
			if uiEvent, ok := a2uiRenderEvent(nextRenderData); ok {
				s.streamHub.push(sessionID, uiEvent)
			}
			continue

		case ai.StreamEventError:
			streamError := event.Error()
			rememberStreamMeta(streamError.TokenCount, streamError.FinishReason)
			if s.streamHub.cancelReason(sessionID) != streamCancelNone {
				break streamLoop
			}

			zap.L().Error("ai stream returned error event",
				zap.Int64("user_id", params.UserID),
				zap.Int64("entity_id", params.ScopeID),
				zap.Int16("scope_type", int16(params.ScopeType)),
				zap.Int64("session_id", sessionID),
				zap.String("error", streamError.Message),
			)
			s.finishModelRun(context.Background(), modelRunMeta{
				ID:           runID,
				Status:       model.ModelRunStatusFailed,
				TokenCount:   tokenCount,
				FinishReason: finishReason,
				ErrorMessage: streamError.Message,
				EndTime:      timePtr(time.Now()),
			})
			s.streamHub.push(sessionID, event)
			return
		}
	}

	if draftStreaming {
		// 通道关闭后补一次最终正文快照，只写 Redis 和落库，不再额外推给前端。
		renderData = setChapterDraftStreamContent(renderData, draftContent.String())
		if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
			SessionID:        sessionID,
			ScopeType:        int16(params.ScopeType),
			ScopeID:          params.ScopeID,
			AssistantContent: visibleContent.String(),
			RenderData:       renderData,
			StartedAt:        startedAt,
			UpdatedAt:        time.Now(),
		}); err != nil {
			zap.L().Error("cache final streaming draft failed", zap.Int64("session_id", sessionID), zap.Error(err))
		}
	}

	cancelReason := s.streamHub.cancelReason(sessionID)
	if cancelReason != streamCancelNone {
		status := model.ModelRunStatusCanceled
		defaultFinishReason := streamFinishReasonCanceled
		if cancelReason == streamCancelServerShutdown {
			defaultFinishReason = streamFinishReasonServerShutdown
		}
		// 底层取消后可能吐 error，也可能只关闭通道；这里统一保存已生成内容并写 canceled。
		if finishReason == "" {
			finishReason = defaultFinishReason
		}
		if tokenCount <= 0 {
			// 主动中断通常拿不到模型最终 usage（这通常在最后几帧的时候存在），用已生成内容估算一个兜底值。
			tokenCount = estimateCanceledStreamTokens(visibleContent.String(), renderData)
		}
		assistantMessage, err := s.persistFinalAssistantReply(context.Background(), params, sessionID, visibleContent.String(), renderData, modelRunMeta{
			ID:           runID,
			Status:       status,
			TokenCount:   tokenCount,
			FinishReason: finishReason,
			EndTime:      timePtr(time.Now()),
		})
		if err != nil {
			s.finishModelRun(context.Background(), modelRunMeta{
				ID:           runID,
				Status:       model.ModelRunStatusFailed,
				TokenCount:   tokenCount,
				FinishReason: finishReason,
				ErrorMessage: err.Error(),
				EndTime:      timePtr(time.Now()),
			})
			zap.L().Error("persist canceled assistant message after stream close failed",
				zap.Int64("user_id", params.UserID),
				zap.Int64("entity_id", params.ScopeID),
				zap.Int16("scope_type", int16(params.ScopeType)),
				zap.Int64("session_id", sessionID),
				zap.Error(err),
			)
			s.deleteStreamingReply(context.Background(), sessionID)
			s.streamHub.push(sessionID, ai.NewStreamErrorWithMeta(err.Error(), tokenCount, finishReason))
			return
		}
		s.deleteStreamingReply(context.Background(), sessionID)
		if assistantMessage.ID <= 0 {
			s.finishModelRun(context.Background(), modelRunMeta{
				ID:           runID,
				Status:       status,
				TokenCount:   tokenCount,
				FinishReason: finishReason,
				EndTime:      timePtr(time.Now()),
			})
			s.streamHub.push(sessionID, ai.NewStreamDone(tokenCount, finishReason, 0))
			return
		}
		s.streamHub.push(sessionID, streamDoneEventFromMessage(assistantMessage, tokenCount, finishReason))
		return
	}

	finalContent := strings.TrimSpace(visibleContent.String())
	if finalContent == "" && len(renderData) == 0 {
		// 模型只发了 done、没有任何可展示内容时，不落库，只通知前端结束。
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusSuccess,
			TokenCount:   tokenCount,
			FinishReason: finishReason,
			EndTime:      timePtr(time.Now()),
		})
		s.deleteStreamingReply(context.Background(), sessionID)
		if sawDone {
			s.streamHub.push(sessionID, ai.NewStreamDone(0, "", 0))
		}
		return
	}

	// 正常结束：把普通文本和最终 renderData 一起保存成助手消息。
	// 如果是章节正文卡片，SaveChapterDrafts 会在事务里顺手生成正文草稿。
	assistantMessage, err := s.persistFinalAssistantReply(context.Background(), params, sessionID, finalContent, renderData, modelRunMeta{
		ID:           runID,
		Status:       model.ModelRunStatusSuccess,
		TokenCount:   tokenCount,
		FinishReason: finishReason,
		EndTime:      timePtr(time.Now()),
	})
	if err != nil {
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   tokenCount,
			FinishReason: finishReason,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		zap.L().Error("persist assistant message failed",
			zap.Int64("user_id", params.UserID),
			zap.Int64("entity_id", params.ScopeID),
			zap.Int16("scope_type", int16(params.ScopeType)),
			zap.Int64("session_id", sessionID),
			zap.Error(err),
		)
		s.streamHub.push(sessionID, ai.NewStreamErrorWithMeta(err.Error(), tokenCount, finishReason))
		return
	}
	doneEvent := streamDoneEventFromMessage(assistantMessage, tokenCount, finishReason)
	if assistantMessage.DraftID > 0 {
		// 正文草稿 ID 只有落库后才知道，所以这里仅回填到 renderData，
		// 不再额外补推完整卡片，避免把正文重复发给前端。
		renderData = setChapterDraftID(renderData, assistantMessage.DraftID)
		assistantMessage.RenderData = renderData
		doneEvent = streamDoneEventFromMessage(assistantMessage, tokenCount, finishReason)
	}
	// 已经持久化成功，删除临时快照，并推送最终 done 事件。
	s.deleteStreamingReply(context.Background(), sessionID)
	s.streamHub.push(sessionID, doneEvent)
}

func streamDoneEventFromMessage(message model.ChatMessage, tokenCount int64, finishReason string) ai.StreamEvent {
	params := ai.StreamDoneParams{}
	if message.ID <= 0 {
		return ai.NewStreamDoneWithParams(0, "", params)
	}
	if message.DraftID > 0 {
		params.DraftID = message.DraftID
	}
	return ai.NewStreamDoneWithParams(tokenCount, finishReason, params)
}

// persistFinalAssistantReply 保存最终或被用户中断时已经生成的助手回复并记录 run。
func (s *aiStreamSupport) persistFinalAssistantReply(ctx context.Context, params streamScopeParams, sessionID int64, content string, renderData model.JSONMap, runMeta modelRunMeta) (model.ChatMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" && len(renderData) == 0 { // 注意空内容不会记录 run。
		return model.ChatMessage{}, nil
	}
	var assistantMessage model.ChatMessage
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		var err error
		assistantMessage, err = repositories.ChatMessages.CreateWithMeta(ctx, sessionID, chatRoleAssistant, content, renderData)
		if err != nil {
			return err
		}
		assistantMessage.Role = "assistant"
		if runMeta.ID > 0 {
			scopeID := assistantMessage.ID
			runMeta.ScopeID = &scopeID
			if err := finishModelRun(ctx, repositories, runMeta); err != nil {
				return err
			}
		}
		if params.SaveChapterDrafts != nil {
			draftID, err := params.SaveChapterDrafts(ctx, repositories, sessionID, assistantMessage.ID, renderData)
			if err != nil {
				return err
			}
			assistantMessage.DraftID = draftID
		}
		return nil
	})
	return assistantMessage, err
}

// createChatMessage 保存普通对话消息，运行元数据由 t_model_runs 单独记录。
func createChatMessage(ctx context.Context, repositories repo.Repositories, sessionID int64, role int16, content string) (model.ChatMessage, error) {
	return repositories.ChatMessages.CreateWithMeta(ctx, sessionID, role, content, model.JSONMap{})
}

func (s *aiStreamSupport) createModelRun(ctx context.Context, runMeta modelRunMeta) int64 {
	run, err := createModelRun(ctx, s.repositories, runMeta.UserID, runMeta)
	if err != nil {
		zap.L().Error("create model run failed", zap.Error(err))
		return 0
	}
	return run.ID
}

func (s *aiStreamSupport) finishModelRun(ctx context.Context, runMeta modelRunMeta) {
	if err := finishModelRun(ctx, s.repositories, runMeta); err != nil {
		zap.L().Error("finish model run failed", zap.Error(err))
	}
}

func (s *aiStreamSupport) beginShutdown() {
	if s == nil {
		return
	}
	s.streamHub.beginShutdown()
	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}
}

func (s *aiStreamSupport) isShuttingDown() bool {
	return s != nil && s.shutdownCtx != nil && s.shutdownCtx.Err() != nil
}

func (s *aiStreamSupport) canceledModelRunFinishReason() string {
	if s.isShuttingDown() {
		return streamFinishReasonServerShutdown
	}
	return streamFinishReasonCanceled
}

func (s *aiStreamSupport) aiCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if s == nil || s.shutdownCtx == nil {
		return ctx, func() {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-s.shutdownCtx.Done():
			cancel()
		case <-runCtx.Done():
		}
	}()
	return runCtx, cancel
}

func createModelRun(ctx context.Context, repositories repo.Repositories, fallbackUserID int64, runMeta modelRunMeta) (model.ModelRun, error) {
	if repositories.ModelRuns == nil {
		return model.ModelRun{}, nil
	}
	userID := fallbackUserID
	if userID <= 0 {
		userID = runMeta.UserID
	}
	if userID <= 0 || runMeta.ScopeType <= 0 {
		return model.ModelRun{}, nil
	}
	status := runMeta.Status
	if status < 0 {
		status = model.ModelRunStatusRunning
	}
	startTime := runMeta.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	return repositories.ModelRuns.Create(ctx, model.ModelRun{
		UserID:       userID,
		ScopeType:    runMeta.ScopeType,
		ScopeID:      runMeta.ScopeID,
		ModelID:      runMeta.ModelID,
		Status:       status,
		TokenCount:   runMeta.TokenCount,
		FinishReason: strings.TrimSpace(runMeta.FinishReason),
		ErrorMessage: compactModelRunError(runMeta.ErrorMessage),
		StartTime:    startTime,
	})
}

func finishModelRun(ctx context.Context, repositories repo.Repositories, runMeta modelRunMeta) error {
	if repositories.ModelRuns == nil || runMeta.ID <= 0 {
		return nil
	}
	endTime := runMeta.EndTime
	if endTime == nil {
		endTime = timePtr(time.Now())
	}
	status := runMeta.Status
	if status <= 0 {
		status = model.ModelRunStatusSuccess
	}
	return repositories.ModelRuns.Finish(ctx, runMeta.ID, model.ModelRun{
		ScopeID:      runMeta.ScopeID,
		Status:       status,
		TokenCount:   runMeta.TokenCount,
		FinishReason: strings.TrimSpace(runMeta.FinishReason),
		ErrorMessage: compactModelRunError(runMeta.ErrorMessage),
		EndTime:      endTime,
	})
}

func compactModelRunError(message string) string {
	message = strings.TrimSpace(message)
	const limit = 1000
	if len([]rune(message)) <= limit {
		return message
	}
	return string([]rune(message)[:limit])
}

func optionalScopeID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func estimateCanceledStreamTokens(content string, renderData model.JSONMap) int64 {
	total := estimateRunTextTokens(content)
	if len(renderData) > 0 {
		if payload, err := json.Marshal(renderData); err == nil {
			total += estimateRunTextTokens(string(payload))
		}
	}
	return total
}

func estimateRunTextTokens(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return int64(utf8.RuneCountInString(text)/2 + 1)
}

func modelIDFromKey(modelKey string) int64 {
	head, _, _ := strings.Cut(strings.TrimSpace(modelKey), ":")
	id, err := strconv.ParseInt(head, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

// resolveUserModelConfig 读取用户选择的模型配置，并叠加用户自定义采样参数。
func (s *aiStreamSupport) resolveUserModelConfig(ctx context.Context, userID int64) (effectiveModelConfig, error) {
	settings, err := resolveUserSettings(ctx, s.repositories, s.redisClient, userID)
	if err != nil {
		return effectiveModelConfig{}, err
	}

	var selected model.ModelConfig
	modelID := settings.General.ModelID
	cacheScope := "default"
	if modelID < 0 {
		return effectiveModelConfig{}, ErrInvalidModel
	}
	if modelID > 0 {
		cacheScope = strconv.FormatInt(modelID, 10)
		if cached, ok := s.readUserModelConfigCache(ctx, userID, cacheScope); ok {
			return cached, nil
		}
		selected, err = s.repositories.ModelConfigs.FindAvailableByID(ctx, userID, modelID)
		if errors.Is(err, sql.ErrNoRows) {
			return effectiveModelConfig{}, ErrInvalidModel
		}
		if err != nil {
			return effectiveModelConfig{}, wrapError("查询用户选中模型失败", err)
		}
	} else {
		available, err := s.repositories.ModelConfigs.ListAvailable(ctx, userID)
		if err != nil {
			return effectiveModelConfig{}, wrapError("查询可用模型列表失败", err)
		}
		if len(available) == 0 {
			return effectiveModelConfig{}, ErrInvalidModel
		}
		selected = available[0]
	}

	effective := effectiveModelConfig{
		ID:          selected.ID,
		UserID:      selected.UserID,
		Name:        selected.Name,
		Provider:    selected.Provider,
		ModelID:     selected.ModelID,
		APIURL:      selected.APIURL,
		APIKey:      selected.APIKey,
		TopP:        defaultModelTopP,
		Temperature: defaultModelTemperature,
		UpdatedAt:   selected.UpdatedAt.UnixNano(),
	}
	s.cacheUserModelConfig(ctx, userID, cacheScope, effective)
	return effective, nil
}

// resolveUserModelConfigByPublicID 按前端传入的模型 ID 解析模型；为空时回退到用户当前设置。
func (s *aiStreamSupport) resolveUserModelConfigByPublicID(ctx context.Context, userID int64, modelID string) (effectiveModelConfig, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return s.resolveUserModelConfig(ctx, userID)
	}
	id, err := strconv.ParseInt(modelID, 10, 64)
	if err != nil || id <= 0 {
		return effectiveModelConfig{}, ErrInvalidModel
	}
	cacheScope := strconv.FormatInt(id, 10)
	if cached, ok := s.readUserModelConfigCache(ctx, userID, cacheScope); ok {
		return cached, nil
	}
	selected, err := s.repositories.ModelConfigs.FindAvailableByID(ctx, userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return effectiveModelConfig{}, ErrInvalidModel
	}
	if err != nil {
		return effectiveModelConfig{}, wrapError("查询指定模型失败", err)
	}
	effective := effectiveModelConfig{
		ID:          selected.ID,
		UserID:      selected.UserID,
		Name:        selected.Name,
		Provider:    selected.Provider,
		ModelID:     selected.ModelID,
		APIURL:      selected.APIURL,
		APIKey:      selected.APIKey,
		TopP:        defaultModelTopP,
		Temperature: defaultModelTemperature,
		UpdatedAt:   selected.UpdatedAt.UnixNano(),
	}
	s.cacheUserModelConfig(ctx, userID, cacheScope, effective)
	return effective, nil
}

// AgentKey 生成用于区分同一用户不同模型配置版本的 Agent 缓存键。
func (cfg effectiveModelConfig) AgentKey() string {
	return fmt.Sprintf("%d:%d", cfg.ID, cfg.UpdatedAt)
}

// RuntimeConfig 将有效模型配置转换为 AI 客户端运行配置。
func (cfg effectiveModelConfig) RuntimeConfig() ai.ModelRuntimeConfig {
	return ai.ModelRuntimeConfig{
		Provider:            cfg.Provider,
		ModelID:             cfg.ModelID,
		APIURL:              cfg.APIURL,
		APIKey:              decryptModelAPIKey(cfg.APIKey),
		Temperature:         cfg.Temperature,
		TopP:                cfg.TopP,
		ContextWindowTokens: ai.ContextWindowTokens(cfg.Provider, cfg.ModelID),
	}
}

// readUserModelConfigCache 只读取 Redis 中的 model_config 缓存，缓存未命中返回 ok=false。
func (s *aiStreamSupport) readUserModelConfigCache(ctx context.Context, userID int64, scope string) (effectiveModelConfig, bool) {
	if s.redisClient == nil || userID <= 0 || scope == "" {
		return effectiveModelConfig{}, false
	}
	payload, err := s.redisClient.Get(ctx, userModelConfigCacheKey(userID, scope)).Bytes()
	if err != nil || len(payload) == 0 {
		return effectiveModelConfig{}, false
	}
	var cached effectiveModelConfig
	if err := json.Unmarshal(payload, &cached); err != nil {
		return effectiveModelConfig{}, false
	}
	return cached, true
}

// cacheUserModelConfig 只写入 Redis 中的 model_config 缓存，api_key 使用数据库中的加密值。
func (s *aiStreamSupport) cacheUserModelConfig(ctx context.Context, userID int64, scope string, cfg effectiveModelConfig) {
	if s.redisClient == nil || userID <= 0 || scope == "" {
		return
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	_ = s.redisClient.Set(ctx, userModelConfigCacheKey(userID, scope), payload, userModelConfigCacheTTL).Err()
}

// decryptModelAPIKey 在真正调用模型前解密密钥，避免 repo 和 Redis 缓存保存明文。
func decryptModelAPIKey(apiKey string) string {
	decrypted, err := crypto.Decrypt(apiKey)
	if err != nil || decrypted == "" {
		return apiKey
	}
	return decrypted
}

// isProtectedHistoryMessage 判断历史消息是否应避开上下文摘要。
//
// 系统消息属于业务上下文，必须原样传递给模型；每个会话的第一条历史消息在组装时额外保护。
func isProtectedHistoryMessage(message model.ChatMessage) bool {
	return message.Role == "system"
}

// resolveMaxConcurrentStreams 读取用户绑定会员计划中的最大 AI 并发数，并用 Redis 缓存解析结果。
func (s *aiStreamSupport) resolveMaxConcurrentStreams(ctx context.Context, userID int64) (int, error) {
	if s.redisClient != nil {
		cached, err := s.redisClient.Get(ctx, userMembershipMaxConcurrentStreamsKey(userID)).Int()
		if err == nil && cached > 0 {
			return cached, nil
		}
	}

	plan, err := s.repositories.Memberships.FindPlanByUserID(ctx, userID)
	if errors.Is(err, repo.ErrMembershipPlanNotFound) {
		return s.cacheMaxConcurrentStreams(ctx, userID, defaultMaxConcurrentStreams), nil
	}
	if err != nil {
		return 0, wrapError("查询用户会员计划失败", err)
	}
	return s.cacheMaxConcurrentStreams(ctx, userID, parseMaxConcurrentStreams(plan.Features)), nil
}

// parseMaxConcurrentStreams 从会员计划 features 中解析最大并发数，配置缺失或异常时返回兜底值。
func parseMaxConcurrentStreams(features model.JSONMap) int {
	value, ok := features["max_concurrent_streams"]
	if !ok {
		return defaultMaxConcurrentStreams
	}
	if v, ok := value.(int); ok {
		return int(v)
	}
	return defaultMaxConcurrentStreams
}

// cacheMaxConcurrentStreams 将解析后的最大并发数写入 Redis，后续流式请求直接使用缓存值。
func (s *aiStreamSupport) cacheMaxConcurrentStreams(ctx context.Context, userID int64, maxConcurrentStreams int) int {
	if maxConcurrentStreams <= 0 {
		maxConcurrentStreams = defaultMaxConcurrentStreams
	}
	if s.redisClient == nil {
		return maxConcurrentStreams
	}
	_ = s.redisClient.Set(ctx, userMembershipMaxConcurrentStreamsKey(userID), maxConcurrentStreams, membershipFeatureTTL).Err()
	return maxConcurrentStreams
}

// resumeStreamingReply 重新订阅后台 AI 任务；若内存订阅已丢失，则用 Redis 快照补偿已生成内容。
func (s *aiStreamSupport) resumeStreamingReply(ctx context.Context, sessionID int64, scopeType int16, scopeID int64) <-chan ai.StreamEvent {
	if stream, ok := s.streamHub.subscribe(ctx, sessionID); ok {
		out := make(chan ai.StreamEvent, 16)
		go func() {
			defer close(out)
			snapshot := streamingReplySnapshot{SessionID: sessionID}
			renderData := model.JSONMap{}
			if current, err := s.readStreamingReply(ctx, sessionID); err == nil {
				snapshot = current
				content := snapshot.AssistantContent
				renderData = snapshot.RenderData
				out <- ai.NewStreamSync(content, renderData)
			}
			for event := range stream {
				select {
				case <-ctx.Done():
					return
				case out <- event:
				}
			}
		}()
		return out
	}

	// 兜底处理。
	// 内存订阅丢失，使用 Redis 快照补偿（因为是有任务在时刻保存对应消息快照）。（快照前后差值补偿）
	out := make(chan ai.StreamEvent, 16)
	go func() {
		defer close(out)
		snapshot, err := s.readStreamingReply(ctx, sessionID)
		if err != nil {
			out <- ai.NewStreamDone(0, "", 0)
			return
		}
		if snapshot.ScopeType != scopeType || snapshot.ScopeID != scopeID {
			out <- ai.NewStreamDone(0, "", 0)
			return
		}
		if snapshot.ModelRunID > 0 {
			s.finishStreamingSnapshotModelRun(context.Background(), snapshot, model.ModelRunStatusFailed, -1, "", "AI 后台任务已丢失，resume 只能恢复缓存快照")
		}
		lastContent := snapshot.AssistantContent
		out <- ai.NewStreamSync(lastContent, model.JSONMap(snapshot.RenderData))
		ticker := time.NewTicker(500 * time.Millisecond) // 模拟流式回复
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next, err := s.readStreamingReply(ctx, sessionID)
				if errors.Is(err, redis.Nil) {
					out <- ai.NewStreamDone(0, "", 0)
					return
				}
				if err != nil {
					out <- ai.NewStreamError(err.Error())
					return
				}
				if len(next.AssistantContent) > len(lastContent) {
					out <- ai.NewStreamDelta(next.AssistantContent[len(lastContent):])
					lastContent = next.AssistantContent
				}
			}
		}
	}()
	return out
}

// cancelStreamingReply 取消指定会话正在运行的后台 AI 任务。（取消 AI 任务，原理其实是调用 cancel，取消对应传入 AI 任务的 jobCtx）
func (s *aiStreamSupport) cancelStreamingReply(ctx context.Context, sessionID int64) error {
	if sessionID <= 0 {
		return ErrResourceNotFound
	}
	if !s.streamHub.cancelJob(sessionID) {
		return ErrResourceNotFound
	}
	// 取消路由：这里只标记并取消后台 jobCtx，运行记录由 forwardAIStream 统一收口。
	return nil
}

// Shutdown 取消正在运行的流式 AI 任务，并等待它们完成运行记录收口。
func (s *aiStreamSupport) Shutdown(ctx context.Context) error {
	if s == nil || s.streamHub == nil {
		return nil
	}
	s.beginShutdown()
	return s.streamHub.Shutdown(ctx)
}

// appendStreamingReply 只追加正在回复的临时消息占位。
// 具体内容和 A2UI 状态由 resume 接口读取 Redis 快照后通过 sync 返回，避免消息列表和 resume 重复返回大段内容。
func (s *aiStreamSupport) appendStreamingReply(ctx context.Context, messages []model.ChatMessage, sessionID int64, scopeType int16, scopeID int64) []model.ChatMessage {
	snapshot, err := s.readStreamingReply(ctx, sessionID)
	if err != nil || snapshot.ScopeType != scopeType || snapshot.ScopeID != scopeID {
		return messages
	}
	return append(messages, model.ChatMessage{
		ID:         0,
		SessionID:  sessionID,
		Role:       "assistant",
		Content:    "",
		RenderData: nil,
		Temporary:  true,
		CreatedAt:  snapshot.StartedAt,
		UpdatedAt:  snapshot.UpdatedAt,
	})
}

// cacheStreamingReply 将流式回复累计内容写入 Redis，供刷新和重连时恢复用户已看到的部分。
func (s *aiStreamSupport) cacheStreamingReply(ctx context.Context, snapshot streamingReplySnapshot) error {
	if s.redisClient == nil || snapshot.SessionID <= 0 {
		return nil
	}
	s.fillStreamingSnapshotRunMeta(ctx, &snapshot)
	if snapshot.StartedAt.IsZero() {
		snapshot.StartedAt = time.Now()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now()
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, streamingReplyCacheKey(snapshot.SessionID), payload, streamingReplyCacheTTL).Err()
}

func (s *aiStreamSupport) fillStreamingSnapshotRunMeta(ctx context.Context, snapshot *streamingReplySnapshot) {
	if snapshot == nil || snapshot.SessionID <= 0 || snapshot.ModelRunID > 0 {
		return
	}
	current, err := s.readStreamingReply(ctx, snapshot.SessionID)
	if err != nil {
		return
	}
	if snapshot.ModelRunID <= 0 {
		snapshot.ModelRunID = current.ModelRunID
	}
}

func (s *aiStreamSupport) finishStreamingSnapshotModelRun(ctx context.Context, snapshot streamingReplySnapshot, status int16, tokenCount int64, finishReason string, errorMessage string) {
	s.fillStreamingSnapshotRunMeta(ctx, &snapshot)
	if snapshot.ModelRunID <= 0 {
		return
	}
	s.finishModelRun(ctx, modelRunMeta{
		ID:           snapshot.ModelRunID,
		Status:       status,
		TokenCount:   tokenCount,
		FinishReason: finishReason,
		ErrorMessage: errorMessage,
		EndTime:      timePtr(time.Now()),
	})
}

// readStreamingReply 从 Redis 读取指定会话的流式回复快照。
func (s *aiStreamSupport) readStreamingReply(ctx context.Context, sessionID int64) (streamingReplySnapshot, error) {
	if s.redisClient == nil {
		return streamingReplySnapshot{}, redis.Nil
	}
	payload, err := s.redisClient.Get(ctx, streamingReplyCacheKey(sessionID)).Bytes()
	if err != nil {
		return streamingReplySnapshot{}, err
	}
	var snapshot streamingReplySnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return streamingReplySnapshot{}, err
	}
	return snapshot, nil
}

// deleteStreamingReply 删除已经落库或确认失败无需恢复的流式回复快照。
func (s *aiStreamSupport) deleteStreamingReply(ctx context.Context, sessionID int64) {
	if s.redisClient == nil || sessionID <= 0 {
		return
	}
	if err := s.redisClient.Del(ctx, streamingReplyCacheKey(sessionID)).Err(); err != nil {
		zap.L().Error("delete streaming reply cache failed", zap.Int64("session_id", sessionID), zap.Error(err))
	}
}

// volumeWordCount 直接按当前正文聚合卷字数，避免 Redis 中产生大量易过期的派生统计键。
func (s *aiStreamSupport) volumeWordCount(ctx context.Context, volumeID int64) int {
	total, err := s.repositories.Chapters.SumWordCountByVolumeID(ctx, volumeID)
	if err != nil {
		zap.L().Error("sum volume word count failed", zap.Int64("volume_id", volumeID), zap.Error(err))
		return 0
	}
	return total
}

// novelWordCount 直接按当前正文聚合小说字数，保证列表和工作台看到的都是实时统计。
func (s *aiStreamSupport) novelWordCount(ctx context.Context, novelID int64) int {
	total, err := s.repositories.Chapters.SumWordCountByNovelID(ctx, novelID)
	if err != nil {
		zap.L().Error("sum novel word count failed", zap.Int64("novel_id", novelID), zap.Error(err))
		return 0
	}
	return total
}

// userMembershipMaxConcurrentStreamsKey 生成用户会员计划最大 AI 并发数缓存键。
func userMembershipMaxConcurrentStreamsKey(userID int64) string {
	return fmt.Sprintf("user:%d:membership:max-concurrent-streams", userID)
}

// streamingReplyCacheKey 生成会话流式回复快照缓存键。
func streamingReplyCacheKey(sessionID int64) string {
	return fmt.Sprintf("chat-session:%d:streaming-reply", sessionID)
}

// openSessionMeta 构造尚未创建真实会话时的可对话元信息。
func openSessionMeta(sessionID int64, scopeType int16, scopeID int64) model.ChatSessionMeta {
	return model.ChatSessionMeta{
		ID:        sessionID,
		ScopeType: scopeType,
		ScopeID:   scopeID,
	}
}

// sessionMeta 将数据库会话转换为前端需要的会话元信息。
func sessionMeta(session model.ChatSession) model.ChatSessionMeta {
	return model.ChatSessionMeta{
		ID:        session.ID,
		ScopeType: session.ScopeType,
		ScopeID:   session.ScopeID,
	}
}

// ensureNovelOwner 校验小说存在且属于当前用户。
func (s *aiStreamSupport) ensureNovelOwner(ctx context.Context, userID int64, novelID int64) (model.Novel, error) {
	return ensureNovelOwnerWithRepositories(ctx, s.repositories, userID, novelID)
}

// ensureVolumeOwner 校验卷存在且所属小说属于当前用户。
func (s *aiStreamSupport) ensureVolumeOwner(ctx context.Context, userID int64, volumeID int64) (model.Volume, error) {
	return ensureVolumeOwnerWithRepositories(ctx, s.repositories, userID, volumeID)
}
