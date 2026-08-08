package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/repo"
	"ai-novel-ide/be/internal/service"

	"go.uber.org/zap"
)

const (
	authCookieName        = "ai_novel_auth"
	authRefreshCookieName = "ai_novel_refresh"
)

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(nil, w, http.StatusOK, model.Response{Code: model.CodeOK, Msg: "success", Data: data})
}

// WriteUnauthorized 记录认证失败并返回 401。
func (h *Handler) WriteUnauthorized(w http.ResponseWriter, req *http.Request, msg string, err error) {
	h.writeLoggedError(w, req, http.StatusUnauthorized, msg, err)
}

// writeLoggedError 记录 handler 调用失败的上下文并返回错误响应。
func (h *Handler) writeLoggedError(w http.ResponseWriter, req *http.Request, code int, msg string, err error) {
	businessCode := businessCodeForError(code, err)
	if err != nil {
		wrappedErr := fmt.Errorf("%s: %w", msg, err)
		h.logHandlerError(wrappedErr)
		h.writeJSON(w, req, code, model.Response{Code: businessCode, Msg: wrappedErr.Error(), Data: nil})
		return
	}
	h.writeJSON(w, req, code, model.Response{Code: businessCode, Msg: msg, Data: nil})
}

// businessCodeForError 根据业务哨兵错误优先返回业务码，兜底按 HTTP 状态归类。
func businessCodeForError(status int, err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrVerificationCode), errors.Is(err, service.ErrInvalidToken), errors.Is(err, service.ErrTurnstileInvalid):
		return model.CodeInvalidCredentials
	case errors.Is(err, service.ErrUsernameTaken):
		return model.CodeUsernameTaken
	case errors.Is(err, service.ErrEmailTaken):
		return model.CodeConflict
	case errors.Is(err, service.ErrInvalidSettings):
		return model.CodeInvalidSettings
	case errors.Is(err, service.ErrInvalidModel):
		return model.CodeInvalidModel
	case errors.Is(err, service.ErrModelTaken):
		return model.CodeModelTaken
	case errors.Is(err, service.ErrInvalidMessage):
		return model.CodeInvalidMessage
	case errors.Is(err, service.ErrInvalidFeedback):
		return model.CodeInvalidFeedback
	case errors.Is(err, service.ErrInvalidFile):
		return model.CodeInvalidFile
	case errors.Is(err, service.ErrFileStorageUnavailable):
		return model.CodeFileStorageUnavailable
	case errors.Is(err, service.ErrSharePasswordRequired):
		return model.CodeSharePasswordRequired
	case errors.Is(err, service.ErrInvalidSharePassword):
		return model.CodeInvalidSharePassword
	case errors.Is(err, service.ErrAIUnavailable):
		return model.CodeAIUnavailable
	case errors.Is(err, service.ErrServiceShuttingDown):
		return model.CodeAIUnavailable
	case errors.Is(err, service.ErrConcurrentStreamLimitExceeded):
		return model.CodeConcurrentStreamLimit
	case errors.Is(err, service.ErrResourceNotFound), errors.Is(err, repo.ErrChatSessionNotFound):
		return model.CodeResourceNotFound
	case errors.Is(err, service.ErrForbidden):
		return model.CodeForbidden
	}

	switch status {
	case http.StatusBadRequest:
		return model.CodeInvalidRequest
	case http.StatusUnauthorized:
		return model.CodeUnauthorized
	case http.StatusForbidden:
		return model.CodeForbidden
	case http.StatusNotFound:
		return model.CodeResourceNotFound
	case http.StatusConflict:
		return model.CodeConflict
	case http.StatusTooManyRequests:
		return model.CodeRateLimited
	default:
		return model.CodeInternalError
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, req *http.Request, status int, payload any) {
	writeJSON(func(err error) {
		h.logHandlerError(fmt.Errorf("写入响应失败: %w", err))
	}, w, status, payload)
}

