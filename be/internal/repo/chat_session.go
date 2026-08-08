package repo

import (
	"context"
	"database/sql"
	"errors"

	"ai-novel-ide/be/internal/model"
)

// ChatSessionRepository 对话会话数据访问接口。
type ChatSessionRepository interface {
	Create(ctx context.Context, userID int64, scopeType int16, scopeID int64, title string) (int64, error)
	FindByUserScope(ctx context.Context, userID int64, scopeType int16, scopeID int64) (model.ChatSession, error)
	ListIDsByScopes(ctx context.Context, scopeType int16, scopeIDs []int64) ([]int64, error)
	DeleteByIDs(ctx context.Context, ids []int64) error
}

type chatSessionRepository struct {
	db DBTX
}

// NewChatSessionRepository 创建对话会话 repo。
func NewChatSessionRepository(db DBTX) ChatSessionRepository {
	return &chatSessionRepository{db: db}
}

// Create 新建对话会话记录。
func (r *chatSessionRepository) Create(ctx context.Context, userID int64, scopeType int16, scopeID int64, title string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO t_chat_sessions (user_id, scope_type, scope_id, title)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, scopeType, scopeID, title).Scan(&id)
	return id, wrapDBError("创建对话会话失败", err)
}

// FindByUserScope 根据用户和范围查询最新会话完整信息。
func (r *chatSessionRepository) FindByUserScope(ctx context.Context, userID int64, scopeType int16, scopeID int64) (model.ChatSession, error) {
	var session model.ChatSession
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, scope_type, scope_id, title, created_at, updated_at
		FROM t_chat_sessions
		WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3 AND is_deleted = 0::smallint
		ORDER BY updated_at DESC, id DESC
		LIMIT 1
	`, userID, scopeType, scopeID).Scan(
		&session.ID,
		&session.UserID,
		&session.ScopeType,
		&session.ScopeID,
		&session.Title,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ChatSession{}, ErrChatSessionNotFound
	}
	return session, wrapDBError("查询对话会话失败", err)
}

// ListIDsByScopes 按范围类型和范围 ID 查询未删除会话 ID。
func (r *chatSessionRepository) ListIDsByScopes(ctx context.Context, scopeType int16, scopeIDs []int64) ([]int64, error) {
	if len(scopeIDs) == 0 {
		return []int64{}, nil
	}
	payload, err := idPayloadJSON(scopeIDs)
	if err != nil {
		return nil, wrapDBError("序列化会话范围 ID 失败", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id
		FROM t_chat_sessions s
		JOIN jsonb_to_recordset($2::jsonb) AS item(id bigint) ON item.id = s.scope_id
		WHERE s.scope_type = $1::smallint AND s.is_deleted = 0::smallint
	`, scopeType, payload)
	if err != nil {
		return nil, wrapDBError("查询会话 ID 失败", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, wrapDBError("扫描会话 ID 失败", err)
		}
		ids = append(ids, id)
	}
	return ids, wrapDBError("遍历会话 ID 失败", rows.Err())
}

// DeleteByIDs 按会话 ID 批量删除会话。
func (r *chatSessionRepository) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	payload, err := idPayloadJSON(ids)
	if err != nil {
		return wrapDBError("序列化会话 ID 失败", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE t_chat_sessions s
		SET is_deleted = 1::smallint, updated_at = now()
		FROM jsonb_to_recordset($1::jsonb) AS item(id bigint)
		WHERE s.id = item.id AND s.is_deleted = 0::smallint
	`, payload); err != nil {
		return wrapDBError("删除会话失败", err)
	}
	return nil
}
