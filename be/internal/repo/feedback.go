package repo

import (
	"context"
	"encoding/json"
)

// FeedbackRepository 用户反馈数据访问接口，负责写入帮助与反馈内容。
type FeedbackRepository interface {
	Create(ctx context.Context, userID int64, content string, imageURLs []string) error
}

type feedbackRepository struct {
	db DBTX
}

// NewFeedbackRepository 创建用户反馈 repo。
func NewFeedbackRepository(db DBTX) FeedbackRepository {
	return &feedbackRepository{db: db}
}

// Create 保存用户反馈内容，默认状态为未处理。
func (r *feedbackRepository) Create(ctx context.Context, userID int64, content string, imageURLs []string) error {
	imageData, err := json.Marshal(imageURLs)
	if err != nil {
		return wrapDBError("序列化反馈图片链接失败", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO t_user_feedbacks (user_id, content, image_urls)
		VALUES ($1, $2, $3)
	`, userID, content, imageData)
	return wrapDBError("创建用户反馈失败", err)
}
