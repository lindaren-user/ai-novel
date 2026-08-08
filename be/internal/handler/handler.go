package handler

import "ai-novel-ide/be/internal/service"

// Handler 承载 HTTP 处理函数和业务服务依赖。
type Handler struct {
	services service.Services
}

// New 创建 HTTP handler 集合。
func New(services service.Services) *Handler {
	return &Handler{services: services}
}

// AuthService 返回认证服务，供路由层挂载认证中间件。
func (h *Handler) AuthService() service.AuthService {
	return h.services.Auth
}
