package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"ai-novel-ide/be/internal/config"
)

const unsignedPayload = "UNSIGNED-PAYLOAD"

// S3Client 使用 S3 兼容协议生成直传凭证，支持 Cloudflare R2。
type S3Client struct {
	endpoint      string
	region        string
	bucket        string
	accessKey     string
	secretKey     string
	publicBaseURL string
}

// NewS3Client 创建 S3 兼容文件存储客户端。
func NewS3Client(cfg config.S3StorageConfig) *S3Client {
	return &S3Client{
		endpoint:      strings.TrimRight(cfg.Endpoint, "/"),
		region:        cfg.Region,
		bucket:        cfg.Bucket,
		accessKey:     cfg.AccessKey,
		secretKey:     cfg.SecretKey,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}
}

// PresignPut 生成可由前端直传对象存储的 PUT 预签名 URL。
func (c *S3Client) PresignPut(ctx context.Context, key string, contentType string, expires time.Duration) (UploadToken, error) {
	if c.endpoint == "" || c.bucket == "" || c.accessKey == "" || c.secretKey == "" {
		return UploadToken{}, fmt.Errorf("S3 存储配置不完整")
	}
	if err := ctx.Err(); err != nil {
		return UploadToken{}, err
	}
	objectKey := strings.TrimLeft(path.Clean(key), "/")
	endpointURL, err := url.Parse(c.endpoint)
	if err != nil {
		return UploadToken{}, fmt.Errorf("解析 S3 endpoint 失败: %w", err)
	}
	now := time.Now().UTC()
	expiresSeconds := int64(expires.Seconds())
	if expiresSeconds <= 0 || expiresSeconds > 604800 {
		expiresSeconds = 300
	}
	uploadURL := *endpointURL
	uploadURL.Path = path.Join(endpointURL.Path, "/", c.bucket, objectKey)
	query := uploadURL.Query()
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", now.Format("20060102"), c.region)
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", c.accessKey+"/"+scope)
	query.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	query.Set("X-Amz-Expires", strconv.FormatInt(expiresSeconds, 10))
	query.Set("X-Amz-SignedHeaders", "host")
	uploadURL.RawQuery = canonicalQuery(query)
	signature := c.presignSignature(http.MethodPut, uploadURL, scope, now)
	query.Set("X-Amz-Signature", signature)
	uploadURL.RawQuery = canonicalQuery(query)
	return UploadToken{
		Key:       objectKey,
		URL:       c.publicURL(objectKey),
		UploadURL: uploadURL.String(),
		Method:    http.MethodPut,
		Headers:   map[string]string{},
		ExpiresAt: now.Add(time.Duration(expiresSeconds) * time.Second),
	}, nil
}

// presignSignature 计算预签名 URL 的 AWS Signature V4 签名。
func (c *S3Client) presignSignature(method string, uploadURL url.URL, scope string, now time.Time) string {
	canonicalRequest := strings.Join([]string{
		method,
		uploadURL.EscapedPath(),
		uploadURL.RawQuery,
		"host:" + uploadURL.Host + "\n",
		"host",
		unsignedPayload,
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		now.Format("20060102T150405Z"),
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hmacSHA256(signingKey(c.secretKey, now.Format("20060102"), c.region), []byte(stringToSign))
	return hex.EncodeToString(signature)
}

// publicURL 根据 publicBaseURL 拼出对象公开访问链接。
func (c *S3Client) publicURL(key string) string {
	if c.publicBaseURL == "" {
		return c.endpoint + "/" + c.bucket + "/" + key
	}
	return c.publicBaseURL + "/" + strings.TrimLeft(key, "/")
}

// canonicalQuery 生成签名需要的规范化查询串。
func canonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, value := range vals {
			parts = append(parts, awsEscape(key)+"="+awsEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

// awsEscape 使用 AWS 要求的查询转义规则。
func awsEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// signingKey 生成 AWS Signature V4 签名密钥。
func signingKey(secret string, date string, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// hmacSHA256 计算 HMAC-SHA256。
func hmacSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// sha256Hex 计算 SHA256 十六进制字符串。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
