package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

const (
	chapterGraphNodePrepare  = "prepare_context"
	chapterGraphNodeNote     = "generate_note"
	chapterGraphNodeGenerate = "generate_draft"
	chapterGraphNodeValidate = "validate_draft"
	chapterGraphNodeFinalize = "finalize"
)

const (
	chapterGenerationStageThinking   = "thinking"
	chapterGenerationStageNote       = "note"
	chapterGenerationStageGenerating = "generating"
	chapterGenerationStageGenerated  = "generated"
	chapterGenerationStagePreview    = "preview"
	chapterGenerationStageValidating = "validating"
	chapterGenerationStageFailedOnce = "failed_once"
	chapterGenerationStageFailed     = "failed"
	chapterGenerationStagePassed     = "passed"
	chapterGenerationStageCollapsed  = "collapsed"
)

type chapterGenerationGraph struct {
	runner compose.Runnable[chapterGenerationInput, chapterGenerationState]
}

type chapterGenerationInput struct {
	UserID    int64
	ChapterID int64
	Content   string
	SessionID int64
	Sink      *chapterGenerationSink
}

type chapterGenerationState struct {
	// Input 是本次 Graph 调用的原始入参。
	Input chapterGenerationInput

	// NovelID 是当前章节所属小说 ID。
	NovelID int64

	// Volume 是当前章节所属卷。
	Volume model.Volume

	// Chapter 是当前要生成正文的章节。
	Chapter model.Chapter

	// ModelConfig 是本次正文生成使用的模型配置。
	ModelConfig effectiveModelConfig

	// WritingSettings 是用户写作设置，包含高一致性重试次数等参数。
	WritingSettings userWritingSettings

	// UserInstructionHistory 是章级对话里的历史用户/助手文本。
	UserInstructionHistory []ai.StreamMessage

	// WritingContext 是普通模式和 Graph 模式共用的章级正文上下文。
	WritingContext chapterWritingContext

	// NovelWritingProfile 是全书写作画像。
	NovelWritingProfile model.JSONMap

	// ReferencedSettings 是当前章 references 命中的设定。
	ReferencedSettings []referencedSetting

	// PreviousContext 是上一章已应用正文或章节梗概。
	PreviousContext string

	// PreviousContextType 标记 PreviousContext 来源。
	PreviousContextType string

	// NextPlanData 是下一章规划，用于避免提前写出后续剧情。
	NextPlanData model.JSONMap

	// Note 是生成前展示给用户的简短说明。
	Note string

	// Draft 是当前轮候选正文。
	Draft string

	// Attempt 是当前生成轮次，从 1 开始。
	Attempt int

	// Validation 是当前轮正文校验结果。
	Validation chapterDraftValidation

	// RepairInstructions 是校验失败后给下一轮重写使用的指令。
	RepairInstructions []string

	// FinalRenderData 是最终通过校验后要保存和展示的正文卡片数据。
	FinalRenderData model.JSONMap

	// StepOutputs 是前端进度卡片展示的阶段输出。
	StepOutputs []chapterGenerationStepOutput
}

type chapterGenerationStepOutput struct {
	Step    string   `json:"step"`
	Attempt int      `json:"attempt"`
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Items   []string `json:"items,omitempty"`
}

type chapterGenerationStepTiming struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type chapterDraftValidation struct {
	Passed             bool                         `json:"passed"`
	Blockers           []chapterDraftValidationItem `json:"blockers"`
	Warnings           []chapterDraftValidationItem `json:"warnings"`
	RepairInstructions []string                     `json:"repair_instructions"`
}

type chapterDraftValidationItem struct {
	Rule     string `json:"rule"`
	Evidence string `json:"evidence"`
	Reason   string `json:"reason"`
}

type chapterGenerationSink struct {
	// ctx 是后台 Graph 任务上下文，用于判断取消和超时。
	ctx context.Context

	// service 提供缓存、推流和持久化所需的章节服务能力。
	service *chapterService

	// sessionID 是当前章级对话会话 ID。
	sessionID int64

	// params 是保存最终助手消息和草稿时需要的流式参数。
	params streamScopeParams

	// startedAt 是本次高一致性生成开始时间。
	startedAt time.Time

	// modelRunID 是本次高一致性生成对应的运行记录 ID。
	modelRunID int64

	// tokenCount 累计本次高一致性生成内多次模型调用的 token 数。
	tokenCount int64

	// finishReason 记录最近一次模型调用返回的结束原因。
	finishReason string

	// currentRender 是当前进度卡片的渲染数据。
	currentRender model.JSONMap

	// currentStep 是当前正在执行的 Graph 步骤 key。
	currentStep string

	// stepStartedAt 是当前步骤开始时间。
	stepStartedAt time.Time

	// stepTimings 记录各 Graph 步骤的起止时间和耗时。
	stepTimings []chapterGenerationStepTiming
}

