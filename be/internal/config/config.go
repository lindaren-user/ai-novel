package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigFile = "config.yaml"

const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

type Config struct {
	Env      string         `yaml:"env"`
	HTTP     HTTPConfig     `yaml:"http"`
	Pprof    PprofConfig    `yaml:"pprof"`
	Postgres PostgresConfig `yaml:"postgres"`
	Redis    RedisConfig    `yaml:"redis"`
	Log      LogConfig      `yaml:"-"`
	Mail     MailConfig     `yaml:"mail"`
	Auth     AuthConfig     `yaml:"auth"`
	Storage  StorageConfig  `yaml:"storage"`
}

type HTTPConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// PprofConfig 控制 Go 运行时性能分析服务；生产环境建议仅通过 SSH 隧道访问。
type PprofConfig struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"`
}

type PostgresConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Database        string `yaml:"database"`
	SSLMode         string `yaml:"sslMode"`
	MaxOpenConns    int    `yaml:"maxOpenConns"`
	MaxIdleConns    int    `yaml:"maxIdleConns"`
	ConnMaxLifetime string `yaml:"connMaxLifetime"`
}

type RedisConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	DialTimeout  string `yaml:"dialTimeout"`
	ReadTimeout  string `yaml:"readTimeout"`
	WriteTimeout string `yaml:"writeTimeout"`
	PoolSize     int    `yaml:"poolSize"`
}

type LogConfig struct {
	Level       string
	Encoding    string
	Development bool
	File        string
}

type AuthConfig struct {
	Secret    string          `yaml:"secret"`
	Turnstile TurnstileConfig `yaml:"turnstile"`
}

type TurnstileConfig struct {
	SiteKey   string `yaml:"siteKey"`
	SecretKey string `yaml:"secretKey"`
}

type MailConfig struct {
	Provider string       `yaml:"provider"`
	SMTP     SMTPConfig   `yaml:"smtp"`
	Resend   ResendConfig `yaml:"resend"`
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type ResendConfig struct {
	ApiKey string `yaml:"api_key"`
	From   string `yaml:"from"`
}
type StorageConfig struct {
	S3 S3StorageConfig `yaml:"s3"`
}

type S3StorageConfig struct {
	Endpoint      string `yaml:"endpoint"`
	Region        string `yaml:"region"`
	Bucket        string `yaml:"bucket"`
	AccessKey     string `yaml:"accessKey"`
	SecretKey     string `yaml:"secretKey"`
	PublicBaseURL string `yaml:"publicBaseUrl"`
	Prefix        string `yaml:"prefix"`
}

func Load() (Config, error) {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		path = defaultConfigFile
	}
	log.Println(path)

	cfg := defaultConfig()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && path == defaultConfigFile {
			return cfg, nil
		}
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	return cfg, nil
}

func defaultConfig() Config {
	cfg := Config{}
	cfg.applyDefaults()
	return cfg
}

func (c *Config) applyDefaults() {
	c.Env = strings.TrimSpace(strings.ToLower(c.Env))
	if c.Env == "" {
		c.Env = EnvDev
	}
	if c.Env != EnvProd {
		c.Env = EnvDev
	}
	if c.HTTP.Host == "" {
		c.HTTP.Host = "127.0.0.1"
	}
	if c.HTTP.Port == 0 {
		c.HTTP.Port = 8080
	}
	if c.Pprof.Addr == "" {
		c.Pprof.Addr = "127.0.0.1:6060"
	}
	if c.Postgres.Host == "" {
		c.Postgres.Host = "localhost"
	}
	if c.Postgres.Port == 0 {
		c.Postgres.Port = 5432
	}
	if c.Postgres.User == "" {
		c.Postgres.User = "ai_novel"
	}
	if c.Postgres.Password == "" {
		c.Postgres.Password = "ai_novel_password"
	}
	if c.Postgres.Database == "" {
		c.Postgres.Database = "ai_novel_ide"
	}
	if c.Postgres.SSLMode == "" {
		c.Postgres.SSLMode = "disable"
	}
	if c.Postgres.MaxOpenConns == 0 {
		c.Postgres.MaxOpenConns = 20
	}
	if c.Postgres.MaxIdleConns == 0 {
		c.Postgres.MaxIdleConns = 5
	}
	if c.Postgres.ConnMaxLifetime == "" {
		c.Postgres.ConnMaxLifetime = "30m"
	}
	if c.Redis.Host == "" {
		c.Redis.Host = "localhost"
	}
	if c.Redis.Port == 0 {
		c.Redis.Port = 6379
	}
	if c.Redis.DialTimeout == "" {
		c.Redis.DialTimeout = "5s"
	}
	if c.Redis.ReadTimeout == "" {
		c.Redis.ReadTimeout = "3s"
	}
	if c.Redis.WriteTimeout == "" {
		c.Redis.WriteTimeout = "3s"
	}
	if c.Redis.PoolSize == 0 {
		c.Redis.PoolSize = 10
	}
	c.Log = logConfigFromEnv(c.Env)
	if c.Auth.Secret == "" {
		c.Auth.Secret = "dev-auth-secret"
	}
	if c.Storage.S3.Region == "" {
		c.Storage.S3.Region = "auto"
	}
	if c.Storage.S3.Prefix == "" {
		c.Storage.S3.Prefix = "uploads"
	}
}

func logConfigFromEnv(env string) LogConfig {
	if env == EnvProd {
		return LogConfig{
			Level:       "info",
			Encoding:    "json",
			Development: false,
			File:        "logs/app.log",
		}
	}
	return LogConfig{
		Level:       "debug",
		Encoding:    "console",
		Development: true,
	}
}

func (c HTTPConfig) ListenAddr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}
