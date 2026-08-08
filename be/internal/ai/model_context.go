package ai

import "strings"

const (
	defaultContextWindowTokens = 128000
	minSummarizationReserve    = 4096
)

// ContextWindowTokens 根据模型厂商和模型 ID 返回上下文窗口大小。
//
// 这里维护系统内置和常用自定义厂商的常见上下文窗口；用户自定义模型无法可靠
// 从 OpenAI 兼容接口反查窗口时，会按厂商默认值兜底，避免完全没有压缩保护。
func ContextWindowTokens(provider string, modelID string) int {
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if tokens := contextWindowByModelID(modelID); tokens > 0 {
		return tokens
	}
	switch provider {
	case "deepseek":
		return 1048576
	case "gemini":
		return 1048576
	case "claude":
		return 200000
	case "qianwen":
		return 131072
	case "kimi":
		return 131072
	case "doubao":
		return 131072
	case "grok":
		return 131072
	case "gpt", "custom_openai":
		return defaultContextWindowTokens
	default:
		return defaultContextWindowTokens
	}
}

// SummarizationTriggerTokens 根据上下文窗口计算摘要触发阈值。
//
// 阈值保留约 20% 余量给工具定义、系统指令和模型输出，避免刚好到窗口上限才
// 压缩导致真实请求仍然超限。
func SummarizationTriggerTokens(provider string, modelID string, configuredWindow int) int {
	window := configuredWindow
	if window <= 0 {
		window = ContextWindowTokens(provider, modelID)
	}
	reserve := window / 5
	if reserve < minSummarizationReserve {
		reserve = minSummarizationReserve
	}
	trigger := window - reserve
	if trigger < minSummarizationReserve {
		return window
	}
	return trigger
}

// contextWindowByModelID 通过模型 ID 中的窗口提示和常见模型名称匹配上下文窗口。
func contextWindowByModelID(modelID string) int {
	switch {
	case modelID == "":
		return 0
	case strings.Contains(modelID, "deepseek-v4"):
		return 1048576
	case strings.Contains(modelID, "deepseek-chat"), strings.Contains(modelID, "deepseek-reasoner"):
		return 1048576
	case strings.Contains(modelID, "gpt-4.1"):
		return 1047576
	case strings.Contains(modelID, "gpt-4o"), strings.Contains(modelID, "o1"), strings.Contains(modelID, "o3"), strings.Contains(modelID, "o4"):
		return 128000
	case strings.Contains(modelID, "gemini"):
		return 1048576
	case strings.Contains(modelID, "claude"):
		return 200000
	case strings.Contains(modelID, "1m"), strings.Contains(modelID, "1000k"), strings.Contains(modelID, "1024k"):
		return 1000000
	case strings.Contains(modelID, "256k"):
		return 262144
	case strings.Contains(modelID, "200k"):
		return 200000
	case strings.Contains(modelID, "128k"):
		return 131072
	case strings.Contains(modelID, "64k"):
		return 65536
	case strings.Contains(modelID, "32k"):
		return 32768
	case strings.Contains(modelID, "16k"):
		return 16384
	case strings.Contains(modelID, "8k"):
		return 8192
	default:
		return 0
	}
}
