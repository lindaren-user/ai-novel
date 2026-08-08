package handler

import (
	"errors"
	"net/http"
	"strings"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/service"

	"github.com/go-chi/chi/v5"
)

// handleCreateShareLink 创建指定小说、卷或章节的分享链接。
func (r *Handler) HandleCreateShareLink(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	var body service.CreateShareRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}

	response, err := r.services.Shares.CreateLink(req.Context(), user.ID, body)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "分享参数不正确", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "分享资源不存在", "无权分享该资源") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "创建分享链接失败", err)
		return
	}
	writeOK(w, response)
}

// handleGetSharedContent 读取分享阅读页公开数据。
func (r *Handler) HandleGetSharedContent(w http.ResponseWriter, req *http.Request) {
	shareType := strings.TrimSpace(chi.URLParam(req, "shareType"))
	token := strings.TrimSpace(chi.URLParam(req, "token"))
	password := strings.TrimSpace(req.URL.Query().Get("pwd"))

	content, err := r.services.Shares.GetContent(req.Context(), shareType, token, password)
	if errors.Is(err, service.ErrSharePasswordRequired) {
		r.writeLoggedError(w, req, http.StatusUnauthorized, "请输入分享密钥", err)
		return
	}
	if errors.Is(err, service.ErrInvalidSharePassword) {
		r.writeLoggedError(w, req, http.StatusForbidden, "分享密钥不正确", err)
		return
	}
	if errors.Is(err, service.ErrResourceNotFound) {
		r.writeLoggedError(w, req, http.StatusNotFound, "分享链接不存在或已失效", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "读取分享内容失败", err)
		return
	}
	writeOK(w, content)
}
