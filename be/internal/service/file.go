package service

import (
	"context"
	"time"

	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/storage"
)

// FileService 文件服务接口，负责生成用户文件直传凭证。
type FileService interface {
	CreateToken(ctx context.Context, key string, contentType string, ttl time.Duration) (model.FileUploadToken, error)
}

type fileService struct {
	storage storage.Client
}

// NewFileService 创建文件服务。
func NewFileService(storageClient storage.Client) FileService {
	return &fileService{storage: storageClient}
}

// CreateToken 按业务方传入的对象 key 和内容类型生成直传凭证。
func (s *fileService) CreateToken(ctx context.Context, key string, contentType string, ttl time.Duration) (model.FileUploadToken, error) {
	if s.storage == nil {
		return model.FileUploadToken{}, ErrFileStorageUnavailable
	}
	token, err := s.storage.PresignPut(ctx, key, contentType, ttl)
	if err != nil {
		return model.FileUploadToken{}, wrapError("生成文件上传凭证失败", err)
	}
	return model.FileUploadToken{
		Key:       token.Key,
		URL:       token.URL,
		UploadURL: token.UploadURL,
		Method:    token.Method,
		Headers:   token.Headers,
		ExpiresAt: token.ExpiresAt,
	}, nil
}
