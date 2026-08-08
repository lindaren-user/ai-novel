package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ai-novel-ide/be/internal/model"
)

// ChapterRepository 章节数据访问接口
type ChapterRepository interface {
	ListByVolumeID(ctx context.Context, volumeID int64) ([]model.Chapter, error)
	ListByNovelID(ctx context.Context, novelID int64) ([]model.Chapter, error)
	FindByID(ctx context.Context, id int64) (model.Chapter, error)
	FindByVolumeSortOrder(ctx context.Context, volumeID int64, sortOrder int) (model.Chapter, error)
	CreateManyByVolumeID(ctx context.Context, volumeID int64, chapters []model.Chapter) ([]model.Chapter, error)
	DeleteByIDs(ctx context.Context, ids []int64) error
	Update(ctx context.Context, id int64, fields UpdateFields) error
	SumWordCountByVolumeID(ctx context.Context, volumeID int64) (int, error)
	SumWordCountByNovelID(ctx context.Context, novelID int64) (int, error)
}

type chapterRepository struct {
	db DBTX
}

// NewChapterRepository 创建章节 repo
func NewChapterRepository(db DBTX) ChapterRepository {
	return &chapterRepository{db: db}
}

// ListByVolumeID 查询卷下的章节列表。
func (r *chapterRepository) ListByVolumeID(ctx context.Context, volumeID int64) ([]model.Chapter, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.volume_id, c.plan_data, COALESCE(cc.content, ''), c.sort_order, c.status,
			COALESCE(cc.word_count, 0), c.is_deleted, c.created_at, c.updated_at, c.completed_at
		FROM t_chapters c
		LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE c.volume_id = $1 AND c.is_deleted = 0::smallint
		ORDER BY c.sort_order ASC, c.id ASC
	`, volumeID)
	if err != nil {
		return nil, wrapDBError("查询章节列表失败", err)
	}
	defer rows.Close()
	return scanChapterRows(rows, "扫描章节列表失败", "遍历章节列表失败")
}

func scanChapterRows(rows *sql.Rows, scanAction string, rowsAction string) ([]model.Chapter, error) {
	chapters := make([]model.Chapter, 0)
	for rows.Next() {
		var chapter model.Chapter
		var completedAt sql.NullTime
		if err := rows.Scan(
			&chapter.ID,
			&chapter.VolumeID,
			&chapter.PlanData,
			&chapter.Content,
			&chapter.SortOrder,
			&chapter.Status,
			&chapter.WordCount,
			&chapter.IsDeleted,
			&chapter.CreatedAt,
			&chapter.UpdatedAt,
			&completedAt,
		); err != nil {
			return nil, wrapDBError(scanAction, err)
		}
		if completedAt.Valid {
			chapter.CompletedAt = &completedAt.Time
		}
		chapters = append(chapters, chapter)
	}
	return chapters, wrapDBError(rowsAction, rows.Err())
}

// ListByNovelID 查询小说下的章节列表。
func (r *chapterRepository) ListByNovelID(ctx context.Context, novelID int64) ([]model.Chapter, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.volume_id, c.plan_data, COALESCE(cc.content, ''), c.sort_order, c.status,
			COALESCE(cc.word_count, 0), c.is_deleted, c.created_at, c.updated_at, c.completed_at
		FROM t_chapters c
		JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
		LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE v.novel_id = $1 AND c.is_deleted = 0::smallint
		ORDER BY v.sort_order ASC, c.sort_order ASC, c.id ASC
	`, novelID)
	if err != nil {
		return nil, wrapDBError("查询小说章节列表失败", err)
	}
	defer rows.Close()
	return scanChapterRows(rows, "扫描小说章节列表失败", "遍历小说章节列表失败")
}

// FindByID 根据 ID 查询章节。
func (r *chapterRepository) FindByID(ctx context.Context, id int64) (model.Chapter, error) {
	return r.findOne(ctx, `
		SELECT c.id, c.volume_id, c.plan_data, COALESCE(cc.content, ''), c.sort_order, c.status,
			COALESCE(cc.word_count, 0), c.is_deleted, c.created_at, c.updated_at, c.completed_at
		FROM t_chapters c
		LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE c.id = $1 AND c.is_deleted = 0::smallint
	`, id)
}

// FindByVolumeSortOrder 根据卷和章节顺序查询章节。
func (r *chapterRepository) FindByVolumeSortOrder(ctx context.Context, volumeID int64, sortOrder int) (model.Chapter, error) {
	return r.findOne(ctx, `
		SELECT c.id, c.volume_id, c.plan_data, COALESCE(cc.content, ''), c.sort_order, c.status,
			COALESCE(cc.word_count, 0), c.is_deleted, c.created_at, c.updated_at, c.completed_at
		FROM t_chapters c
		LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE c.volume_id = $1 AND c.sort_order = $2 AND c.is_deleted = 0::smallint
	`, volumeID, sortOrder)
}

