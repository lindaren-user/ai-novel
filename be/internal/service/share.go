package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/redis/go-redis/v9"
)

type CreateShareRequest struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	Password string `json:"password"`
}

type CreateShareResponse struct {
	URL              string `json:"url"`
	Type             string `json:"type"`
	RequiresPassword bool   `json:"requiresPassword"`
}

type shareTokenPayload struct {
	ScopeType model.ScopeType
	ScopeID   int64
	UserID    int64
}

// ShareService 分享服务接口，负责创建分享链接和公开阅读数据访问。
type ShareService interface {
	CreateLink(ctx context.Context, userID int64, request CreateShareRequest) (CreateShareResponse, error)
	GetContent(ctx context.Context, shareType string, token string, password string) (model.SharedContent, error)
}

type shareService struct {
	repositories repo.Repositories
	redisClient  *redis.Client
	authSecret   string
}

// NewShareService 创建分享服务。
func NewShareService(repositories repo.Repositories, redisClient *redis.Client, authSecret string) ShareService {
	return &shareService{
		repositories: repositories,
		redisClient:  redisClient,
		authSecret:   authSecret,
	}
}

// CreateLink 校验资源归属后创建无状态分享链接。
func (s *shareService) CreateLink(ctx context.Context, userID int64, request CreateShareRequest) (CreateShareResponse, error) {
	scopeType, err := shareScopeType(request.Type)
	if err != nil {
		return CreateShareResponse{}, wrapError("解析分享类型失败", err)
	}
	scopeID := request.ID
	if scopeID <= 0 {
		return CreateShareResponse{}, ErrInvalidMessage
	}
	if _, err := s.ensureShareOwner(ctx, userID, scopeType, scopeID); err != nil {
		return CreateShareResponse{}, wrapError("校验分享资源归属失败", err)
	}

	password := strings.TrimSpace(request.Password)
	if password == "" {
		password = strings.TrimSpace(s.defaultSharePassword(ctx, userID))
	}
	token := s.signShareToken(shareTokenPayload{ScopeType: scopeType, ScopeID: scopeID, UserID: userID})
	shareURL := fmt.Sprintf("/share/%s/%s", request.Type, token)
	return CreateShareResponse{
		URL:              shareURL,
		Type:             request.Type,
		RequiresPassword: password != "",
	}, nil
}

// GetContent 校验无状态 token 和用户分享密钥后返回公开阅读数据。
func (s *shareService) GetContent(ctx context.Context, shareType string, token string, password string) (model.SharedContent, error) {
	payload, err := s.parseShareToken(strings.TrimSpace(token))
	if err != nil {
		return model.SharedContent{}, ErrResourceNotFound
	}
	actualType := scopeTypeName(payload.ScopeType)
	if shareType != "" && actualType != shareType {
		return model.SharedContent{}, ErrResourceNotFound
	}

	expectedPassword := strings.TrimSpace(s.defaultSharePassword(ctx, payload.UserID))
	if expectedPassword != "" {
		password = strings.TrimSpace(password)
		if password == "" {
			return model.SharedContent{Type: actualType, RequiresPassword: true}, ErrSharePasswordRequired
		}
		if !hmac.Equal([]byte(sharePasswordHash(expectedPassword)), []byte(sharePasswordHash(password))) {
			return model.SharedContent{}, ErrInvalidSharePassword
		}
	}

	content := model.SharedContent{
		Type:             actualType,
		RequiresPassword: expectedPassword != "",
	}
	switch payload.ScopeType {
	case model.ScopeTypeNovel:
		content.Novel, err = sharedNovelContent(ctx, s.repositories, payload.ScopeID)
	case model.ScopeTypeVolume:
		content.Novel, content.SelectedVolumeID, err = sharedVolumeContent(ctx, s.repositories, payload.ScopeID)
	case model.ScopeTypeChapter:
		content.Novel, content.SelectedVolumeID, content.SelectedChapterID, err = sharedChapterContent(ctx, s.repositories, payload.ScopeID)
	default:
		err = ErrResourceNotFound
	}
	if errors.Is(err, repo.ErrNovelNotFound) || errors.Is(err, repo.ErrVolumeNotFound) || errors.Is(err, repo.ErrChapterNotFound) {
		return model.SharedContent{}, ErrResourceNotFound
	}
	return content, wrapError("查询分享内容失败", err)
}

