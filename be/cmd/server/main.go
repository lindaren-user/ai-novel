package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crypto/sha256"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/config"
	mycrypto "ai-novel-ide/be/internal/crypto"
	"ai-novel-ide/be/internal/diagnostics"
	"ai-novel-ide/be/internal/infra"
	"ai-novel-ide/be/internal/logging"
	"ai-novel-ide/be/internal/mail"
	"ai-novel-ide/be/internal/repo"
	"ai-novel-ide/be/internal/router"
	"ai-novel-ide/be/internal/service"
	"ai-novel-ide/be/internal/storage"
)

// main 负责装配配置、基础设施、仓储、服务和路由，并启动 HTTP 服务。
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("load config failed: %v", err)
		os.Exit(1)
	}

	logger, err := logging.Init(cfg.Log)
	if err != nil {
		log.Printf("init logger failed: %v", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()
	encKey := sha256.Sum256([]byte(cfg.Auth.Secret))
	mycrypto.Init(encKey[:])

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := infra.NewPostgres(ctx, cfg.Postgres)
	if err != nil {
		log.Printf("connect postgres failed: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	redisClient, err := infra.NewRedis(ctx, cfg.Redis)
	if err != nil {
		log.Printf("connect redis failed: %v", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	repositories := repo.NewRepositories(db)
	aiClient := ai.NewClient()
	mailSender, err := mail.NewSender(cfg.Mail)
	if err != nil {
		log.Printf("init mail sender failed: %v", err)
		os.Exit(1)
	}
	storageClient, err := storage.NewClient(cfg.Storage)
	if err != nil {
		log.Printf("init storage client failed: %v", err)
		os.Exit(1)
	}
	services := service.NewServices(repositories, redisClient, aiClient, cfg.Auth, mailSender, storageClient, cfg.Storage)
	router := router.New(services, redisClient)

	listenAddr := cfg.HTTP.ListenAddr()
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var pprofServer *http.Server
	if cfg.Pprof.Enabled {
		pprofServer = diagnostics.NewPprofServer(cfg.Pprof.Addr)
		go func() {
			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server stopped: %v", err)
			}
		}()
		log.Printf("pprof server started addr=%s", cfg.Pprof.Addr)
	}
	log.Printf("server started addr=%s", listenAddr)
	serverErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop) // 释放这个信号注释表，这里是在 main 函数中，可以不写，因为退出 main 函数时所有相关资源会自动释放

	select {
	case err := <-serverErr:
		if err != nil {
			log.Printf("server stopped: %v", err)
			os.Exit(1)
		}
		return
	case sig := <-stop:
		log.Printf("shutdown signal received signal=%s", sig.String())
	}
	services.BeginShutdown()

	// 15 秒：允许活跃请求和 AI 任务自然完成
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
	if pprofServer != nil {
		if err := pprofServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("pprof shutdown failed: %v", err)
		}
	}

	// 90 秒：取消 AI jobCtx，最多等待 90 秒完成落库、run 收口、Redis 清理
	serviceCtx, serviceCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer serviceCancel()
	if err := services.Shutdown(serviceCtx); err != nil {
		log.Printf("service shutdown timed out: %v", err)
		return
	}
	log.Println("server shutdown completed")
}
