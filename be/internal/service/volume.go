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

const volumeListCacheTTL = 7 * 24 * time.Hour

// VolumeService 卷服务接口，负责卷列表、卷级消息和卷级 AI 流。
type VolumeService interface {
	ListVolumes(ctx context.Context, userID int64, novelID int64) ([]model.Volume, error)
	ListVolumeMessages(ctx context.Context, userID int64, volumeID int64) (model.ChatMessagesResponse, error)
	StreamVolume(ctx context.Context, userID int64, volumeID int64, content string) (<-chan ai.StreamEvent, error)
	ResumeVolumeStream(ctx context.Context, userID int64, volumeID int64) (<-chan ai.StreamEvent, error)
	CancelVolumeStream(ctx context.Context, userID int64, volumeID int64) error
	ApplyChapterPlan(ctx context.Context, userID int64, volumeID int64, plans []ChapterPlan, force bool) ([]model.Chapter, error)
}

type volumeService struct {
	*aiStreamSupport
}

// NewVolumeService 创建卷服务。
func NewVolumeService(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client) VolumeService {
	return &volumeService{aiStreamSupport: newAIStreamSupport(repositories, redisClient, aiClient)}
}

// ListVolumes 查询当前用户某本小说下的卷列表。
func (s *volumeService) ListVolumes(ctx context.Context, userID int64, novelID int64) ([]model.Volume, error) {
	if _, err := s.ensureNovelOwner(ctx, userID, novelID); err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	if cached, ok := s.cachedNovelVolumes(ctx, userID, novelID); ok {
		return cached, nil
	}
	volumes, err := s.repositories.Volumes.ListByNovelID(ctx, novelID)
	if err != nil {
		return nil, wrapError("查询卷列表失败", err)
	}
	for i := range volumes {
		volumes[i].WordCount = s.volumeWordCount(ctx, volumes[i].ID)
	}
	s.cacheNovelVolumes(ctx, userID, novelID, volumes)
	return volumes, nil
}

// cachedNovelVolumes 从 Redis 读取小说卷列表缓存，缓存缺失或内容异常时返回 false。
func (s *volumeService) cachedNovelVolumes(ctx context.Context, userID int64, novelID int64) ([]model.Volume, bool) {
	if s.redisClient == nil || userID <= 0 || novelID <= 0 {
		return nil, false
	}
	payload, err := s.redisClient.Get(ctx, novelVolumeListCacheKey(userID, novelID)).Bytes()
	if err != nil || len(payload) == 0 {
		return nil, false
	}
	var volumes []model.Volume
	if err := json.Unmarshal(payload, &volumes); err != nil {
		return nil, false
	}
	return volumes, true
}

// cacheNovelVolumes 将小说卷列表写入 Redis，缓存失败不影响主流程。
func (s *volumeService) cacheNovelVolumes(ctx context.Context, userID int64, novelID int64, volumes []model.Volume) {
	if s.redisClient == nil || userID <= 0 || novelID <= 0 {
		return
	}
	payload, err := json.Marshal(volumes)
	if err != nil {
		return
	}
	_ = s.redisClient.Set(ctx, novelVolumeListCacheKey(userID, novelID), payload, volumeListCacheTTL).Err()
}

// deleteNovelVolumeListCache 删除小说卷列表缓存，用于卷规划、章节数量或卷字数变化后失效。
func (s *volumeService) deleteNovelVolumeListCache(ctx context.Context, userID int64, novelID int64) {
	deleteNovelVolumeListCache(ctx, s.redisClient, userID, novelID)
}

// deleteNovelVolumeListCache 删除小说卷列表缓存，供会影响卷列表展示的业务调用。
func deleteNovelVolumeListCache(ctx context.Context, redisClient *redis.Client, userID int64, novelID int64) {
	if redisClient == nil || userID <= 0 || novelID <= 0 {
		return
	}
	_ = redisClient.Del(ctx, novelVolumeListCacheKey(userID, novelID)).Err()
}

// novelVolumeListCacheKey 生成小说卷列表缓存键。
func novelVolumeListCacheKey(userID int64, novelID int64) string {
	return fmt.Sprintf("user:%d:novel:%d:volumes", userID, novelID)
}

