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

	"github.com/redis/go-redis/v9"
)

const chapterDraftTypeAIOriginal int16 = 1
const chapterDraftTypeEditable int16 = 2
const chapterDraftStatusNormal int16 = 1
const chapterDraftStatusCurrent int16 = 2
const chapterStatusCompleted int16 = 2
const chapterListCacheTTL = 7 * 24 * time.Hour

const chapterWritingPriorityRule = "正文必须持续遵守 novel_writing_profile 中的叙述视角、题材、类型、基调、文风和雷点；其中雷点是规避约束，不是可正向发挥的剧情标签。剧情事实只使用六类上下文：referenced_settings 所需设定、relationships 本章已出现人物之间的关系、current_volume_plan_data 卷规划、previous_chapter_context 上一章上下文、current_chapter_plan_data 本章规划、next_chapter_plan_data 下一章规划。previous_chapter_context_type=applied_content 时，上一章上下文是已应用正文，优先用于承接人物状态、场景余波、线索和语气；previous_chapter_context_type=summary 时，它只是上一章梗概。正文必须以 current_chapter_plan_data.summary 这条事件骨架为主线展开；梗概中人物谈论到的事物、人物、事件和意图不得更改；卷规划只限定本卷边界，上一章上下文只保证连续性，下一章规划只用于控制本章结尾衔接和避免提前写出下一章事件，intertextual_links 和 writing_focus 只用于补充跨章关联与语言表现。正文只能在原梗概上增加符合文风和写作重点的描写、动作、对话、心理和节奏，不得改变原梗概含义，不得改变剧情走向。"
const chapterWritingSettingRule = "正文必须严格遵守 current_chapter_plan_data.summary、referenced_settings、relationships、current_volume_plan_data、previous_chapter_context 和 next_chapter_plan_data；不得擅自新增或修改重要信息。所有关乎剧情的人物、人物关系、地点、势力、组织、职业、力量体系、规则、技能、装备、任务、历史事件等内容，均应来源于本章 summary、referenced_settings、relationships、卷规划、上一章上下文或下一章规划。卷规划中的临时设定位置固定为 current_volume_plan_data.temporary_settings；如果 summary、referenced_settings 或 relationships 中找不到依据，必须继续检查 current_volume_plan_data.temporary_settings.characters、current_volume_plan_data.temporary_settings.relationships、current_volume_plan_data.temporary_settings.maps、current_volume_plan_data.temporary_settings.forces、current_volume_plan_data.temporary_settings.other_settings。不得新增 summary 之外的人物、关键事件、重要物品、任务、规则、能力、装备、组织、地点机制或状态变化；不得提前完成下一章规划中的核心事件；找不到依据就放弃该描述，改写为动作、情绪、对话或已知信息，不要自行编造。"
const chapterWritingWordCountRule = "正文默认目标是至少写 2000 个中文非空白字符；用户明确要求更长时，以用户要求为准。该规则只作为生成要求提示，不作为一致性校验项。"

type chapterWritingContext struct {
	SystemContext       string
	Payload             model.JSONMap
	NovelWritingProfile model.JSONMap
	ReferencedSettings  []referencedSetting
	Relationships       []referencedRelationship
	PreviousContext     string
	PreviousContextType string
	NextPlanData        model.JSONMap
}

// ChapterService 章节服务接口，负责章节列表、章级消息和章级 AI 流。
type ChapterService interface {
	ListChapters(ctx context.Context, userID int64, volumeID int64) ([]model.Chapter, error)
	ListChapterMessages(ctx context.Context, userID int64, chapterID int64) (model.ChatMessagesResponse, error)
	StreamChapter(ctx context.Context, userID int64, chapterID int64, content string, opts ChapterStreamOptions) (<-chan ai.StreamEvent, error)
	ResumeChapterStream(ctx context.Context, userID int64, chapterID int64) (<-chan ai.StreamEvent, error)
	CancelChapterStream(ctx context.Context, userID int64, chapterID int64) error
	ListChapterDrafts(ctx context.Context, userID int64, chapterID int64) ([]model.ChapterContentDraftRecord, error)
	JoinChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) (model.ChapterContentDraftRecord, error)
	CreateDraftFromContent(ctx context.Context, userID int64, chapterID int64, content string) (model.ChapterContentDraftRecord, error)
	UpdateChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64, content string, draftName string) error
	UseChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) error
	DeleteChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) error
	HumanizeChapterContent(ctx context.Context, userID int64, chapterID int64, draftID int64, content string) (model.ChapterHumanizeResult, error)
	ProofreadChapterContent(ctx context.Context, userID int64, chapterID int64, draftID int64, content string) ([]model.ChapterProofreadSuggestion, error)
}

type chapterService struct {
	*aiStreamSupport
	generationGraph *chapterGenerationGraph
}

// ChapterStreamOptions 定义章级流式对话的运行模式。
type ChapterStreamOptions struct {
	GraphMode bool
}