// findOne 查询单章并统一扫描字段。
func (r *chapterRepository) findOne(ctx context.Context, sqlText string, args ...any) (model.Chapter, error) {
	var chapter model.Chapter
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, sqlText, args...).Scan(
		&chapter.ID,
		&chapter.VolumeID,
		&chapter.PlanData,
		&chapter.Content,
		&chapter.SortOrder,
		&chapter.Status,
		&chapter.WordCount,
		&chapter.IsDeleted,
		&chapter.CreatedAt,
		&chapter.UpdatedAt,
		&completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Chapter{}, ErrChapterNotFound
	}
	if completedAt.Valid {
		chapter.CompletedAt = &completedAt.Time
	}
	return chapter, wrapDBError("查询单章失败", err)
}

// CreateManyByVolumeID 按 sort_order 批量保存章节列表。
func (r *chapterRepository) CreateManyByVolumeID(ctx context.Context, volumeID int64, chapters []model.Chapter) ([]model.Chapter, error) {
	if len(chapters) == 0 {
		return []model.Chapter{}, nil
	}

	payload := make([]map[string]any, 0, len(chapters))
	for _, chapter := range chapters {
		planDataJSON, err := json.Marshal(chapter.PlanData)
		if err != nil {
			return nil, wrapDBError("序列化章节规划数据失败", err)
		}
		payload = append(payload, map[string]any{
			"plan_data":  json.RawMessage(planDataJSON),
			"sort_order": chapter.SortOrder,
			"status":     chapter.Status,
			"word_count": chapter.WordCount,
		})
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, wrapDBError("序列化章节批量保存数据失败", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		INSERT INTO t_chapters (volume_id, plan_data, sort_order, status, word_count)
		SELECT $1, item.plan_data, item.sort_order, item.status, item.word_count
		FROM jsonb_to_recordset($2::jsonb) AS item(
			plan_data jsonb,
			sort_order integer,
			status smallint,
			word_count integer
		)
		ORDER BY item.sort_order ASC
		RETURNING id, volume_id, plan_data, sort_order, status, word_count, is_deleted, created_at, updated_at, completed_at
	`, volumeID, string(payloadJSON))
	if err != nil {
		return nil, wrapDBError("批量保存章节规划失败", err)
	}
	defer rows.Close()

	saved := make([]model.Chapter, 0, len(chapters))
	for rows.Next() {
		var chapter model.Chapter
		var completedAt sql.NullTime
		if err := rows.Scan(
			&chapter.ID,
			&chapter.VolumeID,
			&chapter.PlanData,
			&chapter.SortOrder,
			&chapter.Status,
			&chapter.WordCount,
			&chapter.IsDeleted,
			&chapter.CreatedAt,
			&chapter.UpdatedAt,
			&completedAt,
		); err != nil {
			return nil, wrapDBError("扫描批量保存章节规划失败", err)
		}
		if completedAt.Valid {
			chapter.CompletedAt = &completedAt.Time
		}
		saved = append(saved, chapter)
	}
	return saved, wrapDBError("遍历批量保存章节规划失败", rows.Err())
}

// DeleteByIDs 按章节 ID 批量删除章节。
func (r *chapterRepository) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	payload, err := idPayloadJSON(ids)
	if err != nil {
		return wrapDBError("序列化章节 ID 失败", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE t_chapters c
		SET is_deleted = 1::smallint, updated_at = now()
		FROM jsonb_to_recordset($1::jsonb) AS item(id bigint)
		WHERE c.id = item.id AND c.is_deleted = 0::smallint
	`, payload); err != nil {
		return wrapDBError("删除旧章节规划失败", err)
	}
	return nil
}

// Update 按 ID 更新章节白名单字段，不包含业务判断。
func (r *chapterRepository) Update(ctx context.Context, id int64, fields UpdateFields) error {
	return execUpdateFields(ctx, r.db, "t_chapters", id, fields, chapterUpdateFields, ErrChapterNotFound, "更新章节失败")
}

var chapterUpdateFields = map[string]updateFieldSpec{
	"volume_id":    columnUpdateField("volume_id"),
	"plan_data":    jsonbUpdateField("plan_data"),
	"sort_order":   columnUpdateField("sort_order"),
	"status":       smallintUpdateField("status"),
	"word_count":   columnUpdateField("word_count"),
	"completed_at": columnUpdateField("completed_at"),
}

// SumWordCountByVolumeID 汇总指定卷下当前正文的字数。
func (r *chapterRepository) SumWordCountByVolumeID(ctx context.Context, volumeID int64) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cc.word_count), 0)::int
		FROM t_chapters c
		JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE c.volume_id = $1 AND c.is_deleted = 0::smallint
	`, volumeID).Scan(&total)
	return total, wrapDBError("汇总卷字数失败", err)
}

// SumWordCountByNovelID 汇总指定小说下当前正文的字数。
func (r *chapterRepository) SumWordCountByNovelID(ctx context.Context, novelID int64) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cc.word_count), 0)::int
		FROM t_chapters c
		JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
		JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE v.novel_id = $1 AND c.is_deleted = 0::smallint
	`, novelID).Scan(&total)
	return total, wrapDBError("汇总小说字数失败", err)
}
