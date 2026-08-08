package repo

import (
	"context"
	"database/sql"
	"errors"

	"ai-novel-ide/be/internal/model"
)

// ErrMembershipPlanNotFound 表示用户绑定的会员计划不存在或未启用。
var ErrMembershipPlanNotFound = errors.New("membership plan not found")

// MembershipRepository 负责读取会员计划及其权益配置。
type MembershipRepository interface {
	FindPlanByUserID(ctx context.Context, userID int64) (model.MembershipPlan, error)
}

type membershipRepository struct {
	db DBTX
}

// NewMembershipRepository 创建会员计划 repo。
func NewMembershipRepository(db DBTX) MembershipRepository {
	return &membershipRepository{db: db}
}

// FindPlanByUserID 根据用户绑定的会员计划 ID 查询权益配置。
func (r *membershipRepository) FindPlanByUserID(ctx context.Context, userID int64) (model.MembershipPlan, error) {
	var plan model.MembershipPlan
	err := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.name, p.level, p.features, p.status, p.created_at, p.updated_at
		FROM t_users u
		JOIN t_membership_plans p ON p.id = u.membership_plan_id
		WHERE u.id = $1 AND p.status = $2
	`, userID, model.MembershipPlanStatusEnabled).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Level,
		&plan.Features,
		&plan.Status,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.MembershipPlan{}, ErrMembershipPlanNotFound
	}
	if err != nil {
		return model.MembershipPlan{}, wrapDBError("查询用户会员计划失败", err)
	}
	if plan.Features == nil {
		plan.Features = model.JSONMap{}
	}
	return plan, nil
}
