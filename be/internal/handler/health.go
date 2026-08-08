package handler

import "net/http"

// handleHealth 健康检查接口
func (r *Handler) HandleHealth(w http.ResponseWriter, req *http.Request) {
	r.services.Health.Check(req.Context())
	writeOK(w, "ok")
}
