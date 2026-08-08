package service

import (
	"strings"
	"testing"

	"ai-novel-ide/be/internal/model"
)

func TestBuildNovelSettingIndexAndValidateReferences(t *testing.T) {
	planData := model.JSONMap{
		"characters": []any{
			map[string]any{"name": "林默", "notes": "主角"},
		},
		"maps": []any{
			map[string]any{"name": "怪谈学院", "notes": "主场景"},
		},
		"forces": []any{
			map[string]any{"name": "管理局", "notes": "官方组织"},
		},
		"other_settings": []any{
			map[string]any{
				"title": "时间线设定",
				"items": []any{
					map[string]any{"name": "主时间线", "notes": "第2年3月至5月"},
				},
			},
		},
		"temporary_settings": map[string]any{
			"maps": []any{map[string]any{"name": "卷临时地点"}},
		},
	}

	index := buildNovelSettingIndex(planData)
	for _, name := range []string{"林默", "怪谈学院", "管理局", "时间线设定", "主时间线"} {
		if _, ok := index[name]; !ok {
			t.Fatalf("expected setting %q in index", name)
		}
	}
	if _, ok := index["卷临时地点"]; ok {
		t.Fatal("temporary settings must not be indexed")
	}

	_, err := completeChapterPlanReferences([]ChapterPlan{{
		Title:      "入局",
		Summary:    "林默进入怪谈学院，沿主时间线推进到第2年5月。",
		references: []string{"林默", "怪谈学院", "主时间线"},
	}}, index)
	if err != nil {
		t.Fatalf("expected valid references, got %v", err)
	}
}

func TestValidateChapterPlanReferencesRejectsInvalidData(t *testing.T) {
	index := buildNovelSettingIndex(model.JSONMap{
		"characters": []any{map[string]any{"name": "林默"}},
	})

	cases := []struct {
		name string
		plan ChapterPlan
		want string
	}{
		{
			name: "summary without settings",
			plan: ChapterPlan{Summary: "主角行动。"},
			want: "references 不能为空",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := completeChapterPlanReferences([]ChapterPlan{tc.plan}, index)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCompleteChapterPlanReferencesAddsMentionedSettings(t *testing.T) {
	index := buildNovelSettingIndex(model.JSONMap{
		"characters": []any{
			map[string]any{"name": "林墨"},
			map[string]any{"name": "血影"},
		},
		"maps":   []any{map[string]any{"name": "东荒山脉"}},
		"forces": []any{map[string]any{"name": "血煞宗"}},
	})

	plans, err := completeChapterPlanReferences([]ChapterPlan{{
		Summary:    "林墨16岁初夏，在东荒山脉得知血影已率血煞宗小队围剿自己。",
		references: []string{"林墨"},
	}}, index)
	if err != nil {
		t.Fatalf("expected completed references to pass validation, got %v", err)
	}
	refs := stringSet(plans[0].references)
	for _, name := range []string{"林墨", "血影", "东荒山脉", "血煞宗"} {
		if _, ok := refs[name]; !ok {
			t.Fatalf("expected reference %q, got %#v", name, plans[0].references)
		}
	}
}

func TestReferencedRelationshipsOnlyIncludesAppearedCharacters(t *testing.T) {
	planData := model.JSONMap{
		"characters": []any{
			map[string]any{"name": "林默"},
			map[string]any{"name": "许知夏"},
			map[string]any{"name": "周衡"},
		},
		"relationships": []any{
			map[string]any{"character_a": "char_001", "character_b": "char_002", "description": "同伴"},
			map[string]any{"character_a": "char_001", "character_b": "char_003", "description": "旧识"},
		},
	}
	index := buildNovelSettingIndex(planData)
	settings, err := referencedSettingsForChapter(model.Chapter{PlanData: model.JSONMap{
		"references": []any{"林默", "许知夏"},
	}}, index)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	relationships := referencedRelationshipsForChapter(model.Chapter{}, planData, settings)
	if len(relationships) != 1 {
		t.Fatalf("expected one relationship, got %#v", relationships)
	}
	if relationships[0].NameA != "林默" || relationships[0].NameB != "许知夏" {
		t.Fatalf("unexpected relationship: %#v", relationships[0])
	}
}

func TestReferencedSettingsSystemContextOnlyContainsReferencedSettings(t *testing.T) {
	contextText, err := referencedSettingsSystemContext([]referencedSetting{{
		Name:           "林默",
		AppearanceTime: "前期",
		Category:       "character",
		Notes:          "主角",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(contextText, "referenced_settings") {
		t.Fatal("expected referenced_settings payload")
	}
	if strings.Contains(contextText, "novel_plan_data") || strings.Contains(contextText, "plan_data") {
		t.Fatalf("context must not expose complete novel plan data: %s", contextText)
	}
	if strings.Contains(contextText, `"data"`) {
		t.Fatalf("context must not nest referenced setting data: %s", contextText)
	}
	if !strings.Contains(contextText, `"appearance_time":"前期"`) || !strings.Contains(contextText, `"notes":"主角"`) {
		t.Fatalf("context must expose flattened setting fields: %s", contextText)
	}
}