// newChapterGenerationGraph 使用 Anthropic Evaluator-Optimizer 模式，校验失败时根据反馈重写正文。
func (s *chapterService) newChapterGenerationGraph(ctx context.Context) (*chapterGenerationGraph, error) {
	graph := compose.NewGraph[chapterGenerationInput, chapterGenerationState]()
	if err := graph.AddLambdaNode(chapterGraphNodePrepare, compose.InvokableLambda(s.prepareChapterGenerationContext)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode(chapterGraphNodeNote, compose.InvokableLambda(s.generateChapterWritingNote)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode(chapterGraphNodeGenerate, compose.InvokableLambda(s.generateChapterDraftCandidate)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode(chapterGraphNodeValidate, compose.InvokableLambda(s.validateChapterDraftCandidate)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode(chapterGraphNodeFinalize, compose.InvokableLambda(s.finalizeChapterGeneration)); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(compose.START, chapterGraphNodePrepare); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(chapterGraphNodePrepare, chapterGraphNodeNote); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(chapterGraphNodeNote, chapterGraphNodeGenerate); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(chapterGraphNodeGenerate, chapterGraphNodeValidate); err != nil {
		return nil, err
	}
	branch := compose.NewGraphBranch(func(ctx context.Context, state chapterGenerationState) (string, error) {
		// 通过 → finalizeChapterGeneration
		if state.Validation.Passed {
			return chapterGraphNodeFinalize, nil
		}
		// 不通过 && Attempt < ConsistencyCheckCount → 回到 generateChapterDraftCandidate
		if state.Attempt < state.WritingSettings.ConsistencyCheckCount {
			return chapterGraphNodeGenerate, nil
		}
		// 不通过 && Attempt >= ConsistencyCheckCount → finalizeChapterGeneration
		return chapterGraphNodeFinalize, nil
	},
		// 目标节点白名单
		map[string]bool{
			chapterGraphNodeGenerate: true,
			chapterGraphNodeFinalize: true,
		})
	if err := graph.AddBranch(chapterGraphNodeValidate, branch); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(chapterGraphNodeFinalize, compose.END); err != nil {
		return nil, err
	}
	// 限制最大节点步数，防止重写分支异常循环。
	runner, err := graph.Compile(ctx, compose.WithMaxRunSteps(40), compose.WithGraphName("chapter_generation"))
	if err != nil {
		return nil, err
	}
	return &chapterGenerationGraph{runner: runner}, nil
}

// streamChapterWithGraph 使用高一致性 Graph 生成章节正文。
func (s *chapterService) streamChapterWithGraph(ctx context.Context, userID int64, chapterID int64, content string) (<-chan ai.StreamEvent, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrInvalidMessage
	}
	if s.aiClient == nil || s.generationGraph == nil {
		return nil, ErrAIUnavailable
	}
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return nil, wrapError("校验章节归属失败", err)
	}
	_, err = s.repositories.Volumes.FindByID(ctx, chapter.VolumeID)
	if errors.Is(err, repo.ErrVolumeNotFound) {
		return nil, ErrResourceNotFound
	}
	if err != nil {
		return nil, wrapError("查询章节所属卷失败", err)
	}
	sessionID, err := s.prepareChapterStreamSession(ctx, userID, chapter.ID)
	if err != nil {
		return nil, err
	}
	if stream, ok := s.streamHub.subscribe(ctx, sessionID); ok {
		return stream, nil
	}
	maxConcurrentStreams, err := s.resolveMaxConcurrentStreams(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("解析用户并发配置失败: %w", err)
	}
	jobCtx, started, err := s.streamHub.startJob(sessionID, userID, maxConcurrentStreams)
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
	startedAt := time.Now()
	modelConfig, err := s.resolveUserModelConfig(ctx, userID)
	if err != nil {
		s.streamHub.finishJob(sessionID)
		return nil, fmt.Errorf("解析用户模型配置失败: %w", err)
	}
	modelRunID := s.createModelRun(context.Background(), modelRunMeta{
		UserID:    userID,
		ScopeType: model.ModelRunScopeMessage,
		ModelID:   modelConfig.ID,
		Status:    model.ModelRunStatusRunning,
		StartTime: startedAt,
	})
	params := streamScopeParams{
		UserID:        userID,
		ScopeID:       chapterID,
		ScopeType:     model.ScopeTypeChapter,
		SessionTitle:  chapterChatTitle,
		ModelConfigID: modelConfig.ID,
		Content:       content,
		SaveChapterDrafts: func(ctx context.Context, repositories repo.Repositories, sessionID int64, assistantMessageID int64, renderData model.JSONMap) (int64, error) {
			return s.saveChapterDraftsFromAssistant(ctx, repositories, userID, chapterID, assistantMessageID, renderData)
		},
	}
	sink := &chapterGenerationSink{
		ctx:        jobCtx,
		service:    s,
		sessionID:  sessionID,
		params:     params,
		startedAt:  startedAt,
		modelRunID: modelRunID,
	}
	if err := s.cacheStreamingReply(context.Background(), streamingReplySnapshot{
		SessionID:  sessionID,
		ScopeType:  int16(model.ScopeTypeChapter),
		ScopeID:    chapterID,
		ModelRunID: modelRunID,
		StartedAt:  startedAt,
		UpdatedAt:  time.Now(),
	}); err != nil {
		// 缓存失败不阻断生成。
	}
	stream, _ := s.streamHub.subscribe(ctx, sessionID)
	// 后台任务使用 jobCtx，避免前端断开时中断生成。
	go s.runChapterGenerationGraph(jobCtx, chapterGenerationInput{
		UserID:    userID,
		ChapterID: chapterID,
		Content:   content,
		SessionID: sessionID,
		Sink:      sink,
	})
	return stream, nil
}

// prepareChapterStreamSession 查找或创建章节正文对话会话。
func (s *chapterService) prepareChapterStreamSession(ctx context.Context, userID int64, chapterID int64) (int64, error) {
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeChapter), chapterID)
	if errors.Is(err, repo.ErrChatSessionNotFound) {
		sessionID, createErr := s.repositories.ChatSessions.Create(ctx, userID, int16(model.ScopeTypeChapter), chapterID, chapterChatTitle)
		if createErr != nil {
			return 0, wrapError("准备对话会话失败", createErr)
		}
		return sessionID, nil
	}
	if err != nil {
		return 0, wrapError("准备对话会话失败", err)
	}
	return session.ID, nil
}

