package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestContextToolInjectsStyleStats(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	progress := &domain.Progress{TotalChapters: 10}
	body := "# 第N章\n他不是迟疑，而是恐惧。沉默了几息。像一道光。\n夜色落下。\n他走了。"
	for ch := 1; ch <= 6; ch++ {
		if err := st.Drafts.SaveFinalChapter(ch, body); err != nil {
			t.Fatalf("SaveFinalChapter: %v", err)
		}
		progress.CompletedChapters = append(progress.CompletedChapters, ch)
	}
	if err := st.Progress.Save(progress); err != nil {
		t.Fatalf("Save progress: %v", err)
	}

	tool := NewContextTool(st, References{}, "default", rules.LoadOptions{})
	args, _ := json.Marshal(map[string]any{"chapter": 7})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Episodic map[string]json.RawMessage `json:"episodic_memory"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	statsRaw, ok := payload.Episodic["style_stats"]
	if !ok {
		t.Fatalf("expected episodic_memory.style_stats, got keys %v", keysOf(payload.Episodic))
	}
	var stats struct {
		Chapters int `json:"chapters"`
		Patterns []struct {
			Name  string `json:"name"`
			Total int    `json:"total"`
		} `json:"patterns"`
	}
	if err := json.Unmarshal(statsRaw, &stats); err != nil {
		t.Fatalf("Unmarshal stats: %v", err)
	}
	if stats.Chapters != 6 || len(stats.Patterns) == 0 {
		t.Errorf("stats content: %+v", stats)
	}
	if usage, ok := payload.Episodic["_usage"]; !ok || len(usage) == 0 {
		t.Error("expected episodic_memory._usage annotation")
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestContextToolReportsWarningsForCorruptedState(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "outline.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write outline.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta", "progress.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write progress.json: %v", err)
	}

	tool := NewContextTool(store, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Warnings []string `json:"_warnings"`
		Summary  string   `json:"_loading_summary"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Warnings) == 0 {
		t.Fatal("expected context warnings for corrupted files")
	}
	if !containsWarning(payload.Warnings, "outline") {
		t.Fatalf("expected outline warning, got %v", payload.Warnings)
	}
	if !containsWarning(payload.Warnings, "progress") {
		t.Fatalf("expected progress warning, got %v", payload.Warnings)
	}
	if !strings.Contains(payload.Summary, "告警:") {
		t.Fatalf("expected loading summary to contain warning count, got %q", payload.Summary)
	}
}

func containsWarning(warnings []string, key string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, key) {
			return true
		}
	}
	return false
}

func TestContextToolInjectsReversalToolkitReference(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	refs := References{}
	refValue := reflect.ValueOf(&refs).Elem()
	field := refValue.FieldByName("ReversalToolkit")
	if !field.IsValid() {
		t.Fatal("References should expose ReversalToolkit for phase 1 reference expansion")
	}
	field.SetString("反转工具")
	hookField := refValue.FieldByName("HookTechniques")
	if !hookField.IsValid() {
		t.Fatal("References should expose HookTechniques for architect hook planning")
	}
	hookField.SetString("钩子技法")

	tool := NewContextTool(s, refs, "default", rules.LoadOptions{})

	t.Run("writer chapter context", func(t *testing.T) {
		args, err := json.Marshal(map[string]any{"chapter": 1})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		var payload struct {
			ReferencePack struct {
				References map[string]string `json:"references"`
			} `json:"reference_pack"`
		}
		if err := json.Unmarshal(result, &payload); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got := payload.ReferencePack.References["reversal_toolkit"]; got != "反转工具" {
			t.Fatalf("expected writer reversal_toolkit reference, got %q", got)
		}
	})

	t.Run("architect planning context", func(t *testing.T) {
		args, err := json.Marshal(map[string]any{})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		var payload struct {
			ReferencePack struct {
				References map[string]string `json:"references"`
			} `json:"reference_pack"`
		}
		if err := json.Unmarshal(result, &payload); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got := payload.ReferencePack.References["reversal_toolkit"]; got != "反转工具" {
			t.Fatalf("expected architect reversal_toolkit reference, got %q", got)
		}
		if got := payload.ReferencePack.References["hook_techniques"]; got != "钩子技法" {
			t.Fatalf("expected architect hook_techniques reference, got %q", got)
		}
	})
}

