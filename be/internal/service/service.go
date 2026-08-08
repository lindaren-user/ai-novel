package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/config"
	"ai-novel-ide/be/internal/mail"
	"ai-novel-ide/be/internal/repo"
	"ai-novel-ide/be/internal/storage"

	"github.com/redis/go-redis/v9"
)

// ErrInvalidCredentials 用户名/邮箱或密码无效
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrUsernameTaken 用户名已被占用
var ErrUsernameTaken = errors.New("username already taken")

// ErrEmailTaken 邮箱已被注册
var ErrEmailTaken = errors.New("email already taken")

// ErrVerificationCode 验证码错误
var ErrVerificationCode = errors.New("verification code invalid")

// ErrTurnstileInvalid 人机验证失败
var ErrTurnstileInvalid = errors.New("turnstile invalid")

// ErrCodeRateLimit 验证码发送频率限制
var ErrCodeRateLimit = errors.New("code rate limit exceeded")

// ErrMailDisabled 邮件服务未配置
var ErrMailDisabled = errors.New("mail service not configured")

// ErrInvalidToken 无效或过期的令牌
var ErrInvalidToken = errors.New("invalid token")

// ErrResourceNotFound 资源不存在
var ErrResourceNotFound = errors.New("resource not found")

// ErrForbidden 无权访问资源
var ErrForbidden = errors.New("forbidden")

// ErrInvalidSettings 设置格式不正确
var ErrInvalidSettings = errors.New("invalid settings")

// ErrInvalidModel 模型配置不正确
var ErrInvalidModel = errors.New("invalid model")

// ErrModelTaken 模型名称已存在
var ErrModelTaken = errors.New("model already taken")

// ErrInvalidMessage 消息内容不正确
var ErrInvalidMessage = errors.New("invalid message")

// ErrPlanOverwriteRequired 已存在规划，覆盖前需要用户确认
var ErrPlanOverwriteRequired = errors.New("plan overwrite confirmation required")

// ErrAIUnavailable AI 客户端不可用
var ErrAIUnavailable = errors.New("ai unavailable")

// ErrConcurrentStreamLimitExceeded AI 流式任务并发数超过会员权益限制
var ErrConcurrentStreamLimitExceeded = errors.New("concurrent stream limit exceeded")

// ErrServiceShuttingDown 服务正在关闭，不再接受新的后台任务
var ErrServiceShuttingDown = errors.New("service shutting down")

// ErrSharePasswordRequired 分享访问需要密钥
var ErrSharePasswordRequired = errors.New("share password required")

// ErrInvalidSharePassword 分享访问密钥不正确
var ErrInvalidSharePassword = errors.New("invalid share password")

// ErrInvalidFeedback 用户反馈内容不正确
var ErrInvalidFeedback = errors.New("invalid feedback")

// ErrInvalidFile 上传文件不正确
var ErrInvalidFile = errors.New("invalid file")

// ErrFileStorageUnavailable 文件存储不可用
var ErrFileStorageUnavailable = errors.New("file storage unavailable")

// wrapError 为下层错误补充业务动作上下文；调用边界统一记录日志时可直接看到失败阶段。
func wrapError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

// Services 聚合所有 service 实例
type Services struct {
	Health       HealthService
	Auth         AuthService
	Settings     SettingsService
	Novels       NovelService
	Volumes      VolumeService
	Chapters     ChapterService
	ModelConfigs ModelConfigService
	Shares       ShareService
	Downloads    DownloadService
	Feedbacks    FeedbackService
	Files        FileService
}

// NewServices 创建所有 service 的工厂方法
func NewServices(repositories repo.Repositories, redisClient *redis.Client, aiClient ai.Client, authConfig config.AuthConfig, mailSender mail.Sender, storageClient storage.Client, storageConfig config.StorageConfig) Services {
	files := NewFileService(storageClient)
	return Services{
		Health:       NewHealthService(repositories, aiClient),
		Auth:         NewAuthService(repositories, redisClient, authConfig, mailSender),
		Settings:     NewSettingsService(repositories, redisClient),
		Novels:       NewNovelService(repositories, redisClient, aiClient),
		Volumes:      NewVolumeService(repositories, redisClient, aiClient),
		Chapters:     NewChapterService(repositories, redisClient, aiClient),
		ModelConfigs: NewModelConfigService(repositories, redisClient, aiClient),
		Shares:       NewShareService(repositories, redisClient, authConfig.Secret),
		Downloads:    NewDownloadService(repositories),
		Files:        files,
		Feedbacks:    NewFeedbackService(repositories, files, storageConfig),
	}
}

// BeginShutdown 先标记服务进入关闭状态，让正在运行的普通 AI 请求能把 run 写成 server_shutdown。
func (s Services) BeginShutdown() {
	for _, item := range []any{s.Novels, s.Volumes, s.Chapters} {
		marker, ok := item.(interface {
			beginShutdown()
		})
		if ok {
			marker.beginShutdown()
		}
	}
}

// Shutdown 通知带后台任务的服务进入关闭流程，并等待它们在 ctx 超时前收口。
func (s Services) Shutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	// 这几个 service 里“实现了 Shutdown 方法的对象”找出来，然后统一关闭它们。
	for _, item := range []any{s.Novels, s.Volumes, s.Chapters} {
		shutdowner, ok := item.(interface {
			Shutdown(context.Context) error
		})
		if !ok {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := shutdowner.Shutdown(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		return err
	}
	return ctx.Err()
}
