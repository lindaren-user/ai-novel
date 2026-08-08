package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
)

type VolumePlan struct {
	Title                string            `json:"title" jsonschema:"description=卷标题，不要包含第几卷序号；内容中不要使用未转义英文双引号"`
	Summary              string            `json:"summary,omitempty" jsonschema:"description=本卷梗概；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	Timeline             string            `json:"timeline,omitempty" jsonschema:"description=本卷所处的具体时间段或时间线位置，必须与前后卷连续；内容中不要使用未转义英文双引号"`
	Locations            []string          `json:"locations,omitempty" jsonschema:"description=本卷主要地点列表，只写已确认地点或本卷 temporary_settings.maps 中的临时地点"`
	Characters           []string          `json:"characters,omitempty" jsonschema:"description=本卷主要人物列表，只写已确认人物或本卷 temporary_settings.characters 中的临时人物"`
	CurrentState         string            `json:"current_state,omitempty" jsonschema:"description=本卷开始时人物、地点、势力、关键设定和伏笔所处的当前状态；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	EndState             string            `json:"end_state,omitempty" jsonschema:"description=本卷结束时必须推进到的人物、地点、势力、关键设定和伏笔状态；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	CharacterDevelopment string            `json:"character_development,omitempty" jsonschema:"description=本卷人物弧光，只写人物目标、关系、认知、立场、能力使用方式或心理变化，不写世界规则；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	SettingDevelopment   string            `json:"setting_development,omitempty" jsonschema:"description=本卷客观设定演变，只写世界规则、魔法/修仙/异能/科技体系、地点规则、势力格局、资源、制度、代价、禁忌或能力体系变化，不写人物成长；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	SettingBoundaries    []SettingBoundary `json:"setting_boundaries,omitempty" jsonschema:"description=本卷涉及到的人物、地点、势力、时间线或其他设定边界；必须包含本卷所处时间线边界，没在本卷出场或变化的设定不要强行写入"`
	TemporarySettings    NovelSetupInput   `json:"temporary_settings,omitempty" jsonschema:"description=本卷新增且仅限本卷使用的临时设定，字段结构必须与新建小说表单一致；没有临时设定时输出空对象，不允许把重要规则或体系藏在 summary 中"`
	ChapterCount         int               `json:"chapter_count,omitempty" jsonschema:"description=本卷应规划的章节数量，必须为正整数；所有卷的 chapter_count 总和必须符合用户 length_range，后续卷级章节规划会严格按这个数量生成章节"`
	KeyEvents            []string          `json:"key_events,omitempty" jsonschema:"description=本卷关键事件列表"`
	Foreshadowing        string            `json:"foreshadowing,omitempty" jsonschema:"description=为后续埋下的伏笔或隐藏线索；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	OtherHighlights      string            `json:"other_highlights,omitempty" jsonschema:"description=其他重点要素；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	SortOrder            int               `json:"sort_order,omitempty" jsonschema:"description=卷排序，从 1 开始；如果省略则按数组顺序保存"`
}