// NewChapterService 创建章节服务。
func NewChapterService(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client) ChapterService {
	service := &chapterService{aiStreamSupport: newAIStreamSupport(repositories, redisClient, aiClient)}
	graph, err := service.newChapterGenerationGraph(context.Background())
	if err != nil {
		// Graph 模式初始化失败不影响旧的快速 stream 模式。
		// 运行时开启 Graph 时会返回 AI 不可用，便于用户重试或关闭模式。
		return service
	}
	service.generationGraph = graph
	return service
}

// ListChapters 查询当前用户某卷下的章节列表。
func (s *chapterService) ListChapters(ctx context.Context, userID int64, volumeID int64) ([]model.Chapter, error) {
	if _, err := s.ensureVolumeOwner(ctx, userID, volumeID); err != nil {
		return nil, wrapError("校验卷归属失败", err)
	}
	if cached, ok := s.cachedVolumeChapters(ctx, userID, volumeID); ok {
		return cached, nil
	}
	chapters, err := s.repositories.Chapters.ListByVolumeID(ctx, volumeID)
	if err != nil {
		return nil, wrapError("查询章节列表失败", err)
	}
	s.cacheVolumeChapters(ctx, userID, volumeID, chapters)
	return chapters, nil
}

// cachedVolumeChapters 从 Redis 读取卷章节列表缓存，缓存缺失或内容异常时返回 false。
func (s *chapterService) cachedVolumeChapters(ctx context.Context, userID int64, volumeID int64) ([]model.Chapter, bool) {
	if s.redisClient == nil || userID <= 0 || volumeID <= 0 {
		return nil, false
	}
	payload, err := s.redisClient.Get(ctx, volumeChapterListCacheKey(userID, volumeID)).Bytes()
	if err != nil || len(payload) == 0 {
		return nil, false
	}
	var chapters []model.Chapter
	if err := json.Unmarshal(payload, &chapters); err != nil {
		return nil, false
	}
	return chapters, true
}

// cacheVolumeChapters 将卷章节列表写入 Redis，缓存失败不影响主流程。
func (s *chapterService) cacheVolumeChapters(ctx context.Context, userID int64, volumeID int64, chapters []model.Chapter) {
	if s.redisClient == nil || userID <= 0 || volumeID <= 0 {
		return
	}
	payload, err := json.Marshal(chapters)
	if err != nil {
		return
	}
	_ = s.redisClient.Set(ctx, volumeChapterListCacheKey(userID, volumeID), payload, chapterListCacheTTL).Err()
}

// deleteVolumeChapterListCache 删除卷章节列表缓存，用于章节规划或章节正文状态变化后失效。
func (s *chapterService) deleteVolumeChapterListCache(ctx context.Context, userID int64, volumeID int64) {
	deleteVolumeChapterListCache(ctx, s.redisClient, userID, volumeID)
}

// deleteVolumeChapterListCache 删除卷章节列表缓存，供会影响章节列表展示的业务调用。
func deleteVolumeChapterListCache(ctx context.Context, redisClient *redis.Client, userID int64, volumeID int64) {
	if redisClient == nil || userID <= 0 || volumeID <= 0 {
		return
	}
	_ = redisClient.Del(ctx, volumeChapterListCacheKey(userID, volumeID)).Err()
}

// volumeChapterListCacheKey 生成卷章节列表缓存键。
func volumeChapterListCacheKey(userID int64, volumeID int64) string {
	return fmt.Sprintf("user:%d:volume:%d:chapters", userID, volumeID)
}

// ListChapterMessages 查询当前用户某章节的章级对话消息。
func (s *chapterService) ListChapterMessages(ctx context.Context, userID int64, chapterID int64) (model.ChatMessagesResponse, error) {
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("校验章节归属失败", err)
	}

	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeChapter), chapter.ID)
	if errors.Is(err, repo.ErrChatSessionNotFound) {
		return s.createInitialChapterMessages(ctx, userID, chapter)
	}
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询章节会话失败", err)
	}
	messages, err := s.repositories.ChatMessages.ListBySessionID(ctx, session.ID)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询章节会话消息失败", err)
	}
	messages = s.appendStreamingReply(ctx, messages, session.ID, int16(model.ScopeTypeChapter), chapter.ID)
	return model.ChatMessagesResponse{Messages: messages, Session: sessionMeta(session)}, nil
}

// StreamChapter 保存用户消息，调用章级 AI 流，并在完成后保存 AI 完整回复。
func (s *chapterService) StreamChapter(ctx context.Context, userID int64, chapterID int64, content string, opts ChapterStreamOptions) (<-chan ai.StreamEvent, error) {
	// 高一致性模式
	if opts.GraphMode {
		return s.streamChapterWithGraph(ctx, userID, chapterID, content)
	}

	// 普通模式
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return nil, wrapError("校验章节归属失败", err)
	}
	volume, err := s.repositories.Volumes.FindByID(ctx, chapter.VolumeID)
	if errors.Is(err, repo.ErrVolumeNotFound) {
		return nil, ErrResourceNotFound
	}
	if err != nil {
		return nil, wrapError("查询章节所属卷失败", err)
	}
	writingContext, err := s.chapterWritingContext(ctx, userID, chapter, volume)
	if err != nil {
		return nil, wrapError("构建章节写作上下文失败", err)
	}

	return s.streamScoped(ctx, streamScopeParams{
		UserID:                  userID,
		ScopeID:                 chapterID,
		ScopeType:               model.ScopeTypeChapter,
		SessionTitle:            chapterChatTitle,
		Content:                 content,
		SystemContext:           writingContext.SystemContext,
		InitialAssistantMessage: initialChapterAssistantMessage(chapter),
		HistoryFilter: func(messages []model.ChatMessage) []model.ChatMessage {
			return chapterContextMessages(messages)
		},
		SaveChapterDrafts: func(ctx context.Context, repositories repo.Repositories, sessionID int64, assistantMessageID int64, renderData model.JSONMap) (int64, error) {
			return s.saveChapterDraftsFromAssistant(ctx, repositories, userID, chapterID, assistantMessageID, renderData)
		},
	})
}