// ListVolumeMessages 查询当前用户某卷的卷级对话消息。
func (s *volumeService) ListVolumeMessages(ctx context.Context, userID int64, volumeID int64) (model.ChatMessagesResponse, error) {
	volume, err := s.ensureVolumeOwner(ctx, userID, volumeID)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("校验卷归属失败", err)
	}

	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeVolume), volumeID)
	if errors.Is(err, repo.ErrChatSessionNotFound) {
		return s.createInitialVolumeMessages(ctx, userID, volume)
	}
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询卷会话失败", err)
	}
	messages, err := s.repositories.ChatMessages.ListBySessionID(ctx, session.ID)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询卷会话消息失败", err)
	}
	messages = s.appendStreamingReply(ctx, messages, session.ID, int16(model.ScopeTypeVolume), volumeID)
	return model.ChatMessagesResponse{Messages: messages, Session: sessionMeta(session)}, nil
}

// StreamVolume 保存用户消息，调用卷级 AI 流，并在完成后保存 AI 完整回复。
func (s *volumeService) StreamVolume(ctx context.Context, userID int64, volumeID int64, content string) (<-chan ai.StreamEvent, error) {
	volume, err := s.ensureVolumeOwner(ctx, userID, volumeID)
	if err != nil {
		return nil, wrapError("校验卷归属失败", err)
	}
	contextText, err := s.volumePlanningSystemContext(ctx, userID, volume)
	if err != nil {
		return nil, wrapError("构建卷规划上下文失败", err)
	}
	return s.streamScoped(ctx, streamScopeParams{
		UserID:                  userID,
		ScopeID:                 volumeID,
		ScopeType:               model.ScopeTypeVolume,
		SessionTitle:            volumeChatTitle,
		Content:                 content,
		SystemContext:           contextText,
		InitialAssistantMessage: initialVolumeAssistantMessage(volume),
	})
}

