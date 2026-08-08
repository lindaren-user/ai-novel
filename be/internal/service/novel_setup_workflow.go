package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-novel-ide/be/internal/ai"
	"ai-novel-ide/be/internal/model"

	"go.uber.org/zap"
)

// NovelSetupWorkflowInput 承载新建小说模板工作流的原始输入和运行模型。
type NovelSetupWorkflowInput struct {
	UserID      int64
	RawText     string
	ModelKey    string
	ModelConfig ai.ModelRuntimeConfig
}

// NovelSetupWorkflowState 保存新建小说模板工作流的阶段性结构化结果。
type NovelSetupWorkflowState struct {
	UserID      int64
	RawText     string
	ModelKey    string
	ModelConfig ai.ModelRuntimeConfig
	Setup       NovelSetupInput
}

type novelSetupWorkflowStep struct {
	service     *novelService
	name        string
	task        string
	fields      []string
	temperature float64
}

type novelSetupTask struct {
	name        string
	text        string
	fields      []string
	temperature float64
}

const novelSetupStepMaxAttempts = 3

// newNovelSetupWorkflow 使用 Anthropic Prompt Chaining 模式，按固定阶段逐步补全设定。
func (s *novelService) newNovelSetupWorkflow(ctx context.Context) (ai.Workflow[NovelSetupWorkflowInput, NovelSetupInput], error) {
	return ai.NewSequentialWorkflow(ctx, ai.SequentialWorkflowConfig[NovelSetupWorkflowInput, NovelSetupWorkflowState, NovelSetupInput]{
		Name: "novel_setup",
		Init: func(input NovelSetupWorkflowInput) NovelSetupWorkflowState {
			return NovelSetupWorkflowState{
				UserID:      input.UserID,
				RawText:     input.RawText,
				ModelKey:    input.ModelKey,
				ModelConfig: input.ModelConfig,
			}
		},
		Steps:    s.newNovelSetupWorkflowSteps(),
		Finalize: finalizeNovelSetupWorkflow,
	})
}

// newNovelSetupWorkflowSteps 根据固定任务表创建模板生成步骤。
func (s *novelService) newNovelSetupWorkflowSteps() []ai.WorkflowStep[NovelSetupWorkflowState] {
	tasks := novelSetupWorkflowTasks()
	steps := make([]ai.WorkflowStep[NovelSetupWorkflowState], 0, len(tasks))
	for _, task := range tasks {
		steps = append(steps, novelSetupWorkflowStep{
			service:     s,
			name:        task.name,
			task:        task.text,
			fields:      task.fields,
			temperature: task.temperature,
		})
	}
	return steps
}

// Name 返回模板生成步骤的用户可见阶段名称。
func (s novelSetupWorkflowStep) Name() string {
	return s.name
}

// Run 执行单个模板生成步骤，并把当前阶段结果合并到工作流状态。
func (s novelSetupWorkflowStep) Run(ctx context.Context, state NovelSetupWorkflowState) (NovelSetupWorkflowState, error) {
	partial, err := s.generatePartialSetup(ctx, state)
	if err != nil {
		return state, err
	}
	applyNovelSetupStepFields(&state.Setup, partial, s.fields)
	return state, nil
}

