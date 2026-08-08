package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/redis/go-redis/v9"
)

const novelChatTitle = "小说对话"
const volumeChatTitle = "卷对话"
const chapterChatTitle = "章节对话"
const novelOverviewCacheTTL = 7 * 24 * time.Hour
const novelStatusSetup int16 = 1
const novelStatusNormal int16 = 2
const novelStatusArchived int16 = 3

// NovelSetupInput 保存用户新建小说表单中的关键创作信息。
type NovelSetupInput struct {
	OriginalText string              `json:"originalText"`
	Title        string              `json:"title"`
	Direction    string              `json:"direction"`
	TagGroups    map[string][]string `json:"tagGroups"`
	Characters   []struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	} `json:"characters"`
	Relationships []struct {
		CharacterA  string `json:"characterA"`
		CharacterB  string `json:"characterB"`
		Description string `json:"description"`
	} `json:"relationships"`
	Maps []struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	} `json:"maps"`
	Forces []struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	} `json:"forces"`
	OtherSettings []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Items       []struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		} `json:"items"`
	} `json:"other_settings"`
	Perspective string `json:"perspective"`
	Length      string `json:"length"`
	LengthRange string `json:"lengthRange"`
}

// NovelSetupCompleteInput 承载 AI 自动识别新建小说表单所需的原始文本和模型选择。
type NovelSetupCompleteInput struct {
	Text    string `json:"text"`
	ModelID string `json:"modelId"`
}

// NovelService 小说服务接口，负责小说本体、小说级消息和小说级 AI 流。
type NovelService interface {
	CompleteSetupStream(ctx context.Context, userID int64, input NovelSetupCompleteInput) (<-chan ai.StreamEvent, error)
	CreateNovel(ctx context.Context, userID int64, setup NovelSetupInput) (model.CreateNovelResponse, error)
	SaveSetupDraft(ctx context.Context, userID int64, setup NovelSetupInput) (model.Novel, error)
	UpdateSetupDraft(ctx context.Context, userID int64, novelID int64, setup NovelSetupInput) error
	StartSetupDraft(ctx context.Context, userID int64, novelID int64, setup NovelSetupInput) (model.CreateNovelResponse, error)
	ListNovels(ctx context.Context, userID int64, archived bool) ([]model.Novel, error)
	GetNovelOverview(ctx context.Context, userID int64, novelID int64) (model.NovelOverviewItem, error)
	GetDashboard(ctx context.Context, userID int64) (model.WorkspaceDashboard, error)
	ArchiveNovel(ctx context.Context, userID int64, novelID int64) error
	RestoreNovel(ctx context.Context, userID int64, novelID int64) error
	ListNovelMessages(ctx context.Context, userID int64, novelID int64) (model.ChatMessagesResponse, error)
	StreamNovel(ctx context.Context, userID int64, novelID int64, content string) (<-chan ai.StreamEvent, error)
	ResumeNovelStream(ctx context.Context, userID int64, novelID int64) (<-chan ai.StreamEvent, error)
	CancelNovelStream(ctx context.Context, userID int64, novelID int64) error
	ApplyVolumePlan(ctx context.Context, userID int64, novelID int64, plans []VolumePlan, force bool) ([]model.Volume, error)
}

type novelService struct {
	*aiStreamSupport
	setupWorkflow ai.Workflow[NovelSetupWorkflowInput, NovelSetupInput]
}

// NewNovelService 创建小说服务。
func NewNovelService(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client) NovelService {
	service := &novelService{aiStreamSupport: newAIStreamSupport(repositories, redisClient, aiClient)}
	workflow, err := service.newNovelSetupWorkflow(context.Background())
	if err != nil {
		panic(fmt.Sprintf("创建新建小说模板工作流失败: %v", err))
	}
	service.setupWorkflow = workflow
	return service
}

// CompleteSetupStream 使用新建小说模板工作流识别表单，并通过 A2UI 输出阶段和结果事件。
func (s *novelService) CompleteSetupStream(ctx context.Context, userID int64, input NovelSetupCompleteInput) (<-chan ai.StreamEvent, error) {
	rawText := strings.TrimSpace(input.Text)
	if rawText == "" {
		return nil, ErrInvalidMessage
	}
	modelConfig, err := s.resolveUserModelConfigByPublicID(ctx, userID, input.ModelID)
	if err != nil {
		return nil, wrapError("解析表单识别模型失败", err)
	}
	stream := make(chan ai.StreamEvent, 16)
	go func() {
		defer close(stream)
		setup, err := s.setupWorkflow.Run(ctx, NovelSetupWorkflowInput{
			UserID:      userID,
			RawText:     rawText,
			ModelKey:    modelConfig.AgentKey(),
			ModelConfig: modelConfig.RuntimeConfig(),
		}, setupStreamSink{stream: stream})
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				sendSetupStreamEvent(ctx, stream, ai.NewStreamError(err.Error()))
			}
			return
		}
		sendSetupStreamEvent(ctx, stream, a2uiSetupEvent(model.JSONMap{
			"kind":     renderKindSetupComplete,
			"complete": true,
			"setup":    setup,
		}))
		sendSetupStreamEvent(ctx, stream, ai.NewStreamDone(0, "", 0))
	}()
	return stream, nil
}