// runChapterGenerationGraph 执行高一致性正文生成 Graph，并只在校验通过后推送正式正文卡片。
func (s *chapterService) runChapterGenerationGraph(ctx context.Context, input chapterGenerationInput) {
	defer s.streamHub.finishJob(input.SessionID)
	state, err := s.generationGraph.runner.Invoke(ctx, input)
	if err != nil {
		s.deleteStreamingReply(context.Background(), input.SessionID)
		if ctx.Err() != nil || s.streamHub.cancelReason(input.SessionID) != streamCancelNone {
			s.finishCanceledChapterGenerationRun(input.Sink)
			s.streamHub.push(input.SessionID, ai.NewStreamDone(0, streamFinishReasonCanceled, 0))
			return
		}
		zap.L().Error("chapter generation graph failed",
			zap.Int64("user_id", input.UserID),
			zap.Int64("chapter_id", input.ChapterID),
			zap.Int64("session_id", input.SessionID),
			zap.Error(err),
		)
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           input.Sink.modelRunID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   input.Sink.tokenCount,
			FinishReason: input.Sink.finishReason,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		s.streamHub.push(input.SessionID, ai.NewStreamError(err.Error()))
		return
	}
	finalRenderData := state.FinalRenderData
	if ctx.Err() != nil || s.streamHub.cancelReason(input.SessionID) != streamCancelNone {
		s.finishCanceledChapterGenerationRun(input.Sink)
		s.deleteStreamingReply(context.Background(), input.SessionID)
		s.streamHub.push(input.SessionID, ai.NewStreamDone(0, streamFinishReasonCanceled, 0))
		return
	}
	if len(finalRenderData) == 0 {
		issues := validationIssueTexts(state.Validation)
		state.StepOutputs = appendStepOutput(state.StepOutputs, chapterGenerationStepOutput{
			Step:    "校验不通过",
			Attempt: state.Attempt,
			Type:    "issues",
			Items:   issues,
		})
		input.Sink.emitProgress(chapterGenerationProgress{
			Stage:       chapterGenerationStageFailed,
			Text:        "校验未通过，请调整章节规划或重试。",
			Attempts:    state.Attempt,
			Steps:       chapterGenerationStepLabels(chapterGenerationStageFailed, state.Attempt > 1),
			Complete:    true,
			Failed:      true,
			Issues:      issues,
			StepOutputs: state.StepOutputs,
		})
		assistantMessage, err := s.persistFinalAssistantReply(context.Background(), input.Sink.params, input.SessionID, "高一致性校验未通过，请重试或调整章节规划后再生成。", input.Sink.currentRender, modelRunMeta{
			ID:           input.Sink.modelRunID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   input.Sink.tokenCount,
			FinishReason: "failed",
			ErrorMessage: "高一致性校验未通过",
			EndTime:      timePtr(time.Now()),
		})
		if err != nil {
			s.finishModelRun(context.Background(), modelRunMeta{
				ID:           input.Sink.modelRunID,
				Status:       model.ModelRunStatusFailed,
				TokenCount:   input.Sink.tokenCount,
				FinishReason: "failed",
				ErrorMessage: err.Error(),
				EndTime:      timePtr(time.Now()),
			})
			zap.L().Error("persist failed chapter generation message failed",
				zap.Int64("user_id", input.UserID),
				zap.Int64("chapter_id", input.ChapterID),
				zap.Int64("session_id", input.SessionID),
				zap.Error(err),
			)
			s.deleteStreamingReply(context.Background(), input.SessionID)
			s.streamHub.push(input.SessionID, ai.NewStreamError(err.Error()))
			return
		}
		s.deleteStreamingReply(context.Background(), input.SessionID)
		s.streamHub.push(input.SessionID, streamDoneEventFromMessage(assistantMessage, 0, "failed"))
		return
	}
	input.Sink.emitProgress(chapterGenerationProgress{
		Stage:     chapterGenerationStageCollapsed,
		Text:      "校验通过",
		Attempts:  state.Attempt,
		Steps:     chapterGenerationStepLabels(chapterGenerationStagePassed, state.Attempt > 1),
		Complete:  true,
		Collapsed: true,
	})
	assistantMessage, err := s.persistFinalAssistantReply(context.Background(), input.Sink.params, input.SessionID, "", finalRenderData, modelRunMeta{
		ID:           input.Sink.modelRunID,
		Status:       model.ModelRunStatusSuccess,
		TokenCount:   input.Sink.tokenCount,
		FinishReason: firstNonEmpty(input.Sink.finishReason, "stop"),
		EndTime:      timePtr(time.Now()),
	})
	if err != nil {
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           input.Sink.modelRunID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   input.Sink.tokenCount,
			FinishReason: firstNonEmpty(input.Sink.finishReason, "stop"),
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		zap.L().Error("persist chapter generation message failed",
			zap.Int64("user_id", input.UserID),
			zap.Int64("chapter_id", input.ChapterID),
			zap.Int64("session_id", input.SessionID),
			zap.Error(err),
		)
		s.deleteStreamingReply(context.Background(), input.SessionID)
		s.streamHub.push(input.SessionID, ai.NewStreamError(err.Error()))
		return
	}
	doneEvent := streamDoneEventFromMessage(assistantMessage, 0, "stop")
	if assistantMessage.DraftID > 0 {
		finalRenderData = setChapterDraftID(finalRenderData, assistantMessage.DraftID)
		assistantMessage.RenderData = finalRenderData
		doneEvent = streamDoneEventFromMessage(assistantMessage, 0, "stop")
	}
	s.deleteStreamingReply(context.Background(), input.SessionID)
	s.streamHub.push(input.SessionID, doneEvent)
}

func (s *chapterService) finishCanceledChapterGenerationRun(sink *chapterGenerationSink) {
	if sink == nil {
		return
	}
	finishReason := firstNonEmpty(sink.finishReason, streamFinishReasonCanceled)
	if sink.service != nil && sink.service.streamHub.cancelReason(sink.sessionID) == streamCancelServerShutdown {
		finishReason = firstNonEmpty(sink.finishReason, streamFinishReasonServerShutdown)
	}
	s.finishModelRun(context.Background(), modelRunMeta{
		ID:           sink.modelRunID,
		Status:       model.ModelRunStatusCanceled,
		TokenCount:   sink.tokenCount,
		FinishReason: finishReason,
		EndTime:      timePtr(time.Now()),
	})
}

