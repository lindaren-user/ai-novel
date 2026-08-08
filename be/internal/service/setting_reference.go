package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-novel-ide/be/internal/model"

	"github.com/redis/go-redis/v9"
)

const referencedSettingsCacheTTL = 2 * 24 * time.Hour

type referencedSetting struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	AppearanceTime string `json:"appearance_time"`
	Category       string `json:"category"`
	Notes          string `json:"notes"`
}

type referencedRelationship struct {
	CharacterA  string `json:"character_a"`
	CharacterB  string `json:"character_b"`
	NameA       string `json:"name_a"`
	NameB       string `json:"name_b"`
	Description string `json:"description"`
}

type chapterReferencedContext struct {
	Settings      []referencedSetting      `json:"settings"`
	Relationships []referencedRelationship `json:"relationships"`
}

type settingIndex map[string]referencedSetting

// buildNovelSettingIndex 将小说全局 plan_data 扁平化为按名称索引的设定库，不读取卷临时设定。
func buildNovelSettingIndex(planData model.JSONMap) settingIndex {
	index := settingIndex{}
	addNamedSettings(index, "character", planData["characters"])
	addNamedSettings(index, "map", planData["maps"])
	addNamedSettings(index, "force", planData["forces"])
	addOtherSettings(index, planData["other_settings"])
	return index
}

// addNamedSettings 将带 name/title 的设定数组加入索引。
func addNamedSettings(index settingIndex, category string, value any) {
	for i, item := range jsonArray(value) {
		data, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstNonEmptyText(
			jsonValueText(data["name"]),
			jsonValueText(data["title"]),
		)
		id := jsonValueText(data["id"])
		if id == "" && category == "character" {
			id = fmt.Sprintf("char_%03d", i+1)
		}
		addSetting(index, referencedSettingFromData(id, name, category, data))
	}
}

// addOtherSettings 将其它设定分组及其子项加入索引。
func addOtherSettings(index settingIndex, value any) {
	for _, item := range jsonArray(value) {
		setting, ok := item.(map[string]any)
		if !ok {
			continue
		}
		groupName := firstNonEmptyText(
			jsonValueText(setting["title"]),
			jsonValueText(setting["name"]),
		)
		addSetting(index, referencedSettingFromData("", groupName, "other_setting_group", setting))
		for _, rawChild := range jsonArray(setting["items"]) {
			child, ok := rawChild.(map[string]any)
			if !ok {
				continue
			}
			childName := firstNonEmptyText(
				jsonValueText(child["name"]),
				jsonValueText(child["title"]),
			)
			addSetting(index, referencedSettingFromData("", childName, "other_setting", child))
		}
	}
}

// referencedSettingFromData 从结构化 plan_data 提取可引用设定。
func referencedSettingFromData(id string, name string, category string, data map[string]any) referencedSetting {
	return referencedSetting{
		ID:             id,
		Name:           name,
		AppearanceTime: jsonValueText(data["appearance_time"]),
		Category:       category,
		Notes: firstNonEmptyText(
			jsonValueText(data["notes"]),
			jsonValueText(data["description"]),
		),
	}
}

// addSetting 按名称和 ID 将设定写入索引。
func addSetting(index settingIndex, setting referencedSetting) {
	setting.Name = strings.TrimSpace(setting.Name)
	if setting.Name == "" {
		return
	}
	index[setting.Name] = setting
	if setting.ID != "" {
		index[setting.ID] = setting
	}
}

// jsonArray 将 JSON 数组值统一转为 []any。
func jsonArray(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []model.JSONMap:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, map[string]any(item))
		}
		return items
	default:
		return nil
	}
}

