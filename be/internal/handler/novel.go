package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"

	"github.com/go-chi/chi/v5"
)

type createNovelRequest struct {
	SetupData service.NovelSetupInput `json:"setupData"`
}

type applyVolumePlanRequest struct {
	Plans []service.VolumePlan `json:"plans"`
	Force bool                 `json:"force"`
}

type applyChapterPlanRequest struct {
	Plans []service.ChapterPlan `json:"plans"`
	Force bool                  `json:"force"`
}

const novelSetupUploadLimit = 10 << 20

// handleCreateNovel 新建小说并创建小说级对话会话。
func (r *Handler) HandleCreateNovel(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	var body createNovelRequest
	if req.Body != nil {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			if !errors.Is(err, io.EOF) {
				r.writeLoggedError(w, req, http.StatusBadRequest, "请求格式不正确", err)
				return
			}
		}
	}

	response, err := r.services.Novels.CreateNovel(req.Context(), user.ID, body.SetupData)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "小说名称不能为空", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "新建小说失败", err)
		return
	}
	writeOK(w, response)
}

// handleSaveNovelSetupDraft 暂存新建小说表单，暂不进入正式创作。
func (r *Handler) HandleSaveNovelSetupDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	var body createNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	novel, err := r.services.Novels.SaveSetupDraft(req.Context(), user.ID, body.SetupData)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "小说名称不能为空", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "暂存小说设定失败", err)
		return
	}
	writeOK(w, novel)
}

// handleUpdateNovelSetupDraft 覆盖暂存小说的设定表单。
func (r *Handler) HandleUpdateNovelSetupDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	var body createNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	err := r.services.Novels.UpdateSetupDraft(req.Context(), user.ID, novelID, body.SetupData)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "小说名称不能为空", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "更新小说设定暂存失败", err)
		return
	}
	writeOK(w, nil)
}

// handleStartNovelSetupDraft 将暂存的新建小说表单更新为正式小说。
func (r *Handler) HandleStartNovelSetupDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	var body createNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	response, err := r.services.Novels.StartSetupDraft(req.Context(), user.ID, novelID, body.SetupData)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "小说名称不能为空", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "开始创作暂存小说失败", err)
		return
	}
	writeOK(w, response)
}

// handleCompleteNovelSetup 根据用户的一段描述或上传文件，用 AI 自动补全新建小说表单。
func (r *Handler) HandleCompleteNovelSetup(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	text, modelID, err := readNovelSetupCompleteRequest(req)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusBadRequest, "读取小说设定补全请求失败", err)
		return
	}
	stream, err := r.services.Novels.CompleteSetupStream(req.Context(), user.ID, service.NovelSetupCompleteInput{
		Text:    text,
		ModelID: modelID,
	})
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "请先输入想法或上传文件", err)
		return
	}
	if errors.Is(err, service.ErrInvalidModel) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "模型配置不正确", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "生成小说表单失败", err)
		return
	}
	r.writeSSEStream(req, w, stream)
}

// readNovelSetupCompleteRequest 读取 multipart 表单中的文本、模型 ID 和可选上传文件。
func readNovelSetupCompleteRequest(req *http.Request) (string, string, error) {
	if err := req.ParseMultipartForm(novelSetupUploadLimit); err != nil {
		return "", "", fmt.Errorf("请求格式不正确: %w", err)
	}
	text := strings.TrimSpace(req.FormValue("text"))
	modelID := strings.TrimSpace(req.FormValue("modelId"))
	fileTexts, err := readNovelSetupUploadedFiles(req)
	if err != nil {
		return "", "", err
	}
	if len(fileTexts) == 0 {
		return text, modelID, nil
	}
	return strings.TrimSpace(text + "\n" + strings.Join(fileTexts, "\n")), modelID, nil
}