// ResumeVolumeStream 重建卷级临时回复 SSE，只转发 Redis 快照后续变化。
func (s *volumeService) ResumeVolumeStream(ctx context.Context, userID int64, volumeID int64) (<-chan ai.StreamEvent, error) {
	if _, err := s.ensureVolumeOwner(ctx, userID, volumeID); err != nil {
		return nil, wrapError("校验卷归属失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeVolume), volumeID)
	if err != nil {
		return nil, wrapError("查询卷会话失败", err)
	}
	return s.resumeStreamingReply(ctx, session.ID, int16(model.ScopeTypeVolume), volumeID), nil
}

// CancelVolumeStream 取消卷级正在生成的 AI 回复。
func (s *volumeService) CancelVolumeStream(ctx context.Context, userID int64, volumeID int64) error {
	if _, err := s.ensureVolumeOwner(ctx, userID, volumeID); err != nil {
		return wrapError("校验卷归属失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeVolume), volumeID)
	if err != nil {
		return wrapError("查询卷会话失败", err)
	}
	return s.cancelStreamingReply(ctx, session.ID)
}

// ApplyChapterPlan 由用户点击“应用规划”后保存当前展示的完整章节规划。
func (s *volumeService) ApplyChapterPlan(ctx context.Context, userID int64, volumeID int64, plans []ChapterPlan, force bool) ([]model.Chapter, error) {
	plans = normalizeChapterPlans(plans)
	if len(plans) == 0 {
		return nil, ErrInvalidMessage
	}
	if _, err := s.ensureVolumeOwner(ctx, userID, volumeID); err != nil {
		return nil, wrapError("校验卷归属失败", err)
	}
	existing, err := s.repositories.Chapters.ListByVolumeID(ctx, volumeID)
	if err != nil {
		return nil, wrapError("查询当前卷已保存章节失败", err)
	}
	if len(existing) > 0 && !force {
		return nil, ErrPlanOverwriteRequired
	}
	if _, err := s.saveVolumeChapters(ctx, userID, volumeID, plans, existing); err != nil {
		return nil, err
	}
	chapters, err := s.repositories.Chapters.ListByVolumeID(ctx, volumeID)
	if err != nil {
		return nil, wrapError("读取已应用章节规划失败", err)
	}
	return chapters, nil
}

// volumePlanningSystemContext 组装卷级章节规划上下文，直接使用小说规划和当前卷规划。
func (s *volumeService) volumePlanningSystemContext(ctx context.Context, userID int64, volume model.Volume) (string, error) {
	novel, err := s.ensureNovelOwner(ctx, userID, volume.NovelID)
	if err != nil {
		return "", wrapError("校验小说归属失败", err)
	}
	savedChapters, err := s.repositories.Chapters.ListByVolumeID(ctx, volume.ID)
	if err != nil {
		return "", wrapError("查询当前卷已保存章节失败", err)
	}
	payload := model.JSONMap{
		"novel_plan_data":                      novel.PlanData,
		"novel_writing_profile":                novelWritingProfileFromPlanData(novel.PlanData),
		"current_volume_plan_data":             volume.PlanData,
		"current_volume_chapter_count":         volumeChapterCount(volume.PlanData),
		"current_volume_saved_chapter_count":   len(savedChapters),
		"current_volume_has_saved_chapters":    len(savedChapters) > 0,
		"current_volume_has_no_saved_chapters": len(savedChapters) == 0,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", wrapError("序列化卷规划上下文失败", err)
	}
	return "卷级章节规划必须参考以下上下文。不要向用户复述这些内部上下文，只用于生成详细章节规划；如果 current_volume_has_saved_chapters 为 true，用户点击“应用规划”会覆盖旧章节、正文草稿和相关对话；AI 只能提示用户点击卡片右上角按钮，不能自行保存：\n" + string(raw), nil
}

// volumeChapterCount 从卷规划 JSON 中读取本卷应生成的章节数量。
func volumeChapterCount(planData model.JSONMap) int {
	value, ok := planData["chapter_count"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		return 0
	}
}

// createInitialVolumeMessages 创建卷级初始对话，并返回持久化后的助手开场白。
func (s *volumeService) createInitialVolumeMessages(ctx context.Context, userID int64, volume model.Volume) (model.ChatMessagesResponse, error) {
	sessionID, err := s.repositories.ChatSessions.Create(ctx, userID, int16(model.ScopeTypeVolume), volume.ID, volumeChatTitle)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("创建卷会话失败", err)
	}
	message, err := createChatMessage(ctx, s.repositories, sessionID, chatRoleAssistant, initialVolumeAssistantMessage(volume))
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("创建卷初始消息失败", err)
	}
	message.Role = "assistant"
	return model.ChatMessagesResponse{
		Messages: []model.ChatMessage{message},
		Session:  openSessionMeta(sessionID, model.ScopeTypeVolume, volume.ID),
	}, nil
}

// initialVolumeAssistantMessage 根据卷梗概生成卷级对话开场白，引导用户规划章节。
func initialVolumeAssistantMessage(volume model.Volume) string {
	title := jsonMapText(volume.PlanData, "title")
	if title == "" {
		title = fmt.Sprintf("第%d卷", volume.SortOrder)
	}
	summary := firstNonEmptyJSONMapText(volume.PlanData, "summary", "description")
	if summary == "" {
		summary = "当前卷还没有详细梗概。"
	}
	return fmt.Sprintf("## 本卷：%s\n\n**卷梗概：** %s\n\n接下来你希望本卷下的章节怎么规划？可以告诉你的想法，或者直接让我按本卷梗概生成章节列表。", title, summary)
}

// saveVolumeChapters 校验卷归属后保存用户手动应用的完整章节规划。
func (s *volumeService) saveVolumeChapters(ctx context.Context, userID int64, volumeID int64, plans []ChapterPlan, oldChapters []model.Chapter) ([]SavedChapter, error) {
	volume, err := s.ensureVolumeOwner(ctx, userID, volumeID)
	if err != nil {
		return nil, wrapError("校验卷归属失败", err)
	}
	novel, err := s.ensureNovelOwner(ctx, userID, volume.NovelID)
	if err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	settingIndex := buildNovelSettingIndex(novel.PlanData)
	plans, err = completeChapterPlanReferences(plans, settingIndex)
	if err != nil {
		return nil, err
	}

	chapters := make([]model.Chapter, 0, len(plans))
	for i, plan := range plans {
		sortOrder := plan.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		title := strings.TrimSpace(plan.Title)
		if title == "" {
			title = fmt.Sprintf("第%d章", sortOrder)
		}
		chapters = append(chapters, model.Chapter{
			VolumeID:  volumeID,
			PlanData:  formatChapterPlanData(plan),
			SortOrder: sortOrder,
			Status:    1,
			WordCount: 0,
		})
	}

	var saved []model.Chapter
	err = s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		if len(oldChapters) > 0 {
			oldChapterIDs := chapterIDs(oldChapters)
			if err := deleteScopedChatSessions(ctx, repositories, model.ScopeTypeChapter, oldChapterIDs); err != nil {
				return wrapError("删除旧章节对话失败", err)
			}
			if err := repositories.ChapterContents.DeleteByChapterIDs(ctx, oldChapterIDs); err != nil {
				return wrapError("删除旧章节正文失败", err)
			}
			if err := repositories.Chapters.DeleteByIDs(ctx, oldChapterIDs); err != nil {
				return wrapError("删除旧章节失败", err)
			}
		}
		nextSaved, err := repositories.Chapters.CreateManyByVolumeID(ctx, volumeID, chapters)
		if err != nil {
			return wrapError("保存章节规划失败", err)
		}
		saved = nextSaved
		return nil
	})
	if err != nil {
		return nil, wrapError("写入章节规划事务失败", err)
	}
	deleteVolumeChapterListCache(ctx, s.redisClient, userID, volumeID)
	deleteVolumeReferencedSettingsCaches(ctx, s.redisClient, userID, saved)
	s.deleteNovelVolumeListCache(ctx, userID, volume.NovelID)
	deleteNovelOverviewCache(ctx, s.redisClient, userID, volume.NovelID)
	result := make([]SavedChapter, 0, len(saved))
	for _, chapter := range saved {
		result = append(result, SavedChapter{
			ID:        chapter.ID,
			Title:     chapterPlanTitle(chapter),
			Summary:   jsonMapText(chapter.PlanData, "summary"),
			SortOrder: chapter.SortOrder,
		})
	}
	return result, nil
}