// generatePartialSetup 调用模型生成当前阶段 JSON，解析失败时降低随机性重试。
func (s novelSetupWorkflowStep) generatePartialSetup(ctx context.Context, state NovelSetupWorkflowState) (NovelSetupInput, error) {
	var lastErr error
	for attempt := 1; attempt <= novelSetupStepMaxAttempts; attempt++ {
		logNovelSetupAttempt("attempt_start", s.name, attempt, nil)
		runStartedAt := time.Now()
		runID := s.service.createModelRun(ctx, modelRunMeta{
			UserID:    state.UserID,
			ScopeType: model.ModelRunScopeNovelSetup,
			ModelID:   modelIDFromKey(state.ModelKey),
			Status:    model.ModelRunStatusRunning,
			StartTime: runStartedAt,
		})
		callCtx, cancelCall := s.service.aiCallContext(ctx)
		result, err := s.service.aiClient.GenerateChat(callCtx, ai.ChatGenerateRequest{
			UserID:   state.UserID,
			ModelKey: state.ModelKey,
			Model:    s.modelConfigForAttempt(state.ModelConfig, attempt),
			Messages: []ai.StreamMessage{
				{Role: "system", Content: novelSetupWorkflowSystemPrompt()},
				{Role: "user", Content: novelSetupStepPrompt(state.RawText, state.Setup, s.name, s.task, attempt, lastErr)},
			},
		})
		callCanceled := ctx.Err() != nil || callCtx.Err() != nil
		cancelCall()
		if err != nil {
			status := model.ModelRunStatusFailed
			finishReason := ""
			if errors.Is(err, context.Canceled) || callCanceled {
				status = model.ModelRunStatusCanceled
				finishReason = s.service.canceledModelRunFinishReason()
			}
			s.service.finishModelRun(context.Background(), modelRunMeta{
				ID:           runID,
				Status:       status,
				FinishReason: finishReason,
				ErrorMessage: err.Error(),
				EndTime:      timePtr(time.Now()),
			})
			if errors.Is(err, context.Canceled) || callCanceled {
				return NovelSetupInput{}, err
			}
			lastErr = err
			logNovelSetupAttempt("attempt_error", s.name, attempt, err)
			continue
		}
		partial, err := decodeNovelSetupReply(result.Content)
		if err == nil {
			err = validateNovelSetupPartial(s.name, state.RawText, state.Setup, partial)
		}
		if err == nil {
			logNovelSetupAttempt("attempt_done", s.name, attempt, nil)
			s.service.finishModelRun(context.Background(), modelRunMeta{
				ID:           runID,
				Status:       model.ModelRunStatusSuccess,
				TokenCount:   result.TokenCount,
				FinishReason: result.FinishReason,
				EndTime:      timePtr(time.Now()),
			})
			return partial, nil
		}
		s.service.finishModelRun(context.Background(), modelRunMeta{
			ID:           runID,
			Status:       model.ModelRunStatusFailed,
			TokenCount:   result.TokenCount,
			FinishReason: result.FinishReason,
			ErrorMessage: err.Error(),
			EndTime:      timePtr(time.Now()),
		})
		lastErr = err
		logNovelSetupAttempt("attempt_error", s.name, attempt, err)
	}
	return NovelSetupInput{}, lastErr
}

// logNovelSetupAttempt 输出模板生成阶段的单次模型调用状态，便于确认重试是否发生。
func logNovelSetupAttempt(event string, stepName string, attempt int, err error) {
	fields := []zap.Field{
		zap.String("event", event),
		zap.String("workflow", "novel_setup"),
		zap.String("step", stepName),
		zap.Int("attempt", attempt),
		zap.Int("max_attempts", novelSetupStepMaxAttempts),
	}
	if err != nil {
		fields = append(fields, zap.String("error", err.Error()))
	}
	zap.L().Debug("workflow attempt", fields...)
}

// modelConfigForAttempt 根据步骤类型和重试次数调整模型参数。
func (s novelSetupWorkflowStep) modelConfigForAttempt(config ai.ModelRuntimeConfig, attempt int) ai.ModelRuntimeConfig {
	if attempt > 1 {
		config.Temperature = 0.1
		return config
	}
	if s.temperature > 0 {
		config.Temperature = s.temperature
	}
	return config
}

// finalizeNovelSetupWorkflow 归一化最终模板，并校验前端表单必需的核心字段。
func finalizeNovelSetupWorkflow(_ context.Context, _ NovelSetupWorkflowInput, state NovelSetupWorkflowState) (NovelSetupInput, error) {
	setup := normalizeCompletedNovelSetup(state.Setup)
	if strings.TrimSpace(setup.Title) == "" || strings.TrimSpace(setup.Direction) == "" {
		return NovelSetupInput{}, ErrAIUnavailable
	}
	return setup, nil
}