type VolumePlanPresentation struct {
	Title                string            `json:"title" jsonschema:"description=卷标题，不要包含第几卷序号；不超过 12 个字"`
	Summary              string            `json:"summary" jsonschema:"description=本卷梗概，100 到 150 个中文字符；不要写成长篇正文"`
	Timeline             string            `json:"timeline" jsonschema:"description=本卷所处具体时间段，必须与前后卷连续"`
	Locations            []string          `json:"locations" jsonschema:"description=本卷主要地点列表，2 到 6 项"`
	Characters           []string          `json:"characters" jsonschema:"description=本卷主要人物列表，3 到 8 项"`
	CurrentState         string            `json:"current_state" jsonschema:"description=本卷开始时人物、地点、势力、关键设定和伏笔的当前状态，120 到 200 个中文字符"`
	EndState             string            `json:"end_state" jsonschema:"description=本卷结束时必须推进到的人物、地点、势力、关键设定和伏笔状态，120 到 200 个中文字符"`
	CharacterDevelopment string            `json:"character_development" jsonschema:"description=本卷人物弧光，只写人物目标、关系、认知、立场、能力使用方式或心理变化，120 到 200 个中文字符"`
	SettingDevelopment   string            `json:"setting_development" jsonschema:"description=本卷客观设定演变，只写世界规则、魔法/修仙/异能/科技体系、地点规则、势力格局、资源、制度、代价、禁忌或能力体系变化，120 到 200 个中文字符"`
	SettingBoundaries    []SettingBoundary `json:"setting_boundaries" jsonschema:"description=本卷涉及到的设定边界列表，必须包含本卷所处时间线边界，只写本卷会出场、会变化或会被限制的设定"`
	TemporarySettings    NovelSetupInput   `json:"temporary_settings,omitempty" jsonschema:"description=本卷新增且仅限本卷使用的临时设定，结构与新建小说表单一致；没有则输出空对象"`
	ChapterCount         int               `json:"chapter_count" jsonschema:"description=本卷应规划的章节数量，必须为正整数；所有批次合计后必须符合用户 length_range"`
	KeyEvents            []string          `json:"key_events" jsonschema:"description=本卷关键事件列表，2 到 5 项，每项不超过 30 个中文字符"`
	Foreshadowing        string            `json:"foreshadowing" jsonschema:"description=后续伏笔或隐藏线索，不超过 80 个中文字符"`
	OtherHighlights      string            `json:"other_highlights" jsonschema:"description=其他重点要素，不超过 80 个中文字符"`
}

type SavedVolume struct {
	ID        int64          `json:"id"`
	PlanData  map[string]any `json:"plan_data"`
	SortOrder int            `json:"sort_order"`
}

type ChapterPlan struct {
	Title             string `json:"title" jsonschema:"description=章节标题，不要包含第几章序号；内容中不要使用未转义英文双引号"`
	Summary           string `json:"summary" jsonschema:"description=普通文本事件骨架，不设固定字数，以清楚、完整、干脆为准；开局必须先写具体时间和具体地点，再写具体人物姓名、人物状态、做了什么、导致什么变化；必须达到什么时间、什么地点、什么人、什么状态、做了什么事、引起什么变化的基本详细程度；禁止用两人、三人、众人、几人、一行人、他们等模糊称呼代替具体人物；禁止生成Markdown语法，禁止加粗、列表或标题；禁止用等等、类似、一系列、相关、大概、若干等模糊词代替具体事物；禁止正文式描写、外貌堆叠、气氛铺陈、规则全文、批注全文或靠细节撑长度；正文生成会完全依据summary展开，不得给正文阶段留下自行新增人物或事件的空间；内容中不要使用未转义英文双引号"`
	IntertextualLinks string `json:"intertextual_links,omitempty" jsonschema:"description=本章与前后章节的跨章关联，包含伏笔、前文呼应、后文铺垫或待回收线索；没有则写无；内容中不要使用未转义英文双引号，引用名称请用中文引号"`
	WritingFocus      string `json:"writing_focus,omitempty" jsonschema:"description=正文写作时要重点表现的语言质感、动作、冲突、情绪、节奏和信息释放方式，必须具体可写，不要空泛；内容中不要使用未转义英文双引号"`
	SortOrder         int    `json:"sort_order,omitempty" jsonschema:"description=章节排序，从 1 开始；如果省略则按数组顺序保存"`
	references        []string
}

