package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ai-novel-ide/be/internal/ai"
)

type mockSetupAIClient struct {
	replies []string
	index   int
}

// StreamAgent 是测试桩的未使用流式 Agent 方法。
func (c *mockSetupAIClient) StreamAgent(context.Context, ai.AgentStreamRequest) (<-chan ai.StreamEvent, error) {
	return nil, errors.New("StreamAgent should not be called")
}

// StreamChat 是测试桩的未使用流式 Chat 方法。
func (c *mockSetupAIClient) StreamChat(context.Context, ai.ChatGenerateRequest) (<-chan ai.StreamEvent, error) {
	return nil, errors.New("StreamChat should not be called")
}

// GenerateChat 按顺序返回测试预设的 JSON 回复。
func (c *mockSetupAIClient) GenerateChat(context.Context, ai.ChatGenerateRequest) (ai.GenerateResult, error) {
	if c.index >= len(c.replies) {
		return ai.GenerateResult{Content: "{}"}, nil
	}
	reply := c.replies[c.index]
	c.index++
	return ai.GenerateResult{Content: reply}, nil
}

// GenerateAgent 是测试桩的未使用非流式 Agent 方法。
func (c *mockSetupAIClient) GenerateAgent(context.Context, ai.AgentGenerateRequest) (ai.GenerateResult, error) {
	return ai.GenerateResult{}, errors.New("GenerateAgent should not be called")
}

type collectSetupWorkflowSink struct {
	events []string
}

// Step 记录 setup workflow 推送的阶段文案。
func (s *collectSetupWorkflowSink) Step(_ context.Context, text string) {
	s.events = append(s.events, text)
}

// TestNovelSetupWorkflowEventOrder 校验新建小说模板工作流按固定 A2UI 阶段顺序执行。
func TestNovelSetupWorkflowEventOrder(t *testing.T) {
	service := newNovelSetupWorkflowTestService()
	sink := &collectSetupWorkflowSink{}

	_, err := service.setupWorkflow.Run(context.Background(), NovelSetupWorkflowInput{
		UserID:  1,
		RawText: "魔法学院，少年逆袭",
	}, sink)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := novelSetupWorkflowTaskNames()
	if len(sink.events) != len(want) {
		t.Fatalf("events len = %d, want %d: %#v", len(sink.events), len(want), sink.events)
	}
	for i := range want {
		if sink.events[i] != want[i] {
			t.Fatalf("event %d = %q, want %q", i, sink.events[i], want[i])
		}
	}
}