// ensureShareOwner 校验分享范围资源属于当前用户并返回小说 ID。
func (s *shareService) ensureShareOwner(ctx context.Context, userID int64, scopeType model.ScopeType, scopeID int64) (int64, error) {
	switch scopeType {
	case model.ScopeTypeNovel:
		novel, err := ensureNovelOwnerWithRepositories(ctx, s.repositories, userID, scopeID)
		if err != nil {
			return 0, wrapError("校验分享小说归属失败", err)
		}
		return novel.ID, nil
	case model.ScopeTypeVolume:
		volume, err := ensureVolumeOwnerWithRepositories(ctx, s.repositories, userID, scopeID)
		if err != nil {
			return 0, wrapError("校验分享卷归属失败", err)
		}
		return volume.NovelID, nil
	case model.ScopeTypeChapter:
		chapter, err := ensureChapterOwnerWithRepositories(ctx, s.repositories, userID, scopeID)
		if err != nil {
			return 0, wrapError("校验分享章节归属失败", err)
		}
		volume, err := s.repositories.Volumes.FindByID(ctx, chapter.VolumeID)
		if err != nil {
			return 0, wrapError("查询分享章节所属卷失败", err)
		}
		return volume.NovelID, nil
	default:
		return 0, ErrInvalidMessage
	}
}

// defaultSharePassword 从用户设置中读取默认分享密钥。
func (s *shareService) defaultSharePassword(ctx context.Context, userID int64) string {
	settings, err := resolveUserSettings(ctx, s.repositories, s.redisClient, userID)
	if err != nil {
		return ""
	}
	return settings.General.ShareSecurityKey
}

// signShareToken 生成包含范围、对象和用户的签名 token。
func (s *shareService) signShareToken(payload shareTokenPayload) string {
	body := fmt.Sprintf("%d:%d:%d", payload.ScopeType, payload.ScopeID, payload.UserID)
	mac := hmac.New(sha256.New, []byte(s.authSecret))
	_, _ = mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(body + ":" + signature))
}

// parseShareToken 解析并校验签名 token。
func (s *shareService) parseShareToken(token string) (shareTokenPayload, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return shareTokenPayload{}, wrapError("解码分享令牌失败", err)
	}
	raw := string(decoded)
	parts := strings.Split(raw, ":")
	if len(parts) != 4 {
		return shareTokenPayload{}, ErrInvalidMessage
	}
	body := strings.Join(parts[:3], ":")
	mac := hmac.New(sha256.New, []byte(s.authSecret))
	_, _ = mac.Write([]byte(body))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[3])) {
		return shareTokenPayload{}, ErrInvalidMessage
	}
	scope, err := strconv.ParseInt(parts[0], 10, 16)
	if err != nil {
		return shareTokenPayload{}, wrapError("解析分享范围失败", err)
	}
	scopeID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return shareTokenPayload{}, wrapError("解析分享资源 ID 失败", err)
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return shareTokenPayload{}, wrapError("解析分享用户 ID 失败", err)
	}
	return shareTokenPayload{ScopeType: model.ScopeType(scope), ScopeID: scopeID, UserID: userID}, nil
}

// shareScopeType 将前端分享类型转换为内部范围类型。
func shareScopeType(value string) (model.ScopeType, error) {
	switch strings.TrimSpace(value) {
	case "novel":
		return model.ScopeTypeNovel, nil
	case "volume":
		return model.ScopeTypeVolume, nil
	case "chapter":
		return model.ScopeTypeChapter, nil
	default:
		return 0, ErrInvalidMessage
	}
}

// scopeTypeName 将内部范围类型转换为前端分享类型。
func scopeTypeName(scopeType model.ScopeType) string {
	switch scopeType {
	case model.ScopeTypeNovel:
		return "novel"
	case model.ScopeTypeVolume:
		return "volume"
	case model.ScopeTypeChapter:
		return "chapter"
	default:
		return ""
	}
}

// sharePasswordHash 生成分享密钥摘要，用于常量时间比较。
func sharePasswordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// sharedNovelContent 组装整本小说的公开阅读内容树。
func sharedNovelContent(ctx context.Context, repositories repo.Repositories, novelID int64) (model.SharedNovel, error) {
	novel, err := repositories.Novels.FindByID(ctx, novelID)
	if err != nil {
		return model.SharedNovel{}, err
	}
	shared := sharedNovelFromModel(novel)
	volumes, err := repositories.Volumes.ListByNovelID(ctx, novel.ID)
	if err != nil {
		return model.SharedNovel{}, err
	}
	shared.Volumes = make([]model.SharedVolume, 0, len(volumes))
	for _, volume := range volumes {
		sharedVolume, err := sharedVolumeWithChapters(ctx, repositories, volume)
		if err != nil {
			return model.SharedNovel{}, err
		}
		shared.Volumes = append(shared.Volumes, sharedVolume)
	}
	return shared, nil
}