type SettingBoundary struct {
	Name              string `json:"name" jsonschema:"description=设定名称，必须来自已出现或本层实际涉及的设定；时间线边界写主时间线或对应时间线名称"`
	StateBefore       string `json:"state_before" jsonschema:"description=本层开始时该设定的状态；如果是时间线边界，写开始具体时间点或时间段；内容中不要使用未转义英文双引号"`
	StateAfter        string `json:"state_after" jsonschema:"description=本层结束时该设定的状态；如果是时间线边界，写结束具体时间点或时间段；内容中不要使用未转义英文双引号"`
	AllowedProgress   string `json:"allowed_progress,omitempty" jsonschema:"description=本层允许推进或揭示的范围；如果是时间线边界，写允许推进的时间范围；内容中不要使用未转义英文双引号"`
	ForbiddenProgress string `json:"forbidden_progress,omitempty" jsonschema:"description=本层禁止提前揭示、禁止回滚或禁止越界的范围；如果是时间线边界，写禁止跳到的后续时间或禁止回滚的前序时间；内容中不要使用未转义英文双引号"`
}

type SavedChapter struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	SortOrder int    `json:"sort_order"`
}

type presentPlanOutput struct {
	Kind       string           `json:"kind"`
	OptionType string           `json:"optionType"`
	Options    []map[string]any `json:"options"`
}

type presentVolumePlanInput struct {
	Items []VolumePlanPresentation `json:"items" jsonschema:"description=本批分卷规划列表，建议每批 4 到 5 个；多批调用会由前端合并展示，只包含展示卡片需要的字段"`
}

type presentChapterPlanInput struct {
	Items []ChapterPlan `json:"items" jsonschema:"description=本批章节规划列表，建议每批 4 到 5 个；多批调用会由前端合并展示"`
}

type ChapterDraftPlan struct {
	// Title 是草稿卡片标题，不是最终章节标题的强制覆盖值。
	Title string `json:"title" jsonschema:"description=章节标题"`
	// RevisionNotes 是给用户看的本次写作重点或改写说明，不承载正文内容。
	RevisionNotes string `json:"revision_notes" jsonschema:"description=本次正文重点或改写说明"`
}

type presentChapterDraftInput struct {
	// Draft 只用于初始化草稿卡片元信息；正文必须在工具调用结束后继续用普通助手文本输出。
	Draft ChapterDraftPlan `json:"draft" jsonschema:"description=章节正文草稿"`
}

type presentChapterDraftOutput struct {
	Kind  string         `json:"kind"`
	Draft map[string]any `json:"draft"`
}

// toolRepairMiddleware 将普通工具错误转换为工具结果，让模型根据错误提示自行修正参数并重试。
type toolRepairMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

// newPresentVolumePlanTool 创建分卷规划展示工具，只返回 UI 渲染数据，不写入数据库。
func newPresentVolumePlanTool() (tool.BaseTool, error) {
	return utils.InferTool("present_volume_plan", "分批展示全书分卷规划卡片；每批 4 到 5 卷，只用于前端 A2UI 渲染，不保存数据库",
		func(ctx context.Context, in *presentVolumePlanInput) (*presentPlanOutput, error) {
			plans := make([]VolumePlan, 0, len(in.Items))
			for _, item := range in.Items {
				plans = append(plans, volumePresentationToPlan(item))
			}
			volumes := normalizeVolumePlans(plans)
			options := make([]map[string]any, 0, len(volumes))
			for i, item := range volumes {
				option := planOptionJSON{
					"title":                 item.Title,
					"summary":               item.Summary,
					"timeline":              item.Timeline,
					"locations":             item.Locations,
					"characters":            item.Characters,
					"current_state":         item.CurrentState,
					"end_state":             item.EndState,
					"character_development": item.CharacterDevelopment,
					"setting_development":   item.SettingDevelopment,
					"setting_boundaries":    item.SettingBoundaries,
					"temporary_settings":    item.TemporarySettings,
					"chapter_count":         item.ChapterCount,
					"key_events":            item.KeyEvents,
					"foreshadowing":         item.Foreshadowing,
					"other_highlights":      item.OtherHighlights,
				}
				options = append(options, normalizeA2UIPlanOption(option, i, "volume"))
			}
			return &presentPlanOutput{Kind: "plan_options", OptionType: "volume", Options: options}, nil
		})
}