// previousChapterContext 返回当前章需要承接的上一章上下文；跨卷第一章会承接上一卷最后一章。
func (s *chapterService) previousChapterContext(ctx context.Context, chapter model.Chapter) (string, string, error) {
	previousOrder := chapter.SortOrder - 1
	if previousOrder <= 0 {
		return s.previousVolumeLastChapterContext(ctx, chapter)
	}
	previous, err := s.repositories.Chapters.FindByVolumeSortOrder(ctx, chapter.VolumeID, previousOrder)
	if errors.Is(err, repo.ErrChapterNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	previousContext := strings.TrimSpace(previous.Content)
	previousContextType := "applied_content"
	if previousContext == "" {
		previousContext = jsonMapText(previous.PlanData, "summary")
		previousContextType = "summary"
	}
	return previousContext, previousContextType, nil
}

func (s *chapterService) previousVolumeLastChapterContext(ctx context.Context, chapter model.Chapter) (string, string, error) {
	volume, err := s.repositories.Volumes.FindByID(ctx, chapter.VolumeID)
	if err != nil {
		return "", "", err
	}
	volumes, err := s.repositories.Volumes.ListByNovelID(ctx, volume.NovelID)
	if err != nil {
		return "", "", err
	}
	var previousVolume model.Volume
	for _, item := range volumes {
		if item.SortOrder < volume.SortOrder && item.SortOrder > previousVolume.SortOrder {
			previousVolume = item
		}
	}
	if previousVolume.ID == 0 {
		return "", "", nil
	}
	chapters, err := s.repositories.Chapters.ListByVolumeID(ctx, previousVolume.ID)
	if err != nil {
		return "", "", err
	}
	if len(chapters) == 0 {
		return "", "", nil
	}
	previous := chapters[len(chapters)-1]
	previousContext := strings.TrimSpace(previous.Content)
	previousContextType := "applied_content"
	if previousContext == "" {
		previousContext = jsonMapText(previous.PlanData, "summary")
		previousContextType = "summary"
	}
	return previousContext, previousContextType, nil
}

// nextChapterPlanData 返回当前卷中当前章的下一章规划，用于控制本章结尾衔接和防止抢跑。
func (s *chapterService) nextChapterPlanData(ctx context.Context, chapter model.Chapter) (model.JSONMap, error) {
	next, err := s.repositories.Chapters.FindByVolumeSortOrder(ctx, chapter.VolumeID, chapter.SortOrder+1)
	if errors.Is(err, repo.ErrChapterNotFound) {
		return model.JSONMap{}, nil
	}
	if err != nil {
		return model.JSONMap{}, err
	}
	return next.PlanData, nil
}

// chapterWritingContext 汇总普通模式和高一致性 Graph 共用的章级正文上下文。
func (s *chapterService) chapterWritingContext(ctx context.Context, userID int64, chapter model.Chapter, volume model.Volume) (chapterWritingContext, error) {
	novelWritingProfile, referencedContext, err := s.chapterReferencedSettingsContext(ctx, userID, chapter, volume.NovelID)
	if err != nil {
		return chapterWritingContext{}, err
	}
	previousContext, previousContextType, err := s.previousChapterContext(ctx, chapter)
	if err != nil {
		return chapterWritingContext{}, wrapError("读取上一章上下文失败", err)
	}
	nextPlanData, err := s.nextChapterPlanData(ctx, chapter)
	if err != nil {
		return chapterWritingContext{}, wrapError("读取下一章规划失败", err)
	}
	payload := chapterWritingContextPayload(chapter, volume, novelWritingProfile, referencedContext, previousContext, previousContextType, nextPlanData)
	systemContext := chapterWritingConstraintSystemContext(payload)
	return chapterWritingContext{
		SystemContext:       systemContext,
		Payload:             payload,
		NovelWritingProfile: novelWritingProfile,
		ReferencedSettings:  referencedContext.Settings,
		Relationships:       referencedContext.Relationships,
		PreviousContext:     previousContext,
		PreviousContextType: previousContextType,
		NextPlanData:        nextPlanData,
	}, nil
}

func chapterWritingContextPayload(chapter model.Chapter, volume model.Volume, novelWritingProfile model.JSONMap, referencedContext chapterReferencedContext, previousContext string, previousContextType string, nextPlanData model.JSONMap) model.JSONMap {
	if strings.TrimSpace(previousContext) == "" {
		previousContext = "无，当前章没有可用的上一章正文或规划上下文。"
		previousContextType = "none"
	}
	return model.JSONMap{
		"current_chapter_title":         chapterPlanTitle(chapter),
		"novel_writing_profile":         novelWritingProfile,
		"referenced_settings":           referencedContext.Settings,
		"relationships":                 referencedContext.Relationships,
		"current_volume_plan_data":      volume.PlanData, // 里面携带了卷的临时设定
		"previous_chapter_context":      previousContext,
		"previous_chapter_context_type": previousContextType,
		"current_chapter_plan_data":     chapter.PlanData,
		"next_chapter_plan_data":        nextPlanData,
		"priority_rule":                 chapterWritingPriorityRule,
		"setting_rule":                  chapterWritingSettingRule,
		"word_count_rule":               chapterWritingWordCountRule,
	}
}

// chapterWritingConstraintSystemContext 生成章级正文主约束，要求正文优先执行当前章规划。
func chapterWritingConstraintSystemContext(payload model.JSONMap) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "正文写作上下文如下，只能用于生成当前章正文，不要向用户复述这些内部上下文：\n" + string(raw)
}

