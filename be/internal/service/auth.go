package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ai-novel-ide/be/internal/config"
	"ai-novel-ide/be/internal/mail"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// AuthService 认证服务接口
type AuthService interface {
	Register(ctx context.Context, req model.AuthRequest) (model.AuthResponse, error)
	Login(ctx context.Context, req model.AuthRequest) (model.AuthResponse, error)
	Refresh(ctx context.Context, refreshToken string) (model.AuthResponse, error)
	SendVerificationCode(ctx context.Context, email string) error
	TurnstileConfig() model.TurnstileConfigResponse
	Me(ctx context.Context, token string) (model.User, error)
	Logout(ctx context.Context, token string) error
	VerifyPassword(ctx context.Context, userID int64, password string) error
	ChangePassword(ctx context.Context, userID int64, newPassword string) error
	DeleteAccount(ctx context.Context, userID int64) error
}

type authService struct {
	repositories repo.Repositories
	redisClient  *redis.Client
	authSecret   string
	turnstile    config.TurnstileConfig
	mailSender   mail.Sender
}

// tokenClaims 是项目登录 JWT 的载荷。
type tokenClaims struct {
	UserID    int64  `json:"userId"`
	Username  string `json:"username"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

const (
	authTokenTypeAccess  = "access"
	authTokenTypeRefresh = "refresh"
	authAccessTokenTTL   = 15 * time.Minute
	authRefreshTokenTTL  = 30 * 24 * time.Hour
)

// NewAuthService 创建认证服务
func NewAuthService(repositories repo.Repositories, redisClient *redis.Client, authConfig config.AuthConfig, mailSender mail.Sender) AuthService {
	return &authService{
		repositories: repositories,
		redisClient:  redisClient,
		authSecret:   authConfig.Secret,
		turnstile:    authConfig.Turnstile,
		mailSender:   mailSender,
	}
}

// Register 用户注册
func (s *authService) Register(ctx context.Context, req model.AuthRequest) (model.AuthResponse, error) {
	username := normalizeUsername(req.Username)
	email := normalizeEmail(req.Email)
	if username == "" || len(username) > 64 || email == "" || len(req.Password) < 6 {
		return model.AuthResponse{}, ErrInvalidCredentials
	}
	if !s.verifyTurnstile(ctx, req.TurnstileToken, req.RemoteIP) {
		return model.AuthResponse{}, ErrTurnstileInvalid
	}
	if !s.verifyEmailCode(ctx, email, req.Code) {
		return model.AuthResponse{}, ErrVerificationCode
	}

	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return model.AuthResponse{}, wrapError("生成密码哈希失败", err)
	}

	var user model.User
	err = s.repositories.Transactions.WithinTx(ctx, func(repos repo.Repositories) error {
		var txErr error
		user, txErr = repos.Users.Create(ctx, username, email, passwordHash)
		if txErr != nil {
			return txErr
		}
		return repos.Settings.Upsert(ctx, user.ID, json.RawMessage(`{}`))
	})
	if errors.Is(err, repo.ErrEmailExists) {
		return model.AuthResponse{}, ErrEmailTaken
	}
	if errors.Is(err, repo.ErrUserExists) {
		return model.AuthResponse{}, ErrUsernameTaken
	}
	if err != nil {
		return model.AuthResponse{}, wrapError("创建用户失败", err)
	}
	s.deleteEmailCode(ctx, email)

	return s.authResponse(user)
}

// Login 用户登录
func (s *authService) Login(ctx context.Context, req model.AuthRequest) (model.AuthResponse, error) {
	if !s.verifyTurnstile(ctx, req.TurnstileToken, req.RemoteIP) {
		return model.AuthResponse{}, ErrTurnstileInvalid
	}
	if req.Mode == "code" {
		return s.loginByEmailCode(ctx, req)
	}
	user, err := s.loginByPassword(ctx, req)
	if errors.Is(err, repo.ErrUserNotFound) {
		return model.AuthResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return model.AuthResponse{}, wrapError("查询登录用户失败", err)
	}
	if !verifyUserPassword(user, req.Password) {
		return model.AuthResponse{}, ErrInvalidCredentials
	}

	return s.authResponse(user)
}

// Refresh 使用刷新令牌签发新的访问令牌和刷新令牌。
func (s *authService) Refresh(ctx context.Context, refreshToken string) (model.AuthResponse, error) {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return model.AuthResponse{}, wrapError("解析刷新令牌失败", err)
	}
	if claims.TokenType != authTokenTypeRefresh || s.isTokenBlacklisted(ctx, claims.ID) {
		return model.AuthResponse{}, ErrInvalidToken
	}
	user, err := s.repositories.Users.FindByID(ctx, claims.UserID)
	if errors.Is(err, repo.ErrUserNotFound) {
		return model.AuthResponse{}, ErrInvalidToken
	}
	if err != nil {
		return model.AuthResponse{}, wrapError("查询刷新令牌用户失败", err)
	}
	// 暂停刷新令牌轮换黑名单，避免每次无感刷新都生成一个长期 blacklist key。
	// _ = s.blacklistToken(ctx, claims)
	return s.authResponse(user)
}

// loginByPassword 按用户名或邮箱读取用户，由调用方继续校验密码。
func (s *authService) loginByPassword(ctx context.Context, req model.AuthRequest) (model.User, error) {
	identifier := strings.TrimSpace(req.Username)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Email)
	}
	if strings.Contains(identifier, "@") {
		return s.repositories.Users.FindByEmail(ctx, identifier)
	}
	return s.repositories.Users.FindByUsername(ctx, normalizeUsername(identifier))
}

// loginByEmailCode 校验邮箱验证码并完成验证码登录。
func (s *authService) loginByEmailCode(ctx context.Context, req model.AuthRequest) (model.AuthResponse, error) {
	email := normalizeEmail(req.Email)
	if email == "" || !s.verifyEmailCode(ctx, email, req.Code) {
		return model.AuthResponse{}, ErrVerificationCode
	}
	user, err := s.repositories.Users.FindByEmail(ctx, email)
	if errors.Is(err, repo.ErrUserNotFound) {
		return model.AuthResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return model.AuthResponse{}, wrapError("查询登录用户失败", err)
	}
	s.deleteEmailCode(ctx, email)
	return s.authResponse(user)
}

// Me 根据 token 获取当前用户信息
func (s *authService) Me(ctx context.Context, token string) (model.User, error) {
	claims, err := s.parseToken(token)
	if err != nil {
		return model.User{}, wrapError("解析登录令牌失败", err)
	}
	if claims.TokenType != authTokenTypeAccess || s.isTokenBlacklisted(ctx, claims.ID) {
		return model.User{}, ErrInvalidToken
	}
	user, err := s.repositories.Users.FindByID(ctx, claims.UserID)
	if errors.Is(err, repo.ErrUserNotFound) {
		return model.User{}, ErrInvalidToken
	}
	if err != nil {
		return model.User{}, wrapError("查询当前用户失败", err)
	}
	return user, nil
}

// Logout 将当前 JWT 加入 Redis 黑名单。
func (s *authService) Logout(ctx context.Context, token string) error {
	claims, err := s.parseToken(token)
	if err != nil {
		return wrapError("解析登出令牌失败", err)
	}
	return s.blacklistToken(ctx, claims)
}

// blacklistToken 将指定令牌写入 Redis 黑名单直到原过期时间。
func (s *authService) blacklistToken(ctx context.Context, claims tokenClaims) error {
	if s.redisClient == nil || claims.ExpiresAt == nil {
		return nil
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return nil
	}
	return wrapError("写入登出黑名单失败", s.redisClient.Set(ctx, authBlacklistKey(claims.ID), "1", ttl).Err())
}

// SendVerificationCode 发送邮箱验证码。
func (s *authService) SendVerificationCode(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidCredentials
	}
	code, err := generateVerificationCode()
	if err != nil {
		return wrapError("生成验证码失败", err)
	}
	if err := s.redisClient.Set(ctx, emailCodeKey(email), code, 60*time.Second).Err(); err != nil {
		return wrapError("存储验证码失败", err)
	}
	textBody, htmlBody := verificationCodeEmail(code)
	if err := s.mailSender.Send(email, "AI Novel 验证码", textBody, htmlBody); err != nil {
		return wrapError("发送验证码邮件失败", err)
	}
	return nil
}

// verificationCodeEmail 生成验证码邮件正文，HTML 用于主流邮箱展示，纯文本用于不支持 HTML 的客户端。
func verificationCodeEmail(code string) (string, string) {
	textBody := fmt.Sprintf("您的 AI Novel 验证码是：%s。验证码有效期 1 分钟，请勿泄露给他人。", code)
	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AI Novel 验证码</title>
  </head>
  <body style="margin:0;background:#f3f4f6;padding:32px 16px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Microsoft YaHei',Arial,sans-serif;color:#111827;">
    <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="border-collapse:collapse;">
      <tr>
        <td align="center">
          <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="max-width:520px;border-collapse:collapse;border-radius:20px;overflow:hidden;background:#ffffff;box-shadow:0 18px 50px rgba(15,23,42,0.12);">
            <tr>
              <td style="background:#111827;padding:28px 32px;color:#ffffff;">
                <div style="font-size:14px;letter-spacing:0.12em;text-transform:uppercase;color:#a7f3d0;">AI Novel</div>
                <h1 style="margin:10px 0 0;font-size:24px;line-height:1.35;font-weight:700;">邮箱验证码</h1>
              </td>
            </tr>
            <tr>
              <td style="padding:32px;">
                <p style="margin:0 0 18px;font-size:16px;line-height:1.7;color:#374151;">你正在验证 AI Novel 账号邮箱，请在页面中输入下面的验证码：</p>
                <div style="margin:24px 0;padding:20px;border-radius:16px;background:#f9fafb;border:1px solid #e5e7eb;text-align:center;">
                  <div style="font-size:36px;line-height:1;letter-spacing:0.22em;font-weight:800;color:#111827;">%s</div>
                </div>
                <p style="margin:0;font-size:14px;line-height:1.7;color:#6b7280;">验证码有效期为 <strong style="color:#111827;">1 分钟</strong>。如果这不是你的操作，可以忽略这封邮件。</p>
              </td>
            </tr>
            <tr>
              <td style="padding:18px 32px;background:#f9fafb;border-top:1px solid #e5e7eb;color:#9ca3af;font-size:12px;line-height:1.6;">
                为了账号安全，请不要把验证码转发或告知他人。
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`, code)
	return textBody, htmlBody
}

