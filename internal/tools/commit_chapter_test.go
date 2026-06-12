package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestCommitChapterRejectsNonPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "测试重写"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(3, "这是错误章节的正文。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         3,
		"summary":         "错误提交",
		"characters":      []string{"主角"},
		"key_events":      []string{"误提交"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("expected commit to be rejected during rewrite flow")
	}

	if _, err := os.Stat(dir + "/chapters/03.md"); !os.IsNotExist(err) {
		t.Fatalf("chapter should not be persisted, stat err=%v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 0 {
		t.Fatalf("completed chapters should stay empty, got %v", progress.CompletedChapters)
	}
	if progress.CurrentChapter != 0 {
		t.Fatalf("current chapter should not advance, got %d", progress.CurrentChapter)
	}
}

func TestCommitChapterAllowsPendingRewrite(t *testing.T) {
	dir := t.TempDir()
	store := store.NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := store.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := store.Progress.SetPendingRewrites([]int{2}, "测试重写"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := store.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	if err := store.Drafts.SaveDraft(2, "这是正确待重写章节的正文。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(store)
	args, err := json.Marshal(map[string]any{
		"chapter":         2,
		"summary":         "正确提交",
		"characters":      []string{"主角"},
		"key_events":      []string{"完成重写"},
		"timeline_events": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Stat(dir + "/chapters/02.md"); err != nil {
		t.Fatalf("chapter should be persisted: %v", err)
	}

	progress, err := store.Progress.Load()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if len(progress.CompletedChapters) != 1 || progress.CompletedChapters[0] != 2 {
		t.Fatalf("unexpected completed chapters: %v", progress.CompletedChapters)
	}
	pending, err := store.Signals.LoadPendingCommit()
	if err != nil {
		t.Fatalf("LoadPendingCommit: %v", err)
	}
	if pending != nil {
		t.Fatalf("expected pending commit cleared, got %+v", pending)
	}
}

func TestCommitChapterReturnsAndPersistsStyleStats(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.RunMeta.Init("default", "openrouter", "test-model"); err != nil {
		t.Fatalf("RunMeta.Init: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "他说要走。他说要等。他说要看。她没有回答。风停了。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	args, _ := json.Marshal(map[string]any{
		"chapter": 1, "summary": "摘要", "characters": []string{"主角"}, "key_events": []string{"事件"},
	})
	raw, err := NewCommitChapterTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out struct {
		StyleStats *domain.StyleStats `json:"style_stats"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.StyleStats == nil || out.StyleStats.Chapter != 1 || out.StyleStats.Model.Model != "test-model" {
		t.Fatalf("style_stats missing or wrong: %+v", out.StyleStats)
	}
	if !styleStatsHasRule(out.StyleStats, "low_dialogue_ratio") {
		t.Fatalf("expected low_dialogue_ratio hotspot, got %+v", out.StyleStats.Hotspots)
	}
	persisted, err := s.World.LoadStyleStats(1)
	if err != nil {
		t.Fatalf("LoadStyleStats: %v", err)
	}
	if persisted == nil || persisted.Summary != out.StyleStats.Summary {
		t.Fatalf("persisted stats mismatch: %+v vs %+v", persisted, out.StyleStats)
	}
	guidance, err := s.World.LoadDiagnosticGuidance()
	if err != nil {
		t.Fatalf("LoadDiagnosticGuidance: %v", err)
	}
	if guidance == nil || !diagnosticGuidanceHasRule(guidance, "AIFlavorHotspots") {
		t.Fatalf("expected automatic AI flavor guidance, got %+v", guidance)
	}
}

func TestCommitChapterSkipBackfillsMissingStyleStats(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	content := "他说要走。他说要等。他说要看。她没有回答。风停了。"
	if err := s.Drafts.SaveDraft(1, content); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(1, content); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, len([]rune(content)), "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	args, _ := json.Marshal(map[string]any{
		"chapter": 1, "summary": "摘要", "characters": []string{"主角"}, "key_events": []string{"事件"},
	})
	raw, err := NewCommitChapterTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out struct {
		StyleStats *domain.StyleStats `json:"style_stats"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.StyleStats == nil || out.StyleStats.Chapter != 1 {
		t.Fatalf("expected backfilled style stats, got %+v", out.StyleStats)
	}
	persisted, err := s.World.LoadStyleStats(1)
	if err != nil {
		t.Fatalf("LoadStyleStats: %v", err)
	}
	if persisted == nil || persisted.Summary != out.StyleStats.Summary {
		t.Fatalf("persisted backfill mismatch: %+v vs %+v", persisted, out.StyleStats)
	}
}

func TestCommitChapterRewriteOverwritesStyleStats(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	original := "这一刻，仿佛一切都有了答案。命运落下。"
	if err := s.Drafts.SaveDraft(2, original); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, original); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.World.SaveStyleStats(domain.StyleStats{
		SchemaVersion: domain.StyleStatsSchemaVersion,
		Chapter:       2,
		Summary:       "旧统计",
		Metrics: map[string]domain.StyleMetric{
			"sentence_length_stddev":     {Value: 1.0},
			"sentence_start_unique_rate": {Value: 0.2},
			"pattern_density_per_1000":   {Value: 5.0},
		},
	}); err != nil {
		t.Fatalf("SaveStyleStats: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(original)), "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := s.Progress.SetPendingRewrites([]int{2}, "测试重写"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.Progress.SetFlow(domain.FlowRewriting); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}
	polished := "雨落在窗纸上。林墨抬头。\"我会去。\"她把灯芯拨亮。"
	if err := s.Drafts.SaveDraft(2, polished); err != nil {
		t.Fatalf("SaveDraft polished: %v", err)
	}

	args, _ := json.Marshal(map[string]any{
		"chapter": 2, "summary": "重写", "characters": []string{"主角"}, "key_events": []string{"完成"},
	})
	raw, err := NewCommitChapterTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out struct {
		StyleStats      *domain.StyleStats             `json:"style_stats"`
		StyleComparison *domain.StyleRewriteComparison `json:"style_rewrite_comparison"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.StyleStats == nil || out.StyleStats.Summary == "旧统计" {
		t.Fatalf("rewrite style_stats missing or stale: %+v", out.StyleStats)
	}
	if out.StyleComparison == nil || out.StyleComparison.Chapter != 2 || out.StyleComparison.Mode != "rewrite" {
		t.Fatalf("rewrite style comparison missing: %+v", out.StyleComparison)
	}
	persisted, err := s.World.LoadStyleStats(2)
	if err != nil {
		t.Fatalf("LoadStyleStats: %v", err)
	}
	if persisted == nil || persisted.Summary != out.StyleStats.Summary {
		t.Fatalf("persisted rewrite stats mismatch: %+v vs %+v", persisted, out.StyleStats)
	}
	comparisons, err := s.World.LoadStyleRewriteComparisons()
	if err != nil {
		t.Fatalf("LoadStyleRewriteComparisons: %v", err)
	}
	if len(comparisons) != 1 || comparisons[0].Chapter != 2 || comparisons[0].Before.Summary != "旧统计" {
		t.Fatalf("persisted rewrite comparison mismatch: %+v", comparisons)
	}
	guidance, err := s.World.LoadDiagnosticGuidance()
	if err != nil {
		t.Fatalf("LoadDiagnosticGuidance: %v", err)
	}
	if guidance == nil || len(guidance.Items) == 0 {
		t.Fatalf("expected automatic rewrite guidance, got %+v", guidance)
	}
}

func styleStatsHasRule(stats *domain.StyleStats, ruleID string) bool {
	for _, hotspot := range stats.Hotspots {
		if hotspot.RuleID == ruleID {
			return true
		}
	}
	return false
}

func diagnosticGuidanceHasRule(guidance *domain.DiagnosticGuidance, rule string) bool {
	if guidance == nil {
		return false
	}
	for _, item := range guidance.Items {
		if item.Rule == rule {
			return true
		}
	}
	return false
}

// TestCommitChapterUpdatesCastLedger 验证：commit_chapter 把本章 characters 累加进 cast_ledger，
// cast_intros 提供的 brief_role 被采用，且 characters.json 中的核心角色不进入 ledger。
func TestCommitChapterUpdatesCastLedger(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	// 设定核心角色档案（这些不应进 cast_ledger）
	if err := s.Characters.Save([]domain.Character{
		{Name: "林墨", Role: "主角", Tier: "core"},
		{Name: "李清砚", Role: "导师", Tier: "important"},
	}); err != nil {
		t.Fatalf("Save core characters: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "第一章正文，林墨遇到客栈老板老周与小厮阿云。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    1,
		"summary":    "林墨入住客栈",
		"characters": []string{"林墨", "李清砚", "老周", "阿云"},
		"key_events": []string{"入住"},
		"cast_intros": []any{
			map[string]any{"name": "老周", "brief_role": "客栈老板"},
			map[string]any{"name": "阿云", "brief_role": "客栈小厮"},
		},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	entries, err := s.Cast.Load()
	if err != nil {
		t.Fatalf("Cast.Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 ledger entries (老周/阿云), got %d: %+v", len(entries), entries)
	}
	byName := map[string]domain.CastEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if e, ok := byName["老周"]; !ok || e.BriefRole != "客栈老板" || e.FirstSeenChapter != 1 {
		t.Errorf("老周 entry wrong: %+v", e)
	}
	if e, ok := byName["阿云"]; !ok || e.BriefRole != "客栈小厮" || e.AppearanceCount != 1 {
		t.Errorf("阿云 entry wrong: %+v", e)
	}
	if _, ok := byName["林墨"]; ok {
		t.Errorf("核心角色 林墨 不应进 ledger")
	}
	if _, ok := byName["李清砚"]; ok {
		t.Errorf("核心角色 李清砚 不应进 ledger")
	}
}

// TestCommitChapterRejectsPolishWithoutDraftChange 验证：已完成章节进入打磨/重写队列后，
// 若 writer 跳过 draft_chapter 直接 commit（drafts 与 chapters 内容完全相同），
// commit_chapter 必须拒绝，强制 writer 先调 draft_chapter 写入新版本。
func TestCommitChapterRejectsPolishWithoutDraftChange(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 10); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// 模拟第 2 章已正常完成：drafts 与 chapters 内容相同。
	original := "第二章原始正文内容，用于模拟已提交终稿。"
	if err := s.Drafts.SaveDraft(2, original); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(2, original); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(2, len([]rune(original)), "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}

	// 进入打磨队列：Flow=Polishing, PendingRewrites=[2]
	if err := s.Progress.SetPendingRewrites([]int{2}, "测试打磨"); err != nil {
		t.Fatalf("SetPendingRewrites: %v", err)
	}
	if err := s.Progress.SetFlow(domain.FlowPolishing); err != nil {
		t.Fatalf("SetFlow: %v", err)
	}

	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "假装打磨了",
		"characters": []string{"主角"},
		"key_events": []string{"无改动"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to be rejected when drafts equals final content")
	}

	// 再写一版不同的草稿 → 应该通过
	polished := original + "\n\n打磨后新增段落。"
	if err := s.Drafts.SaveDraft(2, polished); err != nil {
		t.Fatalf("SaveDraft (polished): %v", err)
	}
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute after real polish: %v", err)
	}
}

// TestCommitChapterLayeredRejectsOutOfRangeChapter 验证分层模式下，
// 章号越出 layered_outline 的 commit 必须硬失败，而不是 slog.Warn 放行。
// 这是阻止"裁定误判后 writer 一路裸跑"的物理刹车（《凡骨》ch204..347 案例）。
func TestCommitChapterLayeredRejectsOutOfRangeChapter(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// 建一份 layered_outline，只有 1 卷 1 弧 1 章
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "卷一", "theme": "主题",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧一", "goal": "目标",
				"chapters": []map[string]any{
					{"title": "首章", "core_event": "起", "hook": "续"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	// 越界章节 2 的 commit 必须硬失败
	if err := s.Drafts.SaveDraft(2, "越界章节正文，必须被拦下。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter":    2,
		"summary":    "越界章节",
		"characters": []string{"主角"},
		"key_events": []string{"不该被允许"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected commit to fail when chapter out of layered outline range")
	}

	// 章节文件不应落盘、Progress 不应推进
	if _, statErr := os.Stat(dir + "/chapters/02.md"); !os.IsNotExist(statErr) {
		t.Fatalf("chapter 2 should not be persisted, stat err=%v", statErr)
	}
	progress, _ := s.Progress.Load()
	if len(progress.CompletedChapters) != 0 {
		t.Fatalf("CompletedChapters should stay empty, got %v", progress.CompletedChapters)
	}
}

// TestCommitChapterLayeredAutoCompletesWhenDone 验证分层模式确定性完结兜底：
// 大纲全部展开并写完 + 无骨架弧 + 无返工 + 活跃伏笔为零 + 指南针长线收束时，
// 最后一章 commit 自动推 Phase=Complete，不依赖架构师主动调 complete_book。
// 这是 9bf26a5 删掉分层自动完结后引入的 livelock 的修复（终卷末尾模型既不 append
// 也不 complete → 写手裸跑越界死循环）。
func TestCommitChapterLayeredAutoCompletesWhenDone(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	// 单卷单弧两章，全部展开（无骨架弧）
	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "卷一", "theme": "主题",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧一", "goal": "目标",
				"chapters": []map[string]any{
					{"title": "首章", "core_event": "起", "hook": "续"},
					{"title": "次章", "core_event": "承", "hook": "终"},
				},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// 指南针长线已收束（OpenThreads 空）
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "主角归乡"}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	tool := NewCommitChapterTool(s)
	commit := func(ch int) map[string]any {
		if err := s.Drafts.SaveDraft(ch, fmt.Sprintf("第 %d 章正文内容，用于测试确定性完结。", ch)); err != nil {
			t.Fatalf("SaveDraft %d: %v", ch, err)
		}
		args, _ := json.Marshal(map[string]any{
			"chapter": ch, "summary": "摘要", "characters": []string{"主角"}, "key_events": []string{"事件"},
		})
		raw, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute ch%d: %v", ch, err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal ch%d: %v", ch, err)
		}
		return out
	}

	// 第 1 章：未写完，不应完结
	if bc, _ := commit(1)["book_complete"].(bool); bc {
		t.Fatal("写完第 1 章不应触发完结")
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("写完第 1 章 phase 不应为 complete")
	}

	// 第 2 章（最后一章）：应自动完结
	if bc, _ := commit(2)["book_complete"].(bool); !bc {
		t.Fatal("写完最后一章应自动完结")
	}
	if p, _ := s.Progress.Load(); p.Phase != domain.PhaseComplete {
		t.Fatalf("expected phase=complete, got %s", p.Phase)
	}
}

// TestCommitChapterLayeredNoAutoCompleteWithOpenThreads 验证保守性：仍有活跃长线时
// 即使章节写满也不自动完结，把"是否继续"的裁定权留给架构师。
func TestCommitChapterLayeredNoAutoCompleteWithOpenThreads(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 0); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	foundation := NewSaveFoundationTool(s)
	layeredArgs, _ := json.Marshal(map[string]any{
		"type": "layered_outline",
		"content": []map[string]any{{
			"index": 1, "title": "卷一", "theme": "主题",
			"arcs": []map[string]any{{
				"index": 1, "title": "弧一", "goal": "目标",
				"chapters": []map[string]any{{"title": "首章", "core_event": "起", "hook": "续"}},
			}},
		}},
		"scale": "long",
	})
	if _, err := foundation.Execute(context.Background(), layeredArgs); err != nil {
		t.Fatalf("Execute layered: %v", err)
	}
	// 仍有未收束的活跃长线
	if err := s.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "主角归乡", OpenThreads: []string{"宿敌未除"}}); err != nil {
		t.Fatalf("SaveCompass: %v", err)
	}
	_ = s.Progress.UpdatePhase(domain.PhaseWriting)

	if err := s.Drafts.SaveDraft(1, "唯一一章的正文，但长线未收束。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	tool := NewCommitChapterTool(s)
	args, _ := json.Marshal(map[string]any{
		"chapter": 1, "summary": "摘要", "characters": []string{"主角"}, "key_events": []string{"事件"},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if p, _ := s.Progress.Load(); p.Phase == domain.PhaseComplete {
		t.Fatal("活跃长线未收束时不应自动完结")
	}
}
