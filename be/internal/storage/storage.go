package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ai-novel-ide/be/internal/config"
)

// UploadToken 文件直传凭证，返回预签名上传地址和最终访问链接。
type UploadToken struct {
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	UploadURL string            `json:"uploadUrl"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

type Client interface {
	PresignPut(ctx context.Context, key string, contentType string, expires time.Duration) (UploadToken, error)
}

func NewClient(cfg config.StorageConfig) (Client, error) {
	if strings.TrimSpace(cfg.S3.Endpoint) == "" {
		return nil, fmt.Errorf("storage.s3.endpoint 不能为空")
	}
	if strings.TrimSpace(cfg.S3.Bucket) == "" {
		return nil, fmt.Errorf("storage.s3.bucket 不能为空")
	}
	if strings.TrimSpace(cfg.S3.AccessKey) == "" {
		return nil, fmt.Errorf("storage.s3.accessKey 不能为空")
	}
	if strings.TrimSpace(cfg.S3.SecretKey) == "" {
		return nil, fmt.Errorf("storage.s3.secretKey 不能为空")
	}
	return NewS3Client(cfg.S3), nil
}