// readNovelSetupUploadedFiles 读取多个上传文件，并提取可用于 AI 识别表单的文本。
func readNovelSetupUploadedFiles(req *http.Request) ([]string, error) {
	if req.MultipartForm == nil || req.MultipartForm.File == nil {
		return nil, nil
	}
	headers := append([]*multipart.FileHeader{}, req.MultipartForm.File["files"]...)
	texts := make([]string, 0, len(headers))
	for _, header := range headers {
		text, err := readNovelSetupUploadedFile(header)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) != "" {
			texts = append(texts, strings.TrimSpace(text))
		}
	}
	return texts, nil
}

// readNovelSetupUploadedFile 读取单个 txt/docx 上传文件并提取文本。
func readNovelSetupUploadedFile(header *multipart.FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, novelSetupUploadLimit))
	if err != nil {
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	return extractNovelSetupFileText(header.Filename, data)
}

// extractNovelSetupFileText 提取 txt 或 docx 文件中的文本，用于 AI 识别表单。
func extractNovelSetupFileText(filename string, data []byte) (string, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt":
		return string(data), nil
	case ".md":
		return string(data), nil
	case ".docx":
		return extractDocxText(data)
	default:
		return "", errors.New("只支持 txt、md 和 docx 文件")
	}
}

// extractDocxText 从 docx 的 word/document.xml 中提取正文文本。
func extractDocxText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx 文件格式不正确: %w", err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("读取 docx 内容失败: %w", err)
		}
		defer rc.Close()
		return readDocxDocumentXML(rc)
	}
	return "", errors.New("docx 文件缺少正文内容")
}

// readDocxDocumentXML 解析 docx 正文 XML，保留段落换行。
func readDocxDocumentXML(reader io.Reader) (string, error) {
	decoder := xml.NewDecoder(reader)
	var out strings.Builder
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return strings.TrimSpace(out.String()), nil
		}
		if err != nil {
			return "", fmt.Errorf("解析 docx 内容失败: %w", err)
		}
		switch item := token.(type) {
		case xml.StartElement:
			if item.Name.Local == "p" && out.Len() > 0 {
				out.WriteByte('\n')
			}
			if item.Name.Local == "tab" {
				out.WriteByte('\t')
			}
		case xml.CharData:
			out.Write([]byte(item))
		}
	}
}

// handleListNovels 获取当前用户的小说列表。
func (r *Handler) HandleListNovels(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	archived := req.URL.Query().Get("status") == "archived"
	novels, err := r.services.Novels.ListNovels(req.Context(), user.ID, archived)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取小说列表失败", err)
		return
	}
	writeOK(w, novels)
}

// handleGetNovelOverview 按需获取当前用户单本小说梗概。
func (r *Handler) HandleGetNovelOverview(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	overview, err := r.services.Novels.GetNovelOverview(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取小说梗概失败", err)
		return
	}
	writeOK(w, overview)
}

// handleGetWorkspaceDashboard 获取工作台首页统计数据。
func (r *Handler) HandleGetWorkspaceDashboard(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	dashboard, err := r.services.Novels.GetDashboard(req.Context(), user.ID)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取工作台数据失败", err)
		return
	}
	writeOK(w, dashboard)
}

// handleArchiveNovel 将当前用户的小说归档。
func (r *Handler) HandleArchiveNovel(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	err := r.services.Novels.ArchiveNovel(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "归档小说失败", err)
		return
	}
	writeOK(w, nil)
}

// handleRestoreNovel 将当前用户的归档小说恢复。
func (r *Handler) HandleRestoreNovel(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	err := r.services.Novels.RestoreNovel(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "恢复小说失败", err)
		return
	}
	writeOK(w, nil)
}

// handleListVolumes 获取小说下的卷列表。
func (r *Handler) HandleListVolumes(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}

	volumes, err := r.services.Volumes.ListVolumes(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取卷列表失败", err)
		return
	}
	writeOK(w, volumes)
}