// verifyEmailCode 校验邮箱验证码，注册时邮箱和验证码必须匹配。
func (s *authService) verifyEmailCode(ctx context.Context, email string, code string) bool {
	if s.redisClient == nil {
		return false
	}
	expected, err := s.redisClient.Get(ctx, emailCodeKey(email)).Result()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(code)), []byte(expected)) == 1
}

// deleteEmailCode 删除已使用的邮箱验证码，避免同一验证码重复注册。
func (s *authService) deleteEmailCode(ctx context.Context, email string) {
	if s.redisClient == nil {
		return
	}
	_ = s.redisClient.Del(ctx, emailCodeKey(email)).Err()
}

func (s *authService) TurnstileConfig() model.TurnstileConfigResponse {
	siteKey := strings.TrimSpace(s.turnstile.SiteKey)
	return model.TurnstileConfigResponse{SiteKey: siteKey, Enabled: siteKey != "" && strings.TrimSpace(s.turnstile.SecretKey) != ""}
}

// VerifyPassword 查询用户并校验当前密码，供 handler 在敏感操作前统一确认身份。
func (s *authService) VerifyPassword(ctx context.Context, userID int64, password string) error {
	if userID <= 0 {
		return ErrInvalidToken
	}
	user, err := s.repositories.Users.FindByID(ctx, userID)
	if err != nil {
		return wrapError("查询密码校验用户失败", err)
	}
	if !verifyUserPassword(user, password) {
		return ErrInvalidCredentials
	}
	return nil
}

