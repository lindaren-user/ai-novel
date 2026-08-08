package repo

import (
	"context"

	"ai-novel-ide/be/internal/model"
)

type ModelRunRepository interface {
	Create(ctx context.Context, run model.ModelRun) (model.ModelRun, error)
	Finish(ctx context.Context, id int64, run model.ModelRun) error
}

type modelRunRepository struct {
	db DBTX
}

func NewModelRunRepository(db DBTX) ModelRunRepository {
	return &modelRunRepository{db: db}
}

// Create 新增一条 AI 运行记录。
func (r *modelRunRepository) Create(ctx context.Context, run model.ModelRun) (model.ModelRun, error) {
	if run.ScopeID == nil || *run.ScopeID <= 0 {
		run.ScopeID = nil
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO t_model_runs (
			user_id, scope_type, scope_id, model_id, status, start_time
		)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, now()))
		RETURNING id, user_id, scope_type, scope_id, model_id, status,
			token_count, finish_reason, error_message, start_time, end_time
	`,
		run.UserID,
		run.ScopeType,
		run.ScopeID,
		run.ModelID,
		run.Status,
		run.StartTime,
	).Scan(
		&run.ID,
		&run.UserID,
		&run.ScopeType,
		&run.ScopeID,
		&run.ModelID,
		&run.Status,
		&run.TokenCount,
		&run.FinishReason,
		&run.ErrorMessage,
		&run.StartTime,
		&run.EndTime,
	)
	return run, wrapDBError("插入 AI 运行记录失败", err)
}

// Finish 更新 AI 运行结束信息。
func (r *modelRunRepository) Finish(ctx context.Context, id int64, run model.ModelRun) error {
	if id <= 0 {
		return nil
	}
	if run.ScopeID == nil || *run.ScopeID <= 0 {
		run.ScopeID = nil
	}
	endTime := run.EndTime
	if endTime == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE t_model_runs
		SET scope_id = COALESCE($2, scope_id),
			status = $3,
			token_count = CASE WHEN $4::bigint < 0 THEN token_count ELSE $4 END,
			finish_reason = CASE WHEN $5 = '' THEN finish_reason ELSE $5 END,
			error_message = $6,
			end_time = $7
		WHERE id = $1
	`, id, run.ScopeID, run.Status, run.TokenCount, run.FinishReason, run.ErrorMessage, endTime)
	return wrapDBError("更新 AI 运行记录失败", err)
}
