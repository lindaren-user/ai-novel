package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"ai-novel-ide/be/internal/model"
)

// UserRepository 用户数据访问接口
type UserRepository interface {
	Create(ctx context.Context, username string, email string, passwordHash string) (model.User, error)
	FindByUsername(ctx context.Context, username string) (model.User, error)
	FindByEmail(ctx context.Context, email string) (model.User, error)
	FindByID(ctx context.Context, id int64) (model.User, error)
	Update(ctx context.Context, id int64, fields UpdateFields) error
}

type userRepository struct {
	db DBTX
}

type userRowScanner interface {
	Scan(dest ...any) error
}

type userFindField string

const (
	userFindFieldID       userFindField = "id"
	userFindFieldUsername userFindField = "username"
	userFindFieldEmail    userFindField = "email"
)

// NewUserRepository 创建用户 repo
func NewUserRepository(db DBTX) UserRepository {
	return &userRepository{db: db}
}

// Create 创建用户记录，邮箱用于验证码登录和账号找回。
func (r *userRepository) Create(ctx context.Context, username string, email string, passwordHash string) (model.User, error) {
	var user model.User
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO t_users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, username, email, membership_plan_id, password_hash, status, deactivated_at, created_at, updated_at
	`, username, email, passwordHash).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.MembershipPlanID,
		&user.PasswordHash,
		&user.Status,
		&user.DeactivatedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "t_users_email_key") {
			return model.User{}, ErrEmailExists
		}
		if strings.Contains(err.Error(), "duplicate key") ||
			strings.Contains(err.Error(), "t_users_username_key") {
			return model.User{}, ErrUserExists
		}
		return model.User{}, wrapDBError("插入用户失败", err)
	}
	return user, nil
}

// FindByUsername 根据用户名查找用户
func (r *userRepository) FindByUsername(ctx context.Context, username string) (model.User, error) {
	user, err := r.findActiveUserBy(ctx, userFindFieldUsername, username)
	return user, wrapDBError("按用户名查询用户失败", err)
}

// FindByEmail 根据邮箱查找正常状态用户。
func (r *userRepository) FindByEmail(ctx context.Context, email string) (model.User, error) {
	user, err := r.findActiveUserBy(ctx, userFindFieldEmail, strings.ToLower(strings.TrimSpace(email)))
	return user, wrapDBError("按邮箱查询用户失败", err)
}

// FindByID 根据 ID 查找用户
func (r *userRepository) FindByID(ctx context.Context, id int64) (model.User, error) {
	user, err := r.findActiveUserBy(ctx, userFindFieldID, id)
	return user, wrapDBError("按 ID 查询用户失败", err)
}

// findActiveUserBy 按白名单字段构造同一条活跃用户查询 SQL。
func (r *userRepository) findActiveUserBy(ctx context.Context, field userFindField, value any) (model.User, error) {
	column, ok := userFindColumn(field)
	if !ok {
		return model.User{}, ErrUserNotFound
	}
	query := `
		SELECT id, username, COALESCE(email, ''), membership_plan_id, password_hash, status, deactivated_at, created_at, updated_at
		FROM t_users
		WHERE ` + column + ` = $1 AND status = $2
	`
	user, err := scanUserRow(r.db.QueryRowContext(ctx, query, value, model.UserStatusNormal))
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrUserNotFound
	}
	return user, err
}

// userFindColumn 将可查询字段映射为数据库列名，避免把外部输入拼进 SQL。
func userFindColumn(field userFindField) (string, bool) {
	switch field {
	case userFindFieldID:
		return "id", true
	case userFindFieldUsername:
		return "username", true
	case userFindFieldEmail:
		return "email", true
	default:
		return "", false
	}
}

// scanUserRow 将用户查询结果扫描为用户模型，避免不同查询重复维护字段顺序。
func scanUserRow(row userRowScanner) (model.User, error) {
	var user model.User
	var deactivatedAt sql.NullTime
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.MembershipPlanID,
		&user.PasswordHash,
		&user.Status,
		&deactivatedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if deactivatedAt.Valid {
		user.DeactivatedAt = &deactivatedAt.Time
	}
	return user, err
}

// Update 按 ID 更新用户白名单字段，不包含业务判断。
func (r *userRepository) Update(ctx context.Context, id int64, fields UpdateFields) error {
	return execUpdateFields(ctx, r.db, "t_users", id, fields, userUpdateFields, ErrUserNotFound, "更新用户失败")
}

var userUpdateFields = map[string]updateFieldSpec{
	"username":           columnUpdateField("username"),
	"email":              columnUpdateField("email"),
	"membership_plan_id": columnUpdateField("membership_plan_id"),
	"password_hash":      columnUpdateField("password_hash"),
	"status":             smallintUpdateField("status"),
	"deactivated_at":     columnUpdateField("deactivated_at"),
}
