package handler

import (
	"errors"
	"net/http"
	"time"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"
)

// handleRegister 用户注册
func (r *Handler) HandleRegister(w http.ResponseWriter, req *http.Request) {
	var body model.AuthRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	body.RemoteIP = requestIP(req)
	response, err := r.services.Auth.Register(req.Context(), body)
	if errors.Is(err, service.ErrTurnstileInvalid) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "人机验证失败，请重试", err)
		return
	}
	if errors.Is(err, service.ErrUsernameTaken) {
		r.writeLoggedError(w, req, http.StatusConflict, "用户名已存在", err)
		return
	}
	if errors.Is(err, service.ErrEmailTaken) {
		r.writeLoggedError(w, req, http.StatusConflict, "邮箱已被注册", err)
		return
	}
	if errors.Is(err, service.ErrVerificationCode) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "邮箱验证码不正确或已过期", err)
		return
	}
	if errors.Is(err, service.ErrInvalidCredentials) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "用户名、邮箱不能为空，密码至少6位", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "注册失败", err)
		return
	}
	setAuthCookies(w, response.Token, response.RefreshToken)
	response.Token = ""
	response.RefreshToken = ""
	r.writeJSON(w, req, http.StatusCreated, model.Response{Code: model.CodeOK, Msg: "注册成功", Data: response})
}

// handleSendVerificationCode 发送邮箱验证码。
func (r *Handler) HandleSendVerificationCode(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	if err := r.services.Auth.SendVerificationCode(req.Context(), body.Email); err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "发送验证码失败", err)
		return
	}
	writeOK(w, nil)
}

// handleTurnstileConfig 返回前端渲染 Turnstile 所需的 site key。
func (r *Handler) HandleTurnstileConfig(w http.ResponseWriter, req *http.Request) {
	writeOK(w, r.services.Auth.TurnstileConfig())
}

// handleLogin 用户登录
func (r *Handler) HandleLogin(w http.ResponseWriter, req *http.Request) {
	var body model.AuthRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	body.RemoteIP = requestIP(req)
	response, err := r.services.Auth.Login(req.Context(), body)
	if errors.Is(err, service.ErrTurnstileInvalid) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "人机验证失败，请重试", err)
		return
	}
	if errors.Is(err, service.ErrInvalidCredentials) {
		r.writeLoggedError(w, req, http.StatusUnauthorized, "用户名或密码错误", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "登录失败", err)
		return
	}
	setAuthCookies(w, response.Token, response.RefreshToken)
	response.Token = ""
	response.RefreshToken = ""
	writeOK(w, response)
}

// handleRefresh 使用刷新 Cookie 轮换并写入新的登录 Cookie。
func (r *Handler) HandleRefresh(w http.ResponseWriter, req *http.Request) {
	refreshToken := authRefreshTokenFromRequest(req)
	if refreshToken == "" {
		r.writeLoggedError(w, req, http.StatusUnauthorized, "登录已过期", service.ErrInvalidToken)
		return
	}
	response, err := r.services.Auth.Refresh(req.Context(), refreshToken)
	if err != nil {
		clearAuthCookies(w)
		r.writeLoggedError(w, req, http.StatusUnauthorized, "登录已过期", err)
		return
	}
	setAuthCookies(w, response.Token, response.RefreshToken)
	response.Token = ""
	response.RefreshToken = ""
	writeOK(w, response)
}

// handleChangePassword 修改当前用户密码。
func (r *Handler) HandleChangePassword(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式错误") {
		return
	}
	if err := r.services.Auth.VerifyPassword(req.Context(), user.ID, body.OldPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			r.writeLoggedError(w, req, http.StatusBadRequest, "旧密码不正确", err)
			return
		}
		r.writeLoggedError(w, req, http.StatusInternalServerError, "校验密码失败", err)
		return
	}
	if err := r.services.Auth.ChangePassword(req.Context(), user.ID, body.NewPassword); err != nil {
		r.writeLoggedError(w, req, http.StatusBadRequest, "密码修改失败", err)
		return
	}
	writeOK(w, nil)
}

// handleLogout 用户登出，加入 JWT 黑名单并清除 Cookie。
func (r *Handler) HandleLogout(w http.ResponseWriter, req *http.Request) {
	token := authTokenFromRequest(req)
	if token != "" {
		_ = r.services.Auth.Logout(req.Context(), token)
	}
	refreshToken := authRefreshTokenFromRequest(req)
	if refreshToken != "" {
		_ = r.services.Auth.Logout(req.Context(), refreshToken)
	}
	clearAuthCookies(w)
	writeOK(w, nil)
}

// handleDeleteAccount 注销当前用户并清除登录 Cookie。
func (r *Handler) HandleDeleteAccount(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	var body struct {
		Password string `json:"password"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式错误") {
		return
	}
	if err := r.services.Auth.VerifyPassword(req.Context(), user.ID, body.Password); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			r.writeLoggedError(w, req, http.StatusBadRequest, "密码不正确", err)
			return
		}
		r.writeLoggedError(w, req, http.StatusInternalServerError, "校验密码失败", err)
		return
	}
	if err := r.services.Auth.DeleteAccount(req.Context(), user.ID); err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "注销用户失败", err)
		return
	}
	clearAuthCookies(w)
	writeOK(w, nil)
}

// handleMe 获取当前登录用户信息
func (r *Handler) HandleMe(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	writeOK(w, model.MeResponse{User: user})
}

// setAuthCookies 写入访问令牌和刷新令牌 Cookie。
func setAuthCookies(w http.ResponseWriter, token string, refreshToken string) {
	setAuthAccessCookie(w, token)
	http.SetCookie(w, &http.Cookie{
		Name:     authRefreshCookieName,
		Value:    refreshToken,
		Path:     "/api/auth",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// setAuthAccessCookie 写入访问令牌 Cookie。
func setAuthAccessCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((15 * time.Minute).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookies 清除认证相关 Cookie。
func clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     authRefreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
