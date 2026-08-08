package repo

import (
	"context"
	"encoding/json"

	"ai-novel-ide/be/internal/model"
)

type ChatMessageRepository interface {
	CreateWithMeta(ctx context.Context, sessionID int64, role int16, content string, renderData model.JSONMap) (model.ChatMessage, error)
	ListBySessionID(ctx context.Context, sessionID int64) ([]model.ChatMessage, error)
	DeleteBySessionIDs(ctx context.Context, sessionIDs []int64) error
}

type chatMessageRepository struct {
	db DBTX
}

// NewChatMessageRepository 创建对话消息 repo。
func NewChatMessageRepository(db DBTX) ChatMessageRepository {
	return &chatMessageRepository{db: db}
}

// CreateWithMeta 新增一条带结构化渲染数据的对话消息。
func (r *chatMessageRepository) CreateWithMeta(ctx context.Context, sessionID int64, role int16, content string, renderData model.JSONMap) (model.ChatMessage, error) {
	var message model.ChatMessage
	if renderData == nil {
		renderData = model.JSONMap{}
	}
	payload, err := json.Marshal(renderData)
	if err != nil {
		return message, wrapDBError("序列化消息渲染数据失败", err)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO t_chat_messages (session_id, role, content, render_data)
		VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'::jsonb))
		RETURNING id, session_id, content, render_data, created_at
	`, sessionID, role, content, string(payload)).Scan(
		&message.ID,
		&message.SessionID,
		&message.Content,
		&message.RenderData,
		&message.CreatedAt,
	)
	message.Role = model.ChatRoleName(role)
	return message, wrapDBError("插入对话消息失败", err)
}

// ListBySessionID 按会话 ID 查询消息，并挂载助手消息对应的正文草稿 ID。
func (r *chatMessageRepository) ListBySessionID(ctx context.Context, sessionID int64) ([]model.ChatMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.session_id, m.role, m.content, m.render_data, m.created_at,
			COALESCE(d.id, 0) AS draft_id
		FROM t_chat_messages m
		LEFT JOIN LATERAL (
			SELECT id
			FROM t_chapter_contents
			WHERE source_message_id = m.id
			  AND is_deleted = 0::smallint
			ORDER BY id DESC
			LIMIT 1
		) d ON true
		WHERE m.session_id = $1 AND m.is_deleted = 0::smallint
		ORDER BY m.created_at ASC, m.id ASC
	`, sessionID)
	if err != nil {
		return nil, wrapDBError("查询对话消息列表失败", err)
	}
	defer rows.Close()

	messages := make([]model.ChatMessage, 0)
	for rows.Next() {
		var message model.ChatMessage
		var role int16
		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&role,
			&message.Content,
			&message.RenderData,
			&message.CreatedAt,
			&message.DraftID,
		); err != nil {
			return nil, wrapDBError("扫描对话消息列表失败", err)
		}
		message.Role = model.ChatRoleName(role)
		messages = append(messages, message)
	}
	return messages, wrapDBError("遍历对话消息列表失败", rows.Err())
}

// DeleteBySessionIDs 按会话 ID 批量删除对话消息。
func (r *chatMessageRepository) DeleteBySessionIDs(ctx context.Context, sessionIDs []int64) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	payload, err := idPayloadJSON(sessionIDs)
	if err != nil {
		return wrapDBError("序列化会话消息 ID 失败", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE t_chat_messages m
		SET is_deleted = 1::smallint
		FROM jsonb_to_recordset($1::jsonb) AS item(id bigint)
		WHERE m.session_id = item.id AND m.is_deleted = 0::smallint
	`, payload); err != nil {
		return wrapDBError("删除会话消息失败", err)
	}
	return nil
}
