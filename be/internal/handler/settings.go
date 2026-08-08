package handler

import (
	"errors"
	"net/http"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"
)

// handleGetSettings 获取当前用户设置
func (r *Handler) HandleGetSettings(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	settings, err := r.services.Settings.Get(req.Context(), user.ID)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "读取设置失败", err)
		return
	}
	writeOK(w, settings)
}

// handleUpdateSettings 保存当前用户设置
func (r *Handler) HandleUpdateSettings(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	var body model.SettingsRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}

	err := r.services.Settings.Update(req.Context(), user.ID, body)
	if errors.Is(err, service.ErrInvalidSettings) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "设置格式不正确", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "保存设置失败", err)
		return
	}
	writeOK(w, nil)
}
