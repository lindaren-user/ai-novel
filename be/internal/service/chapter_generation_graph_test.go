package service

import (
	"strings"
	"testing"

	"ai-novel-ide/be/internal/model"
)

func TestChapterDraftValidationPromptExcludesWordCountRule(t *testing.T) {
	state := chapterGenerationState{
		WritingContext: chapterWritingContext{
			Payload: model.JSONMap{
				"current_chapter_plan_data": model.JSONMap{"summary": "主角进入旧城调查线索。"},
				"word_count_rule":           chapterWritingWordCountRule,
			},
		},
		Draft: "主角进入旧城调查线索。",
	}

	generatePrompt := chapterGraphDraftUserPrompt(state)
	if !strings.Contains(generatePrompt, "word_count_rule") {
		t.Fatalf("generate prompt should keep word_count_rule as writing requirement: %s", generatePrompt)
	}

	validatePrompt := chapterDraftValidationUserPrompt(state)
	if strings.Contains(validatePrompt, "word_count_rule") || strings.Contains(validatePrompt, chapterWritingWordCountRule) {
		t.Fatalf("validation prompt should not include word_count_rule: %s", validatePrompt)
	}
}
