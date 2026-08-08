package model

const (
	// CodeOK 表示请求成功。
	CodeOK = 0

	// CodeInvalidRequest 表示请求参数、请求体或路径 ID 不正确。
	CodeInvalidRequest = 10001
	// CodeUnauthorized 表示用户未登录、登录态无效或登录凭据错误。
	CodeUnauthorized = 10002
	// CodeForbidden 表示用户无权访问目标资源。
	CodeForbidden = 10003
	// CodeResourceNotFound 表示目标资源不存在。
	CodeResourceNotFound = 10004
	// CodeConflict 表示当前请求与已有资源或业务状态冲突。
	CodeConflict = 10005
	// CodeRateLimited 表示请求触发频率或并发限制。
	CodeRateLimited = 10006

	// CodeInvalidCredentials 表示用户名、密码或验证码不正确。
	CodeInvalidCredentials = 20001
	// CodeUsernameTaken 表示用户名已被占用。
	CodeUsernameTaken = 20002
	// CodeInvalidSettings 表示用户设置格式不正确。
	CodeInvalidSettings = 21001
	// CodeInvalidModel 表示模型配置不正确。
	CodeInvalidModel = 22001
	// CodeModelTaken 表示模型名称已存在。
	CodeModelTaken = 22002
	// CodeInvalidMessage 表示消息或正文内容不正确。
	CodeInvalidMessage = 23001
	// CodeInvalidFeedback 表示反馈内容不正确。
	CodeInvalidFeedback = 24001
	// CodeInvalidFile 表示上传文件不符合要求。
	CodeInvalidFile = 25001
	// CodeFileStorageUnavailable 表示文件存储服务不可用。
	CodeFileStorageUnavailable = 25002
	// CodeSharePasswordRequired 表示访问分享内容需要密钥。
	CodeSharePasswordRequired = 26001
	// CodeInvalidSharePassword 表示分享密钥不正确。
	CodeInvalidSharePassword = 26002

	// CodeAIUnavailable 表示 AI 服务不可用或返回无效结果。
	CodeAIUnavailable = 30001
	// CodeConcurrentStreamLimit 表示 AI 流式任务并发数超过限制。
	CodeConcurrentStreamLimit = 30003

	// CodeInternalError 表示服务端内部错误。
	CodeInternalError = 50001
)

// 统一响应结构体：code=0 表示成功，非0表示具体业务错误码。
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}