// normalizeReferences 清理 references 的空值和重复值。
func normalizeReferences(values []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

// completeChapterPlanReferences 根据章节摘要中出现的设定补全 references。
func completeChapterPlanReferences(plans []ChapterPlan, index settingIndex) ([]ChapterPlan, error) {
	completed := make([]ChapterPlan, len(plans))
	for i, plan := range plans {
		completed[i] = plan
		refs := make([]string, 0)
		refSet := stringSet(refs)
		for _, setting := range settingsMentionedInSummary(plan.Summary, index) {
			if _, ok := refSet[setting.Name]; ok {
				continue
			}
			if setting.ID != "" {
				if _, ok := refSet[setting.ID]; ok {
					continue
				}
			}
			refs = append(refs, setting.Name)
			refSet[setting.Name] = struct{}{}
		}
		if len(refs) == 0 {
			return nil, fmt.Errorf("第%d章 references 不能为空，必须列出本章出现的小说全局设定名称", i+1)
		}
		completed[i].references = refs
	}
	return completed, nil
}

// settingsMentionedInSummary 查找摘要中直接提到的全局设定。
func settingsMentionedInSummary(summary string, index settingIndex) []referencedSetting {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	settings := make([]referencedSetting, 0)
	seen := map[string]struct{}{}
	for key, setting := range index {
		if key != setting.Name || setting.Name == "" {
			continue
		}
		if !strings.Contains(summary, setting.Name) {
			continue
		}
		if _, ok := seen[setting.Name]; ok {
			continue
		}
		seen[setting.Name] = struct{}{}
		settings = append(settings, setting)
	}
	return settings
}

// referencedSettingsForChapter 按章节 references 取出本章需要加载的全局设定。
func referencedSettingsForChapter(chapter model.Chapter, index settingIndex) ([]referencedSetting, error) {
	refs := chapterReferences(chapter)
	if len(refs) == 0 {
		return nil, fmt.Errorf("章节缺少 references，无法按需加载小说设定")
	}
	settings := make([]referencedSetting, 0, len(refs))
	for _, ref := range refs {
		setting, ok := index[ref]
		if !ok {
			return nil, fmt.Errorf("章节 references 包含不存在的小说全局设定：%s", ref)
		}
		settings = append(settings, setting)
	}
	return settings, nil
}

// referencedRelationshipsForChapter 只返回本章已引用人物之间的关系。
func referencedRelationshipsForChapter(chapter model.Chapter, planData model.JSONMap, settings []referencedSetting) []referencedRelationship {
	appeared := map[string]referencedSetting{}
	for _, setting := range settings {
		if setting.Category != "character" {
			continue
		}
		appeared[setting.Name] = setting
		if setting.ID != "" {
			appeared[setting.ID] = setting
		}
	}
	if len(appeared) == 0 {
		return nil
	}
	relationships := make([]referencedRelationship, 0)
	for _, item := range jsonArray(planData["relationships"]) {
		data, ok := item.(map[string]any)
		if !ok {
			continue
		}
		characterA := firstNonEmptyText(jsonValueText(data["character_a"]), jsonValueText(data["characterA"]))
		characterB := firstNonEmptyText(jsonValueText(data["character_b"]), jsonValueText(data["characterB"]))
		settingA, okA := appeared[characterA]
		settingB, okB := appeared[characterB]
		if !okA || !okB {
			continue
		}
		relationships = append(relationships, referencedRelationship{
			CharacterA:  characterA,
			CharacterB:  characterB,
			NameA:       settingA.Name,
			NameB:       settingB.Name,
			Description: jsonValueText(data["description"]),
		})
	}
	return relationships
}

// chapterReferences 从章节 plan_data 中读取并规范化 references。
func chapterReferences(chapter model.Chapter) []string {
	return normalizeReferences(jsonMapStringSlice(chapter.PlanData, "references"))
}

// stringSet 将字符串切片转换为集合。
func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// jsonMapStringSlice 从 JSONMap 中读取字符串数组字段。
func jsonMapStringSlice(values model.JSONMap, key string) []string {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text := jsonValueText(item)
			if text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// referencedSettingsCacheKey 生成章节引用上下文缓存键。
func referencedSettingsCacheKey(userID int64, chapterID int64) string {
	return fmt.Sprintf("user:%d:chapter:%d:referenced_settings", userID, chapterID)
}

// cacheChapterReferencedContext 缓存章节引用上下文。
func cacheChapterReferencedContext(ctx context.Context, redisClient *redis.Client, userID int64, chapterID int64, context chapterReferencedContext) {
	if redisClient == nil || userID <= 0 || chapterID <= 0 {
		return
	}
	payload, err := json.Marshal(context)
	if err != nil {
		return
	}
	_ = redisClient.Set(ctx, referencedSettingsCacheKey(userID, chapterID), payload, referencedSettingsCacheTTL).Err()
}

// cachedChapterReferencedContext 读取章节引用上下文缓存。
func cachedChapterReferencedContext(ctx context.Context, redisClient *redis.Client, userID int64, chapterID int64) (chapterReferencedContext, bool) {
	if redisClient == nil || userID <= 0 || chapterID <= 0 {
		return chapterReferencedContext{}, false
	}
	payload, err := redisClient.Get(ctx, referencedSettingsCacheKey(userID, chapterID)).Bytes()
	if err != nil || len(payload) == 0 {
		return chapterReferencedContext{}, false
	}
	var context chapterReferencedContext
	if err := json.Unmarshal(payload, &context); err != nil {
		return chapterReferencedContext{}, false
	}
	return context, true
}

// deleteReferencedSettingsCache 删除单章引用上下文缓存。
func deleteReferencedSettingsCache(ctx context.Context, redisClient *redis.Client, userID int64, chapterID int64) {
	if redisClient == nil || userID <= 0 || chapterID <= 0 {
		return
	}
	_ = redisClient.Del(ctx, referencedSettingsCacheKey(userID, chapterID)).Err()
}

// deleteVolumeReferencedSettingsCaches 删除一组章节的引用上下文缓存。
func deleteVolumeReferencedSettingsCaches(ctx context.Context, redisClient *redis.Client, userID int64, chapters []model.Chapter) {
	for _, chapter := range chapters {
		deleteReferencedSettingsCache(ctx, redisClient, userID, chapter.ID)
	}
}
