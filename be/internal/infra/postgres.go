package infra

import (
	"context"
	"database/sql"
	"net/url"
	"strconv"
	"time"

	"ai-novel-ide/be/internal/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewPostgres(ctx context.Context, cfg config.PostgresConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", postgresDSN(cfg))
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetime != "" {
		lifetime, err := time.ParseDuration(cfg.ConnMaxLifetime)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		db.SetConnMaxLifetime(lifetime)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func postgresDSN(cfg config.PostgresConfig) string {
	values := url.Values{}
	values.Set("sslmode", cfg.SSLMode)

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host + ":" + strconv.Itoa(cfg.Port),
		Path:     cfg.Database,
		RawQuery: values.Encode(),
	}
	return dsn.String()
}
