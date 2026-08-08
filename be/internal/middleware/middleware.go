package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Middleware func(http.Handler) http.Handler
type contextKey string

const requestIDKey contextKey = "request_id"

var requestCounter uint64

// Chain 按声明顺序组合多个 HTTP 中间件，最后包裹目标处理器。
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// Recoverer 捕获 HTTP 处理链中的 panic，只记录关键信息并返回 500。
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				zap.L().Error("http panic recovered",
					zap.String("request_id", RequestIDFromContext(r.Context())),
					zap.Any("panic", err),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RequestID 为每个请求生成或透传请求 ID，并写入响应头和上下文。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&requestCounter, 1))
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext 从上下文读取请求 ID，缺失时返回空字符串。
func RequestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

// RequestLogger 仅记录失败请求的关键请求信息，避免正常请求产生无效日志。
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		route := ""
		if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
			route = routeCtx.RoutePattern()
		}
		displayRoute := route
		if displayRoute == "" {
			displayRoute = r.URL.Path
		}

		fields := []zap.Field{
			zap.String("request_id", RequestIDFromContext(r.Context())),
			zap.String("query", r.URL.RawQuery),
			zap.String("remote", r.RemoteAddr),
			zap.Duration("duration", time.Since(start)),
			zap.Int("bytes", ww.bytes),
			zap.String("user_agent", r.UserAgent()),
		}
		logger := zap.L().WithOptions(zap.WithCaller(false))
		message := fmt.Sprintf("%d %s %s", ww.status, r.Method, displayRoute)
		switch {
		case ww.status >= http.StatusInternalServerError:
			logger.Error(message, fields...)
		case ww.status >= http.StatusBadRequest:
			logger.Warn(message, fields...)
		default:
			logger.Info(message, fields...)
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader 记录响应状态码并继续写出真实响应头。
func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write 记录响应大小和少量错误响应内容，便于排查服务端错误。
func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush 在底层响应支持流式刷新时立即推送缓冲区内容。
func (w *responseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