// TestNovelSetupWorkflowNormalizeLength 校验用户未明确长篇时，最终篇幅会归一到中篇。
func TestNovelSetupWorkflowNormalizeLength(t *testing.T) {
	service := newNovelSetupWorkflowTestService()

	setup, err := service.setupWorkflow.Run(context.Background(), NovelSetupWorkflowInput{
		UserID:  1,
		RawText: "魔法学院，少年逆袭",
	}, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if setup.Length != "中篇" {
		t.Fatalf("Length = %q, want 中篇", setup.Length)
	}
	if setup.LengthRange != "约 200-400 章" {
		t.Fatalf("LengthRange = %q, want 约 200-400 章", setup.LengthRange)
	}
	if setup.Title == "" || setup.Direction == "" || len(setup.Characters) == 0 || len(setup.OtherSettings) == 0 {
		t.Fatalf("setup is not consumable by form: %#v", setup)
	}
}

// TestNovelSetupWorkflowIgnoresFieldsOutsideStep 校验后续阶段不会覆盖非本阶段负责的字段。
func TestNovelSetupWorkflowIgnoresFieldsOutsideStep(t *testing.T) {
	replies := novelSetupWorkflowReplies()
	replies[1] = `{"length":"长篇","perspective":"第一人称","tagGroups":{"题材":["魔法"],"类型":["升级流"],"基调":["热血"],"文风":["快节奏"],"雷点":["降智"]}}`
	replies[6] = setupWorkflowJSONMap(map[string]any{
		"title":  "错误覆盖标题",
		"length": "长篇",
		"maps":   testSetupMaps(),
		"forces": testSetupForces(),
	})
	service := newNovelSetupWorkflowTestServiceWithReplies(replies)

	setup, err := service.setupWorkflow.Run(context.Background(), NovelSetupWorkflowInput{
		UserID:  1,
		RawText: "魔法学院，少年逆袭",
	}, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if setup.Length != "中篇" {
		t.Fatalf("Length = %q, want 中篇", setup.Length)
	}
	if setup.Title != "星门学院" {
		t.Fatalf("Title = %q, want 星门学院", setup.Title)
	}
	if setup.Perspective != "第一人称" {
		t.Fatalf("Perspective = %q, want 第一人称", setup.Perspective)
	}
}

// TestNovelSetupWorkflowRetryInvalidJSON 校验步骤返回非法 JSON 时会重试后继续执行。
func TestNovelSetupWorkflowRetryInvalidJSON(t *testing.T) {
	replies := append([]string{"不是 JSON"}, novelSetupWorkflowReplies()...)
	service := newNovelSetupWorkflowTestServiceWithReplies(replies)

	setup, err := service.setupWorkflow.Run(context.Background(), NovelSetupWorkflowInput{
		UserID:  1,
		RawText: "魔法学院，少年逆袭",
	}, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if setup.Title != "星门学院" {
		t.Fatalf("Title = %q, want 星门学院", setup.Title)
	}
}

// TestNovelSetupWorkflowRetryInvalidMapName 校验地点阶段字段名错误时会重试，避免 maps 被归一化为空。
func TestNovelSetupWorkflowRetryInvalidMapName(t *testing.T) {
	replies := novelSetupWorkflowReplies()
	wrongMapsReply := `{"maps":[{"title":"星门学院","appearanceTime":"前期","notes":"错误使用 title 作为地点名称。"}],"forces":[{"name":"星门评议会","appearanceTime":"中期","notes":"控制学院资源和星门研究资格的团体。"}]}`
	replies = append(append([]string{}, replies[:6]...), append([]string{wrongMapsReply}, replies[6:]...)...)
	service := newNovelSetupWorkflowTestServiceWithReplies(replies)

	setup, err := service.setupWorkflow.Run(context.Background(), NovelSetupWorkflowInput{
		UserID:  1,
		RawText: "魔法学院，少年逆袭",
	}, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(setup.Maps) != 12 {
		t.Fatalf("maps len = %d, want 12: %#v", len(setup.Maps), setup.Maps)
	}
}

// TestNovelSetupRelevantStateIncludesOtherSettings 校验声明参考体系规则的阶段确实注入 other_settings。
func TestNovelSetupRelevantStateIncludesOtherSettings(t *testing.T) {
	var setup NovelSetupInput
	setup.OtherSettings = append(setup.OtherSettings, struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Items       []struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		} `json:"items"`
	}{Title: "魔法体系"})

	for _, stepName := range []string{"正在扩展关键人物", "正在整理地点势力"} {
		state := novelSetupRelevantState(stepName, setup)
		if _, ok := state["other_settings"]; !ok {
			t.Fatalf("%s missing other_settings in relevant state: %#v", stepName, state)
		}
	}
}

// TestNovelSetupPromptSeparatesOtherSettingsFromCharacterRelations 校验体系设定阶段不写成人物关系。
func TestNovelSetupPromptSeparatesOtherSettingsFromCharacterRelations(t *testing.T) {
	systemPrompt := novelSetupWorkflowSystemPrompt()
	if !strings.Contains(systemPrompt, "other_settings 只介绍设定本体") {
		t.Fatalf("system prompt should require other_settings to describe setting itself: %s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "人物与设定的关系必须留到 characters 阶段") {
		t.Fatalf("system prompt should move character-setting relation to characters stage: %s", systemPrompt)
	}

	tasks := novelSetupWorkflowTasks()
	var otherTask, characterTask string
	for _, task := range tasks {
		switch task.name {
		case "正在补全体系规则":
			otherTask = task.text
		case "正在扩展关键人物":
			characterTask = task.text
		}
	}
	if !strings.Contains(otherTask, "详细介绍设定本体") || !strings.Contains(otherTask, "留到“正在扩展关键人物”阶段写") {
		t.Fatalf("other_settings task should focus on setting itself: %s", otherTask)
	}
	if !strings.Contains(characterTask, "人物与相关体系") || !strings.Contains(characterTask, "这里才详细说明") {
		t.Fatalf("characters task should describe relation with settings: %s", characterTask)
	}
}

// TestNovelSetupScaleUsesExpandedCounts 校验模板生成采用中等增强版数量下限。
func TestNovelSetupScaleUsesExpandedCounts(t *testing.T) {
	cases := []struct {
		length            string
		minCharacters     int
		minMaps           int
		minForces         int
		minOtherSettings  int
		characterHintPart string
	}{
		{length: "短篇", minCharacters: 10, minMaps: 6, minForces: 3, minOtherSettings: 5, characterHintPart: "10 到 14"},
		{length: "中篇", minCharacters: 22, minMaps: 12, minForces: 8, minOtherSettings: 8, characterHintPart: "22 到 30"},
		{length: "长篇", minCharacters: 40, minMaps: 25, minForces: 15, minOtherSettings: 14, characterHintPart: "40 到 60"},
	}
	for _, tc := range cases {
		scale := novelSetupScale(tc.length)
		if scale.MinCharacters != tc.minCharacters || scale.MinMaps != tc.minMaps || scale.MinForces != tc.minForces || scale.MinOtherSettings != tc.minOtherSettings {
			t.Fatalf("%s scale = %#v", tc.length, scale)
		}
		if !strings.Contains(scale.CharacterHint, tc.characterHintPart) {
			t.Fatalf("%s character hint = %q", tc.length, scale.CharacterHint)
		}
	}
}

// TestValidateSetupTimelineCategoryRejectsSplitTimeline 校验普通题材不能把时间线拆成多个小时间线。
func TestValidateSetupTimelineCategoryRejectsSplitTimeline(t *testing.T) {
	var setup NovelSetupInput
	setup.OtherSettings = append(setup.OtherSettings, struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Items       []struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		} `json:"items"`
	}{
		Title: "时间线设定",
		Items: []struct {
			Name           string `json:"name"`
			Notes          string `json:"notes"`
			AppearanceTime string `json:"appearanceTime"`
		}{
			{Name: "入学期", AppearanceTime: "前期", Notes: "时间范围：2178年3月；关键节点：入学。"},
			{Name: "试炼期", AppearanceTime: "前期", Notes: "时间范围：2178年9月；关键节点：试炼。"},
		},
	})

	if err := validateSetupTimelineCategory(setup, "魔法学院"); err == nil {
		t.Fatal("validateSetupTimelineCategory returned nil, want error")
	}
}

// newNovelSetupWorkflowTestService 创建使用固定 AI 回复的 setup workflow 测试服务。
func newNovelSetupWorkflowTestService() *novelService {
	return newNovelSetupWorkflowTestServiceWithReplies(novelSetupWorkflowReplies())
}

// newNovelSetupWorkflowTestServiceWithReplies 创建使用指定 AI 回复的 setup workflow 测试服务。
func newNovelSetupWorkflowTestServiceWithReplies(replies []string) *novelService {
	service := &novelService{aiStreamSupport: &aiStreamSupport{aiClient: &mockSetupAIClient{replies: replies}}}
	workflow, err := service.newNovelSetupWorkflow(context.Background())
	if err != nil {
		panic(err)
	}
	service.setupWorkflow = workflow
	return service
}

// novelSetupWorkflowTaskNames 返回 setup workflow 测试期望的阶段名称。
func novelSetupWorkflowTaskNames() []string {
	tasks := novelSetupWorkflowTasks()
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		names = append(names, task.name)
	}
	return names
}

