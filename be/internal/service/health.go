package service

import (
	"context"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"
)

// HealthService 健康检查服务接口
type HealthService interface {
	Check(ctx context.Context) model.HealthResponse
}

type healthService struct {
	repositories repo.Repositories
	aiClient     ai.Client
}

// NewHealthService 创建健康检查服务
func NewHealthService(repositories repo.Repositories, aiClient ai.Client) HealthService {
	return &healthService{
		repositories: repositories,
		aiClient:     aiClient,
	}
}

// Check 健康检查
func (s *healthService) Check(ctx context.Context) model.HealthResponse {
	return model.HealthResponse{
		Status: "ok",
	}
}
