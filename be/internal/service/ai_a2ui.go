package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"
)

const (
	renderKindPlanOptions     = "plan_options" // 规划选项
	renderKindChapterDraft    = "chapter_draft"
	renderKindChapterProgress = "chapter_generation_progress"
	renderKindSetupStep       = "novel_setup_step"
	renderKindSetupComplete   = "novel_setup_complete"
)

type planOptionJSON map[string]any

// renderDataFromToolResult 将展示类工具结果转换为消息渲染数据。
func renderDataFromToolResult(toolName string, raw string) (model.JSONMap, bool, error) {
	if !isPresentationTool(toolName) {
		return nil, false, nil
	}
	if isToolRepairResult(raw) {
		return nil, false, nil
	}
	var data model.JSONMap
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, true, err
	}
	return data, true, nil
}

// loadingRenderDataForPresentationTool 在工具刚开始调用时生成 A2UI 占位数据。
func loadingRenderDataForPresentationTool(toolName string) (model.JSONMap, bool) {
	switch toolName {
	case "present_volume_plan":
		return model.JSONMap{
			"kind":       renderKindPlanOptions,
			"optionType": "volume",
			"options":    []any{},
		}, true
	case "present_chapter_plan":
		return model.JSONMap{
			"kind":       renderKindPlanOptions,
			"optionType": "chapter",
			"options":    []any{},
		}, true
	case "present_chapter_draft":
		return model.JSONMap{
			"kind":  renderKindChapterDraft,
			"draft": map[string]any{},
		}, true
	default:
		return nil, false
	}
}

// isToolRepairResult 判断工具结果是否是自纠中间件返回给模型的错误说明。
func isToolRepairResult(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "[tool error]")
}

// isPresentationTool 判断工具结果是否只用于前端结构化展示。
func isPresentationTool(toolName string) bool {
	switch toolName {
	case "present_volume_plan", "present_chapter_plan", "present_chapter_draft":
		return true
	default:
		return false
	}
}

// a2uiSetupEvent 将 setup 展示工具结果包装为统一 A2UI 事件。
func a2uiSetupEvent(renderData model.JSONMap) ai.StreamEvent {
	return ai.NewStreamA2UI(planString(renderData["kind"]), renderData)
}

// a2uiRenderEvent 将渲染数据包装成统一 A2UI 事件。
func a2uiRenderEvent(renderData model.JSONMap) (ai.StreamEvent, bool) {
	if len(renderData) == 0 {
		return ai.StreamEvent{}, false
	}
	return ai.NewStreamA2UI(planString(renderData["kind"]), renderData), true
}

// mergeRenderData 合并最新渲染数据；同类规划卡片按批追加，章节草稿等其他展示仍直接替换。
func mergeRenderData(current model.JSONMap, next model.JSONMap) model.JSONMap {
	if len(next) == 0 {
		return current
	}
	if canAppendPlanRenderData(current, next) {
		return appendPlanRenderData(current, next)
	}
	return next
}

// canAppendPlanRenderData 判断两个渲染数据是否属于同一类规划列表。
func canAppendPlanRenderData(current model.JSONMap, next model.JSONMap) bool {
	return current["kind"] == renderKindPlanOptions &&
		next["kind"] == renderKindPlanOptions &&
		planString(current["optionType"]) == planString(next["optionType"])
}

// appendPlanRenderData 将分批展示工具返回的规划卡片追加到当前消息渲染数据中。
func appendPlanRenderData(current model.JSONMap, next model.JSONMap) model.JSONMap {
	merged := model.JSONMap{}
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range next {
		if key != "options" {
			merged[key] = value
		}
	}
	optionType := planString(next["optionType"])
	options := append(planOptionsFromRenderData(current), planOptionsFromRenderData(next)...)
	merged["optionType"] = optionType
	merged["options"] = normalizeMergedPlanOptions(options, optionType)
	return merged
}

// planOptionsFromRenderData 从渲染数据中读取规划卡片，兼容内存对象和 JSON 反序列化后的对象。
func planOptionsFromRenderData(renderData model.JSONMap) []map[string]any {
	rawOptions, ok := renderData["options"].([]map[string]any)
	if ok {
		return rawOptions
	}
	rawList, ok := renderData["options"].([]any)
	if !ok {
		return nil
	}
	options := make([]map[string]any, 0, len(rawList))
	for _, raw := range rawList {
		option, ok := raw.(map[string]any)
		if ok {
			options = append(options, option)
		}
	}
	return options
}