// novelSetupWorkflowReplies 返回 setup workflow 测试用的七阶段 JSON。
func novelSetupWorkflowReplies() []string {
	return []string{
		`{"length":"中篇","perspective":"第三人称"}`,
		`{"tagGroups":{"题材":["魔法","学院"],"类型":["升级流"],"基调":["热血"],"文风":["快节奏"],"雷点":["降智"]}}`,
		`{"title":"星门学院","direction":"少年林澈进入星门学院后，发现学院魔法体系背后隐藏着古老星门的代价。他从被低估的新生开始，在同伴、导师和敌对社团的推动下逐步揭开星门规则，目标是在学院竞赛、秘境试炼和势力斗争中守住自我，并阻止禁忌魔法被滥用。"}`,
		setupWorkflowJSON("other_settings", testSetupOtherSettings()),
		setupWorkflowJSON("characters", testSetupCharacters()),
		`{"relationships":[{"characterA":"char_001","characterB":"char_002","description":"前期互相试探，中期并肩调查，后期共同承担星门代价。"}]}`,
		setupWorkflowJSONMap(map[string]any{"maps": testSetupMaps(), "forces": testSetupForces()}),
	}
}

// setupWorkflowJSON 将单字段测试数据序列化为模型回复 JSON。
func setupWorkflowJSON(key string, value any) string {
	return setupWorkflowJSONMap(map[string]any{key: value})
}