// sendSetupStreamEvent 在请求未取消时向前端推送 setup SSE 事件。
func sendSetupStreamEvent(ctx context.Context, stream chan<- ai.StreamEvent, event ai.StreamEvent) {
	select {
	case <-ctx.Done():
	case stream <- event:
	}
}

// CreateNovel 创建小说和小说级对话会话，并保存新建表单信息。
func (s *novelService) CreateNovel(ctx context.Context, userID int64, setup NovelSetupInput) (model.CreateNovelResponse, error) {
	var response model.CreateNovelResponse
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		setupData := normalizeNovelSetupData(setup)
		title := strings.TrimSpace(setup.Title)
		if title == "" {
			return ErrInvalidMessage
		}
		planData := initialNovelPlanData(title, setupData)
		novel, err := repositories.Novels.Create(ctx, userID, title, planData, strings.TrimSpace(setup.OriginalText), novelStatusNormal)
		if err != nil {
			return err
		}
		s.deleteNovelOverviewCache(ctx, userID, novel.ID)

		if _, err := repositories.ChatSessions.Create(ctx, userID, int16(model.ScopeTypeNovel), novel.ID, novelChatTitle); err != nil {
			return err
		}

		response = model.CreateNovelResponse{
			Novel: novel,
		}
		return nil
	})
	return response, wrapError("创建小说失败", err)
}

// SaveSetupDraft 将新建小说表单暂存为设定阶段小说，不创建对话会话。
func (s *novelService) SaveSetupDraft(ctx context.Context, userID int64, setup NovelSetupInput) (model.Novel, error) {
	setupData := normalizeNovelSetupData(setup)
	title := strings.TrimSpace(setup.Title)
	if title == "" {
		return model.Novel{}, ErrInvalidMessage
	}
	planData := initialNovelPlanData(title, setupData)
	novel, err := s.repositories.Novels.Create(ctx, userID, title, planData, strings.TrimSpace(setup.OriginalText), novelStatusSetup)
	if err == nil {
		s.deleteNovelOverviewCache(ctx, userID, novel.ID)
	}
	return novel, wrapError("暂存小说设定失败", err)
}

// UpdateSetupDraft 覆盖当前设定阶段小说的表单数据。
func (s *novelService) UpdateSetupDraft(ctx context.Context, userID int64, novelID int64, setup NovelSetupInput) error {
	novel, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return err
	}
	if novel.Status != novelStatusSetup {
		return ErrForbidden
	}
	setupData := normalizeNovelSetupData(setup)
	title := strings.TrimSpace(setup.Title)
	if title == "" {
		return ErrInvalidMessage
	}
	planData := initialNovelPlanData(title, setupData)
	err = s.repositories.Novels.Update(ctx, novelID, repo.UpdateFields{
		"title":               title,
		"plan_data":           planData,
		"setup_original_text": strings.TrimSpace(setup.OriginalText),
		"status":              novelStatusSetup,
	})
	if err == nil {
		s.deleteNovelOverviewCache(ctx, userID, novelID)
	}
	return wrapError("更新小说设定暂存失败", err)
}

// StartSetupDraft 将设定阶段小说更新为正式小说，并创建小说级对话会话。
func (s *novelService) StartSetupDraft(ctx context.Context, userID int64, novelID int64, setup NovelSetupInput) (model.CreateNovelResponse, error) {
	var response model.CreateNovelResponse
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		novel, err := ensureNovelOwnerWithRepositories(ctx, repositories, userID, novelID)
		if err != nil {
			return err
		}
		if novel.Status != novelStatusSetup {
			return ErrForbidden
		}
		setupData := normalizeNovelSetupData(setup)
		title := strings.TrimSpace(setup.Title)
		if title == "" {
			return ErrInvalidMessage
		}
		planData := initialNovelPlanData(title, setupData)
		novel.Title = title
		novel.PlanData = planData
		novel.SetupOriginalText = strings.TrimSpace(setup.OriginalText)
		novel.Status = novelStatusNormal
		novel.UpdatedAt = time.Now()
		if err = repositories.Novels.Update(ctx, novelID, repo.UpdateFields{
			"title":               novel.Title,
			"plan_data":           novel.PlanData,
			"setup_original_text": novel.SetupOriginalText,
			"status":              novel.Status,
		}); err != nil {
			return err
		}
		s.deleteNovelOverviewCache(ctx, userID, novelID)
		if _, err := repositories.ChatSessions.Create(ctx, userID, int16(model.ScopeTypeNovel), novel.ID, novelChatTitle); err != nil {
			return err
		}
		response = model.CreateNovelResponse{Novel: novel}
		return nil
	})
	return response, wrapError("开始创作暂存小说失败", err)
}

