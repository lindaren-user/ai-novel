package repo

import (
	"context"
	"database/sql"
)

// DBTX 是 sql.DB 和 sql.Tx 共有的最小查询能力。
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// TransactionRunner 统一管理事务边界。
type TransactionRunner interface {
	WithinTx(ctx context.Context, fn func(Repositories) error) error
}

type transactionRunner struct {
	db *sql.DB
}

func (r *transactionRunner) WithinTx(ctx context.Context, fn func(Repositories) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return wrapDBError("开启数据库事务失败", err)
	}

	if err := fn(newRepositories(tx, r)); err != nil {
		_ = tx.Rollback()
		return wrapDBError("执行数据库事务失败", err)
	}

	return wrapDBError("提交数据库事务失败", tx.Commit())
}