// setupWorkflowJSONMap 将测试数据序列化为模型回复 JSON。
func setupWorkflowJSONMap(value map[string]any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// testSetupOtherSettings 返回满足中篇数量要求的体系设定测试数据。
func testSetupOtherSettings() []map[string]any {
	return []map[string]any{
		{
			"title":       "时间线设定",
			"description": "全书主时间线。",
			"items": []map[string]string{{
				"name":           "学院主时间线",
				"appearanceTime": "前期",
				"notes":          "时间范围：2178年3月-2179年12月；关键节点：2178年3月入学、2178年9月第一次秘境试炼、2179年6月学院内战、2179年12月禁忌星门终局。",
			}},
		},
		{
			"title":       "魔法体系",
			"description": "星门魔法等级、代价与禁忌。",
			"items": []map[string]string{{
				"name":           "星辉级",
				"appearanceTime": "前期",
				"notes":          "时间段：2178年3月-2178年9月；入门等级，依靠星纹施法，代价是短暂体力透支。",
			}},
		},
		{
			"title":       "竞赛制度",
			"description": "学院推动冲突的公开制度。",
			"items": []map[string]string{{
				"name":           "星门积分赛",
				"appearanceTime": "前期",
				"notes":          "首次出现：2178年4月；用积分决定秘境资格和研究资源。",
			}},
		},
		{
			"title":       "禁忌规则",
			"description": "星门研究不可触碰的限制。",
			"items": []map[string]string{{
				"name":           "逆纹禁令",
				"appearanceTime": "中期",
				"notes":          "首次出现：2179年1月；禁止反向刻写星纹，否则会污染星门记忆。",
			}},
		},
		{
			"title":       "资源体系",
			"description": "魔法修习消耗的核心资源。",
			"items": []map[string]string{{
				"name":           "星砂",
				"appearanceTime": "前期",
				"notes":          "时间段：2178年3月-2179年12月；用于稳定星纹，是学院资源争夺的基础。",
			}},
		},
		{
			"title":       "装备体系",
			"description": "星纹装备的来源、用途与限制。",
			"items": []map[string]string{{
				"name":           "星纹导器",
				"appearanceTime": "前期",
				"notes":          "首次出现：2178年4月；用于辅助刻写星纹，必须消耗星砂并承受精神负荷。",
			}},
		},
		{
			"title":       "任务机制",
			"description": "学院分派任务和结算奖励的制度。",
			"items": []map[string]string{{
				"name":           "秘境委托",
				"appearanceTime": "中期",
				"notes":          "首次出现：2178年9月；学生按评级进入秘境完成调查、采集和守卫任务，失败会扣除积分。",
			}},
		},
		{
			"title":       "历史事件",
			"description": "推动主线调查的旧案与历史真相。",
			"items": []map[string]string{{
				"name":           "旧星门事故",
				"appearanceTime": "中期",
				"notes":          "首次出现：2179年1月；评议会曾封存一次星门污染事故，留下逆纹禁令和旧案档案。",
			}},
		},
	}
}

// testSetupCharacters 返回满足中篇数量要求的人物测试数据。
func testSetupCharacters() []map[string]string {
	names := []string{"林澈", "许青岚", "顾沉舟", "沈薇", "周凛", "白羽", "秦鹤", "纪南", "陆昭", "韩砚", "苏棠", "穆岚", "许靖", "方岐", "洛闻", "唐静", "魏长明", "叶知秋", "宁川", "贺星河", "赵砚初", "宋砚宁"}
	result := make([]map[string]string, 0, len(names))
	for i, name := range names {
		appearanceTime := "前期"
		if i >= 10 {
			appearanceTime = "中期"
		}
		result = append(result, map[string]string{
			"name":           name,
			"appearanceTime": appearanceTime,
			"notes":          "年龄：16岁；性别：男/女按角色设定；擅长：星纹分析；性格：目标明确；弱点：容易被旧案牵动；首次出现：2178年3月；身份：学院关键人物；能力：星辉级；目标：推动星门旧案调查。",
		})
	}
	return result
}

// testSetupMaps 返回满足中篇数量要求的地点测试数据。
func testSetupMaps() []map[string]string {
	names := []string{"星门学院", "星辉试炼场", "秘境回廊", "旧案档案馆", "王都北境", "地下星砂库", "评议会大厅", "禁忌星门", "星纹工坊", "北塔观测台", "旧城钟楼", "王都档案院"}
	result := make([]map[string]string, 0, len(names))
	for i, name := range names {
		appearanceTime := "前期"
		if i >= 5 {
			appearanceTime = "中期"
		}
		result = append(result, map[string]string{
			"name":           name,
			"appearanceTime": appearanceTime,
			"notes":          "首次出现：2178年3月；位于学院主时间范围内，是学习、试炼或调查星门旧案的关键地点。",
		})
	}
	return result
}

// testSetupForces 返回满足中篇数量要求的势力测试数据。
func testSetupForces() []map[string]string {
	names := []string{"星门评议会", "旧案调查组", "王都监察署", "秘境商会", "逆纹社", "星纹工坊联盟", "北塔守卫队", "旧城档案会"}
	result := make([]map[string]string, 0, len(names))
	for i, name := range names {
		appearanceTime := "前期"
		if i >= 3 {
			appearanceTime = "中期"
		}
		result = append(result, map[string]string{
			"name":           name,
			"appearanceTime": appearanceTime,
			"notes":          "时间段：2178年3月-2179年12月；围绕星门研究资格、旧案调查和资源分配参与冲突。",
		})
	}
	return result
}