// volumePresentationToPlan 将展示工具的精简卷规划转换为内部通用卷规划。
func volumePresentationToPlan(item VolumePlanPresentation) VolumePlan {
	return VolumePlan{
		Title:                item.Title,
		Summary:              item.Summary,
		Timeline:             item.Timeline,
		Locations:            item.Locations,
		Characters:           item.Characters,
		CurrentState:         item.CurrentState,
		EndState:             item.EndState,
		CharacterDevelopment: item.CharacterDevelopment,
		SettingDevelopment:   item.SettingDevelopment,
		SettingBoundaries:    item.SettingBoundaries,
		TemporarySettings:    item.TemporarySettings,
		ChapterCount:         item.ChapterCount,
		KeyEvents:            item.KeyEvents,
		Foreshadowing:        item.Foreshadowing,
		OtherHighlights:      item.OtherHighlights,
	}
}

// newPresentChapterPlanTool 创建章节规划展示工具，只返回 UI 渲染数据，不写入数据库。
func newPresentChapterPlanTool() (tool.BaseTool, error) {
	return utils.InferTool("present_chapter_plan", "分批展示当前卷章节规划卡片；每批 4 到 5 章，只用于前端 A2UI 渲染，不保存数据库",
		func(ctx context.Context, in *presentChapterPlanInput) (*presentPlanOutput, error) {
			chapters := normalizeChapterPlans(in.Items)
			options := make([]map[string]any, 0, len(chapters))
			for i, item := range chapters {
				option := planOptionJSON{
					"title":              item.Title,
					"summary":            item.Summary,
					"intertextual_links": item.IntertextualLinks,
					"writing_focus":      item.WritingFocus,
				}
				options = append(options, normalizeA2UIPlanOption(option, i, "chapter"))
			}
			return &presentPlanOutput{Kind: "plan_options", OptionType: "chapter", Options: options}, nil
		})
}

// newPresentChapterDraftTool 创建章节正文草稿展示工具。
//
// 这个工具只负责打开前端草稿卡片，并把标题、写作说明放进卡片元信息。
// 它不会接收正文，也不会保存正文；正文会在工具调用结束后，由模型继续
// 通过普通 assistant delta 输出，forwardAIStream 再把这些 delta 写入卡片正文。
func newPresentChapterDraftTool() (tool.BaseTool, error) {
	return utils.InferTool("present_chapter_draft", "打开章节正文草稿渲染容器；只传标题和写作说明，正文必须在工具调用后用普通助手文本继续输出",
		func(ctx context.Context, in *presentChapterDraftInput) (*presentChapterDraftOutput, error) {
			// 这里只整理卡片头部信息；正文内容不在工具参数里传，避免大段正文被工具 JSON 包住。
			draft := normalizeChapterDraftPlan(in.Draft)
			return &presentChapterDraftOutput{
				Kind:  "chapter_draft",
				Draft: chapterDraftRenderMap(draft),
			}, nil
		})
}

// newToolRepairMiddleware 创建工具自纠中间件；中断恢复类错误仍交给 Eino 原流程处理。
func newToolRepairMiddleware() adk.ChatModelAgentMiddleware {
	return &toolRepairMiddleware{BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{}}
}

// WrapInvokableToolCall 包装同步工具调用，把非中断错误作为可读 tool result 返回给模型。
func (m *toolRepairMiddleware) WrapInvokableToolCall(_ context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err == nil {
			return result, nil
		}
		if _, ok := compose.IsInterruptRerunError(err); ok {
			return "", err
		}
		return toolRepairMessage(tCtx, err), nil
	}, nil
}

