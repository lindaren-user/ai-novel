package model

import "time"

// FileUploadToken 文件直传凭证，返回预签名上传地址和最终访问链接。
type FileUploadToken struct {
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	UploadURL string            `json:"uploadUrl"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}