// handleApplyVolumePlan 将当前规划卡片中的完整卷规划应用到小说。
func (r *Handler) HandleApplyVolumePlan(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	var body applyVolumePlanRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	volumes, err := r.services.Novels.ApplyVolumePlan(req.Context(), user.ID, novelID, body.Plans, body.Force)
	if errors.Is(err, service.ErrPlanOverwriteRequired) {
		r.writeJSON(w, req, http.StatusConflict, model.Response{Code: model.CodeConflict, Msg: "已有卷规划，覆盖前需要确认", Data: nil})
		return
	}
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "卷规划不能为空", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "应用卷规划失败", err)
		return
	}
	writeOK(w, volumes)
}

// handleListChapters 获取卷下的章节列表。
func (r *Handler) HandleListChapters(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}

	chapters, err := r.services.Chapters.ListChapters(req.Context(), user.ID, volumeID)
	if r.writeResourceAccessError(w, req, err, "卷不存在", "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取章节列表失败", err)
		return
	}
	writeOK(w, chapters)
}

// handleApplyChapterPlan 将当前规划卡片中的完整章节规划应用到卷。
func (r *Handler) HandleApplyChapterPlan(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}
	var body applyChapterPlanRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	chapters, err := r.services.Volumes.ApplyChapterPlan(req.Context(), user.ID, volumeID, body.Plans, body.Force)
	if errors.Is(err, service.ErrPlanOverwriteRequired) {
		r.writeJSON(w, req, http.StatusConflict, model.Response{Code: model.CodeConflict, Msg: "已有章节规划，覆盖前需要确认", Data: nil})
		return
	}
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "章节规划不能为空", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "卷不存在", "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "应用章节规划失败", err)
		return
	}
	writeOK(w, chapters)
}

// handleListNovelMessages 获取小说级对话消息列表。
func (r *Handler) HandleListNovelMessages(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}

	messages, err := r.services.Novels.ListNovelMessages(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取对话消息失败", err)
		return
	}
	writeOK(w, messages)
}

type streamNovelRequest struct {
	Content   string `json:"content"`
	GraphMode bool   `json:"graphMode"`
}

// handleStreamNovel 接收小说级用户消息，并以 SSE 流式返回 AI 回复。
func (r *Handler) HandleStreamNovel(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}

	var body streamNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}

	stream, err := r.services.Novels.StreamNovel(req.Context(), user.ID, novelID, body.Content)
	if r.writeAIStreamStartError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "启动 AI 流失败", err)
		return
	}

	r.writeSSEStream(req, w, stream)
}

// handleResumeNovelStream 重建小说级进行中 AI 回复的 SSE。
func (r *Handler) HandleResumeNovelStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	stream, err := r.services.Novels.ResumeNovelStream(req.Context(), user.ID, novelID)
	if r.writeResourceAccessError(w, req, err, "小说不存在", "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "重建 AI 流失败", err)
		return
	}
	r.writeSSEStream(req, w, stream)
}

// handleCancelNovelStream 取消小说级进行中的 AI 回复。
func (r *Handler) HandleCancelNovelStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	novelID, ok := r.pathID(w, req, "novelID")
	if !ok {
		return
	}
	err := r.services.Novels.CancelNovelStream(req.Context(), user.ID, novelID)
	if r.writeCancelStreamError(w, req, err, "无权访问该小说") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "取消 AI 回复失败", err)
		return
	}
	writeOK(w, nil)
}

// handleStreamVolume 接收卷级用户消息，并以 SSE 流式返回 AI 回复。
func (r *Handler) HandleStreamVolume(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}

	var body streamNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}

	stream, err := r.services.Volumes.StreamVolume(req.Context(), user.ID, volumeID, body.Content)
	if r.writeAIStreamStartError(w, req, err, "卷不存在", "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "启动 AI 流失败", err)
		return
	}

	r.writeSSEStream(req, w, stream)
}

// handleResumeVolumeStream 重建卷级进行中 AI 回复的 SSE。
func (r *Handler) HandleResumeVolumeStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}
	stream, err := r.services.Volumes.ResumeVolumeStream(req.Context(), user.ID, volumeID)
	if r.writeResourceAccessError(w, req, err, "卷不存在", "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "重建 AI 流失败", err)
		return
	}
	r.writeSSEStream(req, w, stream)
}