// setupStreamSink 将通用工作流阶段事件转成现有 setup.step A2UI 事件。
type setupStreamSink struct {
	stream chan<- ai.StreamEvent
}

// Step 推送新建小说模板生成的当前阶段。
func (s setupStreamSink) Step(ctx context.Context, text string) {
	sendSetupStreamEvent(ctx, s.stream, a2uiSetupEvent(model.JSONMap{
		"kind":     renderKindSetupStep,
		"text":     strings.TrimSpace(text),
		"complete": false,
	}))
}

// decodeNovelSetupReply 从模型回复中提取严格 JSON 并反序列化为表单片段。
func decodeNovelSetupReply(reply string) (NovelSetupInput, error) {
	var setup NovelSetupInput
	if err := json.Unmarshal([]byte(extractJSONObject(reply)), &setup); err != nil {
		return NovelSetupInput{}, err
	}
	return setup, nil
}

// validateNovelSetupPartial 校验当前阶段确实产出了该阶段需要的结构，避免字段名错误被静默归一化为空。
func validateNovelSetupPartial(stepName string, rawText string, current NovelSetupInput, partial NovelSetupInput) error {
	scale := novelSetupScale(firstNonEmptyText(current.Length, partial.Length))
	switch stepName {
	case "正在读取故事想法":
		if strings.TrimSpace(partial.Length) == "" || strings.TrimSpace(partial.Perspective) == "" {
			return fmt.Errorf("本阶段必须输出 length 和 perspective")
		}
	case "正在识别题材基调":
		if len(partial.TagGroups) == 0 {
			return fmt.Errorf("本阶段必须输出非空 tagGroups")
		}
	case "正在提炼核心主线":
		if strings.TrimSpace(partial.Title) == "" || strings.TrimSpace(partial.Direction) == "" {
			return fmt.Errorf("本阶段必须输出 title 和 direction")
		}
	case "正在补全体系规则":
		return validateSetupOtherSettings(partial, rawText, scale)
	case "正在扩展关键人物":
		if len(partial.Characters) < scale.MinCharacters {
			return fmt.Errorf("本阶段 characters 至少需要 %d 个重点人物", scale.MinCharacters)
		}
		if err := validateSetupNamedItems("characters", len(partial.Characters), func(index int) string {
			return partial.Characters[index].Name
		}); err != nil {
			return err
		}
	case "正在梳理人物关系":
		if len(partial.Relationships) == 0 {
			return fmt.Errorf("本阶段必须输出非空 relationships")
		}
		for i, relationship := range partial.Relationships {
			if strings.TrimSpace(relationship.CharacterA) == "" || strings.TrimSpace(relationship.CharacterB) == "" {
				return fmt.Errorf("relationships[%d] 必须包含 characterA 和 characterB", i)
			}
		}
	case "正在整理地点势力":
		if len(partial.Maps) < scale.MinMaps {
			return fmt.Errorf("本阶段 maps 至少需要 %d 个地点", scale.MinMaps)
		}
		if len(partial.Forces) < scale.MinForces {
			return fmt.Errorf("本阶段 forces 至少需要 %d 个势力", scale.MinForces)
		}
		if err := validateSetupNamedItems("maps", len(partial.Maps), func(index int) string {
			return partial.Maps[index].Name
		}); err != nil {
			return err
		}
		if err := validateSetupNamedItems("forces", len(partial.Forces), func(index int) string {
			return partial.Forces[index].Name
		}); err != nil {
			return err
		}
	}
	return nil
}

// validateSetupNamedItems 校验列表中每个条目都使用 name 字段提供名称。
func validateSetupNamedItems(field string, length int, nameAt func(int) string) error {
	if length == 0 {
		return fmt.Errorf("本阶段必须输出非空 %s", field)
	}
	for i := 0; i < length; i++ {
		if strings.TrimSpace(nameAt(i)) == "" {
			return fmt.Errorf("%s[%d] 必须包含非空 name，不能用 title 代替 name", field, i)
		}
	}
	return nil
}