// prepareChapterGenerationContext 汇总正文生成所需的设定、卷规划、前后章节和模型配置。
func (s *chapterService) prepareChapterGenerationContext(ctx context.Context, input chapterGenerationInput) (chapterGenerationState, error) {
	chapter, err := s.ensureChapterOwner(ctx, input.UserID, input.ChapterID)
	if err != nil {
		return chapterGenerationState{}, err
	}
	volume, err := s.repositories.Volumes.FindByID(ctx, chapter.VolumeID)
	if err != nil {
		return chapterGenerationState{}, err
	}
	writingContext, err := s.chapterWritingContext(ctx, input.UserID, chapter, volume)
	if err != nil {
		return chapterGenerationState{}, err
	}
	writingSettings, err := resolveUserWritingSettings(ctx, s.repositories, s.redisClient, input.UserID)
	if err != nil {
		return chapterGenerationState{}, err
	}
	modelConfig, err := s.resolveUserModelConfig(ctx, input.UserID)
	if err != nil {
		return chapterGenerationState{}, err
	}
	if input.Sink != nil {
		input.Sink.params.ModelConfigID = modelConfig.ID
		if input.Sink.modelRunID <= 0 {
			input.Sink.modelRunID = s.createModelRun(ctx, modelRunMeta{
				UserID:    input.UserID,
				ScopeType: model.ModelRunScopeMessage,
				ModelID:   modelConfig.ID,
				Status:    model.ModelRunStatusRunning,
				StartTime: input.Sink.startedAt,
			})
		}
	}
	userInstructionHistory, err := s.chapterGraphUserInstructionHistory(ctx, input.SessionID)
	if err != nil {
		return chapterGenerationState{}, err
	}
	state := chapterGenerationState{
		Input:                  input,
		NovelID:                volume.NovelID,
		Volume:                 volume,
		Chapter:                chapter,
		ModelConfig:            modelConfig,
		WritingSettings:        writingSettings,
		UserInstructionHistory: userInstructionHistory,
		WritingContext:         writingContext,
		NovelWritingProfile:    writingContext.NovelWritingProfile,
		ReferencedSettings:     writingContext.ReferencedSettings,
		PreviousContext:        writingContext.PreviousContext,
		PreviousContextType:    writingContext.PreviousContextType,
		NextPlanData:           writingContext.NextPlanData,
	}
	input.Sink.emitProgress(chapterGenerationProgress{Stage: chapterGenerationStageThinking, Text: "正在梳理上下文", Attempts: 0, Steps: chapterGenerationStepLabels(chapterGenerationStageThinking, false)})
	return state, nil
}