// normalizeMergedPlanOptions 去重并重新编号，避免多批工具调用产生重复 id。
func normalizeMergedPlanOptions(options []map[string]any, optionType string) []map[string]any {
	normalized := make([]map[string]any, 0, len(options))
	seen := map[string]struct{}{}
	for _, option := range options {
		key := planOptionMergeKey(option)
		if key != "" {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		next := map[string]any{}
		for field, value := range option {
			next[field] = value
		}
		next["id"] = fmt.Sprintf("p%d", len(normalized))
		normalized = append(normalized, next)
	}
	return normalized
}

// planOptionMergeKey 构造规划卡片去重键，优先使用标题和梗概避免重复展示同一批内容。
func planOptionMergeKey(option map[string]any) string {
	title := planString(option["title"])
	description := planString(option["description"])
	if title == "" {
		details, _ := option["details"].(map[string]any)
		title = planString(details["title"])
		description = firstPlanString(description, details["summary"], details["core_conflict"])
	}
	return strings.TrimSpace(title + "\n" + description)
}

// normalizeChapterDraftPlan 规范化章节正文草稿，去除空段落并补齐展示字段。
func normalizeChapterDraftPlan(draft ChapterDraftPlan) ChapterDraftPlan {
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		title = "章节正文"
	}
	return ChapterDraftPlan{
		Title:         title,
		RevisionNotes: strings.TrimSpace(draft.RevisionNotes),
	}
}

// chapterDraftRenderMap 将章节草稿转成前端可直接消费的渲染对象。
func chapterDraftRenderMap(draft ChapterDraftPlan) map[string]any {
	return map[string]any{
		"title":          strings.TrimSpace(draft.Title),
		"content":        "",
		"revision_notes": strings.TrimSpace(draft.RevisionNotes),
	}
}

// setChapterDraftStreamContent 将普通文本流写入章节草稿渲染数据。
// chapter_draft 不维护额外状态字段；前端用流状态和 draftId 判断是否可操作。
func setChapterDraftStreamContent(renderData model.JSONMap, content string) model.JSONMap {
	if renderData["kind"] != renderKindChapterDraft {
		return renderData
	}
	next := model.JSONMap{}
	for key, value := range renderData {
		next[key] = value
	}
	draft, _ := next["draft"].(map[string]any)
	if draft == nil {
		draft = map[string]any{}
	}
	nextDraft := map[string]any{}
	for key, value := range draft {
		nextDraft[key] = value
	}
	text := strings.TrimSpace(content)
	nextDraft["content"] = text
	next["draft"] = nextDraft
	return next
}

// setChapterDraftID 将持久化后的草稿公开 ID 写回章节草稿渲染数据。
func setChapterDraftID(renderData model.JSONMap, draftID int64) model.JSONMap {
	if draftID <= 0 || renderData["kind"] != renderKindChapterDraft {
		return renderData
	}
	next := model.JSONMap{}
	for key, value := range renderData {
		next[key] = value
	}
	draft, _ := next["draft"].(map[string]any)
	if draft == nil {
		draft = map[string]any{}
	}
	nextDraft := map[string]any{}
	for key, value := range draft {
		nextDraft[key] = value
	}
	nextDraft["draft_id"] = draftID
	next["draft"] = nextDraft
	return next
}

// extractChapterDraftContentFromRenderData 从消息渲染数据中提取正文草稿内容。
func extractChapterDraftContentFromRenderData(renderData model.JSONMap) string {
	if renderData["kind"] != renderKindChapterDraft {
		return ""
	}
	draft, ok := renderData["draft"].(map[string]any)
	if !ok {
		return ""
	}
	if content, ok := draft["content"].(string); ok && strings.TrimSpace(content) != "" {
		return strings.TrimSpace(content)
	}
	return ""
}

// normalizeA2UIPlanOption 将任意规划对象归一成前端既有 PlanOption 结构。
func normalizeA2UIPlanOption(option planOptionJSON, index int, optionType string) map[string]any {
	title := planString(option["title"])
	if title == "" {
		title = fmt.Sprintf("选项 %d", index+1)
	}
	description := firstPlanString(option["summary"], option["core_conflict"])
	if optionType == "volume" {
		description = firstPlanString(option["summary"])
	} else if optionType == "chapter" {
		description = firstPlanString(option["summary"])
	}
	return map[string]any{
		"id":          fmt.Sprintf("p%d", index),
		"title":       title,
		"description": description,
		"details":     map[string]any(option),
	}
}

// planString 将规划字段转换为展示字符串。
func planString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// firstPlanString 返回第一个非空规划文本。
func firstPlanString(values ...any) string {
	for _, value := range values {
		if text := planString(value); text != "" {
			return text
		}
	}
	return ""
}