// joinSystemContexts 合并多个系统上下文片段，过滤空白内容。
func joinSystemContexts(contexts ...string) string {
	parts := make([]string, 0, len(contexts))
	for _, contextText := range contexts {
		contextText = strings.TrimSpace(contextText)
		if contextText != "" {
			parts = append(parts, contextText)
		}
	}
	return strings.Join(parts, "\n\n")
}

// chapterReferencedSettingsContext 为章级正文按需补充当前章 references 命中的小说设定和全书写作画像。
func (s *chapterService) chapterReferencedSettingsContext(ctx context.Context, userID int64, chapter model.Chapter, novelID int64) (model.JSONMap, chapterReferencedContext, error) {
	novel, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return nil, chapterReferencedContext{}, wrapError("校验小说归属失败", err)
	}
	profile := novelWritingProfileFromPlanData(novel.PlanData)
	if cached, ok := cachedChapterReferencedContext(ctx, s.redisClient, userID, chapter.ID); ok {
		return profile, cached, nil
	}
	settings, err := referencedSettingsForChapter(chapter, buildNovelSettingIndex(novel.PlanData))
	if err != nil {
		return nil, chapterReferencedContext{}, err
	}
	context := chapterReferencedContext{
		Settings:      settings,
		Relationships: referencedRelationshipsForChapter(chapter, novel.PlanData, settings),
	}
	cacheChapterReferencedContext(ctx, s.redisClient, userID, chapter.ID, context)
	return profile, context, nil
}

func referencedSettingsSystemContext(settings []referencedSetting) (string, error) {
	if len(settings) == 0 {
		return "", nil
	}
	payload := model.JSONMap{"referenced_settings": settings}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", wrapError("序列化章节引用设定上下文失败", err)
	}
	return "本章引用设定如下，正文只能把这些设定作为可用设定白名单；不要读取或推测完整小说梗概，不要向用户复述这些内部上下文：\n" + string(raw), nil
}

// ResumeChapterStream 重建章级临时回复 SSE，只转发 Redis 快照后续变化。
func (s *chapterService) ResumeChapterStream(ctx context.Context, userID int64, chapterID int64) (<-chan ai.StreamEvent, error) {
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return nil, wrapError("校验章节归属失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeChapter), chapter.ID)
	if err != nil {
		return nil, err
	}
	return s.resumeStreamingReply(ctx, session.ID, int16(model.ScopeTypeChapter), chapter.ID), nil
}

// CancelChapterStream 取消章级正在生成的 AI 回复。
func (s *chapterService) CancelChapterStream(ctx context.Context, userID int64, chapterID int64) error {
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return wrapError("查询章节会话失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeChapter), chapter.ID)
	if err != nil {
		return err
	}
	return s.cancelStreamingReply(ctx, session.ID)
}

// chapterContextMessages 只保留章级对话文本，正文生成不读取历史 render_data 草稿。
func chapterContextMessages(messages []model.ChatMessage) []model.ChatMessage {
	history := make([]model.ChatMessage, 0, len(messages))
	for _, message := range messages {
		message.Content = strings.TrimSpace(message.Content)
		if message.Content == "" {
			continue
		}
		history = append(history, message)
	}
	return history
}

// ListChapterDrafts 查询当前章节已加入编辑器的草稿列表。
func (s *chapterService) ListChapterDrafts(ctx context.Context, userID int64, chapterID int64) ([]model.ChapterContentDraftRecord, error) {
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return nil, wrapError("校验章节归属失败", err)
	}
	drafts, err := s.repositories.ChapterContents.ListDraftsByChapter(ctx, chapterID, chapterDraftTypeEditable, chapterDraftStatusCurrent)
	return drafts, wrapError("查询章节草稿列表失败", err)
}

// JoinChapterDraft 将 AI 原始草稿复制为用户可编辑草稿。
func (s *chapterService) JoinChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) (model.ChapterContentDraftRecord, error) {
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return model.ChapterContentDraftRecord{}, wrapError("校验章节归属失败", err)
	}
	var draft model.ChapterContentDraftRecord
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		var err error
		draft, err = repositories.ChapterContents.CopyDraft(ctx, chapterID, draftID, chapterDraftTypeAIOriginal, chapterDraftTypeEditable, defaultEditableDraftName(time.Now()))
		return err
	})
	if err != nil {
		if errors.Is(err, repo.ErrChapterNotFound) {
			return model.ChapterContentDraftRecord{}, ErrResourceNotFound
		}
		return model.ChapterContentDraftRecord{}, wrapError("加入章节草稿失败", err)
	}
	return draft, nil
}

