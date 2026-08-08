package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"ai-novel-ide/be/internal/model"
)

// ModelConfigRepository 模型配置数据访问接口
type ModelConfigRepository interface {
	ListAvailable(ctx context.Context, userID int64) ([]model.ModelConfig, error)
	FindByID(ctx context.Context, id int64) (model.ModelConfig, error)
	FindAvailableByID(ctx context.Context, userID int64, id int64) (model.ModelConfig, error)
	Create(ctx context.Context, item model.ModelConfig) (model.ModelConfig, error)
	Update(ctx context.Context, id int64, fields UpdateFields) error
}

type modelConfigRepository struct {
	db DBTX
}

// NewModelConfigRepository 创建模型配置 repo
func NewModelConfigRepository(db DBTX) ModelConfigRepository {
	return &modelConfigRepository{db: db}
}

// ListAvailable 查询内置模型和当前用户启用的自定义模型。
func (r *modelConfigRepository) ListAvailable(ctx context.Context, userID int64) ([]model.ModelConfig, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, provider, model_id, api_url, api_key, top_p, temperature, status, created_at, updated_at
		FROM t_models
		WHERE status = $2 AND (user_id = 0 OR user_id = $1)
		ORDER BY user_id ASC, id ASC
	`, userID, model.ModelConfigStatusEnabled)
	if err != nil {
		return nil, wrapDBError("查询可用模型列表失败", err)
	}
	defer rows.Close()

	modelConfigs := make([]model.ModelConfig, 0)
	for rows.Next() {
		var item model.ModelConfig
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Name,
			&item.Provider,
			&item.ModelID,
			&item.APIURL,
			&item.APIKey,
			&item.TopP,
			&item.Temperature,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, wrapDBError("扫描模型列表失败", err)
		}
		modelConfigs = append(modelConfigs, item)
	}
	return modelConfigs, wrapDBError("遍历模型配置列表失败", rows.Err())
}

// FindByID 按模型配置 ID 查询完整记录。
func (r *modelConfigRepository) FindByID(ctx context.Context, id int64) (model.ModelConfig, error) {
	var item model.ModelConfig
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, provider, model_id, api_url, api_key, top_p, temperature, status, created_at, updated_at
		FROM t_models
		WHERE id = $1
	`, id).Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Provider,
		&item.ModelID,
		&item.APIURL,
		&item.APIKey,
		&item.TopP,
		&item.Temperature,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ModelConfig{}, ErrModelNotFound
	}
	return item, wrapDBError("按 ID 查询模型配置失败", err)
}

// FindAvailableByID 查询当前用户可用的某个模型，包含内置模型和用户自己的启用模型。
func (r *modelConfigRepository) FindAvailableByID(ctx context.Context, userID int64, id int64) (model.ModelConfig, error) {
	var item model.ModelConfig
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, provider, model_id, api_url, api_key, top_p, temperature, status, created_at, updated_at
		FROM t_models
		WHERE id = $1 AND status = $3 AND (user_id = 0 OR user_id = $2)
		LIMIT 1
	`, id, userID, model.ModelConfigStatusEnabled).Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Provider,
		&item.ModelID,
		&item.APIURL,
		&item.APIKey,
		&item.TopP,
		&item.Temperature,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ModelConfig{}, ErrModelNotFound
	}
	if err != nil {
		return model.ModelConfig{}, wrapDBError("按 ID 查询可用模型失败", err)
	}
	return item, nil
}

// Create 新建当前用户的自定义模型配置记录。
func (r *modelConfigRepository) Create(ctx context.Context, input model.ModelConfig) (model.ModelConfig, error) {
	var saved model.ModelConfig
	if input.Status == 0 {
		input.Status = model.ModelConfigStatusEnabled
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO t_models (user_id, name, provider, model_id, api_url, api_key, top_p, temperature, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, name, provider, model_id, api_url, api_key, top_p, temperature, status, created_at, updated_at
	`, input.UserID, input.Name, input.Provider, input.ModelID, input.APIURL, input.APIKey, input.TopP, input.Temperature, input.Status).Scan(
		&saved.ID,
		&saved.UserID,
		&saved.Name,
		&saved.Provider,
		&saved.ModelID,
		&saved.APIURL,
		&saved.APIKey,
		&saved.TopP,
		&saved.Temperature,
		&saved.Status,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "t_models_user_id_name_key") {
			return model.ModelConfig{}, ErrModelExists
		}
		return model.ModelConfig{}, wrapDBError("创建模型配置失败", err)
	}
	return saved, nil
}

// Update 按 ID 更新模型配置白名单字段，不包含业务判断。
func (r *modelConfigRepository) Update(ctx context.Context, id int64, fields UpdateFields) error {
	return execUpdateFields(ctx, r.db, "t_models", id, fields, modelUpdateFields, ErrModelNotFound, "更新模型配置失败")
}

var modelUpdateFields = map[string]updateFieldSpec{
	"user_id":     columnUpdateField("user_id"),
	"name":        columnUpdateField("name"),
	"provider":    columnUpdateField("provider"),
	"model_id":    columnUpdateField("model_id"),
	"api_url":     columnUpdateField("api_url"),
	"api_key":     columnUpdateField("api_key"),
	"top_p":       columnUpdateField("top_p"),
	"temperature": columnUpdateField("temperature"),
	"status":      smallintUpdateField("status"),
}
