package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ai-novel-ide/be/internal/model"
)

// ChapterContentRepository 章节正文数据访问接口。
type ChapterContentRepository interface {
	CreateDraft(ctx context.Context, chapterID int64, sourceMessageID int64, content string, wordCount int, draftType int16, draftName string) (model.ChapterContentDraftRecord, error)
	ListDraftsByChapter(ctx context.Context, chapterID int64, draftType int16, status int16) ([]model.ChapterContentDraftRecord, error)
	CopyDraft(ctx context.Context, chapterID int64, draftID int64, sourceDraftType int16, targetDraftType int16, draftName string) (model.ChapterContentDraftRecord, error)
	FindByID(ctx context.Context, id int64) (model.ChapterContentDraftRecord, error)
	Update(ctx context.Context, id int64, fields UpdateFields) error
	DeleteByID(ctx context.Context, id int64) error
	DeleteByChapterIDs(ctx context.Context, chapterIDs []int64) error
}

type chapterContentRepository struct {
	db DBTX
}

// NewChapterContentRepository 创建章节正文 repo。
func NewChapterContentRepository(db DBTX) ChapterContentRepository {
	return &chapterContentRepository{db: db}
}

// CreateDraft 创建章节正文草稿，可通过 draftType 区分 AI 原始草稿和可编辑草稿。
func (r *chapterContentRepository) CreateDraft(ctx context.Context, chapterID int64, sourceMessageID int64, content string, wordCount int, draftType int16, draftName string) (model.ChapterContentDraftRecord, error) {
	draft, err := scanChapterDraft(r.db.QueryRowContext(ctx, `
		INSERT INTO t_chapter_contents (chapter_id, source_message_id, content, status, word_count, draft_type, draft_name)
		VALUES ($1, $2, $3, 1, $4, $5, BTRIM($6))
		RETURNING id, chapter_id, source_message_id, draft_type, origin_draft_id, draft_name, content, status, word_count, is_deleted, used_at, created_at, updated_at
	`, chapterID, sourceMessageID, content, wordCount, draftType, draftName))
	return draft, wrapDBError("创建章节草稿失败", err)
}

// ListDraftsByChapter 按草稿类型或状态查询章节草稿。
func (r *chapterContentRepository) ListDraftsByChapter(ctx context.Context, chapterID int64, draftType int16, status int16) ([]model.ChapterContentDraftRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chapter_id, source_message_id, draft_type, origin_draft_id, draft_name, content, status, word_count, is_deleted, used_at, created_at, updated_at
		FROM t_chapter_contents
		WHERE chapter_id = $1
		  AND (draft_type = $2 OR status = $3)
		  AND is_deleted = 0::smallint
		ORDER BY updated_at DESC, id DESC
	`, chapterID, draftType, status)
	if err != nil {
		return nil, wrapDBError("查询章节草稿列表失败", err)
	}
	defer rows.Close()

	drafts := make([]model.ChapterContentDraftRecord, 0)
	for rows.Next() {
		draft, err := scanChapterDraft(rows)
		if err != nil {
			return nil, wrapDBError("扫描章节草稿列表失败", err)
		}
		drafts = append(drafts, draft)
	}
	return drafts, wrapDBError("遍历章节草稿列表失败", rows.Err())
}

// CopyDraft 按指定源草稿类型复制一份目标类型草稿。
func (r *chapterContentRepository) CopyDraft(ctx context.Context, chapterID int64, draftID int64, sourceDraftType int16, targetDraftType int16, draftName string) (model.ChapterContentDraftRecord, error) {
	draft, err := scanChapterDraft(r.db.QueryRowContext(ctx, `
		INSERT INTO t_chapter_contents (chapter_id, source_message_id, content, status, word_count, draft_type, origin_draft_id, draft_name)
		SELECT chapter_id, 0, content, 1, word_count, $4, id, BTRIM($5)
		FROM t_chapter_contents
		WHERE id = $1
		  AND chapter_id = $2
		  AND draft_type = $3
		  AND is_deleted = 0::smallint
		  AND content <> ''
		RETURNING id, chapter_id, source_message_id, draft_type, origin_draft_id, draft_name, content, status, word_count, is_deleted, used_at, created_at, updated_at
	`, draftID, chapterID, sourceDraftType, targetDraftType, draftName))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ChapterContentDraftRecord{}, ErrChapterNotFound
	}
	return draft, wrapDBError("加入章节草稿失败", err)
}

// FindByID 按正文草稿 ID 查询完整记录。
func (r *chapterContentRepository) FindByID(ctx context.Context, id int64) (model.ChapterContentDraftRecord, error) {
	draft, err := scanChapterDraft(r.db.QueryRowContext(ctx, `
		SELECT id, chapter_id, source_message_id, draft_type, origin_draft_id, draft_name, content, status, word_count, is_deleted, used_at, created_at, updated_at
		FROM t_chapter_contents
		WHERE id = $1 AND is_deleted = 0::smallint
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ChapterContentDraftRecord{}, ErrChapterNotFound
	}
	return draft, wrapDBError("查询章节正文草稿失败", err)
}

