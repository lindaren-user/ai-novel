package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/crypto"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/redis/go-redis/v9"
)

const (
	userModelConfigCacheTTL = 30 * time.Minute
	defaultModelTopP        = 0.9
	defaultModelTemperature = 0.7
)

var supportedModelProviders = map[string]bool{
	"gpt":                       true,
	"deepseek":                  true,
	"gemini":                    true,
	"claude":                    true,
	"kimi":                      true,
	"doubao":                    true,
	"qianwen":                   true,
	"grok":                      true,
	"custom_openai_completions": true,
	"custom_openai_responses":   true,
}

// ModelConfigService 模型配置服务接口
type ModelConfigService interface {
	List(ctx context.Context, userID int64) ([]model.ModelConfig, error)
	Create(ctx context.Context, userID int64, req model.CreateModelRequest) (model.ModelConfig, error)
	Delete(ctx context.Context, userID int64, id int64) error
	Test(ctx context.Context, req model.CreateModelRequest) (model.TestModelResponse, error)
}

type modelConfigService struct {
	repositories repo.Repositories
	redisClient  *redis.Client
	aiClient     ai.Client
}

// NewModelConfigService 创建模型配置服务
func NewModelConfigService(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client) ModelConfigService {
	return &modelConfigService{repositories: repositories, redisClient: redisClient, aiClient: aiClient}
}

// List 获取内置模型和当前用户模型
func (s *modelConfigService) List(ctx context.Context, userID int64) ([]model.ModelConfig, error) {
	items, err := s.repositories.ModelConfigs.ListAvailable(ctx, userID)
	if err != nil {
		return nil, wrapError("查询模型列表失败", err)
	}
	for i := range items {
		items[i].APIKey = ""
		if items[i].UserID == 0 {
			items[i].APIURL = ""
			items[i].Provider = "official"
			items[i].ModelID = ""
			items[i].Name = "官方模型"
		}
	}
	return items, nil
}

// Create 新建当前用户自定义模型
func (s *modelConfigService) Create(ctx context.Context, userID int64, req model.CreateModelRequest) (model.ModelConfig, error) {
	if !validModelRequest(req) {
		return model.ModelConfig{}, ErrInvalidModel
	}
	input := model.ModelConfig{
		UserID:      userID,
		Name:        req.Name,
		Provider:    req.Provider,
		ModelID:     req.ModelID,
		APIURL:      req.APIURL,
		APIKey:      encryptModelAPIKey(req.APIKey),
		TopP:        defaultModelTopP,
		Temperature: defaultModelTemperature,
		Status:      req.Status,
	}
	item, err := s.repositories.ModelConfigs.Create(ctx, input)
	if errors.Is(err, repo.ErrModelExists) {
		return model.ModelConfig{}, ErrModelTaken
	}
	if err != nil {
		return model.ModelConfig{}, wrapError("创建模型配置失败", err)
	}
	item.APIKey = ""
	return item, nil
}

// Delete 删除当前用户自己的自定义模型，内置模型不允许删除。
func (s *modelConfigService) Delete(ctx context.Context, userID int64, id int64) error {
	item, err := s.repositories.ModelConfigs.FindByID(ctx, id)
	if errors.Is(err, repo.ErrModelNotFound) {
		return ErrResourceNotFound
	}
	if err != nil {
		return wrapError("查询模型配置失败", err)
	}
	if item.UserID != userID || item.UserID <= 0 || item.Status != model.ModelConfigStatusEnabled {
		return ErrResourceNotFound
	}
	if err := s.repositories.ModelConfigs.Update(ctx, item.ID, repo.UpdateFields{
		"status": model.ModelConfigStatusDisabled,
	}); err != nil {
		if errors.Is(err, repo.ErrModelNotFound) {
			return ErrResourceNotFound
		}
		return wrapError("删除模型配置失败", err)
	}
	s.deleteUserSettingsCache(ctx, userID)
	return nil
}

// Test 使用用户填写的模型配置发起一次最小聊天请求，验证 API 地址、Key 和模型 ID 是否可用。
func (s *modelConfigService) Test(ctx context.Context, req model.CreateModelRequest) (model.TestModelResponse, error) {
	if !validModelRequest(req) {
		return model.TestModelResponse{}, ErrInvalidModel
	}
	if s.aiClient == nil {
		return model.TestModelResponse{}, ErrAIUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// 使用 eino 内置的请求函数，只能用于测试官方的 API（内部自动拼接 URL）
	result, err := s.aiClient.GenerateChat(ctx, ai.ChatGenerateRequest{
		ModelKey: "model-test",
		Model: ai.ModelRuntimeConfig{
			Provider:    req.Provider,
			ModelID:     req.ModelID,
			APIURL:      req.APIURL,
			APIKey:      req.APIKey,
			TopP:        defaultModelTopP,
			Temperature: defaultModelTemperature,
		},
		Messages: []ai.StreamMessage{
			{Role: "user", Content: "Reply with OK only."},
		},
	})
	if err != nil {
		if errors.Is(err, ai.ErrInvalidChatModelConfig) {
			return model.TestModelResponse{}, ErrInvalidModel
		}
		return model.TestModelResponse{}, fmt.Errorf("%w: %v", ErrAIUnavailable, err)
	}
	if strings.TrimSpace(result.Content) == "" {
		return model.TestModelResponse{}, fmt.Errorf("%w: 模型测试响应不是有效的模型输出", ErrInvalidModel)
	}
	return model.TestModelResponse{OK: true, Message: "连接成功"}, nil
}

// encryptModelAPIKey 加密模型密钥，失败时保留原值以避免阻断用户保存配置。
func encryptModelAPIKey(key string) string {
	encrypted, err := crypto.Encrypt(key)
	if err != nil || encrypted == "" {
		return key
	}
	return encrypted
}

// deleteUserSettingsCache 删除用户设置和 model_config 缓存，避免模型列表变更后继续使用旧模型选择。
func (s *modelConfigService) deleteUserSettingsCache(ctx context.Context, userID int64) {
	if s.redisClient == nil || userID <= 0 {
		return
	}
	_ = s.redisClient.Del(ctx, userSettingsCacheKey(userID)).Err()
}

// userModelConfigCacheKey 生成用户 model_config 缓存键，scope 为模型 ID 或 default。
func userModelConfigCacheKey(userID int64, scope string) string {
	return fmt.Sprintf("user:%d:model_config:%s", userID, scope)
}

// validModelRequest 校验模型配置是否满足后续调用所需的最低字段要求。
func validModelRequest(req model.CreateModelRequest) bool {
	return supportedModelProviders[req.Provider] &&
		req.ModelID != "" &&
		req.APIURL != "" &&
		req.APIKey != "" &&
		(req.Status == model.ModelConfigStatusEnabled || req.Status == model.ModelConfigStatusDisabled)
}