// CreateDraftFromContent 将一段文本直接保存为该章节的可编辑草稿，不依赖消息或已有草稿。
func (s *chapterService) CreateDraftFromContent(ctx context.Context, userID int64, chapterID int64, content string) (model.ChapterContentDraftRecord, error) {
	if strings.TrimSpace(content) == "" {
		return model.ChapterContentDraftRecord{}, ErrInvalidMessage
	}
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return model.ChapterContentDraftRecord{}, wrapError("校验章节归属失败", err)
	}
	var draft model.ChapterContentDraftRecord
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		var err error
		draft, err = repositories.ChapterContents.CreateDraft(ctx, chapterID, 0, content, countTextWords(content), chapterDraftTypeEditable, defaultEditableDraftName(time.Now()))
		return err
	})
	if err != nil {
		return model.ChapterContentDraftRecord{}, wrapError("创建草稿失败", err)
	}
	return draft, nil
}

// UpdateChapterDraft 更新当前用户章节下的可编辑草稿正文。
func (s *chapterService) UpdateChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64, content string, draftName string) error {
	draftName = strings.TrimSpace(draftName)
	if strings.TrimSpace(content) == "" {
		return ErrInvalidMessage
	}
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return wrapError("校验章节归属失败", err)
	}
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		draft, err := repositories.ChapterContents.FindByID(ctx, draftID)
		if err != nil {
			return err
		}
		if !editableChapterDraftBelongsToChapter(draft, chapterID) {
			return repo.ErrChapterNotFound
		}
		fields := repo.UpdateFields{
			"content":    content,
			"word_count": countTextWords(content),
		}
		if draftName != "" {
			fields["draft_name"] = draftName
		}
		return repositories.ChapterContents.Update(ctx, draft.ID, fields)
	})
	if err != nil {
		if errors.Is(err, repo.ErrChapterNotFound) {
			return ErrResourceNotFound
		}
		return wrapError("更新章节草稿失败", err)
	}
	return nil
}

// DeleteChapterDraft 删除可编辑草稿；当前正文需要保留，不允许在草稿列表中删除。
func (s *chapterService) DeleteChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) error {
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return wrapError("校验章节归属失败", err)
	}
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		draft, err := repositories.ChapterContents.FindByID(ctx, draftID)
		if err != nil {
			return err
		}
		if draft.ChapterID != chapterID || draft.DraftType != chapterDraftTypeEditable || draft.Status != chapterDraftStatusNormal {
			return repo.ErrChapterNotFound
		}
		return repositories.ChapterContents.DeleteByID(ctx, draft.ID)
	})
	if err != nil {
		if errors.Is(err, repo.ErrChapterNotFound) {
			return ErrResourceNotFound
		}
		return wrapError("删除章节草稿失败", err)
	}
	return nil
}

// UseChapterDraft 将指定正文草稿设为当前章节正文。
func (s *chapterService) UseChapterDraft(ctx context.Context, userID int64, chapterID int64, draftID int64) error {
	chapter, err := s.ensureChapterOwner(ctx, userID, chapterID)
	if err != nil {
		return wrapError("校验章节归属失败", err)
	}
	volume, err := s.ensureVolumeOwner(ctx, userID, chapter.VolumeID)
	if err != nil {
		return wrapError("校验卷归属失败", err)
	}
	err = s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		draft, err := repositories.ChapterContents.FindByID(ctx, draftID)
		if err != nil {
			return err
		}
		if !editableChapterDraftBelongsToChapter(draft, chapterID) || strings.TrimSpace(draft.Content) == "" {
			return repo.ErrChapterNotFound
		}
		drafts, err := repositories.ChapterContents.ListDraftsByChapter(ctx, chapterID, chapterDraftTypeEditable, chapterDraftStatusCurrent)
		if err != nil {
			return err
		}
		now := time.Now()
		for _, item := range drafts {
			if item.ID == draft.ID || item.Status != chapterDraftStatusCurrent {
				continue
			}
			if err = repositories.ChapterContents.Update(ctx, item.ID, repo.UpdateFields{
				"status": chapterDraftStatusNormal,
			}); err != nil {
				return err
			}
		}
		draft.UsedAt = &now
		if err = repositories.ChapterContents.Update(ctx, draft.ID, repo.UpdateFields{
			"status":  chapterDraftStatusCurrent,
			"used_at": draft.UsedAt,
		}); err != nil {
			return err
		}
		chapterFields := repo.UpdateFields{
			"status":     chapterStatusCompleted,
			"word_count": draft.WordCount,
		}
		if chapter.CompletedAt == nil {
			chapterFields["completed_at"] = &now
		}
		return repositories.Chapters.Update(ctx, chapter.ID, chapterFields)
	})
	if err != nil {
		if errors.Is(err, repo.ErrChapterNotFound) {
			return ErrResourceNotFound
		}
		return wrapError("应用章节草稿失败", err)
	}
	s.deleteVolumeChapterListCache(ctx, userID, chapter.VolumeID)
	deleteNovelVolumeListCache(ctx, s.redisClient, userID, volume.NovelID)
	return nil
}