// decodeJSON 解析 JSON 请求体并统一返回请求格式错误，避免各 handler 重复编写解码错误处理。
func (h *Handler) decodeJSON(w http.ResponseWriter, req *http.Request, target any, msg string) bool {
	if err := json.NewDecoder(req.Body).Decode(target); err != nil {
		h.writeLoggedError(w, req, http.StatusBadRequest, msg, err)
		return false
	}
	return true
}

// writeResourceAccessError 将资源不存在和无权访问错误统一转换为 HTTP 响应。
func (h *Handler) writeResourceAccessError(w http.ResponseWriter, req *http.Request, err error, notFoundMsg string, forbiddenMsg string) bool {
	if errors.Is(err, service.ErrResourceNotFound) {
		h.writeLoggedError(w, req, http.StatusNotFound, notFoundMsg, err)
		return true
	}
	if errors.Is(err, service.ErrForbidden) {
		h.writeLoggedError(w, req, http.StatusForbidden, forbiddenMsg, err)
		return true
	}
	return false
}

// writeAIStreamStartError 将启动 AI 流时的通用业务错误统一转换为 HTTP 响应。
func (h *Handler) writeAIStreamStartError(w http.ResponseWriter, req *http.Request, err error, notFoundMsg string, forbiddenMsg string) bool {
	if h.writeResourceAccessError(w, req, err, notFoundMsg, forbiddenMsg) {
		return true
	}
	if errors.Is(err, service.ErrInvalidMessage) {
		h.writeLoggedError(w, req, http.StatusBadRequest, "消息内容不能为空", err)
		return true
	}
	if errors.Is(err, service.ErrAIUnavailable) {
		h.writeLoggedError(w, req, http.StatusServiceUnavailable, "AI 服务不可用", err)
		return true
	}
	if errors.Is(err, service.ErrServiceShuttingDown) {
		h.writeLoggedError(w, req, http.StatusServiceUnavailable, "服务正在关闭，请稍后重试", err)
		return true
	}
	if errors.Is(err, service.ErrConcurrentStreamLimitExceeded) {
		h.writeLoggedError(w, req, http.StatusTooManyRequests, "AI 任务并发数已达上限，请等待其他回复完成", err)
		return true
	}
	return false
}

// writeCancelStreamError 将取消流式回复时的通用业务错误统一转换为 HTTP 响应。
func (h *Handler) writeCancelStreamError(w http.ResponseWriter, req *http.Request, err error, forbiddenMsg string) bool {
	if errors.Is(err, service.ErrResourceNotFound) || errors.Is(err, repo.ErrChatSessionNotFound) {
		h.writeLoggedError(w, req, http.StatusNotFound, "没有正在生成的 AI 回复", err)
		return true
	}
	if errors.Is(err, service.ErrForbidden) {
		h.writeLoggedError(w, req, http.StatusForbidden, forbiddenMsg, err)
		return true
	}
	return false
}

// writeJSON 写入 JSON 响应，并在编码失败时交给调用方记录 handler 错误。
func writeJSON(onError func(error), w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		if onError != nil {
			onError(err)
			return
		}
		zap.L().Error("写入响应失败", zap.Error(fmt.Errorf("写入响应失败: %w", err)))
	}
}

// logHandlerError 只记录错误原因，请求维度由 HTTP 请求日志负责输出。
func (h *Handler) logHandlerError(err error) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Error(err.Error())
}

// authRefreshTokenFromRequest 从 HttpOnly Cookie 读取刷新令牌。
func authRefreshTokenFromRequest(req *http.Request) string {
	if cookie, err := req.Cookie(authRefreshCookieName); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func authTokenFromRequest(req *http.Request) string {
	return middleware.AuthTokenFromRequest(req)
}

func requestIP(req *http.Request) string {
	forwardedFor := strings.TrimSpace(req.Header.Get("X-Forwarded-For"))
	if forwardedFor != "" {
		return strings.TrimSpace(strings.Split(forwardedFor, ",")[0])
	}
	realIP := strings.TrimSpace(req.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(req.RemoteAddr)
	}
	return host
}
