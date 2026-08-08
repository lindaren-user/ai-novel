package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/service"

	"github.com/go-chi/chi/v5"
)

type createDownloadRequest struct {
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	Format string `json:"format"`
	Layout string `json:"layout"`
}

// handleListDownloads 查询当前用户的内存下载任务列表。
func (r *Handler) HandleListDownloads(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	jobs, err := r.services.Downloads.List(req.Context(), user.ID)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取下载任务失败", err)
		return
	}
	writeOK(w, jobs)
}

// handleCreateDownload 创建下载任务。
func (r *Handler) HandleCreateDownload(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	var body createDownloadRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	var scopeID int64
	if body.Type != "all" {
		scopeID = body.ID
		if scopeID <= 0 {
			r.writeLoggedError(w, req, http.StatusBadRequest, "ID 参数不正确", fmt.Errorf("%s ID 必须是正整数", body.Type))
			return
		}
	}

	job, err := r.services.Downloads.Start(req.Context(), user.ID, body.Type, scopeID, body.Format, body.Layout)
	if r.writeResourceAccessError(w, req, err, "下载内容不存在", "无权下载该内容") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "创建下载任务失败", err)
		return
	}
	writeOK(w, job)
}

// handleGetDownload 查询下载任务进度。
func (r *Handler) HandleGetDownload(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	job, err := r.services.Downloads.Status(req.Context(), user.ID, chi.URLParam(req, "jobID"))
	if errors.Is(err, service.ErrResourceNotFound) {
		r.writeLoggedError(w, req, http.StatusNotFound, "下载任务不存在", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "查询下载进度失败", err)
		return
	}
	writeOK(w, job)
}

// handleGetDownloadFile 返回下载任务生成的文件。
func (r *Handler) HandleGetDownloadFile(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	filename, mime, data, err := r.services.Downloads.File(req.Context(), user.ID, chi.URLParam(req, "jobID"))
	if errors.Is(err, service.ErrResourceNotFound) {
		r.writeLoggedError(w, req, http.StatusNotFound, "下载文件不存在或尚未生成", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "读取下载文件失败", err)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