// initialNovelPlanData 将新建小说表单整理为初始小说梗概，保证规划完成前也能展示用户填写的方向。
func initialNovelPlanData(title string, setupData model.JSONMap) model.JSONMap {
	planData := model.JSONMap{
		"title":   strings.TrimSpace(title),
		"summary": "",
	}
	if setupData == nil {
		return planData
	}
	if direction, ok := setupData["direction"].(string); ok {
		planData["summary"] = strings.TrimSpace(direction)
	}
	for key, value := range setupData {
		if key != "direction" {
			planData[key] = value
		}
	}
	return planData
}

// ListNovels 查询当前用户的小说列表，可按正常或归档状态过滤。
func (s *novelService) ListNovels(ctx context.Context, userID int64, archived bool) ([]model.Novel, error) {
	var (
		novels []model.Novel
		err    error
	)
	if archived {
		novels, err = s.repositories.Novels.ListArchivedByUserID(ctx, userID)
	} else {
		novels, err = s.repositories.Novels.ListByUserID(ctx, userID)
	}
	if err != nil {
		return nil, wrapError("查询小说列表失败", err)
	}
	for i := range novels {
		if novels[i].Status == novelStatusArchived { // 归档的小说不查询字数
			continue
		}
		novels[i].WordCount = s.novelWordCount(ctx, novels[i].ID)
	}
	return novels, nil
}

// GetNovelOverview 查询单本小说梗概，并使用 Redis 缓存稳定的规划数据。
func (s *novelService) GetNovelOverview(ctx context.Context, userID int64, novelID int64) (model.NovelOverviewItem, error) {
	if cached, ok := s.cachedNovelOverview(ctx, userID, novelID); ok {
		return cached, nil
	}
	item, err := s.repositories.Novels.FindOverviewByUserID(ctx, userID, novelID)
	if err != nil {
		return model.NovelOverviewItem{}, wrapError("查询小说梗概失败", err)
	}
	s.cacheNovelOverview(ctx, userID, item)
	return item, nil
}

// cachedNovelOverview 从 Redis 读取单本小说梗概缓存，缓存缺失时返回 false。
func (s *novelService) cachedNovelOverview(ctx context.Context, userID int64, novelID int64) (model.NovelOverviewItem, bool) {
	if s.redisClient == nil || userID <= 0 || novelID <= 0 {
		return model.NovelOverviewItem{}, false
	}
	payload, err := s.redisClient.Get(ctx, novelOverviewCacheKey(userID, novelID)).Bytes()
	if err != nil {
		return model.NovelOverviewItem{}, false
	}
	var cached model.NovelOverviewItem
	if err := json.Unmarshal(payload, &cached); err != nil {
		return model.NovelOverviewItem{}, false
	}
	return cached, true
}

// cacheNovelOverview 将单本小说梗概写入 Redis，缓存失败不影响主流程。
func (s *novelService) cacheNovelOverview(ctx context.Context, userID int64, item model.NovelOverviewItem) {
	if s.redisClient == nil || userID <= 0 || item.ID <= 0 {
		return
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return
	}
	_ = s.redisClient.Set(ctx, novelOverviewCacheKey(userID, item.ID), payload, novelOverviewCacheTTL).Err()
}

// deleteNovelOverviewCache 删除单本小说梗概缓存，用于标题、规划或生命周期变化后失效。
func (s *novelService) deleteNovelOverviewCache(ctx context.Context, userID int64, novelID int64) {
	deleteNovelOverviewCache(ctx, s.redisClient, userID, novelID)
}

// deleteNovelOverviewCache 删除单本小说梗概缓存，供会影响小说梗概展示的业务调用。
func deleteNovelOverviewCache(ctx context.Context, redisClient *redis.Client, userID int64, novelID int64) {
	if redisClient == nil || userID <= 0 || novelID <= 0 {
		return
	}
	_ = redisClient.Del(ctx, novelOverviewCacheKey(userID, novelID)).Err()
}

// novelOverviewCacheKey 生成单本小说梗概缓存键。
func novelOverviewCacheKey(userID int64, novelID int64) string {
	return fmt.Sprintf("user:%d:novel:%d:overview", userID, novelID)
}

