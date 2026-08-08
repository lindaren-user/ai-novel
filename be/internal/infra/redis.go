package infra

import (
	"context"
	"strconv"
	"time"

	"ai-novel-ide/be/internal/config"

	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	dialTimeout, err := time.ParseDuration(cfg.DialTimeout)
	if err != nil {
		return nil, err
	}
	readTimeout, err := time.ParseDuration(cfg.ReadTimeout)
	if err != nil {
		return nil, err
	}
	writeTimeout, err := time.ParseDuration(cfg.WriteTimeout)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Host + ":" + strconv.Itoa(cfg.Port),
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:     cfg.PoolSize,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