// Update 按 ID 更新正文草稿白名单字段，不包含业务判断。
func (r *chapterContentRepository) Update(ctx context.Context, id int64, fields UpdateFields) error {
	return execUpdateFields(ctx, r.db, "t_chapter_contents", id, fields, chapterContentUpdateFields, ErrChapterNotFound, "更新章节正文草稿失败")
}

// DeleteByID 按 ID 删除正文草稿。
func (r *chapterContentRepository) DeleteByID(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE t_chapter_contents
		SET is_deleted = 1::smallint, updated_at = now()
		WHERE id = $1 AND is_deleted = 0::smallint
	`, id)
	if err != nil {
		return wrapDBError("删除章节正文草稿失败", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapDBError("获取章节正文草稿删除结果失败", err)
	}
	if affected == 0 {
		return ErrChapterNotFound
	}
	return nil
}

// DeleteByChapterIDs 按章节 ID 批量删除正文草稿。
func (r *chapterContentRepository) DeleteByChapterIDs(ctx context.Context, chapterIDs []int64) error {
	if len(chapterIDs) == 0 {
		return nil
	}
	payload, err := idPayloadJSON(chapterIDs)
	if err != nil {
		return wrapDBError("序列化章节正文 ID 失败", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE t_chapter_contents cc
		SET is_deleted = 1::smallint, updated_at = now()
		FROM jsonb_to_recordset($1::jsonb) AS item(id bigint)
		WHERE cc.chapter_id = item.id AND cc.is_deleted = 0::smallint
	`, payload); err != nil {
		return wrapDBError("删除章节正文失败", err)
	}
	return nil
}

var chapterContentUpdateFields = map[string]updateFieldSpec{
	"chapter_id":        columnUpdateField("chapter_id"),
	"source_message_id": columnUpdateField("source_message_id"),
	"draft_type":        smallintUpdateField("draft_type"),
	"origin_draft_id":   columnUpdateField("origin_draft_id"),
	"draft_name":        {assignment: func(placeholder int) string { return fmt.Sprintf("draft_name = BTRIM($%d)", placeholder) }},
	"content":           columnUpdateField("content"),
	"status":            smallintUpdateField("status"),
	"word_count":        columnUpdateField("word_count"),
	"is_deleted":        smallintUpdateField("is_deleted"),
	"used_at":           columnUpdateField("used_at"),
}

type chapterDraftScanner interface {
	Scan(dest ...any) error
}

// scanChapterDraft 统一扫描章节正文草稿记录，避免多处字段顺序不一致。
func scanChapterDraft(scanner chapterDraftScanner) (model.ChapterContentDraftRecord, error) {
	var draft model.ChapterContentDraftRecord
	var usedAt sql.NullTime
	err := scanner.Scan(
		&draft.ID,
		&draft.ChapterID,
		&draft.SourceMessageID,
		&draft.DraftType,
		&draft.OriginDraftID,
		&draft.DraftName,
		&draft.Content,
		&draft.Status,
		&draft.WordCount,
		&draft.IsDeleted,
		&usedAt,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	)
	if usedAt.Valid {
		draft.UsedAt = &usedAt.Time
	}
	return draft, err
}