func editableChapterDraftBelongsToChapter(draft model.ChapterContentDraftRecord, chapterID int64) bool {
	return draft.ChapterID == chapterID &&
		draft.DraftType == chapterDraftTypeEditable &&
		draft.IsDeleted == 0
}

// HumanizeChapterContent 对当前章节正文执行 AI 消痕，返回润色文本和 Markdown 报告，不直接写库。
func (s *chapterService) HumanizeChapterContent(ctx context.Context, userID int64, chapterID int64, draftID int64, content string) (model.ChapterHumanizeResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return model.ChapterHumanizeResult{}, ErrInvalidMessage
	}
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return model.ChapterHumanizeResult{}, wrapError("校验章节归属失败", err)
	}
	modelConfig, err := s.resolveUserModelConfig(ctx, userID)
	if err != nil {
		return model.ChapterHumanizeResult{}, wrapError("解析 AI 消痕模型失败", err)
	}
	humanizerAgent := buildStoryHumanizerAgentDefinition(s.storyEditSkill)
	runStartedAt := time.Now()
	runID := s.createModelRun(ctx, modelRunMeta{
		UserID:    userID,
		ScopeType: model.ModelRunScopeDraftHumanize,
		ScopeID:   optionalScopeID(draftID),
		ModelID:   modelConfig.ID,
		Status:    model.ModelRunStatusRunning,
		StartTime: runStartedAt,
	})
	callCtx, cancelCall := s.aiCallContext(ctx)
	defer cancelCall()
	result, err := s.aiClient.GenerateAgent(callCtx, ai.AgentGenerateRequest{
		ChatGenerateRequest: ai.ChatGenerateRequest{
			UserID:   userID,
			ModelKey: modelConfig.AgentKey(),
			Model:    modelConfig.RuntimeConfig(),
			Messages: []ai.StreamMessage{{
				Role:    "user",
				Content: storyHumanizerUserPrompt(content),
			}},
		},
		Agent: humanizerAgent,
	})
	if err != nil {
		status := model.ModelRunStatusFailed
		finishReason := ""
		if errors.Is(err, context.Canceled) || ctx.Err() != nil || callCtx.Err() != nil {
			status = model.ModelRunStatusCanceled
			finishReason = s.canceledModelRunFinishReason()
		}
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       status,
			FinishReason: finishReason,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		if errors.Is(err, ai.ErrInvalidChatModelConfig) {
			return model.ChapterHumanizeResult{}, ErrInvalidModel
		}
		return model.ChapterHumanizeResult{}, wrapError("执行 AI 消痕失败", err)
	}
	humanizeResult := parseHumanizerResult(result.Content)
	if strings.TrimSpace(humanizeResult.Content) == "" {
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   result.TokenCount,
			FinishReason: result.FinishReason,
			ErrorMessage: "AI 消痕未返回有效正文",
			EndTime:      timePtr(time.Now()),
		})
		return model.ChapterHumanizeResult{}, ErrAIUnavailable
	}
	if sameHumanizedContent(content, humanizeResult.Content) {
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   result.TokenCount,
			FinishReason: result.FinishReason,
			ErrorMessage: "AI 消痕结果与原文实质相同",
			EndTime:      timePtr(time.Now()),
		})
		return model.ChapterHumanizeResult{}, ErrAIUnavailable
	}
	s.finishModelRun(context.Background(), modelRunMeta{
		ID:           runID,
		Status:       model.ModelRunStatusSuccess,
		TokenCount:   result.TokenCount,
		FinishReason: result.FinishReason,
		EndTime:      timePtr(time.Now()),
	})
	return humanizeResult, nil
}

