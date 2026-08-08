package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"ai-novel-ide/be/internal/model"
	"ai-novel-ide/be/internal/service"
)

const authCookieName = "ai_novel_auth"

type authUserContextKey struct{}

// AuthMiddleware 校验登录态，并把当前用户写入请求上下文。
func AuthMiddleware(auth service.AuthService, writeUnauthorized func(http.ResponseWriter, *http.Request, string, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token := AuthTokenFromRequest(req)
			if token == "" {
				writeUnauthorized(w, req, "未登录", fmt.Errorf("请求缺少认证 Cookie"))
				return
			}

			user, err := auth.Me(req.Context(), token)
			if err != nil {
				writeUnauthorized(w, req, "登录已过期", err)
				return
			}

			user.PasswordHash = ""
			ctx := context.WithValue(req.Context(), authUserContextKey{}, user)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

// CurrentUser 从认证中间件写入的上下文中读取当前用户。
func CurrentUser(req *http.Request) model.User {
	user, _ := req.Context().Value(authUserContextKey{}).(model.User)
	return user
}

// AuthTokenFromRequest 从 HttpOnly Cookie 读取 JWT。
func AuthTokenFromRequest(req *http.Request) string {
	if cookie, err := req.Cookie(authCookieName); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
