package handler

import (
	"errors"
	"net/http"
	"strings"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"
)

const fileUploadTypeFeedback = "feedback"

// handleCreateFileUploadToken 创建前端直传对象存储的上传凭证。
func (r *Handler) HandleCreateFileUploadToken(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	var body struct {
		Type        string `json:"type"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式错误") {
		return
	}
	token, err := r.createTypedFileUploadToken(req, user.ID, strings.TrimSpace(body.Type), body.Filename, body.ContentType, body.Size)
	if errors.Is(err, service.ErrInvalidFile) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "仅支持上传 5MB 内的 png/jpg 图片", err)
		return
	}
	if errors.Is(err, service.ErrFileStorageUnavailable) {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "文件存储未配置", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "创建上传凭证失败", err)
		return
	}
	r.writeJSON(w, req, http.StatusCreated, model.Response{Code: model.CodeOK, Msg: "上传凭证已创建", Data: token})
}

// createTypedFileUploadToken 根据上传业务类型分发到对应服务，避免文件入口混入具体业务规则。
func (r *Handler) createTypedFileUploadToken(req *http.Request, userID int64, uploadType string, filename string, contentType string, size int64) (model.FileUploadToken, error) {
	switch uploadType {
	case fileUploadTypeFeedback:
		return r.services.Feedbacks.CreateImageUploadToken(req.Context(), userID, filename, contentType, size)
	default:
		return model.FileUploadToken{}, service.ErrInvalidFile
	}
}
