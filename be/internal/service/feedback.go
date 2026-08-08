package service

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"ai-novel-ide/be/internal/config"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/google/uuid"
)

// FeedbackService 用户反馈服务接口，负责校验并保存反馈内容。
type FeedbackService interface {
	Create(ctx context.Context, userID int64, content string, imageURLs []string) error
	CreateImageUploadToken(ctx context.Context, userID int64, filename string, contentType string, size int64) (model.FileUploadToken, error)
}

type feedbackService struct {
	repositories repo.Repositories
	files        FileService
	uploadPrefix string
}

// NewFeedbackService 创建用户反馈服务。
func NewFeedbackService(repositories repo.Repositories, files FileService, cfg config.StorageConfig) FeedbackService {
	return &feedbackService{
		repositories: repositories,
		files:        files,
		uploadPrefix: strings.Trim(strings.TrimSpace(cfg.S3.Prefix), "/"),
	}
}

// Create 校验反馈内容非空后写入数据库，供后续管理后台拉取处理。
func (s *feedbackService) Create(ctx context.Context, userID int64, content string, imageURLs []string) error {
	content = strings.TrimSpace(content)
	if userID <= 0 {
		return ErrInvalidToken
	}
	if content == "" {
		return ErrInvalidFeedback
	}
	return wrapError("保存用户反馈失败", s.repositories.Feedbacks.Create(ctx, userID, content, cleanImageURLs(imageURLs)))
}

// CreateImageUploadToken 校验反馈图片信息，并委托文件服务生成对象存储直传凭证。
func (s *feedbackService) CreateImageUploadToken(ctx context.Context, userID int64, filename string, contentType string, size int64) (model.FileUploadToken, error) {
	if userID <= 0 {
		return model.FileUploadToken{}, ErrInvalidToken
	}
	if !isAllowedFeedbackImage(contentType) || size <= 0 || size > 5*1024*1024 { // 5MB
		return model.FileUploadToken{}, ErrInvalidFile
	}
	if s.files == nil {
		return model.FileUploadToken{}, ErrFileStorageUnavailable
	}
	token, err := s.files.CreateToken(ctx, s.feedbackImageKey(userID, filename, contentType), contentType, 5*time.Minute)
	return token, wrapError("创建反馈图片上传凭证失败", err)
}

// feedbackImageKey 生成反馈图片对象 key。
func (s *feedbackService) feedbackImageKey(userID int64, filename string, contentType string) string {
	ext := strings.ToLower(path.Ext(filename))
	if ext == "" {
		if contentType == "image/png" {
			ext = ".png"
		} else {
			ext = ".jpg"
		}
	}
	return path.Join(s.uploadPrefix, "feedback", fmt.Sprintf("user-%d", userID), time.Now().UTC().Format("20060102"), uuid.NewString()+ext)
}

// isAllowedFeedbackImage 判断反馈图片 MIME 类型是否允许上传。
func isAllowedFeedbackImage(contentType string) bool {
	return contentType == "image/png" || contentType == "image/jpeg"
}

// cleanImageURLs 清理反馈图片链接中的空白项，避免保存无效空链接。
func cleanImageURLs(imageURLs []string) []string {
	cleaned := make([]string, 0, len(imageURLs))
	for _, item := range imageURLs {
		value := strings.TrimSpace(item)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}
