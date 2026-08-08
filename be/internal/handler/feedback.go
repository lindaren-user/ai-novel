package handler

import (
	"errors"
	"net/http"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"
)

// handleCreateFeedback 保存当前用户提交的帮助与反馈内容。
func (r *Handler) HandleCreateFeedback(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	var body struct {
		Content   string   `json:"content"`
		ImageURLs []string `json:"imageUrls"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式错误") {
		return
	}
	err := r.services.Feedbacks.Create(req.Context(), user.ID, body.Content, body.ImageURLs)
	if errors.Is(err, service.ErrInvalidFeedback) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "反馈内容不能为空", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "保存用户反馈失败", err)
		return
	}
	r.writeJSON(w, req, http.StatusCreated, model.Response{Code: model.CodeOK, Msg: "反馈已提交", Data: nil})
}