// chapterGraphUserInstructionHistory 读取章级历史消息，只携带 role 和 content。
func (s *chapterService) chapterGraphUserInstructionHistory(ctx context.Context, sessionID int64) ([]ai.StreamMessage, error) {
	messages, err := s.repositories.ChatMessages.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, wrapError("读取章节历史说明失败", err)
	}
	if len(messages) > 0 {
		messages = messages[:len(messages)-1] // 去掉本轮已入库的用户消息，因为这是单独传递给 AI 的。
	}
	history := make([]ai.StreamMessage, 0, len(messages))
	for _, msg := range messages {
		history = append(history, ai.StreamMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return history, nil
}

// generateChapterWritingNote 生成面向用户的简短切入说明，不参与最终正文保存。
func (s *chapterService) generateChapterWritingNote(ctx context.Context, state chapterGenerationState) (chapterGenerationState, error) {
	summary := jsonMapText(state.Chapter.PlanData, "summary")
	if summary == "" {
		summary = chapterPlanTitle(state.Chapter)
	}
	state.Note = fmt.Sprintf("我会从「%s」切入，按本章规划推进正文，并在完成后校验一致性。", compactText(summary, 36))
	state.Input.Sink.emitProgress(chapterGenerationProgress{Stage: chapterGenerationStageNote, Text: state.Note, Attempts: state.Attempt, Steps: chapterGenerationStepLabels(chapterGenerationStageNote, false)})
	return state, nil
}

// generateChapterDraftCandidate 根据当前上下文生成一次候选正文，候选内容不会直接入库。
func (s *chapterService) generateChapterDraftCandidate(ctx context.Context, state chapterGenerationState) (chapterGenerationState, error) {
	state.Attempt++
	text := "正在生成正文"
	if state.Attempt > 1 {
		text = "校验不通过，正在重写"
	}
	retried := state.Attempt > 1
	state.Input.Sink.emitProgress(chapterGenerationProgress{
		Stage:       chapterGenerationStageGenerating,
		Text:        text,
		Attempts:    state.Attempt,
		Steps:       chapterGenerationStepLabels(chapterGenerationStageGenerating, retried),
		StepOutputs: state.StepOutputs,
	})
	agentDefinition, err := buildChapterAgentDefinition(streamScopeParams{
		ChapterSkill:             s.chapterSkill,
		ChapterWritingMode:       chapterWritingModeGraphDraft,
		ChapterDraftToolDisabled: true,
	})
	if err != nil {
		return state, err
	}
	messages := append([]ai.StreamMessage{}, state.UserInstructionHistory...)
	messages = append(messages, ai.StreamMessage{Role: "user", Content: chapterGraphDraftUserPrompt(state)})
	stream, err := s.aiClient.StreamAgent(ctx, ai.AgentStreamRequest{
		UserID:   state.Input.UserID,
		ModelKey: state.ModelConfig.AgentKey() + ":graph-draft",
		Model:    state.ModelConfig.RuntimeConfig(),
		Messages: messages,
		Agent:    agentDefinition,
	})
	if err != nil {
		return state, err
	}
	var draft strings.Builder
	for event := range stream {
		switch event.Type {
		case ai.StreamEventDelta:
			delta := event.Delta().Text
			draft.WriteString(delta)
			state.Input.Sink.emitTextDelta(delta, draft.String(), rewriteStepName(retried), state.Attempt)
		case ai.StreamEventDone:
			done := event.Done()
			state.Input.Sink.addModelUsage(done.TokenCount, done.FinishReason)
		case ai.StreamEventError:
			return state, errors.New(event.Error().Message)
		}
	}
	state.Draft = strings.TrimSpace(draft.String())
	if state.Draft == "" {
		state.StepOutputs = appendStepOutput(state.StepOutputs, chapterGenerationStepOutput{
			Step:    rewriteStepName(retried),
			Attempt: state.Attempt,
			Type:    "issues",
			Items:   []string{"本次没有生成正文，已跳过校验并准备重新生成。"},
		})
		state.Input.Sink.emitProgress(chapterGenerationProgress{
			Stage:       chapterGenerationStageFailedOnce,
			Text:        "没有生成正文，准备重写",
			Attempts:    state.Attempt,
			Steps:       chapterGenerationStepLabels(chapterGenerationStageFailedOnce, retried),
			Issues:      []string{"本次没有生成正文。"},
			StepOutputs: state.StepOutputs,
		})
		return state, nil
	}
	state.StepOutputs = appendStepOutput(state.StepOutputs, chapterGenerationStepOutput{
		Step:    rewriteStepName(retried),
		Attempt: state.Attempt,
		Type:    "text",
		Text:    compactText(state.Draft, 900),
	})
	state.Input.Sink.emitProgress(chapterGenerationProgress{
		Stage:       chapterGenerationStagePreview,
		Text:        "正文已生成，准备校验一致性",
		Attempts:    state.Attempt,
		Steps:       chapterGenerationStepLabels(chapterGenerationStageGenerated, retried),
		Preview:     compactText(state.Draft, 420),
		StepOutputs: state.StepOutputs,
	})
	return state, nil
}

// validateChapterDraftCandidate 用低温模型校验候选正文是否严格遵守章节规划和上下文。
func (s *chapterService) validateChapterDraftCandidate(ctx context.Context, state chapterGenerationState) (chapterGenerationState, error) {
	retried := state.Attempt > 1
	state.Input.Sink.emitProgress(chapterGenerationProgress{
		Stage:       chapterGenerationStageValidating,
		Text:        "正在校验一致性",
		Attempts:    state.Attempt,
		Steps:       chapterGenerationStepLabels(chapterGenerationStageValidating, retried),
		Preview:     compactText(state.Draft, 420),
		StepOutputs: state.StepOutputs,
	})
	config := state.ModelConfig.RuntimeConfig()
	config.Temperature = 0.1
	result, err := s.aiClient.GenerateChat(ctx, ai.ChatGenerateRequest{
		UserID:   state.Input.UserID,
		ModelKey: state.ModelConfig.AgentKey() + ":validator",
		Model:    config,
		Messages: []ai.StreamMessage{
			{Role: "system", Content: chapterDraftValidationSystemPrompt()},
			{Role: "user", Content: chapterDraftValidationUserPrompt(state)},
		},
	})
	if err != nil {
		return state, err
	}
	state.Input.Sink.addModelUsage(result.TokenCount, result.FinishReason)
	validation := parseChapterDraftValidation(result.Content)
	if validation.Passed && len(validation.Blockers) > 0 {
		validation.Passed = false
	}
	state.Validation = validation
	state.RepairInstructions = validation.RepairInstructions
	if validation.Passed {
		state.StepOutputs = appendStepOutput(state.StepOutputs, chapterGenerationStepOutput{
			Step:    "校验通过",
			Attempt: state.Attempt,
			Type:    "issues",
			Items:   []string{"校验通过。"},
		})
		state.Input.Sink.emitProgress(chapterGenerationProgress{
			Stage:       chapterGenerationStagePassed,
			Text:        "校验通过",
			Attempts:    state.Attempt,
			Steps:       chapterGenerationStepLabels(chapterGenerationStagePassed, retried),
			StepOutputs: state.StepOutputs,
		})
		return state, nil
	}
	issues := validationIssueTexts(validation)
	state.StepOutputs = appendStepOutput(state.StepOutputs, chapterGenerationStepOutput{
		Step:    "校验不通过",
		Attempt: state.Attempt,
		Type:    "issues",
		Items:   issues,
	})
	state.Input.Sink.emitProgress(chapterGenerationProgress{
		Stage:       chapterGenerationStageFailedOnce,
		Text:        "校验不通过，准备重写",
		Attempts:    state.Attempt,
		Steps:       chapterGenerationStepLabels(chapterGenerationStageFailedOnce, retried),
		Preview:     compactText(state.Draft, 420),
		Issues:      issues,
		StepOutputs: state.StepOutputs,
	})
	return state, nil
}

// finalizeChapterGeneration 将通过校验的正文封装成章节草稿渲染数据。
func (s *chapterService) finalizeChapterGeneration(ctx context.Context, state chapterGenerationState) (chapterGenerationState, error) {
	if !state.Validation.Passed {
		return state, nil
	}
	state.FinalRenderData = model.JSONMap{
		"kind": renderKindChapterDraft,
		"draft": map[string]any{
			"title":          strings.TrimSpace(chapterPlanTitle(state.Chapter)),
			"content":        strings.TrimSpace(state.Draft),
			"revision_notes": "高一致性模式已完成上下文校验。",
		},
	}
	return state, nil
}

type chapterGenerationProgress struct {
	Stage       string
	Text        string
	Attempts    int
	Steps       []string
	Preview     string
	Issues      []string
	StepOutputs []chapterGenerationStepOutput
	Complete    bool
	Collapsed   bool
	Failed      bool
}

// emitProgress 把 Graph 进度转换为 A2UI 事件和可恢复的 streaming snapshot。
func (s *chapterGenerationSink) emitProgress(progress chapterGenerationProgress) {
	if s == nil || s.service == nil || s.ctx.Err() != nil {
		return
	}
	currentStep := chapterGenerationCurrentStep(progress.Stage, progress.Attempts)
	currentStepStartedAt := s.markStep(currentStep, progress.Complete)
	renderData := model.JSONMap{
		"kind":                    renderKindChapterProgress,
		"stage":                   progress.Stage,
		"text":                    progress.Text,
		"attempt":                 progress.Attempts,
		"steps":                   progress.Steps,
		"preview":                 progress.Preview,
		"issues":                  progress.Issues,
		"step_outputs":            progress.StepOutputs,
		"current_step_label":      currentStep.Label,
		"current_step_started_at": currentStepStartedAt,
		"step_timings":            s.stepTimings,
		"complete":                progress.Complete,
		"collapsed":               progress.Collapsed,
		"failed":                  progress.Failed,
	}
	s.currentRender = renderData
	_ = s.service.cacheStreamingReply(context.Background(), streamingReplySnapshot{
		SessionID:        s.sessionID,
		ScopeType:        int16(s.params.ScopeType),
		ScopeID:          s.params.ScopeID,
		ModelRunID:       s.modelRunID,
		AssistantContent: "",
		RenderData:       renderData,
		StartedAt:        s.startedAt,
		UpdatedAt:        time.Now(),
	})
	if event, ok := a2uiRenderEvent(renderData); ok {
		s.service.streamHub.push(s.sessionID, event)
	}
}

// emitTextDelta 更新候选正文预览；Redis 保留完整快照，前端只接收当前增量。
func (s *chapterGenerationSink) emitTextDelta(delta string, content string, step string, attempt int) {
	if s == nil || s.service == nil || s.ctx.Err() != nil {
		return
	}
	output := chapterGenerationStepOutput{
		Step:    step,
		Attempt: attempt,
		Type:    "text",
		Text:    compactText(content, 900),
	}
	renderData := s.currentRender
	if renderData != nil && strings.TrimSpace(step) != "" {
		renderData = cloneJSONMap(renderData)
		renderData["step_outputs"] = appendStepOutput(jsonMapStepOutputs(renderData, "step_outputs"), output)
		s.currentRender = renderData
	}
	_ = s.service.cacheStreamingReply(context.Background(), streamingReplySnapshot{
		SessionID:        s.sessionID,
		ScopeType:        int16(s.params.ScopeType),
		ScopeID:          s.params.ScopeID,
		ModelRunID:       s.modelRunID,
		AssistantContent: "",
		RenderData:       renderData,
		StartedAt:        s.startedAt,
		UpdatedAt:        time.Now(),
	})
	if delta != "" {
		s.service.streamHub.push(s.sessionID, ai.NewStreamA2UI(renderKindChapterProgress, model.JSONMap{
			"step_output_delta": chapterGenerationStepOutput{
				Step:    step,
				Attempt: attempt,
				Type:    "text",
				Text:    delta,
			},
		}))
	}
}

// markStep 记录当前 Graph 步骤的开始时间，并在切换或完成时关闭上一步。
func (s *chapterGenerationSink) markStep(step chapterGenerationStepTiming, complete bool) time.Time {
	now := time.Now()
	if step.Key == "" {
		return now
	}
	// 首次进入 Graph 步骤时，记录当前步骤的开始时间。
	if s.currentStep == "" {
		s.currentStep = step.Key
		s.stepStartedAt = now
		s.stepTimings = append(s.stepTimings, chapterGenerationStepTiming{
			Key:       step.Key,
			Label:     step.Label,
			StartedAt: now,
		})
	}
	// 步骤切换时，先关闭上一步，再开启新步骤。
	if s.currentStep != step.Key {
		s.finishCurrentStep(now)
		s.currentStep = step.Key
		s.stepStartedAt = now
		s.stepTimings = append(s.stepTimings, chapterGenerationStepTiming{
			Key:       step.Key,
			Label:     step.Label,
			StartedAt: now,
		})
	}
	// 流程完成时，补齐当前步骤的结束时间。
	if complete {
		s.finishCurrentStep(now)
	}
	return s.stepStartedAt
}

// finishCurrentStep 为当前步骤写入结束时间。
func (s *chapterGenerationSink) finishCurrentStep(endedAt time.Time) {
	if s.currentStep == "" {
		return
	}
	for i := len(s.stepTimings) - 1; i >= 0; i-- {
		if s.stepTimings[i].Key != s.currentStep || s.stepTimings[i].EndedAt != nil {
			continue
		}
		end := endedAt
		s.stepTimings[i].EndedAt = &end
		return
	}
}

func (s *chapterGenerationSink) addModelUsage(tokenCount int64, finishReason string) {
	if s == nil {
		return
	}
	if tokenCount > 0 {
		s.tokenCount += tokenCount
	}
	s.finishReason = firstNonEmpty(finishReason, s.finishReason)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// chapterGenerationCurrentStep 将进度阶段转换为前端展示的步骤信息。
func chapterGenerationCurrentStep(stage string, attempt int) chapterGenerationStepTiming {
	label := chapterGenerationStageStepLabel(stage, attempt)
	if label == "" {
		label = "正在处理"
	}
	return chapterGenerationStepTiming{
		Key:   chapterGenerationStepKey(stage),
		Label: label,
	}
}

// chapterGenerationStepKey 将进度阶段映射到稳定的步骤 key。
func chapterGenerationStepKey(stage string) string {
	switch stage {
	case chapterGenerationStageThinking:
		return chapterGraphNodePrepare
	case chapterGenerationStageNote:
		return chapterGraphNodeNote
	case chapterGenerationStageGenerating, chapterGenerationStageGenerated, chapterGenerationStagePreview:
		return chapterGraphNodeGenerate
	case chapterGenerationStageValidating:
		return chapterGraphNodeValidate
	case chapterGenerationStageFailedOnce, chapterGenerationStageFailed:
		return "validation_failed"
	case chapterGenerationStagePassed, chapterGenerationStageCollapsed:
		return chapterGraphNodeFinalize
	default:
		return stage
	}
}

// chapterGenerationStageStepLabel 返回当前阶段对应的中文步骤名。
func chapterGenerationStageStepLabel(stage string, attempt int) string {
	switch stage {
	case chapterGenerationStageThinking:
		return "梳理上下文"
	case chapterGenerationStageNote:
		return "说明写作切入"
	case chapterGenerationStageGenerating, chapterGenerationStageGenerated, chapterGenerationStagePreview:
		if attempt > 1 {
			return "按校验意见重写"
		}
		return "生成正文"
	case chapterGenerationStageValidating:
		return "校验一致性"
	case chapterGenerationStageFailedOnce, chapterGenerationStageFailed:
		return "校验不通过"
	case chapterGenerationStagePassed, chapterGenerationStageCollapsed:
		return "校验通过"
	default:
		return ""
	}
}

// rewriteStepName 返回首轮生成或重写阶段的展示名。
func rewriteStepName(retried bool) string {
	if retried {
		return "按校验意见重写"
	}
	return "生成正文"
}

// appendStepOutput 按步骤和轮次覆盖旧输出，避免同一步骤重复追加。
func appendStepOutput(outputs []chapterGenerationStepOutput, output chapterGenerationStepOutput) []chapterGenerationStepOutput {
	for i := len(outputs) - 1; i >= 0; i-- {
		if outputs[i].Step == output.Step && outputs[i].Attempt == output.Attempt {
			next := append([]chapterGenerationStepOutput{}, outputs...)
			next[i] = output
			return next
		}
	}
	return append(outputs, output)
}

// cloneJSONMap 浅拷贝渲染数据，避免直接修改已缓存的 map。
func cloneJSONMap(values model.JSONMap) model.JSONMap {
	next := model.JSONMap{}
	for key, value := range values {
		next[key] = value
	}
	return next
}

// jsonMapStepOutputs 从渲染数据里恢复步骤输出列表。
func jsonMapStepOutputs(values model.JSONMap, key string) []chapterGenerationStepOutput {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	if outputs, ok := raw.([]chapterGenerationStepOutput); ok {
		return append([]chapterGenerationStepOutput{}, outputs...)
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var outputs []chapterGenerationStepOutput
	if err := json.Unmarshal(payload, &outputs); err != nil {
		return nil
	}
	return outputs
}

// chapterGenerationStepLabels 返回前端展示的已到达高一致性流程步骤。
func chapterGenerationStepLabels(stage string, retried bool) []string {
	steps := []string{"梳理上下文"}
	if stage == chapterGenerationStageThinking {
		return steps
	}
	steps = append(steps, "说明写作切入")
	if stage == chapterGenerationStageNote {
		return steps
	}

	steps = append(steps, "生成正文")
	steps = append(steps, "校验一致性")
	if !retried {
		if stage == chapterGenerationStageGenerating || stage == chapterGenerationStageGenerated {
			return steps[:len(steps)-1]
		}
		if stage == chapterGenerationStageValidating {
			return steps
		}
		if stage == chapterGenerationStageFailedOnce || stage == chapterGenerationStageFailed {
			return append(steps, "校验不通过")
		}
		return append(steps, "校验通过")
	}

	steps = append(steps, "校验不通过", "按校验意见重写")
	if stage == chapterGenerationStageGenerating || stage == chapterGenerationStageGenerated {
		return steps
	}
	steps = append(steps, "校验一致性")
	if stage == chapterGenerationStageValidating {
		return steps
	}
	if stage == chapterGenerationStageFailedOnce || stage == chapterGenerationStageFailed {
		return append(steps, "校验不通过")
	}
	return append(steps, "校验通过")
}

// chapterGraphDraftUserPrompt 构造正文生成节点的用户提示词 payload。
func chapterGraphDraftUserPrompt(state chapterGenerationState) string {
	payload := chapterGraphContextPayload(state)
	payload["user_request"] = state.Input.Content
	if len(state.RepairInstructions) > 0 {
		payload["repair_instructions"] = state.RepairInstructions
	}
	if previous := previousValidationIssues(state.Validation); len(previous) > 0 {
		payload["previous_validation_issues"] = previous
	}
	raw, _ := json.Marshal(payload)
	return "请根据下面上下文生成当前章正文：\n" + string(raw)
}

// chapterDraftValidationSystemPrompt 返回正文一致性校验节点的系统提示词。
func chapterDraftValidationSystemPrompt() string {
	return novelOnlyIdentityPrompt + `

你是章节正文一致性校验器。只校验正文是否严格遵守全书写作画像和六类剧情上下文，不评价文笔，不校验字数。

通过标准：
- 正文必须遵守 novel_writing_profile：叙述视角不能冲突；题材、类型、基调用于作品方向；文风用于语言质感；雷点是规避约束，正文不得主动踩中雷点。
- 正文必须严格按照 current_chapter_plan_data.summary 写。
- 如果输入中存在 previous_validation_issues，本轮必须先逐条复核这些旧问题是否已经修复；旧问题只要仍然存在，必须继续判定为 blocker，不能因为本轮还有其他检查而忽略旧问题。
- 正文必须逐项核对人物、时间、地点、人物状态、设定状态和事件顺序：人物姓名、身份、关系、出场状态不能错；时间点、时间跨度、先后顺序不能改；地点和地点规则不能错；能力、装备、组织、任务、历史事件和限制条件不能改。任一项与 summary、referenced_settings、relationships、前后章上下文不一致，必须判定为 blocker。
- 所有剧情相关内容只能来自 referenced_settings、relationships、current_volume_plan_data、previous_chapter_context、current_chapter_plan_data、next_chapter_plan_data。
- 正文中出现的任何设定，必须和 referenced_settings 中对应设定完全一致；包括 name、category、appearance_time、notes 中写明的身份、能力、规则、等级、关系、时间、地点、限制、状态和作用。
- 正文中出现的人物关系，必须和 relationships 中对应人物关系一致；没有出现在 relationships 或章节梗概中的人物关系不得自行新增或改写。
- current_chapter_plan_data.summary 是正文扩写基础；梗概中人物谈论到的事物、人物、事件和意图不得更改。
- 不得改名、换类别、改变设定含义、扩大设定能力、弱化限制、提前或延后出场阶段；如果正文描述与 referenced_settings 的 notes 不一致，必须判定为 blocker。
- 如果正文需要描写某个能力、规则、装备、组织、地点机制、历史事件或人物身份，但 referenced_settings 和本章/卷/前后章上下文没有明确依据，必须判定为 blocker。
- plan_data 没有出现的剧情实体不能新增。例如 plan_data 没有“野猫”，正文就不能写“路边看见一只野猫”。
- 不得新增 summary 之外的人物、事件、物品、生物、任务、规则、能力、装备、组织、地点机制或状态变化。
- 不得让上一章已发生状态回滚。
- 不得提前完成下一章核心事件。

只返回严格 JSON：
{
  "passed": true,
  "blockers": [{"rule": "规则名", "evidence": "正文证据", "reason": "原因"}],
  "warnings": [{"rule": "规则名", "evidence": "正文证据", "reason": "原因"}],
  "repair_instructions": ["重写指令"]
}

存在 blocker 时 passed 必须为 false。`
}

// chapterDraftValidationUserPrompt 构造正文一致性校验节点的用户提示词 payload。
func chapterDraftValidationUserPrompt(state chapterGenerationState) string {
	payload := chapterGraphContextPayload(state)
	delete(payload, "word_count_rule") // 字数要求不作为校验依据
	payload["draft"] = state.Draft
	if previous := previousValidationIssues(state.Validation); len(previous) > 0 {
		payload["previous_validation_issues"] = previous
	}
	raw, _ := json.Marshal(payload)
	return "请校验下面正文：\n" + string(raw)
}

// previousValidationIssues 提取上一轮未通过的校验问题，供重写和复核使用。
func previousValidationIssues(validation chapterDraftValidation) []model.JSONMap {
	if validation.Passed {
		return nil
	}
	items := append([]chapterDraftValidationItem{}, validation.Blockers...)
	if len(items) == 0 {
		items = append(items, validation.Warnings...)
	}
	if len(items) == 0 {
		return nil
	}
	issues := make([]model.JSONMap, 0, len(items))
	for _, item := range items {
		issues = append(issues, model.JSONMap{
			"rule":     strings.TrimSpace(item.Rule),
			"evidence": strings.TrimSpace(item.Evidence),
			"reason":   strings.TrimSpace(item.Reason),
		})
	}
	return issues
}

// chapterGraphContextPayload 汇总 Graph 生成和校验共用的写作画像与六类剧情上下文。
func chapterGraphContextPayload(state chapterGenerationState) model.JSONMap {
	payload := model.JSONMap{}
	for key, value := range state.WritingContext.Payload {
		payload[key] = value
	}
	return payload
}

// parseChapterDraftValidation 从模型回复中解析严格 JSON 校验结果。
func parseChapterDraftValidation(reply string) chapterDraftValidation {
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)
	if start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}"); start >= 0 && end > start {
		reply = reply[start : end+1]
	}
	var validation chapterDraftValidation
	if err := json.Unmarshal([]byte(reply), &validation); err != nil {
		return chapterDraftValidation{
			Passed: false,
			Blockers: []chapterDraftValidationItem{{
				Rule:   "校验返回格式",
				Reason: "模型未返回合法 JSON",
			}},
			RepairInstructions: []string{"重新生成正文，严格遵守章节规划和已给定设定。"},
		}
	}
	if !validation.Passed && len(validation.RepairInstructions) == 0 {
		validation.RepairInstructions = validationIssueTexts(validation)
	}
	return validation
}

// validationIssueTexts 将校验问题整理成前端可读文本。
func validationIssueTexts(validation chapterDraftValidation) []string {
	issues := make([]string, 0, len(validation.Blockers)+len(validation.Warnings))
	for _, item := range validation.Blockers {
		issues = append(issues, validationIssueText(item))
	}
	for _, item := range validation.Warnings {
		issues = append(issues, validationIssueText(item))
	}
	if len(issues) == 0 && !validation.Passed {
		issues = append(issues, "正文与章节规划或设定不一致。")
	}
	return issues
}

// validationIssueText 将单条校验问题压缩成一行说明。
func validationIssueText(item chapterDraftValidationItem) string {
	parts := []string{}
	if strings.TrimSpace(item.Rule) != "" {
		parts = append(parts, strings.TrimSpace(item.Rule))
	}
	if strings.TrimSpace(item.Reason) != "" {
		parts = append(parts, strings.TrimSpace(item.Reason))
	}
	if strings.TrimSpace(item.Evidence) != "" {
		parts = append(parts, "证据："+compactText(item.Evidence, 80))
	}
	return strings.Join(parts, "：")
}

// compactText 压缩长文本，用于进度提示而不是正文内容。
func compactText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}