// handleCancelVolumeStream 取消卷级进行中的 AI 回复。
func (r *Handler) HandleCancelVolumeStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}
	err := r.services.Volumes.CancelVolumeStream(req.Context(), user.ID, volumeID)
	if r.writeCancelStreamError(w, req, err, "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "取消 AI 回复失败", err)
		return
	}
	writeOK(w, nil)
}

// handleStreamChapter 接收章级用户消息，并以 SSE 流式返回 AI 回复。
func (r *Handler) HandleStreamChapter(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	var body streamNovelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	stream, err := r.services.Chapters.StreamChapter(req.Context(), user.ID, chapterID, body.Content, service.ChapterStreamOptions{GraphMode: body.GraphMode})
	if r.writeAIStreamStartError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "启动 AI 流失败", err)
		return
	}
	r.writeSSEStream(req, w, stream)
}

// handleResumeChapterStream 重建章级进行中 AI 回复的 SSE。
func (r *Handler) HandleResumeChapterStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	stream, err := r.services.Chapters.ResumeChapterStream(req.Context(), user.ID, chapterID)
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "重建 AI 流失败", err)
		return
	}
	r.writeSSEStream(req, w, stream)
}

// handleCancelChapterStream 取消章级进行中的 AI 回复。
func (r *Handler) HandleCancelChapterStream(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	err := r.services.Chapters.CancelChapterStream(req.Context(), user.ID, chapterID)
	if r.writeCancelStreamError(w, req, err, "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "取消 AI 回复失败", err)
		return
	}
	writeOK(w, nil)
}

// writeSSEStream 将 AI 流事件编码成 SSE 响应，并定期发送心跳避免代理层空闲超时。
func (r *Handler) writeSSEStream(req *http.Request, w http.ResponseWriter, stream <-chan ai.StreamEvent) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "当前服务不支持流式响应", fmt.Errorf("response writer does not implement http.Flusher"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-stream:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, "data: "); err != nil {
				return
			}
			if err := encoder.Encode(sseEventData(event)); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// sseEventData 返回 SSE data payload；协议层 event 已表达类型，data 只保留载荷。
func sseEventData(event ai.StreamEvent) any {
	if event.Data == nil {
		return map[string]any{}
	}
	return event.Data
}

// handleListVolumeMessages 获取卷级对话消息列表。
func (r *Handler) HandleListVolumeMessages(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	volumeID, ok := r.pathID(w, req, "volumeID")
	if !ok {
		return
	}

	messages, err := r.services.Volumes.ListVolumeMessages(req.Context(), user.ID, volumeID)
	if r.writeResourceAccessError(w, req, err, "卷不存在", "无权访问该卷") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取对话消息失败", err)
		return
	}
	writeOK(w, messages)
}

// handleListChapterMessages 获取章级对话消息列表。
func (r *Handler) HandleListChapterMessages(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}

	messages, err := r.services.Chapters.ListChapterMessages(req.Context(), user.ID, chapterID)
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取对话消息失败", err)
		return
	}
	writeOK(w, messages)
}

// handleListChapterDrafts 获取当前章节可编辑正文草稿列表。
func (r *Handler) HandleListChapterDrafts(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	drafts, err := r.services.Chapters.ListChapterDrafts(req.Context(), user.ID, chapterID)
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取正文草稿失败", err)
		return
	}
	writeOK(w, drafts)
}

// handleJoinChapterDraft 将 AI 原始草稿加入编辑器，复制为可编辑草稿。
func (r *Handler) HandleJoinChapterDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	draftID, ok := r.pathID(w, req, "draftID")
	if !ok {
		return
	}
	draft, err := r.services.Chapters.JoinChapterDraft(req.Context(), user.ID, chapterID, draftID)
	if r.writeResourceAccessError(w, req, err, "AI 原始草稿不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "加入正文草稿失败", err)
		return
	}
	writeOK(w, draft)
}