// GetDashboard 聚合工作台首页数据。
func (s *novelService) GetDashboard(ctx context.Context, userID int64) (model.WorkspaceDashboard, error) {
	dashboard, err := s.repositories.Novels.GetDashboard(ctx, userID, novelStatusNormal)
	if err != nil {
		return model.WorkspaceDashboard{}, wrapError("查询工作台统计失败", err)
	}
	user, err := s.repositories.Users.FindByID(ctx, userID)
	if err != nil {
		return model.WorkspaceDashboard{}, wrapError("查询用户创作时长失败", err)
	}
	dashboard.WritingHours = math.Round(time.Since(user.CreatedAt).Hours()*10) / 10
	return dashboard, nil
}

// ArchiveNovel 将当前用户的小说标记为归档状态。
func (s *novelService) ArchiveNovel(ctx context.Context, userID int64, novelID int64) error {
	novel, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return err
	}
	if novel.Status == novelStatusSetup {
		return ErrForbidden
	}
	err = s.repositories.Novels.Update(ctx, novelID, repo.UpdateFields{
		"status": novelStatusArchived,
	})
	if err == nil {
		s.deleteNovelOverviewCache(ctx, userID, novelID)
	}
	return wrapError("归档小说失败", err)
}

// RestoreNovel 将当前用户的归档小说恢复为正常状态。
func (s *novelService) RestoreNovel(ctx context.Context, userID int64, novelID int64) error {
	_, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return err
	}
	err = s.repositories.Novels.Update(ctx, novelID, repo.UpdateFields{
		"status": novelStatusNormal,
	})
	if err == nil {
		s.deleteNovelOverviewCache(ctx, userID, novelID)
	}
	return wrapError("恢复小说失败", err)
}

// ListNovelMessages 查询当前用户某本小说的小说级对话消息。
func (s *novelService) ListNovelMessages(ctx context.Context, userID int64, novelID int64) (model.ChatMessagesResponse, error) {
	novel, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return model.ChatMessagesResponse{}, err
	}
	if novel.Status == novelStatusSetup {
		return model.ChatMessagesResponse{}, ErrForbidden
	}

	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeNovel), novelID)
	if errors.Is(err, repo.ErrChatSessionNotFound) {
		return model.ChatMessagesResponse{Messages: []model.ChatMessage{}, Session: openSessionMeta(0, model.ScopeTypeNovel, novelID)}, nil
	}
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询小说会话失败", err)
	}
	messages, err := s.repositories.ChatMessages.ListBySessionID(ctx, session.ID)
	if err != nil {
		return model.ChatMessagesResponse{}, wrapError("查询小说会话消息失败", err)
	}
	messages = s.appendStreamingReply(ctx, messages, session.ID, int16(model.ScopeTypeNovel), novelID)
	return model.ChatMessagesResponse{Messages: messages, Session: sessionMeta(session)}, nil
}

// StreamNovel 保存用户消息，调用小说级 AI 流，并在完成后保存 AI 完整回复。
func (s *novelService) StreamNovel(ctx context.Context, userID int64, novelID int64, content string) (<-chan ai.StreamEvent, error) {
	novel, err := s.ensureNovelOwner(ctx, userID, novelID)
	if err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	if novel.Status == novelStatusSetup {
		return nil, ErrForbidden
	}
	savedVolumes, err := s.repositories.Volumes.ListByNovelID(ctx, novelID)
	if err != nil {
		return nil, wrapError("查询当前小说已保存卷失败", err)
	}
	return s.streamScoped(ctx, streamScopeParams{
		UserID:        userID,
		ScopeID:       novelID,
		ScopeType:     model.ScopeTypeNovel,
		SessionTitle:  novelChatTitle,
		Content:       content,
		SystemContext: novelPlanningSystemContext(novel.PlanData, len(savedVolumes)),
	})
}

// ResumeNovelStream 重建小说级临时回复 SSE，只转发 Redis 快照后续变化。
func (s *novelService) ResumeNovelStream(ctx context.Context, userID int64, novelID int64) (<-chan ai.StreamEvent, error) {
	if _, err := s.ensureNovelOwner(ctx, userID, novelID); err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeNovel), novelID)
	if err != nil {
		return nil, wrapError("查询小说会话失败", err)
	}
	return s.resumeStreamingReply(ctx, session.ID, int16(model.ScopeTypeNovel), novelID), nil
}

// CancelNovelStream 取消小说级正在生成的 AI 回复。
func (s *novelService) CancelNovelStream(ctx context.Context, userID int64, novelID int64) error {
	if _, err := s.ensureNovelOwner(ctx, userID, novelID); err != nil {
		return wrapError("校验小说归属失败", err)
	}
	session, err := s.repositories.ChatSessions.FindByUserScope(ctx, userID, int16(model.ScopeTypeNovel), novelID)
	if err != nil {
		return wrapError("查询小说会话失败", err)
	}
	return s.cancelStreamingReply(ctx, session.ID)
}