// toolRepairMessage 生成给模型看的工具错误提示，要求模型修正参数后重新调用。
func toolRepairMessage(tCtx *adk.ToolContext, err error) string {
	toolName := ""
	if tCtx != nil {
		toolName = tCtx.Name
	}
	if toolName == "" {
		toolName = "unknown"
	}
	return fmt.Sprintf("[tool error] 工具 %s 执行失败：%v。请检查工具名称、JSON 参数结构、字段类型、字符串转义和数量限制，修正后重新调用正确工具；不要向用户复述本错误。", toolName, err)
}

// normalizeChapterPlans 规范化章节规划，过滤空章节并补齐标题和排序。
func normalizeChapterPlans(chapters []ChapterPlan) []ChapterPlan {
	normalized := make([]ChapterPlan, 0, len(chapters))
	for _, chapter := range chapters {
		title := strings.TrimSpace(chapter.Title)
		summary := strings.TrimSpace(chapter.Summary)
		if title == "" && summary == "" {
			continue
		}
		if title == "" {
			title = fmt.Sprintf("第%d章", len(normalized)+1)
		}
		normalized = append(normalized, ChapterPlan{
			Title:             title,
			Summary:           summary,
			IntertextualLinks: strings.TrimSpace(chapter.IntertextualLinks),
			WritingFocus:      strings.TrimSpace(chapter.WritingFocus),
			SortOrder:         len(normalized) + 1,
		})
	}
	return normalized
}

// trimStringList 去除字符串列表中的空白项，保证工具入库内容干净。
func trimStringList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

// normalizeVolumePlans 规范化分卷规划，过滤空卷并补齐标题和排序。
func normalizeVolumePlans(volumes []VolumePlan) []VolumePlan {
	normalized := make([]VolumePlan, 0, len(volumes))
	for _, volume := range volumes {
		title := strings.TrimSpace(volume.Title)
		summary := strings.TrimSpace(volume.Summary)
		if title == "" && summary == "" {
			continue
		}
		if title == "" {
			title = fmt.Sprintf("第%d卷", len(normalized)+1)
		}
		normalized = append(normalized, VolumePlan{
			Title:                title,
			Summary:              summary,
			Timeline:             strings.TrimSpace(volume.Timeline),
			Locations:            trimStringList(volume.Locations),
			Characters:           trimStringList(volume.Characters),
			CurrentState:         strings.TrimSpace(volume.CurrentState),
			EndState:             strings.TrimSpace(volume.EndState),
			CharacterDevelopment: strings.TrimSpace(volume.CharacterDevelopment),
			SettingDevelopment:   strings.TrimSpace(volume.SettingDevelopment),
			SettingBoundaries:    normalizeSettingBoundaries(volume.SettingBoundaries),
			TemporarySettings:    volume.TemporarySettings,
			ChapterCount:         volume.ChapterCount,
			KeyEvents:            trimStringList(volume.KeyEvents),
			Foreshadowing:        strings.TrimSpace(volume.Foreshadowing),
			OtherHighlights:      strings.TrimSpace(volume.OtherHighlights),
			SortOrder:            len(normalized) + 1,
		})
	}
	return normalized
}

// normalizeSettingBoundaries 规范化设定边界列表，过滤没有名称和状态的空项。
func normalizeSettingBoundaries(boundaries []SettingBoundary) []SettingBoundary {
	normalized := make([]SettingBoundary, 0, len(boundaries))
	for _, boundary := range boundaries {
		name := strings.TrimSpace(boundary.Name)
		stateBefore := strings.TrimSpace(boundary.StateBefore)
		stateAfter := strings.TrimSpace(boundary.StateAfter)
		if name == "" && stateBefore == "" && stateAfter == "" {
			continue
		}
		normalized = append(normalized, SettingBoundary{
			Name:              name,
			StateBefore:       stateBefore,
			StateAfter:        stateAfter,
			AllowedProgress:   strings.TrimSpace(boundary.AllowedProgress),
			ForbiddenProgress: strings.TrimSpace(boundary.ForbiddenProgress),
		})
	}
	return normalized
}
