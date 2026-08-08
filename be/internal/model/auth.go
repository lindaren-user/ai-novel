package model

import (
	"time"
)

const (
	UserStatusNormal            int16 = 1 // 用户状态正常，可登录和使用系统。
	UserStatusDisabled          int16 = 2 // 用户被禁用，不允许登录和使用系统。
	UserStatusDeactivateCooling int16 = 3 // 用户进入注销冷静期，等待后续清理任务处理。
)

type User struct {
	ID               int64      `json:"id"`
	Username         string     `json:"username"`
	Email            string     `json:"email"`
	MembershipPlanID int64      `json:"-"`
	Status           int16      `json:"status"`
	PasswordHash     string     `json:"-"`
	DeactivatedAt    *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// AuthRequest 登录/注册请求
type AuthRequest struct {
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	Email          string `json:"email,omitempty"`
	Code           string `json:"code,omitempty"`
	Mode           string `json:"mode,omitempty"`           // "password" 或 "code"，登录模式
	TurnstileToken string `json:"turnstileToken,omitempty"` // Cloudflare Turnstile 返回的校验 token
	RemoteIP       string `json:"-"`
}

// SendCodeRequest 发送验证码请求
type SendCodeRequest struct {
	Email string `json:"email"`
}

type TurnstileConfigResponse struct {
	SiteKey string `json:"siteKey"`
	Enabled bool   `json:"enabled"`
}

type AuthResponse struct {
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	User         User   `json:"user"`
}

type MeResponse struct {
	User User `json:"user"`
}
