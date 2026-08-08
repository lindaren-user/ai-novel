package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ai-novel-ide/be/internal/model"
)

// SettingsRepository 用户设置数据访问接口
type SettingsRepository interface {
	GetByUserID(ctx context.Context, userID int64) (model.SettingsResponse, error)
	Upsert(ctx context.Context, userID int64, settings json.RawMessage) error
}

type settingsRepository struct {
	db DBTX
}

// NewSettingsRepository 创建设置 repo
func NewSettingsRepository(db DBTX) SettingsRepository {
	return &settingsRepository{db: db}
}

// GetByUserID 根据用户 ID 读取设置
func (r *settingsRepository) GetByUserID(ctx context.Context, userID int64) (model.SettingsResponse, error) {
	var raw []byte
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT settings, updated_at
		FROM t_user_settings
		WHERE user_id = $1
	`, userID).Scan(&raw, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SettingsResponse{}, ErrSettingsNotFound
	}
	if err != nil {
		return model.SettingsResponse{}, wrapDBError("查询用户设置失败", err)
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	return model.SettingsResponse{Settings: json.RawMessage(raw), UpdatedAt: updatedAt}, nil
}

// Upsert 新建或更新用户设置，不返回更新后的模型。
func (r *settingsRepository) Upsert(ctx context.Context, userID int64, settings json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO t_user_settings (user_id, settings)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (user_id)
		DO UPDATE SET settings = EXCLUDED.settings, updated_at = now()
	`, userID, string(settings))
	return wrapDBError("保存用户设置失败", err)
}