type novelSetupScaleRule struct {
	MinCharacters    int
	CharacterHint    string
	MinMaps          int
	MapHint          string
	MinForces        int
	ForceHint        string
	MinOtherSettings int
	OtherHint        string
	TimelineHint     string
}

// novelSetupScale 根据篇幅返回设定资产数量要求。
func novelSetupScale(length string) novelSetupScaleRule {
	switch strings.TrimSpace(length) {
	case "短篇":
		return novelSetupScaleRule{
			MinCharacters:    10,
			CharacterHint:    "短篇生成 10 到 14 个全局重点人物",
			MinMaps:          6,
			MapHint:          "短篇生成 6 到 8 个关键地点",
			MinForces:        3,
			ForceHint:        "短篇生成 3 到 5 个势力/组织",
			MinOtherSettings: 5,
			OtherHint:        "短篇生成 5 到 7 类其他设定，每类 3 到 5 个 items",
			TimelineHint:     "短篇主时间线写 4 到 6 个关键节点",
		}
	case "长篇":
		return novelSetupScaleRule{
			MinCharacters:    40,
			CharacterHint:    "长篇生成 40 到 60 个全局重点人物",
			MinMaps:          25,
			MapHint:          "长篇生成 25 到 40 个关键地点",
			MinForces:        15,
			ForceHint:        "长篇生成 15 到 25 个势力/组织",
			MinOtherSettings: 14,
			OtherHint:        "长篇生成 14 到 20 类其他设定，每类 8 到 16 个 items",
			TimelineHint:     "长篇主时间线写 15 到 25 个关键节点",
		}
	default:
		return novelSetupScaleRule{
			MinCharacters:    22,
			CharacterHint:    "中篇生成 22 到 30 个全局重点人物",
			MinMaps:          12,
			MapHint:          "中篇生成 12 到 18 个关键地点",
			MinForces:        8,
			ForceHint:        "中篇生成 8 到 12 个势力/组织",
			MinOtherSettings: 8,
			OtherHint:        "中篇生成 8 到 12 类其他设定，每类 6 到 10 个 items",
			TimelineHint:     "中篇主时间线写 8 到 12 个关键节点",
		}
	}
}

// validateSetupOtherSettings 校验体系规则分组和条目，保证能力、等级、规则等设定不会被空结构吞掉。
func validateSetupOtherSettings(partial NovelSetupInput, rawText string, scale novelSetupScaleRule) error {
	if len(partial.OtherSettings) == 0 {
		return fmt.Errorf("本阶段必须输出非空 other_settings")
	}
	if len(partial.OtherSettings) < scale.MinOtherSettings {
		return fmt.Errorf("本阶段 other_settings 至少需要 %d 类设定", scale.MinOtherSettings)
	}
	if !setupOtherSettingsHasTimeline(partial) {
		return fmt.Errorf("other_settings 必须包含 title 为 时间线设定 或 含有 时间线 的分组")
	}
	if err := validateSetupTimelineCategory(partial, rawText); err != nil {
		return err
	}
	for i, setting := range partial.OtherSettings {
		// isTimelineSetting := strings.Contains(strings.TrimSpace(setting.Title), "时间线")
		if strings.TrimSpace(setting.Title) == "" {
			return fmt.Errorf("other_settings[%d] 必须包含 title", i)
		}
		if len(setting.Items) == 0 {
			return fmt.Errorf("other_settings[%d].items 不能为空", i)
		}
		for j, item := range setting.Items {
			if strings.TrimSpace(item.Name) == "" {
				return fmt.Errorf("other_settings[%d].items[%d] 必须包含非空 name", i, j)
			}
			if strings.TrimSpace(item.AppearanceTime) == "" {
				return fmt.Errorf("other_settings[%d].items[%d] 必须包含 appearanceTime", i, j)
			}
		}
	}
	return nil
}

