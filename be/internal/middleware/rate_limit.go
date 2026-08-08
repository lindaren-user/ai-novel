package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"ai-novel-ide/be/internal/model"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 固定窗口
var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return current
`)

type rateLimitBodyKey struct{}

// RateLimitRule 描述一个独立限流桶；同一请求可同时命中 IP、用户、邮箱等多个桶。
type RateLimitRule struct {
	Name       string
	Key        func(*http.Request) string // Name 和 Key 共同决定一个窗口，比如 rate:login:ip:1.2.3.4，"rate:login:ip"是 Name，"1.2.3.4"是 Key
	Limit      int64                      // 限流的次数
	Window     time.Duration              // 巧妙利用时间作为一个窗口
	FailClosed bool                       // 表示 Redis 限流检查失败时，是否直接拒绝请求，防止因为 Redis 崩溃导致整个系统崩溃
}

// RateLimiter 使用 Redis 对 HTTP 请求做固定窗口限流。
func RateLimiter(redisClient *redis.Client, rules ...RateLimitRule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if redisClient == nil || len(rules) == 0 {
				next.ServeHTTP(w, req)
				return
			}

			req = withCachedRateLimitBody(req, rules)

			// 每个限流规则都需要检查
			for _, rule := range rules {
				if !validRateLimitRule(rule) {
					continue
				}
				keyPart := strings.TrimSpace(rule.Key(req))
				if keyPart == "" {
					continue
				}

				key := fmt.Sprintf("rate:%s:%s", rule.Name, keyPart)
				count, err := rateLimitScript.Run(req.Context(), redisClient, []string{key}, int(rule.Window.Seconds())).Int64()
				if err != nil {
					zap.L().Warn("rate limit check failed",
						zap.String("rule", rule.Name),
						zap.String("key", key),
						zap.Error(err),
					)
					if rule.FailClosed {
						writeRateLimited(w, "请求过于频繁，请稍后再试")
						return
					}
					continue
				}
				if count > rule.Limit {
					writeRateLimited(w, "请求过于频繁，请稍后再试")
					return
				}
			}

			next.ServeHTTP(w, req)
		})
	}
}

func validRateLimitRule(rule RateLimitRule) bool {
	return strings.TrimSpace(rule.Name) != "" && rule.Limit > 0 && rule.Window > 0 && rule.Key != nil
}

// withCachedRateLimitBody 缓存请求体，供按 email/username 等 JSON 字段限流使用。
// HTTP 请求体只能读取一次；这里读完后必须重新写回 req.Body，保证后续 handler 还能正常 Decode。
func withCachedRateLimitBody(req *http.Request, rules []RateLimitRule) *http.Request {
	if req.Body == nil || !rateLimitRulesNeedBody(rules) {
		return req
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		req.Body = io.NopCloser(bytes.NewReader(nil))
		return req
	}
	// 恢复请求体，避免限流中间件提前消费 body 导致业务 handler 读不到请求参数。
	req.Body = io.NopCloser(bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), rateLimitBodyKey{}, body)
	return req.WithContext(ctx)
}

func rateLimitRulesNeedBody(rules []RateLimitRule) bool {
	for _, rule := range rules {
		if strings.Contains(rule.Name, "email") || strings.Contains(rule.Name, "identity") {
			return true
		}
	}
	return false
}

// RateLimitClientIPKey 按客户端 IP 生成限流 key。
func RateLimitClientIPKey(req *http.Request) string {
	forwardedFor := strings.TrimSpace(req.Header.Get("X-Forwarded-For"))
	if forwardedFor != "" {
		return sanitizeRateLimitKey(strings.Split(forwardedFor, ",")[0])
	}
	realIP := strings.TrimSpace(req.Header.Get("X-Real-IP"))
	if realIP != "" {
		return sanitizeRateLimitKey(realIP)
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return sanitizeRateLimitKey(req.RemoteAddr)
	}
	return sanitizeRateLimitKey(host)
}

// RateLimitUserKey 按当前登录用户生成限流 key。
func RateLimitUserKey(req *http.Request) string {
	user := CurrentUser(req)
	if user.ID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", user.ID)
}

// RateLimitJSONFieldKey 从 JSON 请求体中读取字段并生成限流 key。
func RateLimitJSONFieldKey(field string) func(*http.Request) string {
	return func(req *http.Request) string {
		value := strings.TrimSpace(rateLimitJSONField(req, field))
		if value == "" {
			return ""
		}
		return sanitizeRateLimitKey(strings.ToLower(value))
	}
}

func rateLimitJSONField(req *http.Request, field string) string {
	body, _ := req.Context().Value(rateLimitBodyKey{}).([]byte)
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, _ := payload[field].(string)
	return value
}

func sanitizeRateLimitKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, "/", "_")
	return value
}

func writeRateLimited(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(model.Response{Code: model.CodeRateLimited, Msg: msg, Data: nil})
}