// sharedVolumeContent 组装单卷内容，并保留所属小说上下文。
func sharedVolumeContent(ctx context.Context, repositories repo.Repositories, volumeID int64) (model.SharedNovel, int64, error) {
	volume, err := repositories.Volumes.FindByID(ctx, volumeID)
	if err != nil {
		return model.SharedNovel{}, 0, err
	}
	novel, err := repositories.Novels.FindByID(ctx, volume.NovelID)
	if err != nil {
		return model.SharedNovel{}, 0, err
	}
	sharedVolume, err := sharedVolumeWithChapters(ctx, repositories, volume)
	if err != nil {
		return model.SharedNovel{}, 0, err
	}
	shared := sharedNovelFromModel(novel)
	shared.Volumes = []model.SharedVolume{sharedVolume}
	return shared, volume.ID, nil
}

// sharedChapterContent 组装单章内容，并保留所属卷和小说上下文。
func sharedChapterContent(ctx context.Context, repositories repo.Repositories, chapterID int64) (model.SharedNovel, int64, int64, error) {
	chapter, err := repositories.Chapters.FindByID(ctx, chapterID)
	if err != nil {
		return model.SharedNovel{}, 0, 0, err
	}
	volume, err := repositories.Volumes.FindByID(ctx, chapter.VolumeID)
	if err != nil {
		return model.SharedNovel{}, 0, 0, err
	}
	novel, err := repositories.Novels.FindByID(ctx, volume.NovelID)
	if err != nil {
		return model.SharedNovel{}, 0, 0, err
	}
	sharedVolume := sharedVolumeFromModel(volume)
	sharedVolume.Chapters = []model.SharedChapter{sharedChapterFromModel(chapter)}
	shared := sharedNovelFromModel(novel)
	shared.Volumes = []model.SharedVolume{sharedVolume}
	return shared, volume.ID, chapter.ID, nil
}

// sharedVolumeWithChapters 组装单卷及其全部章节正文。
func sharedVolumeWithChapters(ctx context.Context, repositories repo.Repositories, volume model.Volume) (model.SharedVolume, error) {
	chapters, err := repositories.Chapters.ListByVolumeID(ctx, volume.ID)
	if err != nil {
		return model.SharedVolume{}, err
	}
	shared := sharedVolumeFromModel(volume)
	shared.Chapters = make([]model.SharedChapter, 0, len(chapters))
	for _, chapter := range chapters {
		shared.Chapters = append(shared.Chapters, sharedChapterFromModel(chapter))
	}
	return shared, nil
}

// sharedNovelFromModel 将小说持久化模型转换为公开阅读模型。
func sharedNovelFromModel(novel model.Novel) model.SharedNovel {
	title := novel.Title
	if title == "" {
		title = titleFromPlanData(novel.PlanData, "未命名小说")
	}
	return model.SharedNovel{
		ID:       novel.ID,
		Title:    title,
		PlanData: novel.PlanData,
	}
}

// sharedVolumeFromModel 将卷持久化模型转换为公开阅读模型。
func sharedVolumeFromModel(volume model.Volume) model.SharedVolume {
	return model.SharedVolume{
		ID:        volume.ID,
		NovelID:   volume.NovelID,
		PlanData:  volume.PlanData,
		SortOrder: volume.SortOrder,
	}
}

// sharedChapterFromModel 将章节持久化模型转换为公开阅读模型。
func sharedChapterFromModel(chapter model.Chapter) model.SharedChapter {
	return model.SharedChapter{
		ID:        chapter.ID,
		VolumeID:  chapter.VolumeID,
		PlanData:  chapter.PlanData,
		Content:   chapter.Content,
		SortOrder: chapter.SortOrder,
		WordCount: chapter.WordCount,
		UpdatedAt: chapter.UpdatedAt,
		Completed: chapter.CompletedAt,
	}
}

// titleFromPlanData 从结构化规划 JSON 中读取标题，缺失时使用兜底标题。
func titleFromPlanData(planData model.JSONMap, fallback string) string {
	if title, ok := planData["title"].(string); ok && title != "" {
		return title
	}
	return fallback
}