// ProofreadChapterContent 对当前章节正文执行 AI 校审，返回临时修改建议而不写入数据库。
func (s *chapterService) ProofreadChapterContent(ctx context.Context, userID int64, chapterID int64, draftID int64, content string) ([]model.ChapterProofreadSuggestion, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrInvalidMessage
	}
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return nil, wrapError("校验章节归属失败", err)
	}
	modelConfig, err := s.resolveUserModelConfig(ctx, userID)
	if err != nil {
		return nil, wrapError("解析 AI 校审模型失败", err)
	}
	runStartedAt := time.Now()
	runID := s.createModelRun(ctx, modelRunMeta{
		UserID:    userID,
		ScopeType: model.ModelRunScopeDraftProofread,
		ScopeID:   optionalScopeID(draftID),
		ModelID:   modelConfig.ID,
		Status:    model.ModelRunStatusRunning,
		StartTime: runStartedAt,
	})
	callCtx, cancelCall := s.aiCallContext(ctx)
	defer cancelCall()
	result, err := s.aiClient.GenerateChat(callCtx, ai.ChatGenerateRequest{
		UserID:   userID,
		ModelKey: modelConfig.AgentKey(),
		Model:    modelConfig.RuntimeConfig(),
		Messages: []ai.StreamMessage{
			{Role: "system", Content: chapterProofreadSystemPrompt()},
			{Role: "user", Content: "请校审下面这一章正文，只返回 JSON 数组：\n\n" + content},
		},
	})
	if err != nil {
		status := model.ModelRunStatusFailed
		finishReason := ""
		if errors.Is(err, context.Canceled) || ctx.Err() != nil || callCtx.Err() != nil {
			status = model.ModelRunStatusCanceled
			finishReason = s.canceledModelRunFinishReason()
		}
		s.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       status,
			FinishReason: finishReason,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		if errors.Is(err, ai.ErrInvalidChatModelConfig) {
			return nil, ErrInvalidModel
		}
		return nil, wrapError("执行 AI 校审失败", err)
	}
	suggestions := parseProofreadSuggestions(result.Content)
	s.finishModelRun(context.Background(), modelRunMeta{
		ID:           runID,
		Status:       model.ModelRunStatusSuccess,
		TokenCount:   result.TokenCount,
		FinishReason: result.FinishReason,
		EndTime:      timePtr(time.Now()),
	})
	return suggestions, nil
}

// chapterProofreadSystemPrompt 返回章节校审专用提示词，要求模型输出稳定 JSON。
func chapterProofreadSystemPrompt() string {
	return novelOnlyIdentityPrompt + `

你是小说章节语法校审助手。请只检查用户手动修改后的当前章节正文，重点找出错字、语病、标点误用、明显逻辑矛盾、指代不清等硬性问题。

只能输出严格 JSON 数组，不要 Markdown，不要解释。数组元素字段固定为：
{
  "originalText": "原文本，必须逐字出现在正文中",
  "suggestedText": "建议文本",
  "reason": "修改原因"
}

规则：
- 只返回确有修改价值的问题；没有问题时返回 []。
- originalText 必须短而准确，能在原文中直接定位。
- suggestedText 只能做必要校正，不得润色文风、扩写内容、改剧情、改人物设定或重写句子。
- 不要把正常的小说化表达、人物语气、节奏停顿当作错误。
- 最多返回 10 条。`
}

type proofreadSuggestionJSON struct {
	OriginalTextCN  string `json:"原文本"`
	SuggestedTextCN string `json:"建议文本"`
	ReasonCN        string `json:"修改原因"`
	OriginalText    string `json:"originalText"`
	SuggestedText   string `json:"suggestedText"`
	Reason          string `json:"reason"`
}

// parseProofreadSuggestions 解析模型校审 JSON，过滤空项和无变化项。
func parseProofreadSuggestions(reply string) []model.ChapterProofreadSuggestion {
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)
	if start, end := strings.Index(reply, "["), strings.LastIndex(reply, "]"); start >= 0 && end > start {
		reply = reply[start : end+1]
	}
	var raw []proofreadSuggestionJSON
	if err := json.Unmarshal([]byte(reply), &raw); err != nil {
		return nil
	}
	suggestions := make([]model.ChapterProofreadSuggestion, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		original := firstNonEmptyText(item.OriginalText, item.OriginalTextCN)
		suggested := firstNonEmptyText(item.SuggestedText, item.SuggestedTextCN)
		reason := firstNonEmptyText(item.Reason, item.ReasonCN)
		if original == "" || suggested == "" || original == suggested {
			continue
		}
		key := original + "\n" + suggested
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		suggestions = append(suggestions, model.ChapterProofreadSuggestion{
			OriginalText:  original,
			SuggestedText: suggested,
			Reason:        reason,
		})
	}
	return suggestions
}

// firstNonEmptyText 返回第一个非空文本。
func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

// storyHumanizerUserPrompt 构造 AI 消痕 Agent 的用户输入，确保 Agent 只处理当前章节正文。
func storyHumanizerUserPrompt(content string) string {
	return "请只处理下面这一章正文。必须改写正文，不得原样返回；返回值必须是严格 JSON 对象，字段只能是 content 和 report。\n\n原文如下：\n\n" + content
}

// sameHumanizedContent 判断消痕结果是否实质等同原文，防止模型只生成报告而不改正文。
func sameHumanizedContent(original string, humanized string) bool {
	return normalizeHumanizedCompareText(original) == normalizeHumanizedCompareText(humanized)
}

// normalizeHumanizedCompareText 去掉空白差异，用于判断消痕文本是否仍是原文。
func normalizeHumanizedCompareText(content string) string {
	var builder strings.Builder
	for _, ch := range strings.TrimSpace(content) {
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == '　' {
			continue
		}
		builder.WriteRune(ch)
	}
	return builder.String()
}

// parseHumanizerResult 解析 AI 消痕结果，支持模型误包裹 Markdown 代码块的情况。
func parseHumanizerResult(reply string) model.ChapterHumanizeResult {
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)
	if start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}"); start >= 0 && end > start {
		reply = reply[start : end+1]
	}
	var result model.ChapterHumanizeResult
	if err := json.Unmarshal([]byte(reply), &result); err == nil {
		result.Content = strings.TrimSpace(result.Content)
		result.Report = strings.TrimSpace(result.Report)
		return result
	}
	return model.ChapterHumanizeResult{
		Content: "",
		Report:  "## AI 消痕报告\n\n模型未返回合法 JSON，请重试。",
	}
}