// validateSetupTimelineCategory 校验时间线是单独大类，正常不能拆成一堆小时间线。
func validateSetupTimelineCategory(partial NovelSetupInput, rawText string) error {
	timelineSettings := 0
	for _, setting := range partial.OtherSettings {
		if !strings.Contains(strings.TrimSpace(setting.Title), "时间线") {
			continue
		}
		timelineSettings++
		if !novelSetupAllowsMultipleTimelines(rawText) && len(setting.Items) != 1 {
			return fmt.Errorf("时间线设定正常只能包含一个主时间线 item，不能把关键节点拆成多个小时间线")
		}
		for _, item := range setting.Items {
			name := strings.TrimSpace(item.Name)
			if !novelSetupAllowsMultipleTimelines(rawText) && !strings.Contains(name, "主时间线") {
				return fmt.Errorf("时间线设定正常只生成一个名为主时间线的 item")
			}
		}
	}
	if timelineSettings != 1 {
		return fmt.Errorf("other_settings 中必须且只能有一个时间线设定大类")
	}
	return nil
}

// novelSetupAllowsMultipleTimelines 判断用户是否明确要求多时间线结构。
func novelSetupAllowsMultipleTimelines(rawText string) bool {
	for _, keyword := range []string{"多时间线", "平行世界", "平行时空", "穿越", "前世今生", "双时代", "前传线", "回忆线", "梦境线", "未来线"} {
		if strings.Contains(rawText, keyword) {
			return true
		}
	}
	return false
}

// setupOtherSettingsHasTimeline 判断体系规则中是否包含全书时间线分组。
func setupOtherSettingsHasTimeline(partial NovelSetupInput) bool {
	for _, setting := range partial.OtherSettings {
		if strings.Contains(strings.TrimSpace(setting.Title), "时间线") && len(setting.Items) > 0 {
			return true
		}
	}
	return false
}

// applyNovelSetupStepFields 将当前阶段负责的字段直接写入模板草稿。
func applyNovelSetupStepFields(target *NovelSetupInput, partial NovelSetupInput, fields []string) {
	for _, field := range fields {
		switch field {
		case "title":
			target.Title = partial.Title
		case "direction":
			target.Direction = partial.Direction
		case "tagGroups":
			target.TagGroups = partial.TagGroups
		case "characters":
			target.Characters = partial.Characters
		case "relationships":
			target.Relationships = partial.Relationships
		case "maps":
			target.Maps = partial.Maps
		case "forces":
			target.Forces = partial.Forces
		case "other_settings":
			target.OtherSettings = partial.OtherSettings
		case "perspective":
			target.Perspective = partial.Perspective
		case "length":
			target.Length = partial.Length
		case "lengthRange":
			target.LengthRange = partial.LengthRange
		}
	}
}

// novelSetupStepPrompt 组装单个阶段的输入，要求模型只输出本阶段 JSON。
func novelSetupStepPrompt(rawText string, current NovelSetupInput, stepName string, task string, attempt int, lastErr error) string {
	currentJSON := "{}"
	if payload, err := json.Marshal(novelSetupRelevantState(stepName, current)); err == nil {
		currentJSON = string(payload)
	}
	if stepName == "正在扩展关键人物" {
		task = fmt.Sprintf("%s；%s。", task, novelSetupScale(current.Length).CharacterHint)
	}
	if stepName == "正在补全体系规则" {
		scale := novelSetupScale(current.Length)
		task = fmt.Sprintf("%s；%s；%s。", task, scale.OtherHint, scale.TimelineHint)
	}
	if stepName == "正在整理地点势力" {
		scale := novelSetupScale(current.Length)
		task = fmt.Sprintf("%s；%s；%s。", task, scale.MapHint, scale.ForceHint)
	}
	retryHint := ""
	if attempt > 1 {
		retryHint = "\n\n上一次输出不符合要求"
		if lastErr != nil {
			retryHint += "：" + lastErr.Error()
		}
		retryHint += "。请只返回严格 JSON 对象；tagGroups 必须是对象，且每个分组的值必须是字符串数组，例如 {\"题材\":[\"玄幻\"],\"类型\":[\"升级流\"]}，不能把分组值写成对象；列表条目的名称字段必须叫 name，不能用 title 代替 name；不要 Markdown、解释、代码块或多余文本。"
	}
	return fmt.Sprintf("用户原始想法：\n%s\n\n当前相关表单 JSON：\n%s\n\n本阶段任务：\n%s%s\n\n只输出一个严格 JSON 对象；可以只包含本阶段产出的字段，不要输出 Markdown、解释或代码块。", rawText, currentJSON, task, retryHint)
}

