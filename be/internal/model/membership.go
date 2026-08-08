package model

import "time"

const (
	MembershipPlanStatusEnabled  int16 = 1 // 会员计划启用，可绑定和读取权益。
	MembershipPlanStatusDisabled int16 = 2 // 会员计划禁用，不再对用户生效。
)

type MembershipPlan struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Level     int16     `json:"level"`
	Features  JSONMap   `json:"features"`
	Status    int16     `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