// saveChapterDraftsFromAssistant 从助手消息渲染数据中提取正文草稿，并保存为未使用草稿。
func (s *chapterService) saveChapterDraftsFromAssistant(ctx context.Context, repositories repo.Repositories, userID int64, chapterID int64, assistantMessageID int64, renderData model.JSONMap) (int64, error) {
	draftContent := extractChapterDraftContentFromRenderData(renderData)
	if draftContent == "" {
		return 0, nil
	}
	if _, err := s.ensureChapterOwner(ctx, userID, chapterID); err != nil {
		return 0, wrapError("校验章节归属失败", err)
	}
	draft, err := repositories.ChapterContents.CreateDraft(ctx, chapterID, assistantMessageID, draftContent, countTextWords(draftContent), chapterDraftTypeAIOriginal, "AI 原始草稿")
	if err != nil {
		return 0, wrapError("保存 AI 原始草稿失败", err)
	}
	return draft.ID, nil
}

// defaultEditableDraftName 生成可编辑草稿默认名称，统一由业务层决定命名规则。
func defaultEditableDraftName(now time.Time) string {
	return "草稿 " + now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("01-02 15:04")
}

// createInitialChapterMessages 创建章级初始对话，并返回持久化后的助手开场白。
func (s *chapterService) createInitialChapterMessages(ctx context.Context, userID int64, chapter model.Chapter) (model.ChatMessagesResponse, error) {
	sessionID, err := s.repositories.ChatSessions.Create(ctx, userID, int16(model.ScopeTypeChapter), chapter.ID, chapterChatTitle)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("创建章节会话失败", err)
	}
	message, err := createChatMessage(ctx, s.repositories, sessionID, chatRoleAssistant, initialChapterAssistantMessage(chapter))
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("创建章节初始消息失败", err)
	}
	message.Role = "assistant"
	return model.ChatMessagesResponse{
		Messages: []model.ChatMessage{message},
		Session:  openSessionMeta(sessionID, model.ScopeTypeChapter, chapter.ID),
	}, nil
}

// initialChapterAssistantMessage 根据章节梗概生成章级对话开场白，引导用户写作正文。
func initialChapterAssistantMessage(chapter model.Chapter) string {
	plan := chapterPlanFromData(chapter.PlanData)
	return fmt.Sprintf("## 本章：%s\n\n### 本章规划\n\n**章梗概：**\n%s\n\n**跨章关联：** %s\n\n**写作重点：** %s\n\n接下来你希望这一章怎么写？可以告诉我你的想法，或者直接让我按本章规划写成正文。",
		chapterPlanTitle(chapter),
		plan.Summary,
		plan.IntertextualLinks,
		plan.WritingFocus,
	)
}

type chapterPlanText struct {
	Summary           string
	IntertextualLinks string
	WritingFocus      string
}

// chapterPlanFromData 从章节规划 JSON 中提取章级初始化消息需要的字段。
func chapterPlanFromData(planData model.JSONMap) chapterPlanText {
	plan := chapterPlanText{
		Summary:           "当前章节还没有详细梗概。",
		IntertextualLinks: "无",
		WritingFocus:      "未填写",
	}

	if value := jsonMapText(planData, "summary"); value != "" {
		plan.Summary = value
	}
	if value := jsonMapText(planData, "intertextual_links"); value != "" {
		plan.IntertextualLinks = value
	}
	if value := jsonMapText(planData, "writing_focus"); value != "" {
		plan.WritingFocus = value
	}
	return plan
}

// jsonValueText 将 JSON 字段值转换为文本，空值返回空字符串。
func jsonValueText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// countTextWords 统计正文非空白字符数，作为中文网文章节字数。
func countTextWords(content string) int {
	count := 0
	for _, ch := range content {
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			continue
		}
		count++
	}
	return count
}

// ensureChapterOwner 校验章节存在且所属卷归当前用户所有。
func (s *chapterService) ensureChapterOwner(ctx context.Context, userID int64, chapterID int64) (model.Chapter, error) {
	return ensureChapterOwnerWithRepositories(ctx, s.repositories, userID, chapterID)
}

// ensureChapterOwnerWithRepositories 校验章节存在且所属卷归当前用户所有，统一章节级资源归属错误语义。
func ensureChapterOwnerWithRepositories(ctx context.Context, repositories repo.Repositories, userID int64, chapterID int64) (model.Chapter, error) {
	chapter, err := repositories.Chapters.FindByID(ctx, chapterID)
	if errors.Is(err, repo.ErrChapterNotFound) {
		return model.Chapter{}, ErrResourceNotFound
	}
	if err != nil {
		return model.Chapter{}, err
	}
	if _, err := ensureVolumeOwnerWithRepositories(ctx, repositories, userID, chapter.VolumeID); err != nil {
		return model.Chapter{}, err
	}
	return chapter, nil
}