// ApplyVolumePlan 由用户点击“应用规划”后保存当前展示的完整分卷规划。
func (s *novelService) ApplyVolumePlan(ctx context.Context, userID int64, novelID int64, plans []VolumePlan, force bool) ([]model.Volume, error) {
	plans = normalizeVolumePlans(plans)
	if len(plans) == 0 {
		return nil, ErrInvalidMessage
	}
	if _, err := s.ensureNovelOwner(ctx, userID, novelID); err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	existing, err := s.repositories.Volumes.ListByNovelID(ctx, novelID)
	if err != nil {
		return nil, wrapError("查询当前小说已保存卷失败", err)
	}
	if len(existing) > 0 && !force {
		return nil, ErrPlanOverwriteRequired
	}
	if _, err := s.saveNovelVolumes(ctx, userID, novelID, plans, existing); err != nil {
		return nil, err
	}
	volumes, err := s.repositories.Volumes.ListByNovelID(ctx, novelID)
	if err != nil {
		return nil, wrapError("读取已应用卷规划失败", err)
	}
	for i := range volumes {
		volumes[i].WordCount = s.volumeWordCount(ctx, volumes[i].ID)
	}
	return volumes, nil
}

// normalizeNovelSetupData 清理新建小说表单数据，便于长期存储和后续喂给 AI。
func normalizeNovelSetupData(setup NovelSetupInput) model.JSONMap {
	tagGroups := normalizeSetupTagGroups(setup)
	characters := make([]model.JSONMap, 0, len(setup.Characters))
	for _, character := range setup.Characters {
		name := strings.TrimSpace(character.Name)
		notes := strings.TrimSpace(character.Notes)
		if name == "" && notes == "" {
			continue
		}
		item := model.JSONMap{
			"name":  name,
			"notes": notes,
		}
		if appearanceTime := strings.TrimSpace(character.AppearanceTime); appearanceTime != "" {
			item["appearance_time"] = appearanceTime
		}
		characters = append(characters, item)
	}
	relationships := make([]model.JSONMap, 0, len(setup.Relationships))
	for _, relationship := range setup.Relationships {
		characterA := strings.TrimSpace(relationship.CharacterA)
		characterB := strings.TrimSpace(relationship.CharacterB)
		if characterA == "" || characterB == "" {
			continue
		}
		relationships = append(relationships, model.JSONMap{
			"character_a": characterA,
			"character_b": characterB,
			"description": strings.TrimSpace(relationship.Description),
		})
	}
	maps := make([]model.JSONMap, 0, len(setup.Maps))
	for _, item := range setup.Maps {
		name := strings.TrimSpace(item.Name)
		appearanceTime := strings.TrimSpace(item.AppearanceTime)
		notes := strings.TrimSpace(item.Notes)
		if name == "" && notes == "" {
			continue
		}
		maps = append(maps, model.JSONMap{
			"name":            name,
			"appearance_time": appearanceTime,
			"notes":           notes,
		})
	}
	forces := make([]model.JSONMap, 0, len(setup.Forces))
	for _, item := range setup.Forces {
		name := strings.TrimSpace(item.Name)
		appearanceTime := strings.TrimSpace(item.AppearanceTime)
		notes := strings.TrimSpace(item.Notes)
		if name == "" && notes == "" {
			continue
		}
		forces = append(forces, model.JSONMap{
			"name":            name,
			"appearance_time": appearanceTime,
			"notes":           notes,
		})
	}
	otherSettings := make([]model.JSONMap, 0, len(setup.OtherSettings))
	for _, setting := range setup.OtherSettings {
		title := strings.TrimSpace(setting.Title)
		description := strings.TrimSpace(setting.Description)
		items := make([]model.JSONMap, 0, len(setting.Items))
		for _, item := range setting.Items {
			name := strings.TrimSpace(item.Name)
			notes := strings.TrimSpace(item.Notes)
			if name == "" && notes == "" {
				continue
			}
			itemData := model.JSONMap{
				"name":  name,
				"notes": notes,
			}
			if appearanceTime := strings.TrimSpace(item.AppearanceTime); appearanceTime != "" {
				itemData["appearance_time"] = appearanceTime
			}
			items = append(items, itemData)
		}
		if title == "" || len(items) == 0 {
			continue
		}
		otherSettings = append(otherSettings, model.JSONMap{
			"title":       title,
			"description": description,
			"items":       items,
		})
	}
	return model.JSONMap{
		"direction":      strings.TrimSpace(setup.Direction),
		"tag_groups":     tagGroups,
		"characters":     characters,
		"relationships":  relationships,
		"maps":           maps,
		"forces":         forces,
		"other_settings": otherSettings,
		"perspective":    strings.TrimSpace(setup.Perspective),
		"length":         strings.TrimSpace(setup.Length),
		"length_range":   strings.TrimSpace(setup.LengthRange),
	}
}

