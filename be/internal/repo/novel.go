package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-novel-ide/be/internal/model"
)

// NovelRepository 小说数据访问接口
type NovelRepository interface {
	Create(ctx context.Context, userID int64, title string, planData model.JSONMap, setupOriginalText string, status int16) (model.Novel, error)
	ListByUserID(ctx context.Context, userID int64) ([]model.Novel, error)
	ListArchivedByUserID(ctx context.Context, userID int64) ([]model.Novel, error)
	FindOverviewByUserID(ctx context.Context, userID int64, novelID int64) (model.NovelOverviewItem, error)
	GetDashboard(ctx context.Context, userID int64, status int16) (model.WorkspaceDashboard, error)
	FindByID(ctx context.Context, id int64) (model.Novel, error)
	Update(ctx context.Context, id int64, fields UpdateFields) error
}

type novelRepository struct {
	db DBTX
}

// NewNovelRepository 创建小说 repo
func NewNovelRepository(db DBTX) NovelRepository {
	return &novelRepository{db: db}
}

// Create 新建小说记录，并写入 service 已经整理好的初始规划 JSON 和表单原始描述。
func (r *novelRepository) Create(ctx context.Context, userID int64, title string, planData model.JSONMap, setupOriginalText string, status int16) (model.Novel, error) {
	var novel model.Novel
	if planData == nil {
		planData = model.JSONMap{}
	}
	planDataJSON, err := json.Marshal(planData)
	if err != nil {
		return model.Novel{}, wrapDBError("序列化小说规划数据失败", err)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO t_novels (user_id, title, plan_data, setup_original_text, status)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id, user_id, title, plan_data, setup_original_text, status, word_count, created_at, updated_at
	`, userID, title, string(planDataJSON), setupOriginalText, status).Scan(
		&novel.ID,
		&novel.UserID,
		&novel.Title,
		&novel.PlanData,
		&novel.SetupOriginalText,
		&novel.Status,
		&novel.WordCount,
		&novel.CreatedAt,
		&novel.UpdatedAt,
	)
	return novel, wrapDBError("插入小说失败", err)
}

// ListByUserID 查询用户未归档小说列表，包含设定草稿和正式创作。
func (r *novelRepository) ListByUserID(ctx context.Context, userID int64) ([]model.Novel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, status, word_count, created_at, updated_at
		FROM t_novels
		WHERE user_id = $1
		  AND status <> 3::smallint
		ORDER BY updated_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, wrapDBError("查询小说列表失败", err)
	}
	return scanNovelList(rows)
}

// ListArchivedByUserID 查询用户已归档小说列表。
func (r *novelRepository) ListArchivedByUserID(ctx context.Context, userID int64) ([]model.Novel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, status, word_count, created_at, updated_at
		FROM t_novels
		WHERE user_id = $1
		  AND status = 3::smallint
		ORDER BY updated_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, wrapDBError("查询归档小说列表失败", err)
	}
	return scanNovelList(rows)
}

func scanNovelList(rows *sql.Rows) ([]model.Novel, error) {
	defer rows.Close()
	novels := make([]model.Novel, 0)
	for rows.Next() {
		var novel model.Novel
		if err := rows.Scan(
			&novel.ID,
			&novel.UserID,
			&novel.Title,
			&novel.Status,
			&novel.WordCount,
			&novel.CreatedAt,
			&novel.UpdatedAt,
		); err != nil {
			return nil, wrapDBError("扫描小说列表失败", err)
		}
		novels = append(novels, novel)
	}
	return novels, wrapDBError("遍历小说列表失败", rows.Err())
}

// FindOverviewByUserID 查询当前用户单本小说梗概，供前端按需打开详情。
func (r *novelRepository) FindOverviewByUserID(ctx context.Context, userID int64, novelID int64) (model.NovelOverviewItem, error) {
	var item model.NovelOverviewItem
	err := r.db.QueryRowContext(ctx, `
		SELECT n.id, n.title, n.plan_data,
			CASE WHEN n.status = 1::smallint THEN n.setup_original_text ELSE '' END AS setup_original_text,
			COALESCE(SUM(cc.word_count), 0)::int AS word_count,
			MAX(GREATEST(n.updated_at, COALESCE(cc.updated_at, n.updated_at))) AS updated_at
		FROM t_novels n
		LEFT JOIN t_volumes v ON v.novel_id = n.id AND v.is_deleted = 0::smallint
		LEFT JOIN t_chapters c ON c.volume_id = v.id AND c.is_deleted = 0::smallint
		LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		WHERE n.user_id = $1
		  AND n.id = $2
		GROUP BY n.id, n.title, n.plan_data, n.status, n.setup_original_text
		ORDER BY updated_at DESC, n.id DESC
	`, userID, novelID).Scan(&item.ID, &item.Title, &item.PlanData, &item.SetupOriginalText, &item.WordCount, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.NovelOverviewItem{}, ErrNovelNotFound
	}
	return item, wrapDBError("查询小说梗概失败", err)
}

// GetDashboard 聚合工作台首页数据，只统计当前用户未归档小说。
func (r *novelRepository) GetDashboard(ctx context.Context, userID int64, status int16) (model.WorkspaceDashboard, error) {
	var dashboard model.WorkspaceDashboard
	err := r.db.QueryRowContext(ctx, `
		WITH normal_novels AS (
			SELECT id, title, updated_at
			FROM t_novels
			WHERE user_id = $1 AND status = $2
		),
		volume_stats AS (
			SELECT
				COUNT(*)::int AS volume_count
			FROM t_volumes v
			JOIN normal_novels n ON n.id = v.novel_id
			WHERE v.is_deleted = 0::smallint
		),
		chapter_stats AS (
			SELECT
				COUNT(*)::int AS chapter_count,
				COUNT(cc.id)::int AS completed_chapters,
				COALESCE(SUM(cc.word_count), 0)::int AS total_words
			FROM t_chapters c
			JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
			JOIN normal_novels n ON n.id = v.novel_id
			LEFT JOIN t_chapter_contents cc ON cc.chapter_id = c.id AND cc.status = 2::smallint AND cc.is_deleted = 0::smallint
			WHERE c.is_deleted = 0::smallint
		),
		latest_times AS (
			SELECT updated_at FROM normal_novels
			UNION ALL
			SELECT v.updated_at
			FROM t_volumes v
			JOIN normal_novels n ON n.id = v.novel_id
			WHERE v.is_deleted = 0::smallint
			UNION ALL
			SELECT c.updated_at
			FROM t_chapters c
			JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
			JOIN normal_novels n ON n.id = v.novel_id
			WHERE c.is_deleted = 0::smallint
			UNION ALL
			SELECT cc.updated_at
			FROM t_chapter_contents cc
			JOIN t_chapters c ON c.id = cc.chapter_id AND c.is_deleted = 0::smallint
			JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
			JOIN normal_novels n ON n.id = v.novel_id
			WHERE cc.status = 2::smallint AND cc.is_deleted = 0::smallint
		)
		SELECT
			COALESCE((SELECT total_words FROM chapter_stats), 0)::int,
			COALESCE((SELECT completed_chapters FROM chapter_stats), 0)::int,
			COALESCE((SELECT volume_count FROM volume_stats), 0)::int,
			COALESCE((SELECT MAX(updated_at) FROM latest_times), now())
	`, userID, status).Scan(
		&dashboard.TotalWords,
		&dashboard.CompletedChapters,
		&dashboard.VolumeCount,
		&dashboard.LastEditedAt,
	)
	if err != nil {
		return model.WorkspaceDashboard{}, wrapDBError("查询工作台统计失败", err)
	}

	trend, err := r.dashboardWordTrend(ctx, userID, status)
	if err != nil {
		return model.WorkspaceDashboard{}, err
	}
	dashboard.WordTrend = trend
	return dashboard, nil
}

// dashboardWordTrend 按最近 7 天聚合当前正文的完成字数，用于工作台折线图。
func (r *novelRepository) dashboardWordTrend(ctx context.Context, userID int64, status int16) ([]model.WordTrendPoint, error) {
	shanghaiLocation := time.FixedZone("Asia/Shanghai", 8*60*60)
	endDate := time.Now().In(shanghaiLocation)
	startDate := dateOnly(endDate).AddDate(0, 0, -6)
	rows, err := r.db.QueryContext(ctx, `
		SELECT (cc.updated_at AT TIME ZONE 'Asia/Shanghai')::date AS writing_date,
			COALESCE(SUM(cc.word_count), 0)::int AS words
		FROM t_chapter_contents cc
		JOIN t_chapters c ON c.id = cc.chapter_id AND c.is_deleted = 0::smallint
		JOIN t_volumes v ON v.id = c.volume_id AND v.is_deleted = 0::smallint
		JOIN t_novels n ON n.id = v.novel_id
		WHERE n.user_id = $1
		  AND n.status = $2
		  AND cc.status = 2::smallint
		  AND cc.is_deleted = 0::smallint
		  AND (cc.updated_at AT TIME ZONE 'Asia/Shanghai')::date >= $3::date
		  AND (cc.updated_at AT TIME ZONE 'Asia/Shanghai')::date < ($3::date + INTERVAL '7 days')
		GROUP BY writing_date
	`, userID, status, startDate.Format("2006-01-02"))
	if err != nil {
		return nil, wrapDBError("查询工作台字数趋势失败", err)
	}
	defer rows.Close()

	wordsByDate := map[string]int{}
	for rows.Next() {
		var (
			date  time.Time
			words int
		)
		if err := rows.Scan(&date, &words); err != nil {
			return nil, wrapDBError("扫描工作台字数趋势失败", err)
		}
		wordsByDate[date.Format("2006-01-02")] = words
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDBError("遍历工作台字数趋势失败", err)
	}

	trend := make([]model.WordTrendPoint, 0, 7)
	for i := 0; i < 7; i++ {
		day := startDate.AddDate(0, 0, i)
		date := day.Format("2006-01-02")
		words := wordsByDate[date]
		trend = append(trend, model.WordTrendPoint{
			Date:      date,
			Weekday:   chineseWeekday(day.Weekday()),
			Words:     words,
			WordLabel: fmt.Sprintf("%s 字", formatIntWithComma(words)),
		})
	}
	return trend, nil
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func chineseWeekday(weekday time.Weekday) string {
	switch weekday {
	case time.Monday:
		return "一"
	case time.Tuesday:
		return "二"
	case time.Wednesday:
		return "三"
	case time.Thursday:
		return "四"
	case time.Friday:
		return "五"
	case time.Saturday:
		return "六"
	default:
		return "日"
	}
}

func formatIntWithComma(value int) string {
	raw := fmt.Sprintf("%d", value)
	if len(raw) <= 3 {
		return raw
	}
	result := make([]byte, 0, len(raw)+len(raw)/3)
	prefix := len(raw) % 3
	if prefix == 0 {
		prefix = 3
	}
	result = append(result, raw[:prefix]...)
	for i := prefix; i < len(raw); i += 3 {
		result = append(result, ',')
		result = append(result, raw[i:i+3]...)
	}
	return string(result)
}

// FindByID 根据 ID 查询小说。
func (r *novelRepository) FindByID(ctx context.Context, id int64) (model.Novel, error) {
	var novel model.Novel
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, plan_data, setup_original_text, status, word_count, created_at, updated_at
		FROM t_novels
		WHERE id = $1
	`, id).Scan(
		&novel.ID,
		&novel.UserID,
		&novel.Title,
		&novel.PlanData,
		&novel.SetupOriginalText,
		&novel.Status,
		&novel.WordCount,
		&novel.CreatedAt,
		&novel.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Novel{}, ErrNovelNotFound
	}
	return novel, wrapDBError("按 ID 查询小说失败", err)
}

// Update 按 ID 更新小说白名单字段，不包含业务判断。
func (r *novelRepository) Update(ctx context.Context, id int64, fields UpdateFields) error {
	return execUpdateFields(ctx, r.db, "t_novels", id, fields, novelUpdateFields, ErrNovelNotFound, "更新小说失败")
}

var novelUpdateFields = map[string]updateFieldSpec{
	"user_id":             columnUpdateField("user_id"),
	"title":               columnUpdateField("title"),
	"plan_data":           jsonbUpdateField("plan_data"),
	"setup_original_text": columnUpdateField("setup_original_text"),
	"status":              smallintUpdateField("status"),
	"word_count":          columnUpdateField("word_count"),
}
