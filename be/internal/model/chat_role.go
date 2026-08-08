package model

const (
	// ChatRoleUser 表示用户消息。
	ChatRoleUser int16 = 1
	// ChatRoleAssistant 表示助手消息。
	ChatRoleAssistant int16 = 2
	// ChatRoleSystem 表示系统消息。
	ChatRoleSystem int16 = 3
)

// ChatRoleName 将数据库角色枚举转为前端展示用字符串。
func ChatRoleName(role int16) string {
	switch role {
	case ChatRoleUser:
		return "user"
	case ChatRoleAssistant:
		return "assistant"
	case ChatRoleSystem:
		return "system"
	default:
		return "unknown"
	}
}
