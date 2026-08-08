package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ai-novel-ide/be/internal/model"
)

// VolumeRepository 卷数据访问接口
type VolumeRepository interface {
	ListByNovelID(ctx context.Context, novelID int64) ([]model.Volume, error)
	FindByID(ctx context.Context, id int64) (model.Volume, error)
	CreateManyByNovelID(ctx context.Context, novelID int64, volumes []model.Volume) ([]model.Volume, error)
	DeleteByIDs(ctx context.Context, ids []int64) error
}

type volumeRepository struct {
	db DBTX
}

// NewVolumeRepository 创建卷 repo
func NewVolumeRepository(db DBTX) VolumeRepository {
	return &volumeRepository{db: db}
}

// ListByNovelID 查询小说下的卷列表。
func (r *volumeRepository) ListByNovelID(ctx context.Context, novelID int64) ([]model.Volume, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, v.novel_id, v.plan_data, v.sort_order, v.status, v.word_count, v.is_deleted,
			COUNT(c.id)::int AS chapter_count, v.created_at, v.updated_at
		FROM t_volumes v
		LEFT JOIN t_chapters c ON c.volume_id = v.id AND c.is_deleted = 0::smallint
		WHERE v.novel_id = $1 AND v.is_deleted = 0::smallint
		GROUP BY v.id
		ORDER BY v.sort_order ASC, v.id ASC
	`, novelID)
	if err != nil {
		return nil, wrapDBError("查询卷列表失败", err)
	}
	defer rows.Close()

	volumes := make([]model.Volume, 0)
	for rows.Next() {
		var volume model.Volume
		if err := rows.Scan(
			&volume.ID,
			&volume.NovelID,
			&volume.PlanData,
			&volume.SortOrder,
			&volume.Status,
			&volume.WordCount,
			&volume.IsDeleted,
			&volume.ChapterCount,
			&volume.CreatedAt,
			&volume.UpdatedAt,
		); err != nil {
			return nil, wrapDBError("扫描卷列表失败", err)
		}
		volumes = append(volumes, volume)
	}
	return volumes, wrapDBError("遍历卷列表失败", rows.Err())
}

// FindByID 根据 ID 查询卷。
func (r *volumeRepository) FindByID(ctx context.Context, id int64) (model.Volume, error) {
	var volume model.Volume
	err := r.db.QueryRowContext(ctx, `
		SELECT v.id, v.novel_id, v.plan_data, v.sort_order, v.status, v.word_count, v.is_deleted,
			COUNT(c.id)::int AS chapter_count, v.created_at, v.updated_at
		FROM t_volumes v
		LEFT JOIN t_chapters c ON c.volume_id = v.id AND c.is_deleted = 0::smallint
		WHERE v.id = $1 AND v.is_deleted = 0::smallint
		GROUP BY v.id
	`, id).Scan(
		&volume.ID,
		&volume.NovelID,
		&volume.PlanData,
		&volume.SortOrder,
		&volume.Status,
		&volume.WordCount,
		&volume.IsDeleted,
		&volume.ChapterCount,
		&volume.CreatedAt,
		&volume.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Volume{}, ErrVolumeNotFound
	}
	return volume, wrapDBError("按 ID 查询卷失败", err)
}

// CreateManyByNovelID 按 sort_order 批量保存卷列表。
func (r *volumeRepository) CreateManyByNovelID(ctx context.Context, novelID int64, volumes []model.Volume) ([]model.Volume, error) {
	if len(volumes) == 0 {
		return []model.Volume{}, nil
	}

	payload := make([]map[string]any, 0, len(volumes))
	for _, volume := range volumes {
		planDataJSON, err := json.Marshal(volume.PlanData)
		if err != nil {
			return nil, wrapDBError("序列化卷规划数据失败", err)
		}
		payload = append(payload, map[string]any{
			"plan_data":  json.RawMessage(planDataJSON),
			"sort_order": volume.SortOrder,
			"status":     volume.Status,
			"word_count": volume.WordCount,
		})
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, wrapDBError("序列化卷批量保存数据失败", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		INSERT INTO t_volumes (novel_id, plan_data, sort_order, status, word_count)
		SELECT $1, item.plan_data, item.sort_order, item.status, item.word_count
		FROM jsonb_to_recordset($2::jsonb) AS item(
			plan_data jsonb,
			sort_order integer,
			status smallint,
			word_count integer
		)
		ORDER BY item.sort_order ASC
		RETURNING id, novel_id, plan_data, sort_order, status, word_count, is_deleted, created_at, updated_at
	`, novelID, string(payloadJSON))
	if err != nil {
		return nil, wrapDBError("批量保存卷规划失败", err)
	}
	defer rows.Close()

	saved := make([]model.Volume, 0, len(volumes))
	for rows.Next() {
		var volume model.Volume
		if err := rows.Scan(
			&volume.ID,
			&volume.NovelID,
			&volume.PlanData,
			&volume.SortOrder,
			&volume.Status,
			&volume.WordCount,
			&volume.IsDeleted,
			&volume.CreatedAt,
			&volume.UpdatedAt,
		); err != nil {
			return nil, wrapDBError("扫描批量保存卷规划失败", err)
		}
		saved = append(saved, volume)
	}
	return saved, wrapDBError("遍历批量保存卷规划失败", rows.Err())
}

// DeleteByIDs 按卷 ID 批量删除卷。
func (r *volumeRepository) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	payload, err := idPayloadJSON(ids)
	if err != nil {
		return wrapDBError("序列化卷 ID 失败", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE t_volumes v
		SET is_deleted = 1::smallint, updated_at = now()
		FROM jsonb_to_recordset($1::jsonb) AS item(id bigint)
		WHERE v.id = item.id AND v.is_deleted = 0::smallint
	`, payload); err != nil {
		return wrapDBError("删除旧卷规划失败", err)
	}
	return nil
}
