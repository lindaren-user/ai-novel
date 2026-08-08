package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/redis/go-redis/v9"
)

const userSettingsCacheTTL = 30 * time.Minute
const defaultConsistencyCheckCount = 3

type userAISettings struct {
	General struct {
		ModelID               int64  `json:"modelId"`
		Model                 string `json:"model"`
		ConsistencyCheckCount int    `json:"consistencyCheckCount"`
		ShareSecurityKey      string `json:"shareSecurityKey"`
	} `json:"general"`
}

type userWritingSettings struct {
	ConsistencyCheckCount int `json:"consistencyCheckCount"`
}

// SettingsService 用户设置服务接口
type SettingsService interface {
	Get(ctx context.Context, userID int64) (model.SettingsResponse, error)
	Update(ctx context.Context, userID int64, req model.SettingsRequest) error
}

type settingsService struct {
	repositories repo.Repositories
	redisClient  *redis.Client
}

// NewSettingsService 创建用户设置服务。
func NewSettingsService(repositories repo.Repositories, redisClient *redis.Client) SettingsService {
	return newSettingsService(repositories, redisClient)
}

func newSettingsService(repositories repo.Repositories, redisClient *redis.Client) *settingsService {
	return &settingsService{repositories: repositories, redisClient: redisClient}
}

// Get 获取用户设置，缺失时返回空设置。
func (s *settingsService) Get(ctx context.Context, userID int64) (model.SettingsResponse, error) {
	settings, err := readUserSettings(ctx, s.repositories, s.redisClient, userID)
	if err != nil {
		return model.SettingsResponse{}, err
	}
	return model.SettingsResponse{Settings: settings}, nil
}

// Update 保存用户设置，并同步刷新完整用户设置缓存。
func (s *settingsService) Update(ctx context.Context, userID int64, req model.SettingsRequest) error {
	if !json.Valid(req.Settings) || len(req.Settings) == 0 {
		return ErrInvalidSettings
	}
	if err := json.Unmarshal(req.Settings, &userAISettings{}); err != nil {
		return ErrInvalidSettings
	}
	if err := s.repositories.Settings.Upsert(ctx, userID, req.Settings); err != nil {
		return wrapError("保存用户设置失败", err)
	}
	cacheUserSettings(ctx, s.redisClient, userID, req.Settings)
	return nil
}

// readUserSettings 统一读取用户设置：先读 Redis，未命中再读库，缺失时初始化空设置并写回缓存。
func readUserSettings(ctx context.Context, repositories repo.Repositories, redisClient *redis.Client, userID int64) (json.RawMessage, error) {
	if redisClient == nil || userID <= 0 {
		return loadUserSettings(ctx, repositories, userID)
	}
	payload, err := redisClient.Get(ctx, userSettingsCacheKey(userID)).Bytes()
	if err == nil && len(payload) > 0 {
		if json.Valid(payload) {
			return json.RawMessage(payload), nil
		}
	}
	settings, err := loadUserSettings(ctx, repositories, userID)
	if err != nil {
		return nil, err
	}
	cacheUserSettings(ctx, redisClient, userID, settings)
	return settings, nil
}

// loadUserSettings 从数据库读取原始设置，缺失时初始化为空 JSON。
func loadUserSettings(ctx context.Context, repositories repo.Repositories, userID int64) (json.RawMessage, error) {
	settingsResp, err := repositories.Settings.GetByUserID(ctx, userID)
	if errors.Is(err, repo.ErrSettingsNotFound) {
		empty := json.RawMessage(`{}`)
		if err := repositories.Settings.Upsert(ctx, userID, empty); err != nil {
			return nil, wrapError("初始化用户设置失败", err)
		}
		return empty, nil
	}
	if err != nil {
		return nil, wrapError("读取用户设置失败", err)
	}
	if len(settingsResp.Settings) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(settingsResp.Settings) {
		return nil, ErrInvalidSettings
	}
	return settingsResp.Settings, nil
}

// resolveUserSettings 从统一设置 JSON 中解析 AI 相关字段。
func resolveUserSettings(ctx context.Context, repositories repo.Repositories, redisClient *redis.Client, userID int64) (userAISettings, error) {
	raw, err := readUserSettings(ctx, repositories, redisClient, userID)
	if err != nil {
		return userAISettings{}, err
	}
	var settings userAISettings
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return userAISettings{}, ErrInvalidSettings
		}
	}
	return settings, nil
}

// resolveUserWritingSettings 从完整用户设置中派生写作参数。
func resolveUserWritingSettings(ctx context.Context, repositories repo.Repositories, redisClient *redis.Client, userID int64) (userWritingSettings, error) {
	settings, err := resolveUserSettings(ctx, repositories, redisClient, userID)
	if err != nil {
		return userWritingSettings{}, err
	}
	return normalizeUserWritingSettings(settings.General.ConsistencyCheckCount), nil
}

// userSettingsCacheKey 生成用户完整设置缓存键。
func userSettingsCacheKey(userID int64) string {
	return fmt.Sprintf("user:%d:settings", userID)
}

func cacheUserSettings(ctx context.Context, redisClient *redis.Client, userID int64, settings json.RawMessage) {
	if redisClient == nil || userID <= 0 {
		return
	}
	if len(settings) == 0 {
		settings = json.RawMessage(`{}`)
	}
	if !json.Valid(settings) {
		return
	}
	_ = redisClient.Set(ctx, userSettingsCacheKey(userID), []byte(settings), userSettingsCacheTTL).Err()
}

// normalizeUserWritingSettings 规范化一致性校验次数。
func normalizeUserWritingSettings(consistencyCheckCount int) userWritingSettings {
	if consistencyCheckCount <= 0 {
		consistencyCheckCount = defaultConsistencyCheckCount
	}
	if consistencyCheckCount > 10 {
		consistencyCheckCount = 10
	}
	return userWritingSettings{ConsistencyCheckCount: consistencyCheckCount}
}
