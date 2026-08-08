package service

import (
	"errors"
	"fmt"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// buildStreamAgentDefinition 使用 Anthropic Routing 模式，按业务范围分派到对应 Agent。
func buildStreamAgentDefinition(params streamScopeParams) (ai.AgentDefinition, error) {
	switch params.ScopeType {
	case model.ScopeTypeNovel:
		return buildNovelAgentDefinition(params)
	case model.ScopeTypeVolume:
		return buildVolumeAgentDefinition(params)
	case model.ScopeTypeChapter:
		return buildChapterAgentDefinition(params)
	default:
		return ai.AgentDefinition{}, errors.New("invalid ai stream scope")
	}
}

// buildNovelAgentDefinition 创建小说级 Agent 定义，只挂载分卷规划展示工具。
func buildNovelAgentDefinition(params streamScopeParams) (ai.AgentDefinition, error) {
	volumePlanTool, err := newPresentVolumePlanTool()
	if err != nil {
		return ai.AgentDefinition{}, err
	}
	return ai.AgentDefinition{
		Name:        "novel_assistant",
		Description: "小说结构规划助手，负责规划整体设定与卷结构",
		Instruction: novelSysPrompt,
		CacheKey:    fmt.Sprintf("novel:%d", params.ScopeID),
		Middlewares: []adk.ChatModelAgentMiddleware{newToolRepairMiddleware()},
		Tools: []tool.BaseTool{
			volumePlanTool,
		},
	}, nil
}

// buildVolumeAgentDefinition 创建卷级 Agent 定义，只挂载章节规划展示工具。
func buildVolumeAgentDefinition(params streamScopeParams) (ai.AgentDefinition, error) {
	presentChapterTool, err := newPresentChapterPlanTool()
	if err != nil {
		return ai.AgentDefinition{}, err
	}
	return ai.AgentDefinition{
		Name:        "volume_assistant",
		Description: "卷结构规划助手，负责规划章节数量、节奏和剧情分布",
		Instruction: volumeSysPrompt,
		CacheKey:    fmt.Sprintf("volume:%d", params.ScopeID),
		Middlewares: []adk.ChatModelAgentMiddleware{newToolRepairMiddleware()},
		Tools:       []tool.BaseTool{presentChapterTool},
	}, nil
}

// buildChapterAgentDefinition 创建章级 Agent 定义，只负责生成正文草稿。
func buildChapterAgentDefinition(params streamScopeParams) (ai.AgentDefinition, error) {
	mode := params.ChapterWritingMode
	if mode == "" {
		mode = chapterWritingModeInteractive
	}
	instruction, err := chapterWritingSystemPrompt(mode)
	if err != nil {
		return ai.AgentDefinition{}, err
	}
	definition := ai.AgentDefinition{
		Name:        "chapter_assistant",
		Description: "章节写作助手，负责生成正文",
		Instruction: instruction,
		CacheKey:    fmt.Sprintf("chapter:%s", mode),
		Middlewares: []adk.ChatModelAgentMiddleware{newToolRepairMiddleware()},
	}
	if !params.ChapterDraftToolDisabled {
		presentDraftTool, err := newPresentChapterDraftTool()
		if err != nil {
			return ai.AgentDefinition{}, err
		}
		definition.Tools = []tool.BaseTool{presentDraftTool}
	}
	if params.ChapterSkill != nil {
		definition.Middlewares = append(definition.Middlewares, params.ChapterSkill)
	}
	return definition, nil
}

// buildStoryHumanizerAgentDefinition 创建单章节 AI 消痕 Agent 定义，并挂载故事编辑技能中间件和按需参考资料工具。
func buildStoryHumanizerAgentDefinition(storyEditSkill adk.ChatModelAgentMiddleware) ai.AgentDefinition {
	definition := ai.AgentDefinition{
		Name:        "story_humanizer",
		Description: "单章节 AI 消痕助手",
		Instruction: storyHumanizerSysPrompt,
		CacheKey:    "story-humanizer",
	}
	if storyEditSkill != nil {
		definition.Middlewares = []adk.ChatModelAgentMiddleware{newToolRepairMiddleware(), storyEditSkill}
	} else {
		definition.Middlewares = []adk.ChatModelAgentMiddleware{newToolRepairMiddleware()}
	}
	return definition
}