// normalizeSetupTagGroups 按题材、类型、基调、文风、雷点五类清理标签，避免把雷点混进普通标签。
func normalizeSetupTagGroups(setup NovelSetupInput) model.JSONMap {
	orderedGroups := []string{"题材", "类型", "基调", "文风", "雷点"}
	result := model.JSONMap{}
	for _, group := range orderedGroups {
		values := dedupeSetupTags(setup.TagGroups[group])
		if len(values) > 0 {
			result[group] = values
		}
	}
	return result
}

// normalizeCompletedNovelSetup 规范 AI 识别出的表单，避免前端收到空白、重复或非法枚举。
func normalizeCompletedNovelSetup(setup NovelSetupInput) NovelSetupInput {
	setup.Title = strings.TrimSpace(setup.Title)
	setup.Direction = strings.TrimSpace(setup.Direction)
	setup.Perspective = normalizeSetupChoice(setup.Perspective, []string{"第一人称", "第二人称", "第三人称"}, "第三人称")
	setup.Length = normalizeSetupChoice(setup.Length, []string{"短篇", "中篇", "长篇"}, "中篇")
	setup.LengthRange = setupLengthRange(setup.Length)
	nextGroups := map[string][]string{}
	for _, group := range []string{"题材", "类型", "基调", "文风", "雷点"} {
		values := dedupeSetupTags(setup.TagGroups[group])
		if len(values) > 0 {
			nextGroups[group] = values
		}
	}
	characters := make([]struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	}, 0, len(setup.Characters))
	for _, character := range setup.Characters {
		name := strings.TrimSpace(character.Name)
		notes := strings.TrimSpace(character.Notes)
		if name == "" {
			continue
		}
		characters = append(characters, struct {
			Name           string `json:"name"`
			AppearanceTime string `json:"appearanceTime"`
			Notes          string `json:"notes"`
		}{
			Name:           name,
			AppearanceTime: strings.TrimSpace(character.AppearanceTime),
			Notes:          notes,
		})
	}
	setup.Characters = characters
	relationships := make([]struct {
		CharacterA  string `json:"characterA"`
		CharacterB  string `json:"characterB"`
		Description string `json:"description"`
	}, 0, len(setup.Relationships))
	for _, relationship := range setup.Relationships {
		characterA := strings.TrimSpace(relationship.CharacterA)
		characterB := strings.TrimSpace(relationship.CharacterB)
		if characterA == "" || characterB == "" {
			continue
		}
		relationships = append(relationships, struct {
			CharacterA  string `json:"characterA"`
			CharacterB  string `json:"characterB"`
			Description string `json:"description"`
		}{
			CharacterA:  characterA,
			CharacterB:  characterB,
			Description: strings.TrimSpace(relationship.Description),
		})
	}
	setup.Relationships = relationships
	maps := make([]struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	}, 0, len(setup.Maps))
	for _, item := range setup.Maps {
		name := strings.TrimSpace(item.Name)
		appearanceTime := strings.TrimSpace(item.AppearanceTime)
		notes := strings.TrimSpace(item.Notes)
		if name == "" {
			continue
		}
		maps = append(maps, struct {
			Name           string `json:"name"`
			AppearanceTime string `json:"appearanceTime"`
			Notes          string `json:"notes"`
		}{Name: name, AppearanceTime: appearanceTime, Notes: notes})
	}
	setup.Maps = maps
	forces := make([]struct {
		Name           string `json:"name"`
		AppearanceTime string `json:"appearanceTime"`
		Notes          string `json:"notes"`
	}, 0, len(setup.Forces))
	for _, item := range setup.Forces {
		name := strings.TrimSpace(item.Name)
		appearanceTime := strings.TrimSpace(item.AppearanceTime)
		notes := strings.TrimSpace(item.Notes)
		if name == "" {
			continue
		}
		forces = append(forces, struct {
			Name           string `json:"name"`
			AppearanceTime string `json:"appearanceTime"`
			Notes          string `json:"notes"`
		}{Name: name, AppearanceTime: appearanceTime, Notes: notes})
	}
	setup.Forces = forces
	otherSettings := make([]struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Items       []struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		} `json:"items"`
	}, 0, len(setup.OtherSettings))
	for _, setting := range setup.OtherSettings {
		title := strings.TrimSpace(setting.Title)
		description := strings.TrimSpace(setting.Description)
		items := make([]struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		}, 0, len(setting.Items))
		for _, item := range setting.Items {
			name := strings.TrimSpace(item.Name)
			notes := strings.TrimSpace(item.Notes)
			if name == "" {
				continue
			}
			items = append(items, struct {
				Name           string `json:"name"`
				Notes          string `json:"notes"`
				AppearanceTime string `json:"appearanceTime"`
			}{
				Name:           name,
				Notes:          notes,
				AppearanceTime: strings.TrimSpace(item.AppearanceTime),
			})
		}
		if title == "" || len(items) == 0 {
			continue
		}
		otherSettings = append(otherSettings, struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Items       []struct {
				Name           string `json:"name"`
				Notes          string `json:"notes"`
				AppearanceTime string `json:"appearanceTime"`
			} `json:"items"`
		}{Title: title, Description: description, Items: items})
	}
	setup.OtherSettings = otherSettings
	return setup
}

