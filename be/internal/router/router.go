package router

import (
	"net/http"
	"time"

	"ai-novel-ide/be/internal/handler"
	"ai-novel-ide/be/internal/middleware"
	"ai-novel-ide/be/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// New 创建 HTTP 路由并挂载全局中间件。
func New(services service.Services, redisClient *redis.Client) http.Handler {
	h := handler.New(services)
	mux := chi.NewRouter()
	routes(mux, h, redisClient)

	return middleware.Chain(
		mux,
		middleware.Recoverer,
		middleware.RequestID,
		middleware.RequestLogger,
	)
}

func routes(mux chi.Router, h *handler.Handler, redisClient *redis.Client) {
	mux.Get("/healthz", h.HandleHealth) // 仅用于探测服务状态：curl http://localhost:8080/healthz

	mux.Route("/api", func(api chi.Router) {
		api.Route("/auth", func(auth chi.Router) {
			auth.With(authRateLimiter(redisClient, "register")).Post("/register", h.HandleRegister)
			auth.With(authRateLimiter(redisClient, "login")).Post("/login", h.HandleLogin)
			auth.With(refreshRateLimiter(redisClient)).Post("/refresh", h.HandleRefresh)
			auth.With(sendCodeRateLimiter(redisClient)).Post("/send-code", h.HandleSendVerificationCode)
			auth.Get("/turnstile-config", h.HandleTurnstileConfig)
			auth.Post("/logout", h.HandleLogout)
		})

		api.Route("/shares", func(shares chi.Router) {
			shares.Get("/{shareType}/{token}", h.HandleGetSharedContent)
		})

		api.Group(func(protected chi.Router) {
			protected.Use(middleware.AuthMiddleware(h.AuthService(), h.WriteUnauthorized))

			protected.Post("/auth/change-password", h.HandleChangePassword)
			protected.Delete("/auth/me", h.HandleDeleteAccount)
			protected.Get("/auth/me", h.HandleMe)

			protected.Route("/settings", func(settings chi.Router) {
				settings.Get("/", h.HandleGetSettings)
				settings.Put("/", h.HandleUpdateSettings)
			})

			protected.Route("/models", func(modelConfigs chi.Router) {
				modelConfigs.Get("/", h.HandleListModelConfigs)
				modelConfigs.Post("/", h.HandleCreateModelConfig)
				modelConfigs.With(userRateLimiter(redisClient, "model_test", 10, time.Minute)).Post("/test", h.HandleTestModelConfig)
				modelConfigs.Delete("/{modelID}", h.HandleDeleteModelConfig)
			})

			protected.Post("/shares", h.HandleCreateShareLink)

			protected.Route("/downloads", func(downloads chi.Router) {
				downloads.Get("/", h.HandleListDownloads)
				downloads.Post("/", h.HandleCreateDownload)
				downloads.Get("/{jobID}", h.HandleGetDownload)
				downloads.Get("/{jobID}/file", h.HandleGetDownloadFile)
			})

			protected.Post("/files", h.HandleCreateFileUploadToken)
			protected.Post("/feedbacks", h.HandleCreateFeedback)
			protected.Get("/dashboard", h.HandleGetWorkspaceDashboard)

			protected.Route("/novels", func(novels chi.Router) {
				novels.Get("/", h.HandleListNovels)
				novels.Post("/", h.HandleCreateNovel)
				novels.Post("/setup/drafts", h.HandleSaveNovelSetupDraft)
				novels.With(aiStartRateLimiter(redisClient, "novel_setup_complete")).Post("/setup/complete", h.HandleCompleteNovelSetup)

				novels.Route("/{novelID}", func(novel chi.Router) {
					novel.Post("/setup/draft", h.HandleUpdateNovelSetupDraft)
					novel.Post("/setup/start", h.HandleStartNovelSetupDraft)
					novel.Post("/archive", h.HandleArchiveNovel)
					novel.Post("/restore", h.HandleRestoreNovel)
					novel.Get("/overview", h.HandleGetNovelOverview)
					novel.Get("/volumes", h.HandleListVolumes)
					novel.Post("/volumes/apply-plan", h.HandleApplyVolumePlan)
					novel.Get("/messages", h.HandleListNovelMessages)
					novel.With(aiStartRateLimiter(redisClient, "novel_stream")).Post("/stream", h.HandleStreamNovel)
					novel.Get("/stream/resume", h.HandleResumeNovelStream)
					novel.Post("/stream/cancel", h.HandleCancelNovelStream)
				})
			})

			protected.Route("/volumes/{volumeID}", func(volume chi.Router) {
				volume.Get("/chapters", h.HandleListChapters)
				volume.Post("/chapters/apply-plan", h.HandleApplyChapterPlan)
				volume.Get("/messages", h.HandleListVolumeMessages)
				volume.With(aiStartRateLimiter(redisClient, "volume_stream")).Post("/stream", h.HandleStreamVolume)
				volume.Get("/stream/resume", h.HandleResumeVolumeStream)
				volume.Post("/stream/cancel", h.HandleCancelVolumeStream)
			})

			protected.Route("/chapters/{chapterID}", func(chapter chi.Router) {
				chapter.Get("/messages", h.HandleListChapterMessages)
				chapter.With(aiStartRateLimiter(redisClient, "chapter_stream")).Post("/stream", h.HandleStreamChapter)
				chapter.Get("/stream/resume", h.HandleResumeChapterStream)
				chapter.Post("/stream/cancel", h.HandleCancelChapterStream)
				chapter.Get("/drafts", h.HandleListChapterDrafts)
				chapter.Post("/drafts", h.HandleCreateChapterDraft)
				chapter.Post("/drafts/{draftID}/join", h.HandleJoinChapterDraft)
				chapter.Patch("/drafts/{draftID}", h.HandleUpdateChapterDraft)
				chapter.Delete("/drafts/{draftID}", h.HandleDeleteChapterDraft)
				chapter.Post("/drafts/{draftID}/use", h.HandleUseChapterDraft)
				chapter.With(aiStartRateLimiter(redisClient, "chapter_humanize")).Post("/humanize", h.HandleHumanizeChapter)
				chapter.With(aiStartRateLimiter(redisClient, "chapter_proofread")).Post("/proofread", h.HandleProofreadChapter)
			})
		})
	})
}

func authRateLimiter(redisClient *redis.Client, action string) func(http.Handler) http.Handler {
	return middleware.RateLimiter(redisClient,
		middleware.RateLimitRule{
			Name:       "auth:" + action + ":ip",
			Limit:      20,
			Window:     time.Minute,
			Key:        middleware.RateLimitClientIPKey,
			FailClosed: true,
		},
		middleware.RateLimitRule{
			Name:       "auth:" + action + ":email",
			Limit:      10,
			Window:     time.Minute,
			Key:        middleware.RateLimitJSONFieldKey("email"),
			FailClosed: true,
		},
		middleware.RateLimitRule{
			Name:       "auth:" + action + ":username",
			Limit:      10,
			Window:     time.Minute,
			Key:        middleware.RateLimitJSONFieldKey("username"),
			FailClosed: true,
		},
	)
}

func sendCodeRateLimiter(redisClient *redis.Client) func(http.Handler) http.Handler {
	return middleware.RateLimiter(redisClient,
		middleware.RateLimitRule{
			Name:       "auth:send_code:ip",
			Limit:      10,
			Window:     time.Minute,
			Key:        middleware.RateLimitClientIPKey,
			FailClosed: true,
		},
		middleware.RateLimitRule{
			Name:       "auth:send_code:email",
			Limit:      3,
			Window:     10 * time.Minute,
			Key:        middleware.RateLimitJSONFieldKey("email"),
			FailClosed: true,
		},
	)
}

func refreshRateLimiter(redisClient *redis.Client) func(http.Handler) http.Handler {
	return middleware.RateLimiter(redisClient, middleware.RateLimitRule{
		Name:       "auth:refresh:ip",
		Limit:      60,
		Window:     time.Minute,
		Key:        middleware.RateLimitClientIPKey,
		FailClosed: true,
	})
}

func aiStartRateLimiter(redisClient *redis.Client, action string) func(http.Handler) http.Handler {
	return userRateLimiter(redisClient, "ai:"+action, 12, time.Minute)
}

func userRateLimiter(redisClient *redis.Client, action string, limit int64, window time.Duration) func(http.Handler) http.Handler {
	return middleware.RateLimiter(redisClient, middleware.RateLimitRule{
		Name:       action + ":user",
		Limit:      limit,
		Window:     window,
		Key:        middleware.RateLimitUserKey,
		FailClosed: true,
	})
}
