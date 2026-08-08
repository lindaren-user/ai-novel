package service

import (
	"encoding/json"
	"strings"

	"ai-novel-ide/be/internal/model"
)

var novelWritingProfileTagGroups = []string{"题材", "类型", "基调", "文风", "雷点"}

// novelWritingProfileFromPlanData 提取全书写作画像，供卷规划、章节正文和校验共用。
func novelWritingProfileFromPlanData(planData model.JSONMap) model.JSONMap {
	profile := model.JSONMap{
		"perspective": jsonMapText(planData, "perspective"),
		"tag_groups":  novelWritingTagGroupsFromPlanData(planData),
	}
	return profile
}

// novelWritingTagGroupsFromPlanData 从小说规划中读取固定五类标签，缺失时补空数组保持提示词结构稳定。
func novelWritingTagGroupsFromPlanData(planData model.JSONMap) model.JSONMap {
	groups := model.JSONMap{}
	rawGroups, ok := planData["tag_groups"]
	if !ok || rawGroups == nil {
		for _, group := range novelWritingProfileTagGroups {
			groups[group] = []string{}
		}
		return groups
	}
	groupMap := jsonObject(rawGroups)
	for _, group := range novelWritingProfileTagGroups {
		groups[group] = stringListFromAny(groupMap[group])
	}
	return groups
}

// novelWritingProfileSystemContext 将写作画像序列化为系统上下文，强调叙述视角、标签用途和雷点边界。
func novelWritingProfileSystemContext(profile model.JSONMap) string {
	raw, err := json.Marshal(model.JSONMap{"novel_writing_profile": profile})
	if err != nil {
		return ""
	}
	return "全书写作画像如下，正文必须持续遵守；perspective 控制叙述视角，题材/类型/基调用于作品方向和节奏预期，文风用于正文语言质感，雷点是规避约束而不是正向卖点：\n" + string(raw)
}

// jsonObject 将 JSONMap 或普通 map 统一成 map[string]any，其他类型按空对象处理。
func jsonObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case model.JSONMap:
		return map[string]any(typed)
	default:
		return map[string]any{}
	}
}

// stringListFromAny 将 JSON 反序列化后的数组值整理为去空白的字符串列表。
func stringListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return trimStringList(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(jsonValueText(item))
			if text != "" {
				values = append(values, text)
			}
		}
		return trimStringList(values)
	default:
		return []string{}
	}
}