// novelSetupRelevantState 返回当前阶段真正需要参考的已生成字段，避免 prompt 随步骤膨胀。
func novelSetupRelevantState(stepName string, current NovelSetupInput) model.JSONMap {
	switch stepName {
	case "正在识别题材基调":
		return model.JSONMap{"length": current.Length, "perspective": current.Perspective}
	case "正在提炼核心主线":
		return model.JSONMap{"title": current.Title, "tagGroups": current.TagGroups, "length": current.Length, "perspective": current.Perspective}
	case "正在补全体系规则":
		return model.JSONMap{"title": current.Title, "direction": current.Direction, "tagGroups": current.TagGroups, "length": current.Length, "perspective": current.Perspective}
	case "正在扩展关键人物":
		return model.JSONMap{"title": current.Title, "direction": current.Direction, "tagGroups": current.TagGroups, "length": current.Length, "other_settings": current.OtherSettings}
	case "正在梳理人物关系":
		return model.JSONMap{"characters": current.Characters}
	case "正在整理地点势力":
		return model.JSONMap{"title": current.Title, "direction": current.Direction, "tagGroups": current.TagGroups, "characters": current.Characters, "other_settings": current.OtherSettings}
	default:
		return model.JSONMap{}
	}
}

// novelSetupWorkflowSystemPrompt 返回模板工作流各步骤共用的结构和边界规则。
func novelSetupWorkflowSystemPrompt() string {
	return novelOnlyIdentityPrompt + `

你是小说设定表单结构化助手。你的输出永远只能是严格 JSON 对象，字段来自：
title、direction、tagGroups、characters、relationships、maps、forces、other_settings、perspective、length、lengthRange。

总规则：
- 信息不足时可以合理补全，但不能违背用户原意。
- AI 只能主动选择短篇或中篇；除非用户特别说明长篇、超长篇、百万字级创作、长线连载或 600 章以上，否则禁止返回长篇。
- tagGroups 必须是对象，分组值必须是字符串数组，例如 {"题材":["玄幻"],"类型":["升级流"],"基调":["热血"],"文风":["快节奏"],"雷点":["虐主"]}；禁止把分组值写成对象或复杂结构。
- 文风标签优先从现有知识库中选择，只有用户明确要求且现有知识库明显都不适合时，才允许自定义新的文风标签。现有知识库文风包括：番茄爽文、晋江慢热、轻小说、现实主义、群像史诗、短句风、知乎、盐言、快节奏。其中“知乎”“盐言”视为同一类文风，不要同时重复输出。
- 雷点必须全部是负面风险项，表示 AI 后续写作要避开的内容，例如 虐主、降智、后宫、烂尾、水文、说教、圣母；禁止输出 不虐主、不降智、少误会 这种正向规避句式。
- 每个设定都必须有 appearanceTime，且只能写前期/中期/后期，表示全书中的按需加载时机，不要写第几卷第几章。
- characters、maps、forces、other_settings.items 的名称字段一律叫 name，禁止用 title 代替 name；只有 other_settings 分组本身使用 title。
- other_settings 必须包含且只能包含一个 title 为“时间线设定”的大类，且它必须作为其他设定的基础先出现。时间线是具体时间坐标，不是发展变化描述；正常情况下这个大类只有一个 item，name 写“主时间线”，不要把入学期、试炼期、内战期、终局期拆成一堆小时间线 item。只有用户明确提到穿越、多时间线、平行世界、前世今生、双时代叙事、前传线、回忆线、梦境线等结构时，才允许在同一个“时间线设定”大类中增加额外时间线 item。时间线 notes 只写自身的时间范围和关键节点，保持简单但详细，不要主动耦合人物、地点、势力、体系；例如“时间范围：公元2178年3月-公元2179年12月；关键节点：2178年3月入学、2178年9月第一次秘境试炼、2179年12月终局”。
- 所有 characters、maps、forces、other_settings.items 的 notes 都必须基于具体时间段描述，写清“时间段”或“首次出现”；不能只写“前期/中期/后期”，也不能脱离具体时间孤立描述设定。
- characters 中每个人只能包含 name、appearanceTime、notes；notes 必须写年龄、性别、擅长、性格、弱点、时间段或首次出现、身份、能力、目标、矛盾、作用和关系线索。
- characters 数量必须按篇幅动态调整：短篇 10 到 14 个，中篇 22 到 30 个，长篇 40 到 60 个；覆盖主角、同伴、核心对手、导师、亲属、势力代表、敌方和阶段性关键人物；不要只给主角团。
- maps 数量必须按篇幅动态调整：短篇 6 到 8 个，中篇 12 到 18 个，长篇 25 到 40 个。
- forces 数量必须按篇幅动态调整：短篇 3 到 5 个，中篇 8 到 12 个，长篇 15 到 25 个。
- other_settings 数量必须按篇幅动态调整：短篇 5 到 7 类，中篇 8 到 12 类，长篇 14 到 20 类；每类 items 数量按题材复杂度补足，短篇 3 到 5 个，中篇 6 到 10 个，长篇 8 到 16 个；每个设定 notes 越详细越好。
- relationships 与 characters 同级，characterA/characterB 引用 char_001 这类人物顺序编号；必须覆盖全部人物，不能有孤立节点。
- maps 只表示地点，notes 写环境、位置、地位、势力归属、规则、资源、危险、时间段或首次出现和剧情作用；不要混入组织定义。
- forces 只表示团体，例如国家、阵营、势力、组织、家族、宗门、学院、军团、商会、教会、秘密组织、种族集团、地下势力；notes 必须写时间段或首次出现。
- 能力体系、等级体系、修炼体系、魔法体系、职业体系、资源、装备、禁忌、制度、规则放入 other_settings。
- other_settings 只介绍设定本体，不写主角或某个人物和该设定的关系；不得把 notes 写成“主角如何使用/发现/改变这个设定”。人物与设定的关系必须留到 characters 阶段，在人物 notes 中详细说明。
- 后续卷规划、章节规划和正文写作只能使用本表单中的核心设定；凡是会影响剧情成立的规则、体系、资源、组织、禁忌、道具、制度、能力来源、技能效果、装备用途、任务机制、历史事件、代价限制和世界运行逻辑，都必须在当前表单阶段完整规划到 characters、maps、forces 或 other_settings，不能留到正文阶段临时创造。
- 初始化阶段必须按“正文会不会用到”来反推设定完整度：如果正文可能描写人物能力、技能、装备、职业、势力权限、规则限制、任务目标、历史真相或地点规则，就必须在本表单中提前写清；正文阶段查不到对应设定时会放弃描述，不会临时编。
- 只有临时人物、炮灰人物和不影响剧情的小地点允许在后续正文中按需补充；其他任何会改变剧情逻辑的设定都必须提前进入本表单。
- 任何层次分明的体系都必须写完整层级、晋升/解锁条件、能力差异、资源消耗、代价限制、例外和剧情冲突关系。
- other_settings 每个对象必须包含 title、description、items；items 每项必须包含 name、appearanceTime、notes；notes 要写清设定定义、适用范围、层级结构、运行规则、限制代价、例外情况、资源来源、使用条件、时间段或首次出现，不要写成与主角的关系介绍。`
}