// handleCreateChapterDraft 将一段文本直接保存为可编辑草稿，不依赖消息或已有草稿。
func (r *Handler) HandleCreateChapterDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if !r.decodeJSON(w, req, &body, "请求格式错误") {
		return
	}
	draft, err := r.services.Chapters.CreateDraftFromContent(req.Context(), user.ID, chapterID, body.Content)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "正文内容不能为空", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "创建草稿失败", err)
		return
	}
	writeOK(w, draft)
}

type updateChapterDraftRequest struct {
	Content   string `json:"content"`
	DraftName string `json:"draftName"`
}

// handleUpdateChapterDraft 保存当前章节的可编辑正文草稿。
func (r *Handler) HandleUpdateChapterDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	draftID, ok := r.pathID(w, req, "draftID")
	if !ok {
		return
	}
	var body updateChapterDraftRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	err := r.services.Chapters.UpdateChapterDraft(req.Context(), user.ID, chapterID, draftID, body.Content, body.DraftName)
	if r.writeResourceAccessError(w, req, err, "正文草稿不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "保存正文草稿失败", err)
		return
	}
	writeOK(w, nil)
}

// handleDeleteChapterDraft 删除可编辑正文草稿。
func (r *Handler) HandleDeleteChapterDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	draftID, ok := r.pathID(w, req, "draftID")
	if !ok {
		return
	}
	err := r.services.Chapters.DeleteChapterDraft(req.Context(), user.ID, chapterID, draftID)
	if r.writeResourceAccessError(w, req, err, "正文草稿不存在或不能删除", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "删除正文草稿失败", err)
		return
	}
	writeOK(w, nil)
}

// handleUseChapterDraft 将指定章节正文草稿设为当前正文。
func (r *Handler) HandleUseChapterDraft(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	draftID, ok := r.pathID(w, req, "draftID")
	if !ok {
		return
	}

	err := r.services.Chapters.UseChapterDraft(req.Context(), user.ID, chapterID, draftID)
	if r.writeResourceAccessError(w, req, err, "正文草稿不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "保存章节正文失败", err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

type chapterContentRequest struct {
	Content string `json:"content"`
	DraftID int64  `json:"draftId"`
}

// handleHumanizeChapter 对当前章节编辑区正文执行 AI 消痕，不直接覆盖草稿。
func (r *Handler) HandleHumanizeChapter(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	var body chapterContentRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	result, err := r.services.Chapters.HumanizeChapterContent(req.Context(), user.ID, chapterID, body.DraftID, body.Content)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "正文内容不能为空", err)
		return
	}
	if errors.Is(err, service.ErrInvalidModel) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "模型配置不正确", err)
		return
	}
	if errors.Is(err, service.ErrAIUnavailable) {
		r.writeLoggedError(w, req, http.StatusBadGateway, "AI 消痕未生成有效改写，请重试", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "AI 消痕失败", err)
		return
	}
	writeOK(w, result)
}

// handleProofreadChapter 对当前章节编辑区正文执行 AI 校审，只返回临时修改建议。
func (r *Handler) HandleProofreadChapter(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	chapterID, ok := r.pathID(w, req, "chapterID")
	if !ok {
		return
	}
	var body chapterContentRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	suggestions, err := r.services.Chapters.ProofreadChapterContent(req.Context(), user.ID, chapterID, body.DraftID, body.Content)
	if errors.Is(err, service.ErrInvalidMessage) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "正文内容不能为空", err)
		return
	}
	if errors.Is(err, service.ErrInvalidModel) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "模型配置不正确", err)
		return
	}
	if r.writeResourceAccessError(w, req, err, "章节不存在", "无权访问该章节") {
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "AI 校审失败", err)
		return
	}
	writeOK(w, suggestions)
}

// pathID 解析路由中的数字 ID。
func (r *Handler) pathID(w http.ResponseWriter, req *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(req, name)), 10, 64)
	if err != nil || id <= 0 {
		if err == nil {
			err = fmt.Errorf("%s 必须是正整数", name)
		}
		r.writeLoggedError(w, req, http.StatusBadRequest, "ID 参数不正确", err)
		return 0, false
	}
	return id, true
}
