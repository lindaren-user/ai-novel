package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUserExists 用户名已存在。
var ErrUserExists = errors.New("user already exists")

// ErrEmailExists 邮箱已被注册。
var ErrEmailExists = errors.New("email already exists")

// ErrUserNotFound 用户不存在
var ErrUserNotFound = errors.New("user not found")

// ErrSettingsNotFound 用户设置不存在
var ErrSettingsNotFound = errors.New("settings not found")

// ErrNovelNotFound 小说不存在
var ErrNovelNotFound = errors.New("novel not found")

// ErrVolumeNotFound 卷不存在
var ErrVolumeNotFound = errors.New("volume not found")

// ErrChapterNotFound 章节不存在
var ErrChapterNotFound = errors.New("chapter not found")

// ErrChatSessionNotFound 对话会话不存在
var ErrChatSessionNotFound = errors.New("chat session not found")

// ErrModelExists 模型名称已存在
var ErrModelExists = errors.New("model already exists")

// ErrModelNotFound 模型不存在
var ErrModelNotFound = errors.New("model not found")

// ErrForbidden 资源归属不匹配。
var ErrForbidden = errors.New("forbidden")

// wrapDBError 为数据库错误补充 repo 动作上下文；哨兵错误在调用处直接返回，不包裹。
func wrapDBError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

// UpdateFields 表示 repo 层按 ID 更新的字段集合；字段名必须由各 repo 的白名单显式允许。
type UpdateFields map[string]any

type updateFieldSpec struct {
	assignment func(placeholder int) string
	normalize  func(value any) (any, error)
}

func columnUpdateField(column string) updateFieldSpec {
	return updateFieldSpec{
		assignment: func(placeholder int) string {
			return fmt.Sprintf("%s = $%d", column, placeholder)
		},
	}
}

func smallintUpdateField(column string) updateFieldSpec {
	return updateFieldSpec{
		assignment: func(placeholder int) string {
			return fmt.Sprintf("%s = $%d::smallint", column, placeholder)
		},
	}
}

func jsonbUpdateField(column string) updateFieldSpec {
	return updateFieldSpec{
		assignment: func(placeholder int) string {
			return fmt.Sprintf("%s = $%d::jsonb", column, placeholder)
		},
		normalize: func(value any) (any, error) {
			payload, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return string(payload), nil
		},
	}
}

func idPayloadJSON(ids []int64) (string, error) {
	payload := make([]map[string]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			payload = append(payload, map[string]int64{"id": id})
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func execUpdateFields(ctx context.Context, db DBTX, table string, id int64, fields UpdateFields, allowed map[string]updateFieldSpec, notFoundErr error, action string) error {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%s: unsupported update field %s", action, key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	args := []any{id}
	assignments := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		spec := allowed[key]
		value := fields[key]
		if spec.normalize != nil {
			normalized, err := spec.normalize(value)
			if err != nil {
				return wrapDBError(action, err)
			}
			value = normalized
		}
		args = append(args, value)
		assignments = append(assignments, spec.assignment(len(args)))
	}
	assignments = append(assignments, "updated_at = now()")

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $1", table, strings.Join(assignments, ", "))
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return wrapDBError(action, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapDBError("读取"+action+"结果失败", err)
	}
	if affected == 0 {
		return notFoundErr
	}
	return nil
}

// Repositories 聚合所有 repo 实例
type Repositories struct {
	Users           UserRepository
	Settings        SettingsRepository
	Novels          NovelRepository
	Volumes         VolumeRepository
	Chapters        ChapterRepository
	ChapterContents ChapterContentRepository
	ChatSessions    ChatSessionRepository
	ChatMessages    ChatMessageRepository
	ModelRuns       ModelRunRepository
	ModelConfigs    ModelConfigRepository
	Memberships     MembershipRepository
	Feedbacks       FeedbackRepository
	Transactions    TransactionRunner
}

// NewRepositories 创建所有 repo 的工厂方法
func NewRepositories(db *sql.DB) Repositories {
	return newRepositories(db, &transactionRunner{db: db})
}

// newRepositories 创建指定数据库执行器下的 repo 集合，事务中会复用同一套工厂逻辑。
func newRepositories(q DBTX, transactions TransactionRunner) Repositories {
	return Repositories{
		Users:           NewUserRepository(q),
		Settings:        NewSettingsRepository(q),
		Novels:          NewNovelRepository(q),
		Volumes:         NewVolumeRepository(q),
		Chapters:        NewChapterRepository(q),
		ChapterContents: NewChapterContentRepository(q),
		ChatSessions:    NewChatSessionRepository(q),
		ChatMessages:    NewChatMessageRepository(q),
		ModelRuns:       NewModelRunRepository(q),
		ModelConfigs:    NewModelConfigRepository(q),
		Memberships:     NewMembershipRepository(q),
		Feedbacks:       NewFeedbackRepository(q),
		Transactions:    transactions,
	}
}
