package logging

import (
	"ai-novel-ide/be/internal/config"

	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 根据环境派生出的日志配置创建日志器：开发输出控制台，生产输出文件。
func New(cfg config.LogConfig) (*zap.Logger, error) {
	level := zap.NewAtomicLevel()
	level.SetLevel(resolveLogLevel(cfg))

	zapCfg := zap.NewProductionConfig()
	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.OutputPaths = []string{"stdout"}
		zapCfg.ErrorOutputPaths = []string{"stderr"}
	} else {
		if err := ensureLogDir(cfg.File); err != nil {
			return nil, err
		}
		zapCfg.OutputPaths = []string{cfg.File}
		zapCfg.ErrorOutputPaths = []string{cfg.File}
	}
	zapCfg.Level = level
	zapCfg.Encoding = cfg.Encoding
	zapCfg.DisableCaller = false
	zapCfg.DisableStacktrace = true
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	if cfg.Encoding == "console" {
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	return zapCfg.Build()
}

// Init 初始化全局日志器，日志细节由全局 env 统一派生。
func Init(cfg config.LogConfig) (*zap.Logger, error) {
	logger, err := New(cfg)
	if err != nil {
		return nil, err
	}
	zap.ReplaceGlobals(logger)
	return logger, nil
}

// resolveLogLevel 根据环境派生的日志配置解析级别，开发输出 debug，生产输出 info 及以上。
func resolveLogLevel(cfg config.LogConfig) zapcore.Level {
	if cfg.Development {
		return zapcore.DebugLevel
	}
	levelText := strings.TrimSpace(strings.ToLower(cfg.Level))
	if levelText == "" {
		return zapcore.ErrorLevel
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelText)); err != nil {
		return zapcore.ErrorLevel
	}
	return level
}

// ensureLogDir 确保生产日志文件所在目录存在。
func ensureLogDir(file string) error {
	dir := filepath.Dir(file)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