// novelSetupWorkflowTasks 返回新建小说模板生成的固定阶段和对应业务任务。
func novelSetupWorkflowTasks() []novelSetupTask {
	return []novelSetupTask{
		{name: "正在读取故事想法", text: "理解用户文本和明确限制，只生成 length、perspective；length 默认中篇，用户文本明显适合短篇时可用短篇，未明确长篇时不要用长篇；本阶段不要生成 title。", fields: []string{"length", "perspective"}, temperature: 0.35},
		{name: "正在识别题材基调", text: "生成 tagGroups 和 perspective。tagGroups 必须按题材、类型、基调、文风、雷点分组；每个分组的值只能是字符串数组，例如 {\"题材\":[\"玄幻\"],\"类型\":[\"升级流\"]}；文风分组优先从现有知识库中选择最贴近的一项或少数几项：番茄爽文、晋江慢热、轻小说、现实主义、群像史诗、短句风、知乎、盐言、快节奏；其中“知乎”“盐言”视为同一类文风，不要同时重复输出；只有用户明确要求且这些已有文风都明显不适合时，才允许自定义新的文风标签；雷点只能写负面风险项，表示后续写作要避开的内容，例如 虐主、降智、后宫、烂尾、水文、说教、圣母，禁止写 不虐主、不降智、少误会 这种正向规避句式。", fields: []string{"tagGroups", "perspective"}, temperature: 0.25},
		{name: "正在提炼核心主线", text: "统一生成 title 与 direction。direction 写 200 到 500 字，包含主角、背景、核心冲突、目标、主要看点，必须能支撑后续全书分卷规划。", fields: []string{"title", "direction"}, temperature: 0.75},
		{name: "正在补全体系规则", text: "生成 other_settings。第一类必须是“时间线设定”，它是一个大类，正常只包含一个 item：主时间线；主时间线 notes 写完整时间范围和关键节点，不要把关键节点拆成多个小时间线，也不要在时间线 notes 中主动耦合人物、地点、势力、体系。只有用户明确提到穿越、多时间线、平行世界、前世今生、双时代叙事、前传线、回忆线、梦境线等结构时，才允许在同一个时间线设定大类里增加额外时间线 item。后续能力体系、等级体系、资源、禁忌、制度、规则和世界运行逻辑都必须主动关联这个时间线，notes 必须写清时间段或首次出现。只要题材或用户文本涉及魔法、修仙、异能、科技、等级、职业、技能、装备、任务、资源、禁忌、制度、规则、代价、历史事件、世界运行逻辑等，就必须先补全完整体系；每个分组必须有非空 items。每个设定都要尽量详细介绍设定本体，写清定义、范围、层级、规则、限制、代价、例外、资源来源、使用条件和剧情可用点；不要把 notes 写成主角和该设定的关系，人物如何掌握、使用、误解、背负或改变设定，留到“正在扩展关键人物”阶段写。凡是正文可能用于描写冲突、能力表现、装备效果、任务目标、地点规则或剧情反转的内容，都必须在这里写清楚，不能留给人物或正文阶段临时创造。", fields: []string{"other_settings"}, temperature: 0.55},
		{name: "正在扩展关键人物", text: "生成 characters，按当前篇幅决定人物数量。必须参考 other_settings 中的具体时间线、能力、等级、职业、资源、规则和代价限制；人物 notes 必须写清年龄、性别、擅长、性格、弱点、时间段或首次出现、身份、目标、矛盾、作用，以及人物与相关体系、资源、规则、禁忌、装备或职业设定的关系；这里才详细说明人物如何掌握、使用、误解、背负或改变某个设定；每项只包含 name、appearanceTime、notes；人物不是每章固定出场名单，而是全书重点人物池。", fields: []string{"characters"}, temperature: 0.85},
		{name: "正在梳理人物关系", text: "根据当前 characters 生成 relationships。characterA 和 characterB 必须引用 char_001、char_002 这类顺序编号；所有人物都必须至少出现在一条关系里；description 写前期/中期/后期关系变化。", fields: []string{"relationships"}, temperature: 0.25},
		{name: "正在整理地点势力", text: "生成 maps 和 forces。地点只描述地点；forces 只描述国家、组织、阵营、家族、宗门、学院等团体；每个 notes 必须写清时间段或首次出现，必须和 characters、other_settings 中的体系关系一致，不能把能力体系写进 forces。", fields: []string{"maps", "forces"}, temperature: 0.65},
	}
}