// formatChapterPlanData 将章节结构化规划整理为可持久化的 JSON 数据。
func formatChapterPlanData(plan ChapterPlan) model.JSONMap {
	return model.JSONMap{
		"title":              strings.TrimSpace(plan.Title),
		"summary":            strings.TrimSpace(plan.Summary),
		"references":         normalizeReferences(plan.references),
		"intertextual_links": strings.TrimSpace(plan.IntertextualLinks),
		"writing_focus":      strings.TrimSpace(plan.WritingFocus),
	}
}

// jsonMapText 从 JSONMap 中读取字符串字段。
func jsonMapText(values model.JSONMap, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

// firstNonEmptyJSONMapText 按优先级读取第一个非空 JSONMap 字段。
func firstNonEmptyJSONMapText(values model.JSONMap, keys ...string) string {
	for _, key := range keys {
		if value := jsonMapText(values, key); value != "" {
			return value
		}
	}
	return ""
}

// ensureVolumeOwnerWithRepositories 校验卷存在且所属小说属于当前用户，避免各 service 重复拼接卷和小说归属判断。
func ensureVolumeOwnerWithRepositories(ctx context.Context, repositories repo.Repositories, userID int64, volumeID int64) (model.Volume, error) {
	volume, err := repositories.Volumes.FindByID(ctx, volumeID)
	if errors.Is(err, repo.ErrVolumeNotFound) {
		return model.Volume{}, ErrResourceNotFound
	}
	if err != nil {
		return model.Volume{}, err
	}
	if _, err := ensureNovelOwnerWithRepositories(ctx, repositories, userID, volume.NovelID); err != nil {
		return model.Volume{}, err
	}
	return volume, nil
}