func TestContextToolChapterModeIncludesWorkingAndReferenceFields(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SavePremise(`## 题材和基调
少年成长，偏紧张压迫。

## 题材定位
少年升级流

## 核心冲突
主角必须在宗门竞争中活下来。

## 主角目标
进入内门。

## 终局方向
成为真正的执棋者。

## 写作禁区
不提前揭露师尊真相。

## 差异化卖点
弱者逆袭。

## 差异化钩子
每阶段都要用更高代价换成长。

## 核心兑现承诺
持续兑现危机与突破。

## 故事引擎
试炼、资源争夺与身份升级共同推进。

## 中段转折
主角被迫转向另一条修行路线。
`); err != nil {
		t.Fatalf("SavePremise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "入门", CoreEvent: "主角进入宗门", Scenes: []string{"拜师", "立誓"}},
		{Chapter: 2, Title: "试炼", CoreEvent: "参加外门试炼", Scenes: []string{"集合", "出发"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "林砚", Role: "主角", Description: "少年修士", Arc: "成长", Traits: []string{"冷静"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := s.World.SaveWorldRules([]domain.WorldRule{
		{Category: "magic", Rule: "灵气可以炼化", Boundary: "凡人不可直接驾驭"},
	}); err != nil {
		t.Fatalf("SaveWorldRules: %v", err)
	}
	if err := s.Progress.Init("test", 2); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    1,
		Summary:    "主角拜入宗门，确立目标。",
		Characters: []string{"林砚"},
		KeyEvents:  []string{"拜师"},
	}); err != nil {
		t.Fatalf("SaveSummary: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(1, "第一章正文结尾，留下试炼悬念。"); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 2,
		Title:   "试炼",
		Goal:    "通过第一关",
		Contract: domain.ChapterContract{
			RequiredBeats:    []string{"必须让主角通过第一关", "必须埋下内门试炼邀请"},
			ForbiddenMoves:   []string{"不能提前揭露师尊真实身份"},
			ContinuityChecks: []string{"主角左臂旧伤仍未痊愈"},
			EvaluationFocus:  []string{"重点检查试炼节奏是否拖沓"},
		},
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Volume: 1,
		Arc:    1,
		Prose:  []string{"叙述保持克制"},
		StyleCard: &domain.StyleCard{
			SentenceStdFloor:        3,
			DialogueRatioTarget:     "0.15-0.35",
			ParagraphVarianceTarget: "短中长交错",
			SensoryPreferences:      []string{"触觉", "声音"},
			BannedPatterns:          []string{"章末升华"},
			DialogueDNA:             []domain.StyleDialogueDNA{{Name: "林砚", Traits: []string{"短句"}}},
			ChapterEndingPolicy:     "停在动作",
			ChapterTypeProfiles:     []domain.ChapterTypeProfile{{Type: "过渡", DialogueRatioTarget: "0.1-0.25"}},
		},
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}
	if err := s.RunMeta.SetPlanningTier(domain.PlanningTierLong); err != nil {
		t.Fatalf("SetPlanningTier: %v", err)
	}

	tool := NewContextTool(s, References{
		Consistency:      "一致性检查",
		HookTechniques:   "钩子技巧",
		QualityChecklist: "质量清单",
	}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{
		"premise",
		"premise_sections",
		"premise_structure",
		"outline",
		"world_rules",
		"memory_policy",
		"planning_tier",
		"working_memory",
		"episodic_memory",
		"reference_pack",
		"current_chapter_outline",
		"recent_summaries",
		"chapter_plan",
		"chapter_contract",
		"previous_tail",
		"style_rules",
		"references",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in chapter context", key)
		}
	}
	pack := payload["reference_pack"].(map[string]any)
	styleCard, ok := pack["style_card"].(map[string]any)
	if !ok || styleCard["dialogue_ratio_target"] != "0.15-0.35" {
		t.Fatalf("expected style_card in reference_pack, got %v", pack["style_card"])
	}
	if styleCard["paragraph_variance_target"] != "短中长交错" {
		t.Fatalf("expected full style_card fields in reference_pack, got %v", styleCard)
	}
	if profiles, ok := styleCard["chapter_type_profiles"].([]any); !ok || len(profiles) != 1 {
		t.Fatalf("expected chapter_type_profiles in reference_pack, got %v", styleCard["chapter_type_profiles"])
	}
}

func TestContextToolArchitectModeIncludesPlanningAndFoundation(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SavePremise(`## 题材和基调
群像冒险，偏冷峻史诗。

## 题材定位
群像长篇冒险

## 核心冲突
众人必须在不断失控的旧秩序中寻找新秩序。

## 主角目标
抵达真相核心。

## 终局方向
揭开古老真相并重建秩序。

## 写作禁区
不靠天降设定收尾。

## 差异化卖点
群像关系推进。

## 差异化钩子
每卷都改变队伍关系结构。

## 核心兑现承诺
持续提供发现、牺牲与选择。

## 故事引擎
旅途推进、真相调查与队伍关系共同驱动。

## 关系/成长主线
队伍从互不信任走向分裂再重组。

## 升级路径
从地方事件走向世界级危机。

## 中期转向
真相并非敌人，而是秩序本身有问题。

## 终局命题
秩序应由谁定义。
`); err != nil {
		t.Fatalf("SavePremise: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "起点", CoreEvent: "旅途开始"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "沈曜", Role: "主角", Description: "流浪剑客", Arc: "寻找真相", Traits: []string{"敏锐"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := s.World.SaveWorldRules([]domain.WorldRule{
		{Category: "society", Rule: "城邦林立", Boundary: "皇权不可直辖边地"},
	}); err != nil {
		t.Fatalf("SaveWorldRules: %v", err)
	}
	if err := s.Outline.SaveLayeredOutline([]domain.VolumeOutline{
		{
			Index: 1, Title: "第一卷", Theme: "踏上旅途",
			Arcs: []domain.ArcOutline{
				{Index: 1, Title: "启程", Goal: "建立队伍", Chapters: []domain.OutlineEntry{{Chapter: 1, Title: "起点"}}},
				{Index: 2, Title: "迷雾", Goal: "逼近秘密", EstimatedChapters: 5},
			},
		},
	}); err != nil {
		t.Fatalf("SaveLayeredOutline: %v", err)
	}
	if err := s.Outline.SaveCompass(domain.StoryCompass{
		EndingDirection: "揭开古老真相",
		EstimatedScale:  "预计 3 卷",
	}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Volume: 1,
		Arc:    1,
		Prose:  []string{"保持冷峻节制"},
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}
	if err := s.RunMeta.SetPlanningTier(domain.PlanningTierLong); err != nil {
		t.Fatalf("SetPlanningTier: %v", err)
	}

	tool := NewContextTool(s, References{
		OutlineTemplate:   "大纲模板",
		CharacterTemplate: "角色模板",
		LongformPlanning:  "长篇规划",
	}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{
		"memory_policy",
		"planning_tier",
		"planning_memory",
		"foundation_memory",
		"reference_pack",
		"premise_sections",
		"premise_structure",
		"characters",
		"layered_outline",
		"skeleton_arcs",
		"compass",
		"style_rules",
		"references",
		"foundation_status",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in architect context", key)
		}
	}
}

func TestTrimByBudgetRemovesMirroredMemoryKeys(t *testing.T) {
	result := map[string]any{
		"references": map[string]string{
			"a": strings.Repeat("x", 200),
			"b": strings.Repeat("y", 200),
		},
		"reference_pack": map[string]any{
			"references": map[string]string{
				"a": strings.Repeat("x", 200),
				"b": strings.Repeat("y", 200),
			},
			"style_rules": []string{"克制"},
		},
	}

	trimByBudget(result, 80)

	if _, ok := result["references"]; ok {
		t.Fatal("expected top-level references to be trimmed")
	}
	pack, ok := result["reference_pack"].(map[string]any)
	if !ok {
		t.Fatal("expected reference_pack to remain available")
	}
	if _, ok := pack["references"]; ok {
		t.Fatal("expected mirrored references to be trimmed from reference_pack")
	}
}

func TestContextToolSelectedMemoryRecallsStoryThreadsAndReviewLessons(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "邀约", CoreEvent: "长老暗中给出内门试炼邀请", Scenes: []string{"密谈", "留下试炼令"}},
		{Chapter: 2, Title: "试炼前夜", CoreEvent: "林砚准备回应内门试炼邀请", Hook: "谁在背后推动这场试炼", Scenes: []string{"整理线索", "决定赴约"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "trial_invite", Description: "内门试炼邀请的真实目的", PlantedAt: 1, Status: "planted"},
		{ID: "trial_mastermind", Description: "谁在背后推动这场试炼", PlantedAt: 1, Status: "planted"},
		{ID: "trial_rules", Description: "试炼规则碑文残卷", PlantedAt: 1, Status: "planted"},
		{ID: "outer_disciple", Description: "外门弟子的旧债纠纷", PlantedAt: 1, Status: "planted"},
		{ID: "elder_token", Description: "长老手中令牌的来历", PlantedAt: 1, Status: "planted"},
		{ID: "hidden_gate", Description: "山门背后的隐藏通道", PlantedAt: 1, Status: "planted"},
		{ID: "trial_bet", Description: "试炼盘口的幕后操盘人", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 2,
		Title:   "试炼前夜",
		Goal:    "决定是否回应邀请",
		Contract: domain.ChapterContract{
			PayoffPoints: []string{"回应内门试炼邀请"},
			HookGoal:     "抛出谁在背后推动试炼",
		},
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter:        1,
		Scope:          "chapter",
		Verdict:        "polish",
		Summary:        "主线启动完成，但伏笔不够明确。",
		ContractStatus: "partial",
		ContractMisses: []string{"未明确埋下内门试炼邀请"},
		Issues: []domain.ConsistencyIssue{
			{Type: "hook", Severity: "warning", Description: "章末钩子不够具体"},
		},
	}); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			StoryThreads  []domain.RecallItem `json:"story_threads"`
			ReviewLessons []domain.RecallItem `json:"review_lessons"`
		} `json:"selected_memory"`
		Summary string `json:"_loading_summary"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload.Selected.StoryThreads) == 0 {
		t.Fatal("expected story thread recall items")
	}
	if len(payload.Selected.ReviewLessons) == 0 {
		t.Fatal("expected review lesson recall items")
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "内门试炼邀请") {
		t.Fatalf("expected story thread recall to mention invite, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.StoryThreads, "推动这场试炼") {
		t.Fatalf("expected story thread recall to mention trial mastermind, got %+v", payload.Selected.StoryThreads)
	}
	if containsRecallSummary(payload.Selected.StoryThreads, "试炼规则碑文残卷") {
		t.Fatalf("expected weak-overlap foreshadow to stay out, got %+v", payload.Selected.StoryThreads)
	}
	if containsRecallSummary(payload.Selected.StoryThreads, "建议回看第") {
		t.Fatalf("expected related_chapters not to be duplicated into story_threads, got %+v", payload.Selected.StoryThreads)
	}
	if !containsRecallSummary(payload.Selected.ReviewLessons, "contract 漏项") {
		t.Fatalf("expected review lesson recall to mention contract miss, got %+v", payload.Selected.ReviewLessons)
	}
	if !strings.Contains(payload.Summary, "线索召回:") || !strings.Contains(payload.Summary, "评审召回:") {
		t.Fatalf("expected loading summary to report selected memory, got %q", payload.Summary)
	}
}

func TestContextToolSelectedMemoryIncludesGlobalReviewLessons(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "开端", CoreEvent: "故事开始"},
		{Chapter: 2, Title: "推进", CoreEvent: "主线继续推进"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 1,
		Scope:   "global",
		Verdict: "polish",
		Summary: "全局推进合格，但角色目标表达还不够稳定。",
		Issues: []domain.ConsistencyIssue{
			{Type: "character", Severity: "warning", Description: "主角目标表达不够稳定"},
		},
	}); err != nil {
		t.Fatalf("SaveReview(global): %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			ReviewLessons []domain.RecallItem `json:"review_lessons"`
		} `json:"selected_memory"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !containsRecallSummary(payload.Selected.ReviewLessons, "主角目标表达不够稳定") {
		t.Fatalf("expected global review lesson to be recalled, got %+v", payload.Selected.ReviewLessons)
	}
}

func TestContextToolSelectedMemoryIncludesMinimalContext(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "旧伤", CoreEvent: "林砚带伤守住试炼资格"},
		{Chapter: 2, Title: "试炼", CoreEvent: "林砚必须兑现试炼资格", Hook: "令牌为何发烫"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 4); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Characters.Save([]domain.Character{
		{Name: "林砚", Role: "主角", Description: "外门少年", Arc: "从自保走向主动争取", Traits: []string{"冷静", "带伤"}},
	}); err != nil {
		t.Fatalf("SaveCharacters: %v", err)
	}
	if err := s.World.SaveWorldRules([]domain.WorldRule{
		{Category: "magic", Rule: "试炼令牌只会回应被选中者", Boundary: "令牌不能转赠"},
	}); err != nil {
		t.Fatalf("SaveWorldRules: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "trial_token", Description: "试炼令牌发烫意味着内门在观察林砚", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}
	if err := s.World.AppendStateChanges([]domain.StateChange{
		{Chapter: 1, Entity: "林砚", Field: "左臂旧伤", OldValue: "隐痛", NewValue: "试炼前仍未痊愈"},
	}); err != nil {
		t.Fatalf("AppendStateChanges: %v", err)
	}
	if err := s.World.SaveRelationships([]domain.RelationshipEntry{
		{CharacterA: "林砚", CharacterB: "长老", Relation: "被暗中观察", Chapter: 1},
	}); err != nil {
		t.Fatalf("SaveRelationships: %v", err)
	}
	if err := s.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 2,
		Title:   "试炼",
		Goal:    "让林砚主动回应试炼资格",
		Contract: domain.ChapterContract{
			ContinuityChecks: []string{"林砚左臂旧伤仍未痊愈"},
			PayoffPoints:     []string{"回应试炼令牌发烫"},
			HookGoal:         "让读者追问内门为何观察林砚",
		},
	}); err != nil {
		t.Fatalf("SaveChapterPlan: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload struct {
		Selected struct {
			MinimalContext struct {
				CharacterStates  []string `json:"character_states"`
				CausalHistory    []string `json:"causal_history"`
				WorldConstraints []string `json:"world_constraints"`
				ChapterIntent    string   `json:"chapter_intent"`
			} `json:"minimal_context"`
		} `json:"selected_memory"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	minimal := payload.Selected.MinimalContext
	if minimal.ChapterIntent != "让林砚主动回应试炼资格" {
		t.Fatalf("unexpected chapter intent: %q", minimal.ChapterIntent)
	}
	if !containsStringPart(minimal.CharacterStates, "左臂旧伤") || !containsStringPart(minimal.CharacterStates, "被暗中观察") {
		t.Fatalf("expected character states from state and relationship data, got %+v", minimal.CharacterStates)
	}
	if !containsStringPart(minimal.CausalHistory, "令牌") || !containsStringPart(minimal.CausalHistory, "Payoff") {
		t.Fatalf("expected causal history from foreshadow and contract payoff, got %+v", minimal.CausalHistory)
	}
	if !containsStringPart(minimal.WorldConstraints, "不能转赠") || !containsStringPart(minimal.WorldConstraints, "旧伤仍未痊愈") {
		t.Fatalf("expected world constraints from world rules and continuity checks, got %+v", minimal.WorldConstraints)
	}
}

func TestContextToolMinimalContextUsesStableEmptyFields(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "起点", CoreEvent: "故事开始"}}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 1); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	selected, ok := payload["selected_memory"].(map[string]any)
	if !ok {
		t.Fatalf("expected selected_memory with minimal_context, got %+v", payload["selected_memory"])
	}
	minimal, ok := selected["minimal_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected minimal_context, got %+v", selected)
	}
	for _, key := range []string{"character_states", "causal_history", "world_constraints"} {
		items, ok := minimal[key].([]any)
		if !ok {
			t.Fatalf("expected %s to be an array, got %+v", key, minimal[key])
		}
		if len(items) != 0 {
			t.Fatalf("expected empty %s, got %+v", key, items)
		}
	}
	if got, ok := minimal["chapter_intent"].(string); !ok || got != "故事开始" {
		t.Fatalf("expected chapter_intent to fall back to outline core event, got %+v", minimal["chapter_intent"])
	}
}

func TestContextToolKeepsFullForeshadowWhenRecallNotTriggered(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "起势", CoreEvent: "故事起势"},
		{Chapter: 2, Title: "推进", CoreEvent: "继续推进"},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 4); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "small_1", Description: "第一条小伏笔", PlantedAt: 1, Status: "planted"},
		{ID: "small_2", Description: "第二条小伏笔", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["foreshadow_ledger"]; !ok {
		t.Fatal("expected full foreshadow ledger to remain when selected recall is not triggered")
	}
	if selected, ok := payload["selected_memory"].(map[string]any); ok {
		if _, exists := selected["story_threads"]; exists {
			t.Fatalf("expected no story_threads for small foreshadow sets, got %+v", selected["story_threads"])
		}
		if _, exists := selected["minimal_context"]; !exists {
			t.Fatalf("expected minimal_context to remain stable, got %+v", selected)
		}
	} else {
		t.Fatalf("expected selected_memory with minimal_context, got %+v", payload["selected_memory"])
	}
}

func TestContextToolFallsBackToFullForeshadowWhenSelectionIsTooSparse(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Outline.SaveOutline([]domain.OutlineEntry{
		{Chapter: 1, Title: "邀约", CoreEvent: "长老暗中给出内门试炼邀请"},
		{Chapter: 2, Title: "试炼前夜", CoreEvent: "林砚准备回应内门试炼邀请", Scenes: []string{"整理线索", "决定赴约"}},
	}); err != nil {
		t.Fatalf("SaveOutline: %v", err)
	}
	if err := s.Progress.Init("test", 8); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveForeshadowLedger([]domain.ForeshadowEntry{
		{ID: "trial_invite", Description: "内门试炼邀请的真实目的", PlantedAt: 1, Status: "planted"},
		{ID: "trial_rules", Description: "试炼规则碑文残卷", PlantedAt: 1, Status: "planted"},
		{ID: "outer_disciple", Description: "外门弟子的旧债纠纷", PlantedAt: 1, Status: "planted"},
		{ID: "elder_token", Description: "长老手中令牌的来历", PlantedAt: 1, Status: "planted"},
		{ID: "hidden_gate", Description: "山门背后的隐藏通道", PlantedAt: 1, Status: "planted"},
		{ID: "trial_bet", Description: "试炼盘口的幕后操盘人", PlantedAt: 1, Status: "planted"},
	}); err != nil {
		t.Fatalf("SaveForeshadowLedger: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["foreshadow_ledger"]; !ok {
		t.Fatal("expected full foreshadow ledger when selection is too sparse")
	}
	if selected, ok := payload["selected_memory"].(map[string]any); ok {
		if _, exists := selected["story_threads"]; exists {
			t.Fatalf("expected sparse story_threads to fall back to full ledger, got %+v", selected["story_threads"])
		}
	}
}

func containsRecallSummary(items []domain.RecallItem, want string) bool {
	for _, item := range items {
		if strings.Contains(item.Summary, want) {
			return true
		}
	}
	return false
}

func containsStringPart(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func TestContextToolInjectsRewriteBriefForPendingRewriteChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.SetPendingRewrites([]int{2}, "节奏拖沓，需要压缩前半段"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.World.SaveReview(domain.ReviewEntry{
		Chapter: 2,
		Scope:   "chapter",
		Verdict: "rewrite",
		Summary: "前半段铺垫过长，冲突迟迟不出现。",
		Issues: []domain.ConsistencyIssue{
			{Type: "pacing", Severity: "error", Description: "前 2000 字无推进"},
		},
		ContractMisses: []string{"未兑现试炼开场"},
	}); err != nil {
		t.Fatalf("SaveReview: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	brief, ok := payload["rewrite_brief"].(map[string]any)
	if !ok {
		t.Fatalf("expected rewrite_brief in chapter context, got %T", payload["rewrite_brief"])
	}
	if got := brief["reason"]; got != "节奏拖沓，需要压缩前半段" {
		t.Fatalf("expected rewrite reason, got %v", got)
	}
	if got, _ := brief["review_summary"].(string); !strings.Contains(got, "铺垫过长") {
		t.Fatalf("expected review summary from chapter review, got %v", brief["review_summary"])
	}
	if issues, _ := brief["issues"].([]any); len(issues) == 0 {
		t.Fatalf("expected review issues in rewrite_brief, got %v", brief["issues"])
	}
	if misses, _ := brief["contract_misses"].([]any); len(misses) == 0 {
		t.Fatalf("expected contract misses in rewrite_brief, got %v", brief["contract_misses"])
	}
}

func TestContextToolInjectsCompactStyleStats(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveStyleStats(domain.StyleStats{
		SchemaVersion: domain.StyleStatsSchemaVersion,
		Chapter:       2,
		Summary:       "句长std 1.20；对话比 0.00；热点 1 个，首要 low_dialogue_ratio",
		Metrics: map[string]domain.StyleMetric{
			"sentence_length_stddev":     {Value: 1.2},
			"dialogue_ratio":             {Value: 0},
			"sentence_start_unique_rate": {Value: 0.4},
			"pattern_density_per_1000":   {Value: 3.5},
			"repeated_ngram_rate":        {Value: 0.9},
		},
		Hotspots: []domain.StyleHotspot{
			{ID: "hs_001", RuleID: "low_dialogue_ratio", Severity: "info", Evidence: "dialogue_ratio=0.00", Message: "对话比例偏低", SuggestionType: "check_dialogue_balance"},
		},
	}); err != nil {
		t.Fatalf("SaveStyleStats: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, _ := json.Marshal(map[string]any{"chapter": 2})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	working, ok := payload["working_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing working_memory")
	}
	stats, ok := working["style_stats"].(map[string]any)
	if !ok {
		t.Fatalf("missing style_stats: %v", working)
	}
	if stats["summary"] == "" {
		t.Fatalf("expected summary in style_stats: %v", stats)
	}
	metrics, ok := stats["metrics"].(map[string]any)
	if !ok || metrics["dialogue_ratio"] != float64(0) {
		t.Fatalf("expected compact metrics, got %v", stats["metrics"])
	}
	if _, hasLargeMetric := metrics["repeated_ngram_rate"]; hasLargeMetric {
		t.Fatal("unexpected full metric table in compact style_stats")
	}
	hotspots, ok := stats["hotspots"].([]any)
	if !ok || len(hotspots) != 1 {
		t.Fatalf("expected compact hotspots, got %v", stats["hotspots"])
	}
	hotspot := hotspots[0].(map[string]any)
	if hotspot["id"] != "hs_001" {
		t.Fatalf("expected hotspot id for Editor targets, got %v", hotspot)
	}
}

func TestContextToolInjectsDraftStyleStatsForPendingRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.SetPendingRewrites([]int{2}, "AI 腔明显"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.Drafts.SaveDraft(2, "他说要走。他说要等。他说要看。她没有回答。风停了。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.World.SaveStyleStats(domain.StyleStats{
		SchemaVersion: domain.StyleStatsSchemaVersion,
		Chapter:       2,
		Summary:       "旧终稿统计",
		Metrics: map[string]domain.StyleMetric{
			"sentence_length_stddev":     {Value: 4},
			"dialogue_ratio":             {Value: 0.2},
			"sentence_start_unique_rate": {Value: 0.8},
			"pattern_density_per_1000":   {Value: 1},
		},
	}); err != nil {
		t.Fatalf("SaveStyleStats: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, _ := json.Marshal(map[string]any{"chapter": 2})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	working := payload["working_memory"].(map[string]any)
	if _, ok := working["style_stats_draft"].(map[string]any); !ok {
		t.Fatalf("expected style_stats_draft in working_memory, got %v", working)
	}
	brief := payload["rewrite_brief"].(map[string]any)
	if _, ok := brief["style_stats"].(map[string]any); !ok {
		t.Fatalf("expected style_stats in rewrite_brief, got %v", brief)
	}
}

func TestContextToolInjectsDiagnosticGuidance(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.World.SaveDiagnosticGuidance(domain.DiagnosticGuidance{
		SchemaVersion: domain.DiagnosticGuidanceSchemaVersion,
		GeneratedAt:   "2026-01-02T03:04:05Z",
		Items: []domain.DiagnosticGuidanceItem{{
			Rule:       "RewriteEffectiveness",
			Severity:   "warning",
			Target:     "prompt.writer",
			Title:      "第 2 章打磨后风格指标未改善",
			Signal:     "edit_distance_ratio=0.03",
			Suggestion: "让 Editor issue targets 更具体",
		}},
	}); err != nil {
		t.Fatalf("SaveDiagnosticGuidance: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, _ := json.Marshal(map[string]any{"chapter": 2})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	working := payload["working_memory"].(map[string]any)
	guidance, ok := working["diag_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("missing diag_guidance: %v", working)
	}
	items, ok := guidance["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected guidance items: %v", guidance)
	}
	item := items[0].(map[string]any)
	if item["rule"] != "RewriteEffectiveness" || item["suggestion"] == "" {
		t.Fatalf("unexpected guidance item: %v", item)
	}
}

func TestContextToolOmitsRewriteBriefForNormalChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, err := json.Marshal(map[string]any{"chapter": 2})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := payload["rewrite_brief"]; ok {
		t.Fatal("expected no rewrite_brief for chapter outside PendingRewrites")
	}
}

func TestContextToolInjectsUserDirectivesOnBothPaths(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if _, err := s.Directives.Add(domain.UserDirective{Text: "对话占比提高", Chapter: 2, TotalChapters: 3}); err != nil {
		t.Fatalf("AddDirective: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	for name, chapter := range map[string]int{"writer": 1, "architect": 0} {
		args, _ := json.Marshal(map[string]any{"chapter": chapter})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("[%s] Execute: %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(result, &payload); err != nil {
			t.Fatalf("[%s] Unmarshal: %v", name, err)
		}
		working, ok := payload["working_memory"].(map[string]any)
		if !ok {
			t.Fatalf("[%s] missing working_memory", name)
		}
		directives, ok := working["user_directives"].([]any)
		if !ok || len(directives) != 1 {
			t.Fatalf("[%s] expected 1 directive, got %v", name, working["user_directives"])
		}
		entry, _ := directives[0].(map[string]any)
		if entry["text"] != "对话占比提高" || entry["at_chapter"] != float64(2) || entry["at_total_chapters"] != float64(3) {
			t.Errorf("[%s] unexpected directive entry: %v", name, entry)
		}
		if _, hasCreatedAt := entry["created_at"]; hasCreatedAt {
			t.Errorf("[%s] created_at 是审计信息，不应进 LLM", name)
		}
	}
}

func TestContextToolInjectsEmptyUserDirectives(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewContextTool(s, References{}, "default", rules.LoadOptions{})
	args, _ := json.Marshal(map[string]any{"chapter": 0})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	working, ok := payload["working_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing working_memory")
	}
	// 空列表也注入 []（字段稳定，同 user_rules 先例），不能是 null/缺失
	directives, ok := working["user_directives"].([]any)
	if !ok {
		t.Fatalf("expected stable empty array, got %T", working["user_directives"])
	}
	if len(directives) != 0 {
		t.Errorf("expected empty list, got %v", directives)
	}
}
