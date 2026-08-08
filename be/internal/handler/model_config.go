package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"

	"github.com/go-chi/chi/v5"
)

// handleListModelConfigs 获取可用模型配置列表
func (r *Handler) HandleListModelConfigs(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	modelConfigs, err := r.services.ModelConfigs.List(req.Context(), user.ID)
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "获取模型列表失败", err)
		return
	}
	writeOK(w, modelConfigs)
}

// handleCreateModelConfig 新增当前用户自定义模型配置
func (r *Handler) HandleCreateModelConfig(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)

	var body model.CreateModelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	item, err := r.services.ModelConfigs.Create(req.Context(), user.ID, body)
	if errors.Is(err, service.ErrInvalidModel) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "模型配置不正确", err)
		return
	}
	if errors.Is(err, service.ErrModelTaken) {
		r.writeLoggedError(w, req, http.StatusConflict, "模型名称已存在", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "新增模型失败", err)
		return
	}
	r.writeJSON(w, req, http.StatusCreated, model.Response{Code: model.CodeOK, Msg: "新增模型成功", Data: item})
}

// handleDeleteModelConfig 删除当前用户自己的自定义模型配置。
func (r *Handler) HandleDeleteModelConfig(w http.ResponseWriter, req *http.Request) {
	user := middleware.CurrentUser(req)
	modelID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(req, "modelID")), 10, 64)
	if err != nil || modelID <= 0 {
		if err == nil {
			err = fmt.Errorf("model ID 必须是正整数")
		}
		r.writeLoggedError(w, req, http.StatusBadRequest, "ID 参数不正确", err)
		return
	}
	err = r.services.ModelConfigs.Delete(req.Context(), user.ID, modelID)
	if errors.Is(err, service.ErrResourceNotFound) {
		r.writeLoggedError(w, req, http.StatusNotFound, "模型不存在", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "删除模型失败", err)
		return
	}
	writeOK(w, nil)
}

// handleTestModelConfig 使用用户提交的模型配置进行一次真实连接测试。
func (r *Handler) HandleTestModelConfig(w http.ResponseWriter, req *http.Request) {

	var body model.CreateModelRequest
	if !r.decodeJSON(w, req, &body, "请求格式不正确") {
		return
	}
	result, err := r.services.ModelConfigs.Test(req.Context(), body)
	if errors.Is(err, service.ErrInvalidModel) {
		r.writeLoggedError(w, req, http.StatusBadRequest, "模型配置不正确", err)
		return
	}
	if errors.Is(err, service.ErrAIUnavailable) {
		r.writeLoggedError(w, req, http.StatusBadGateway, "模型连接失败", err)
		return
	}
	if err != nil {
		r.writeLoggedError(w, req, http.StatusInternalServerError, "测试模型失败", err)
		return
	}
	writeOK(w, result)
}