// normalizeSetupChoice 将 AI 返回的枚举值限制在前端支持的选项内。
func normalizeSetupChoice(value string, allowed []string, fallback string) string {
	value = strings.TrimSpace(value)
	for _, item := range allowed {
		if value == item {
			return value
		}
	}
	return fallback
}

// setupLengthRange 根据篇幅标签返回前端表单展示的章节范围。
func setupLengthRange(length string) string {
	switch length {
	case "短篇":
		return "约 20-50 章"
	case "长篇":
		return "约 600-900 章"
	default:
		return "约 200-400 章"
	}
}

// extractJSONObject 从模型输出中提取第一段 JSON 对象，兼容模型误加代码块说明的情况。
func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return content
	}
	return content[start : end+1]
}

// dedupeSetupTags 去除标签中的空白和重复项，保留用户选择顺序。
func dedupeSetupTags(input []string) []string {
	values := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		values = append(values, tag)
	}
	return values
}

// novelPlanningSystemContext 组装小说级规划上下文，包含新建表单和是否已有已保存卷。
func novelPlanningSystemContext(setup model.JSONMap, savedVolumeCount int) string {
	payload := model.JSONMap{
		"novel_plan_data":                    setup,
		"current_novel_saved_volume_count":   savedVolumeCount,
		"current_novel_has_saved_volumes":    savedVolumeCount > 0,
		"current_novel_has_no_saved_volumes": savedVolumeCount == 0,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "小说级分卷规划必须参考以下上下文。不要向用户复述这些内部上下文；novel_plan_data 是用户新建小说时填写的创作表单，后续全书分卷规划必须持续参考；characters、relationships、maps、forces、other_settings 是用户设定资产，不能随意改名或篡改；分卷规划允许加入仅服务本卷的临时人物、炮灰人物、小地点或临时任务/装备/事件，但必须写入该卷 temporary_settings，字段结构与新建小说表单一致；凡是会影响世界观、主线逻辑、力量体系、职业体系、规则、资源、禁忌、制度、势力组织或历史真相的重要设定，不允许临时编造；如果剧情需要的设定没有出现在表单中，必须提示用户补充或重规划；characters、maps、forces、other_settings.items 中的 appearanceTime 是设定在全书中的按需加载时机，用于控制前期/中期/后期什么时候首次出场、什么时候允许详细展开、什么时候只能埋伏笔；relationships 使用 character_a/character_b 引用人物编号，表示人物关系和阶段变化；forces 与人物、地点同级，但只表示团体，例如国家、阵营、势力、组织、家族、宗门、学院、军团、商会、教会、秘密组织、种族集团、地下势力等；能力体系、等级体系、修炼体系、魔法体系、职业体系属于 other_settings，不能当作 forces；tag_groups 中的文风是正文语言质感约束，雷点是规避约束，不是普通题材标签；分卷规划时尤其要根据 length 与 length_range 推导卷数和每卷章节容量，并只给本卷实际涉及的设定写 setting_boundaries；如果 current_novel_has_saved_volumes 为 true，用户点击“应用规划”会覆盖旧卷、旧章节、正文草稿和相关对话；AI 只能提示用户点击卡片右上角按钮，不能自行保存：\n" + string(raw)
}

// saveNovelVolumes 校验小说归属后保存用户手动应用的整本分卷规划。
func (s *novelService) saveNovelVolumes(ctx context.Context, userID int64, novelID int64, plans []VolumePlan, oldVolumes []model.Volume) ([]SavedVolume, error) {
	if _, err := s.ensureNovelOwner(ctx, userID, novelID); err != nil {
		return nil, wrapError("校验小说归属失败", err)
	}
	volumes := make([]model.Volume, 0, len(plans))
	for i, plan := range plans {
		sortOrder := plan.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		summary := strings.TrimSpace(plan.Summary)
		planData := model.JSONMap{
			"title":                 strings.TrimSpace(plan.Title),
			"summary":               summary,
			"timeline":              strings.TrimSpace(plan.Timeline),
			"locations":             trimStringList(plan.Locations),
			"characters":            trimStringList(plan.Characters),
			"current_state":         strings.TrimSpace(plan.CurrentState),
			"end_state":             strings.TrimSpace(plan.EndState),
			"character_development": strings.TrimSpace(plan.CharacterDevelopment),
			"setting_development":   strings.TrimSpace(plan.SettingDevelopment),
			"setting_boundaries":    normalizeSettingBoundaries(plan.SettingBoundaries),
			"temporary_settings":    plan.TemporarySettings,
			"chapter_count":         plan.ChapterCount,
			"key_events":            plan.KeyEvents,
			"foreshadowing":         strings.TrimSpace(plan.Foreshadowing),
			"other_highlights":      strings.TrimSpace(plan.OtherHighlights),
		}
		volumes = append(volumes, model.Volume{
			NovelID:   novelID,
			PlanData:  planData,
			SortOrder: sortOrder,
			Status:    1,
		})
	}

	var saved []model.Volume
	err := s.repositories.Transactions.WithinTx(ctx, func(repositories repo.Repositories) error {
		if len(oldVolumes) > 0 {
			oldChapters, err := repositories.Chapters.ListByNovelID(ctx, novelID)
			if err != nil {
				return wrapError("查询待替换旧章节失败", err)
			}
			oldVolumeIDs := volumeIDs(oldVolumes)
			oldChapterIDs := chapterIDs(oldChapters)
			if err := deleteScopedChatSessions(ctx, repositories, model.ScopeTypeVolume, oldVolumeIDs); err != nil {
				return wrapError("删除旧卷对话失败", err)
			}
			if err := deleteScopedChatSessions(ctx, repositories, model.ScopeTypeChapter, oldChapterIDs); err != nil {
				return wrapError("删除旧章节对话失败", err)
			}
			if err := repositories.ChapterContents.DeleteByChapterIDs(ctx, oldChapterIDs); err != nil {
				return wrapError("删除旧章节正文失败", err)
			}
			if err := repositories.Chapters.DeleteByIDs(ctx, oldChapterIDs); err != nil {
				return wrapError("删除旧章节失败", err)
			}
			if err := repositories.Volumes.DeleteByIDs(ctx, oldVolumeIDs); err != nil {
				return wrapError("删除旧卷失败", err)
			}
		}
		nextSaved, err := repositories.Volumes.CreateManyByNovelID(ctx, novelID, volumes)
		if err != nil {
			return wrapError("保存分卷规划失败", err)
		}
		saved = nextSaved
		return nil
	})
	if err != nil {
		return nil, wrapError("写入分卷规划事务失败", err)
	}
	deleteNovelVolumeListCache(ctx, s.redisClient, userID, novelID)
	deleteNovelOverviewCache(ctx, s.redisClient, userID, novelID)
	result := make([]SavedVolume, 0, len(saved))
	for _, volume := range saved {
		result = append(result, SavedVolume{
			ID:        volume.ID,
			PlanData:  volume.PlanData,
			SortOrder: volume.SortOrder,
		})
	}
	return result, nil
}

func volumeIDs(volumes []model.Volume) []int64 {
	ids := make([]int64, 0, len(volumes))
	for _, volume := range volumes {
		ids = append(ids, volume.ID)
	}
	return ids
}

func chapterIDs(chapters []model.Chapter) []int64 {
	ids := make([]int64, 0, len(chapters))
	for _, chapter := range chapters {
		ids = append(ids, chapter.ID)
	}
	return ids
}

func chapterPlanTitle(chapter model.Chapter) string {
	if title, ok := chapter.PlanData["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if chapter.SortOrder > 0 {
		return fmt.Sprintf("第%d章", chapter.SortOrder)
	}
	return ""
}

func deleteScopedChatSessions(ctx context.Context, repositories repo.Repositories, scopeType model.ScopeType, scopeIDs []int64) error {
	sessionIDs, err := repositories.ChatSessions.ListIDsByScopes(ctx, int16(scopeType), scopeIDs)
	if err != nil {
		return err
	}
	if err := repositories.ChatMessages.DeleteBySessionIDs(ctx, sessionIDs); err != nil {
		return err
	}
	return repositories.ChatSessions.DeleteByIDs(ctx, sessionIDs)
}

// ensureNovelOwnerWithRepositories 校验小说存在且属于当前用户，支持普通仓储和事务仓储复用同一套归属逻辑。
func ensureNovelOwnerWithRepositories(ctx context.Context, repositories repo.Repositories, userID int64, novelID int64) (model.Novel, error) {
	novel, err := repositories.Novels.FindByID(ctx, novelID)
	if errors.Is(err, repo.ErrNovelNotFound) {
		return model.Novel{}, ErrResourceNotFound
	}
	if err != nil {
		return model.Novel{}, err
	}
	if novel.UserID != userID {
		return model.Novel{}, ErrForbidden
	}
	return novel, nil
}
