package model

import "time"

// UserFeedback 用户反馈记录，保存用户提交的问题或建议。
type UserFeedback struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Content   string    `json:"content"`
	ImageURLs []string  `json:"imageUrls"`
	Status    int16     `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