// ChangePassword 校验新密码格式并更新当前用户密码。
func (s *authService) ChangePassword(ctx context.Context, userID int64, newPassword string) error {
	if userID <= 0 {
		return ErrInvalidToken
	}
	if len(newPassword) < 6 {
		return ErrInvalidCredentials
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return wrapError("生成密码哈希失败", err)
	}
	if _, err := s.repositories.Users.FindByID(ctx, userID); err != nil {
		return wrapError("查询密码更新用户失败", err)
	}
	if err := s.repositories.Users.Update(ctx, userID, repo.UpdateFields{
		"password_hash": passwordHash,
	}); err != nil {
		return wrapError("更新密码失败", err)
	}
	return nil
}

// DeleteAccount 将当前用户标记为注销状态；真实数据清理由后续冷静期任务处理。
func (s *authService) DeleteAccount(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrInvalidToken
	}
	err := s.repositories.Transactions.WithinTx(ctx, func(repos repo.Repositories) error {
		if _, err := repos.Users.FindByID(ctx, userID); err != nil {
			return err
		}
		now := time.Now()
		return repos.Users.Update(ctx, userID, repo.UpdateFields{
			"status":         model.UserStatusDeactivateCooling,
			"deactivated_at": &now,
		})
	})
	return wrapError("注销用户失败", err)
}

// verifyTurnstile 校验 Cloudflare Turnstile token；配置不完整时跳过，便于本地开发。
func (s *authService) verifyTurnstile(ctx context.Context, token string, remoteIP string) bool {
	secret := strings.TrimSpace(s.turnstile.SecretKey)
	if secret == "" || strings.TrimSpace(s.turnstile.SiteKey) == "" {
		return true
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	return result.Success
}

// authResponse 生成 token 并组装响应
func (s *authService) authResponse(user model.User) (model.AuthResponse, error) {
	token, err := s.signToken(user, authTokenTypeAccess, authAccessTokenTTL)
	if err != nil {
		return model.AuthResponse{}, wrapError("签发登录令牌失败", err)
	}
	refreshToken, err := s.signToken(user, authTokenTypeRefresh, authRefreshTokenTTL)
	if err != nil {
		return model.AuthResponse{}, wrapError("签发刷新令牌失败", err)
	}
	user.PasswordHash = ""
	return model.AuthResponse{Token: token, RefreshToken: refreshToken, User: user}, nil
}

// generateVerificationCode 生成6位数字验证码。
func generateVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// normalizeUsername 去除用户名前后空白
func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

// normalizeEmail 统一清理邮箱输入，邮箱登录和注册按小写地址处理。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailCodeKey 生成邮箱验证码缓存键。
func emailCodeKey(email string) string {
	return fmt.Sprintf("email:code:%s", email)
}

// hashPassword 使用 PBKDF2-SHA256 哈希密码
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	iterations := 120000
	hash := pbkdf2SHA256([]byte(password), salt, iterations, 32)
	return fmt.Sprintf("pbkdf2$%d$%s$%s", iterations, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(hash)), nil
}

// verifyPassword 校验密码
func verifyPassword(password string, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// verifyUserPassword 使用用户密码哈希校验明文密码，统一认证服务中的密码比对入口。
func verifyUserPassword(user model.User, password string) bool {
	return verifyPassword(password, user.PasswordHash)
}

// pbkdf2SHA256 手动实现 PBKDF2-SHA256
func pbkdf2SHA256(password []byte, salt []byte, iterations int, keyLen int) []byte {
	hashLen := sha256.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	output := make([]byte, 0, numBlocks*hashLen)

	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)

		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		output = append(output, t...)
	}

	return output[:keyLen]
}

// signToken 签发 token
func (s *authService) signToken(user model.User, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		UserID:    user.ID,
		Username:  user.Username,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        randomTokenID(),
			Subject:   strconv.FormatInt(user.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.authSecret))
}

// parseToken 解析并校验 token
func (s *authService) parseToken(rawToken string) (tokenClaims, error) {
	var claims tokenClaims
	token, err := jwt.ParseWithClaims(rawToken, &claims, func(token *jwt.Token) (any, error) {
		return []byte(s.authSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid {
		return tokenClaims{}, ErrInvalidToken
	}
	if claims.ID == "" || claims.ExpiresAt == nil {
		return tokenClaims{}, ErrInvalidToken
	}
	return claims, nil
}

// isTokenBlacklisted 检查 JWT 是否已被登出加入黑名单。
func (s *authService) isTokenBlacklisted(ctx context.Context, jti string) bool {
	if s.redisClient == nil || jti == "" {
		return false
	}
	err := s.redisClient.Get(ctx, authBlacklistKey(jti)).Err()
	return err == nil
}

// authBlacklistKey 生成 Redis 黑名单键。
func authBlacklistKey(jti string) string {
	return "auth:blacklist:" + jti
}

// randomTokenID 生成 JWT ID。
func randomTokenID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
