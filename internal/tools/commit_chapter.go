package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
	styleanalyzer "github.com/voocel/ainovel-cli/internal/style"
)

// CommitChapterTool 提交章节：加载正文 → 保存终稿 → 生成摘要 → 更新状态 → 更新进度。
type CommitChapterTool struct {
	store     *store.Store
	rulesOpts rules.LoadOptions // 可选；空 LoadOptions 时不产生 rule_violations
}

func NewCommitChapterTool(store *store.Store) *CommitChapterTool {
	return &CommitChapterTool{store: store}
}

// WithRules 注入用户规则加载选项，使 rule_violations 中附带用户规则检查结果。
// 不调用此方法时仅执行内置底线 Lint（机制残留检查，始终开启）。
func (t *CommitChapterTool) WithRules(opts rules.LoadOptions) *CommitChapterTool {
	t.rulesOpts = opts
	return t
}

// commitOutput 在 domain.CommitResult 之上嵌入扩展字段，保持 domain 包不依赖 rules。
// 由于嵌入字段会被 JSON marshaler 提升（promoted），序列化结果等同于扁平结构。
type commitOutput struct {
	domain.CommitResult
	RuleViolations []rules.Violation  `json:"rule_violations,omitempty"`
	StyleStats     *domain.StyleStats `json:"style_stats,omitempty"`
}

func (t *CommitChapterTool) Name() string { return "commit_chapter" }
func (t *CommitChapterTool) Description() string {
	return "提交章节终稿。加载草稿正文保存为终稿，更新时间线、伏笔、关系、角色状态和进度。" +
		"返回结构化事实：next_chapter / review_required / arc_end / volume_end / needs_expansion / book_complete / flow 等"
}
func (t *CommitChapterTool) Label() string { return "提交章节" }

// 写工具（跨域原子操作：草稿→终稿→摘要→进度→checkpoint），禁止并发。
func (t *CommitChapterTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *CommitChapterTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *CommitChapterTool) Schema() map[string]any {
	timelineSchema := schema.Object(
		schema.Property("time", schema.String("故事内时间")).Required(),
		schema.Property("event", schema.String("事件描述")).Required(),
		schema.Property("characters", schema.Array("涉及角色", schema.String(""))),
	)
	foreshadowSchema := schema.Object(
		schema.Property("id", schema.String("伏笔 ID")).Required(),
		schema.Property("action", schema.Enum("操作", "plant", "advance", "resolve")).Required(),
		schema.Property("description", schema.String("伏笔描述（仅 plant 时必需）")),
	)
	relationshipSchema := schema.Object(
		schema.Property("character_a", schema.String("角色 A")).Required(),
		schema.Property("character_b", schema.String("角色 B")).Required(),
		schema.Property("relation", schema.String("当前关系描述")).Required(),
	)
	stateChangeSchema := schema.Object(
		schema.Property("entity", schema.String("角色名或实体名")).Required(),
		schema.Property("field", schema.String("变化属性")).Required(),
		schema.Property("old_value", schema.String("变化前的值")),
		schema.Property("new_value", schema.String("变化后的值")).Required(),
		schema.Property("reason", schema.String("变化原因")),
	)
	feedbackSchema := schema.Object(
		schema.Property("deviation", schema.String("偏离大纲的描述")).Required(),
		schema.Property("suggestion", schema.String("对后续大纲的调整建议")).Required(),
	)
	return schema.Object(
		schema.Property("chapter", schema.Int("章节号")).Required(),
		schema.Property("summary", schema.String("本章内容摘要（200字以内）")).Required(),
		schema.Property("characters", schema.Array("本章出场角色名", schema.String(""))).Required(),
		schema.Property("key_events", schema.Array("本章关键事件", schema.String(""))).Required(),
		schema.Property("timeline_events", schema.Array("本章时间线事件", timelineSchema)),
		schema.Property("foreshadow_updates", schema.Array("伏笔操作", foreshadowSchema)),
		schema.Property("relationship_changes", schema.Array("关系变化", relationshipSchema)),
		schema.Property("state_changes", schema.Array("角色/实体状态变化", stateChangeSchema)),
		schema.Property("cast_intros", schema.Array("本章首次引入且后续可能再出现的次要角色简介（不含主角及 characters.json 已有角色）", schema.Object(
			schema.Property("name", schema.String("角色名")).Required(),
			schema.Property("brief_role", schema.String("一句话定位（如：客栈老板/赌坊打手）")).Required(),
		))),
		schema.Property("hook_type", schema.Enum("章末钩子类型", "crisis", "mystery", "desire", "emotion", "choice")),
		schema.Property("dominant_strand", schema.Enum("本章主导叙事线", "quest", "fire", "constellation")),
		schema.Property("feedback", feedbackSchema),
	)
}

func (t *CommitChapterTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter             int                        `json:"chapter"`
		Summary             string                     `json:"summary"`
		Characters          []string                   `json:"characters"`
		KeyEvents           []string                   `json:"key_events"`
		TimelineEvents      []domain.TimelineEvent     `json:"timeline_events"`
		ForeshadowUpdates   []domain.ForeshadowUpdate  `json:"foreshadow_updates"`
		RelationshipChanges []domain.RelationshipEntry `json:"relationship_changes"`
		StateChanges        []domain.StateChange       `json:"state_changes"`
		CastIntros          []domain.CastIntro         `json:"cast_intros"`
		HookType            string                     `json:"hook_type"`
		DominantStrand      string                     `json:"dominant_strand"`
		Feedback            *domain.OutlineFeedback    `json:"feedback"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if t.store.Progress.IsChapterCompleted(a.Chapter) {
		// 清理可能残留的 PendingCommit（崩溃发生在 ProgressMarked 之后、ClearPendingCommit 之前）
		if pending, _ := t.store.Signals.LoadPendingCommit(); pending != nil && pending.Chapter == a.Chapter {
			_ = t.store.Signals.ClearPendingCommit()
		}
		// 打磨/重写路径：章节虽已完成，但仍在 pending_rewrites 中，允许覆盖并 drain 队列
		progress, _ := t.store.Progress.Load()
		if progress != nil && slices.Contains(progress.PendingRewrites, a.Chapter) {
			return t.executeRewriteCommit(a.Chapter, a.Summary, a.Characters, a.KeyEvents,
				a.HookType, a.DominantStrand, progress)
		}
		return t.buildSkipResult(a.Chapter, progress)
	}
	existingPending, err := t.store.Signals.LoadPendingCommit()
	if err != nil {
		return nil, fmt.Errorf("load pending commit: %w: %w", errs.ErrStoreRead, err)
	}
	if existingPending != nil && existingPending.Chapter != a.Chapter {
		return nil, fmt.Errorf("存在未恢复的章节提交：第 %d 章（阶段 %s），请先恢复或重新提交该章: %w", existingPending.Chapter, existingPending.Stage, errs.ErrToolConflict)
	}
	if err := t.store.Progress.ValidateChapterWork(a.Chapter); err != nil {
		// 队列冲突保持原样（已带 ErrToolConflict 分类）；其他 IO 错误归 Precondition。
		if errors.Is(err, errs.ErrToolConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("章节当前不允许提交: %w: %w", errs.ErrToolPrecondition, err)
	}

	// 分层模式越界拦截：必须先于任何写操作，否则越界 commit 会把章节文件、摘要、
	// Progress 都改坏。boundary 复用给下方第 6b 步算弧/卷信号。
	var boundary *store.ArcBoundary
	if progress, perr := t.store.Progress.Load(); perr == nil && progress != nil && progress.Layered {
		b, bErr := t.store.Outline.CheckArcBoundary(a.Chapter)
		if bErr != nil {
			return nil, fmt.Errorf("弧边界检测失败 chapter=%d: %w: %w", a.Chapter, errs.ErrStoreRead, bErr)
		}
		if b == nil {
			return nil, fmt.Errorf(
				"第 %d 章不在分层大纲范围内：写作必须先 expand_arc 扩展弧或 append_volume 追加卷；若全书已完结请调 save_foundation type=complete_book: %w",
				a.Chapter, errs.ErrToolPrecondition)
		}
		boundary = b
	}

	// 1. 加载章节正文
	content, wordCount, err := t.store.Drafts.LoadChapterContent(a.Chapter)
	if err != nil {
		return nil, fmt.Errorf("load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", a.Chapter, errs.ErrToolPrecondition)
	}

	now := time.Now().Format(time.RFC3339)
	pending := domain.PendingCommit{
		Chapter:        a.Chapter,
		Stage:          domain.CommitStageStarted,
		Summary:        a.Summary,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("save pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 2. 保存终稿
	if err := t.store.Drafts.SaveFinalChapter(a.Chapter, content); err != nil {
		return nil, fmt.Errorf("save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. 保存摘要
	summary := domain.ChapterSummary{
		Chapter:    a.Chapter,
		Summary:    a.Summary,
		Characters: a.Characters,
		KeyEvents:  a.KeyEvents,
	}
	if err := t.store.Summaries.SaveSummary(summary); err != nil {
		return nil, fmt.Errorf("save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	// 4. 更新状态增量
	if len(a.TimelineEvents) > 0 {
		for i := range a.TimelineEvents {
			a.TimelineEvents[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendTimelineEvents(a.TimelineEvents); err != nil {
			return nil, fmt.Errorf("append timeline: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.ForeshadowUpdates) > 0 {
		if err := t.store.World.UpdateForeshadow(a.Chapter, a.ForeshadowUpdates); err != nil {
			return nil, fmt.Errorf("update foreshadow: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.RelationshipChanges) > 0 {
		for i := range a.RelationshipChanges {
			a.RelationshipChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.UpdateRelationships(a.RelationshipChanges); err != nil {
			return nil, fmt.Errorf("update relationships: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	if len(a.StateChanges) > 0 {
		for i := range a.StateChanges {
			a.StateChanges[i].Chapter = a.Chapter
		}
		if err := t.store.World.AppendStateChanges(a.StateChanges); err != nil {
			return nil, fmt.Errorf("append state changes: %w: %w", errs.ErrStoreWrite, err)
		}
	}

	// 4b. 累加配角名册：本章出场的非核心角色进 cast_ledger，供 novel_context 召回。
	// 失败时只 warn 不阻断 commit——名册是次要数据，可通过下一章 commit 自愈。
	if len(a.Characters) > 0 {
		coreNames := loadCoreCharacterNameSet(t.store)
		if err := t.store.Cast.MergeAppearances(a.Chapter, a.Characters, a.CastIntros, coreNames); err != nil {
			slog.Warn("配角名册累加失败，跳过", "module", "commit", "chapter", a.Chapter, "err", err)
		}
	}

	// 4c. 机械规则检查与自然度统计（仅返事实，不阻断 flow / verdict）。
	// style_stats 必须在 Progress 标记完成前落盘：否则写入失败会造成章节已完成但统计缺失，重试只能走 skip path。
	violations := t.checkRules(content, wordCount)
	styleStats, err := t.analyzeAndSaveStyleStats(a.Chapter, content)
	if err != nil {
		return nil, fmt.Errorf("save style stats: %w: %w", errs.ErrStoreWrite, err)
	}
	t.refreshDiagnosticGuidance(a.Chapter, styleStats, nil)

	pending.Stage = domain.CommitStageStateApplied
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit stage: %w: %w", errs.ErrStoreWrite, err)
	}

	// 5. 更新进度
	if err := t.store.Progress.MarkChapterComplete(a.Chapter, wordCount, a.HookType, a.DominantStrand); err != nil {
		return nil, fmt.Errorf("mark chapter complete: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. 判断是否需要审阅
	progress, err := t.store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w: %w", errs.ErrStoreRead, err)
	}
	completedCount := 0
	if progress != nil {
		completedCount = len(progress.CompletedChapters)
	}

	// 6b. 长篇模式弧/卷信号：boundary 已在入口前置校验，Layered 时保证非 nil
	var arcEnd, volumeEnd, needsExpansion, needsNewVolume bool
	var vol, arc, nextVol, nextArc int
	if progress != nil && progress.Layered && boundary != nil {
		arcEnd = boundary.IsArcEnd
		volumeEnd = boundary.IsVolumeEnd
		vol = boundary.Volume
		arc = boundary.Arc
		needsExpansion = boundary.NeedsExpansion
		needsNewVolume = boundary.NeedsNewVolume
		nextVol = boundary.NextVolume
		nextArc = boundary.NextArc
		_ = t.store.Progress.UpdateVolumeArc(vol, arc)
	}

	var reviewRequired bool
	var reviewReason string
	if progress != nil && progress.Layered {
		reviewRequired, reviewReason = domain.ShouldArcReview(arcEnd, volumeEnd, vol, arc)
	} else {
		reviewRequired, reviewReason = domain.ShouldReview(completedCount)
	}

	// 7. 构造结构化信号
	result := domain.CommitResult{
		Chapter:        a.Chapter,
		Committed:      true,
		WordCount:      wordCount,
		NextChapter:    a.Chapter + 1,
		ReviewRequired: reviewRequired,
		ReviewReason:   reviewReason,
		HookType:       a.HookType,
		DominantStrand: a.DominantStrand,
		Feedback:       a.Feedback,
		ArcEnd:         arcEnd,
		VolumeEnd:      volumeEnd,
		Volume:         vol,
		Arc:            arc,
		NeedsExpansion: needsExpansion,
		NeedsNewVolume: needsNewVolume,
		NextVolume:     nextVol,
		NextArc:        nextArc,
	}

	// 8. 完成态判定：非分层写完最后一章 / 分层最终卷最后一章 → MarkComplete
	if t.applyCompletion(&result, progress) {
		result.BookComplete = true
	}
	if p, _ := t.store.Progress.Load(); p != nil {
		result.Flow = string(p.Flow)
	}

	pending.Stage = domain.CommitStageProgressMarked
	pending.Result = &result
	pending.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := t.store.Signals.SavePendingCommit(pending); err != nil {
		return nil, fmt.Errorf("update pending commit result: %w: %w", errs.ErrStoreWrite, err)
	}

	// 9. 清除进度中间状态
	if err := t.store.Progress.ClearInProgress(); err != nil {
		return nil, fmt.Errorf("clear in-progress: %w: %w", errs.ErrStoreWrite, err)
	}
	if err := t.store.Signals.ClearPendingCommit(); err != nil {
		return nil, fmt.Errorf("clear pending commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 10. 追加 checkpoint
	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(a.Chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", a.Chapter),
	); err != nil {
		return nil, fmt.Errorf("checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	return json.Marshal(commitOutput{CommitResult: result, RuleViolations: violations, StyleStats: styleStats})
}

// checkRules 对章节正文做机械检查：内置产品底线 Lint（机制残留，始终执行）
// + 用户规则 Check（rulesOpts 全空时 loader 返回空 layers，checker 返 nil）。
func (t *CommitChapterTool) checkRules(text string, wordCount int) []rules.Violation {
	violations := rules.Lint(text)
	bundle := rules.Merge(rules.Load(t.rulesOpts))
	return append(violations, rules.Check(text, wordCount, bundle.Structured)...)
}

func (t *CommitChapterTool) analyzeAndSaveStyleStats(chapter int, text string) (*domain.StyleStats, error) {
	stats := styleanalyzer.AnalyzeChineseProse(text)
	stats.Chapter = chapter
	if meta, err := t.store.RunMeta.Load(); err == nil && meta != nil {
		stats.Model = domain.StyleModelInfo{Provider: meta.Provider, Model: meta.Model}
	}
	if err := t.store.World.SaveStyleStats(*stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// executeRewriteCommit 处理打磨/重写章节的提交：覆盖终稿与摘要、更新字数、drain 队列。
// 跳过所有世界状态追加（timeline / foreshadow / relationship / state_changes）与弧边界检测，
// 这些已在章节原始提交时应用。
func (t *CommitChapterTool) executeRewriteCommit(
	chapter int,
	summary string,
	characters, keyEvents []string,
	hookType, dominantStrand string,
	progress *domain.Progress,
) (json.RawMessage, error) {
	// 1. 加载打磨后的正文
	content, wordCount, err := t.store.Drafts.LoadChapterContent(chapter)
	if err != nil {
		return nil, fmt.Errorf("rewrite: load chapter content: %w: %w", errs.ErrStoreRead, err)
	}
	if content == "" {
		return nil, fmt.Errorf("no content found for chapter %d: %w", chapter, errs.ErrToolPrecondition)
	}

	// 2. 硬校验：drafts 与现终稿完全相同 → 判定为未真正打磨/重写（writer 跳过了 draft_chapter）
	// 拒绝 commit，强制 writer 先调 draft_chapter(mode=write) 写入新版本。
	existingFinal, _ := t.store.Drafts.LoadChapterText(chapter)
	if existingFinal != "" && existingFinal == content {
		mode := "重写"
		if progress != nil && progress.Flow == domain.FlowPolishing {
			mode = "打磨"
		}
		return nil, fmt.Errorf("第 %d 章 drafts 与 chapters 内容完全相同，未检测到%s改动。请先调 draft_chapter(mode=write, chapter=%d) 写入%s后的新正文，再 commit_chapter: %w",
			chapter, mode, chapter, mode, errs.ErrToolPrecondition)
	}
	previousStats, _ := t.store.World.LoadStyleStats(chapter)

	// 3. 覆盖终稿
	if err := t.store.Drafts.SaveFinalChapter(chapter, content); err != nil {
		return nil, fmt.Errorf("rewrite: save final chapter: %w: %w", errs.ErrStoreWrite, err)
	}

	// 3. 覆盖摘要
	if err := t.store.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter:    chapter,
		Summary:    summary,
		Characters: characters,
		KeyEvents:  keyEvents,
	}); err != nil {
		return nil, fmt.Errorf("rewrite: save summary: %w: %w", errs.ErrStoreWrite, err)
	}

	mode := "rewrite"
	if progress.Flow == domain.FlowPolishing {
		mode = "polish"
	}

	// 4. 同主路径：rewrite/polish 也做机械检查与自然度统计。
	// 统计和对比必须先于队列 drain，避免返回错误后队列已清、后续无法补齐诊断事实。
	violations := t.checkRules(content, wordCount)
	styleStats, err := t.analyzeAndSaveStyleStats(chapter, content)
	if err != nil {
		return nil, fmt.Errorf("rewrite: save style stats: %w: %w", errs.ErrStoreWrite, err)
	}
	comparison, err := t.saveStyleRewriteComparison(chapter, mode, existingFinal, content, previousStats, styleStats)
	if err != nil {
		return nil, fmt.Errorf("rewrite: save style comparison: %w: %w", errs.ErrStoreWrite, err)
	}
	t.refreshDiagnosticGuidance(chapter, styleStats, comparison)

	// 5. 更新字数（MarkChapterComplete 对已完成章节是幂等的：replaces word count, slice.Contains 防止重复入队）
	if err := t.store.Progress.MarkChapterComplete(chapter, wordCount, hookType, dominantStrand); err != nil {
		return nil, fmt.Errorf("rewrite: update word count: %w: %w", errs.ErrStoreWrite, err)
	}

	// 6. Drain 待处理队列；队列空时 CompleteRewrite 会自动把 flow 切回 writing
	if err := t.store.Progress.CompleteRewrite(chapter); err != nil {
		return nil, fmt.Errorf("rewrite: complete rewrite: %w: %w", errs.ErrStoreWrite, err)
	}

	// 7. Checkpoint
	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ChapterScope(chapter), "commit",
		fmt.Sprintf("chapters/%02d.md", chapter),
	); err != nil {
		return nil, fmt.Errorf("rewrite: checkpoint commit: %w: %w", errs.ErrStoreWrite, err)
	}

	// 8. 读取 drain 后的 Progress 快照，作为事实返回
	latest, _ := t.store.Progress.Load()
	remaining := []int{}
	nextChapter := chapter + 1
	flow := string(domain.FlowWriting)
	if latest != nil {
		remaining = append(remaining, latest.PendingRewrites...)
		nextChapter = latest.NextChapter()
		flow = string(latest.Flow)
	}
	drained := len(remaining) == 0

	// 队列清空后再判完结：分层长篇若"最后一章恰好走返工路径"，完结只能在此触发
	// （主路径 applyCompletion 不经过 rewrite 提交）——补上旧模型这处时序缺口。
	bookComplete := false
	if drained && latest != nil && latest.Layered && t.layeredBookComplete(latest) {
		if cerr := t.store.Progress.MarkComplete(); cerr == nil {
			bookComplete = true
			if p, _ := t.store.Progress.Load(); p != nil {
				flow = string(p.Flow)
			}
		}
	}

	return json.Marshal(map[string]any{
		"chapter":                  chapter,
		"rewritten":                true,
		"mode":                     mode,
		"word_count":               wordCount,
		"remaining_queue":          remaining,
		"queue_drained":            drained,
		"next_chapter":             nextChapter,
		"flow":                     flow,
		"book_complete":            bookComplete,
		"rule_violations":          violations,
		"style_stats":              styleStats,
		"style_rewrite_comparison": comparison,
	})
}

func (t *CommitChapterTool) saveStyleRewriteComparison(chapter int, mode, beforeText, afterText string, before, after *domain.StyleStats) (*domain.StyleRewriteComparison, error) {
	if before == nil || after == nil {
		return nil, nil
	}
	editDistanceRatio, changedRuneRatio := rewriteTextChange(beforeText, afterText)
	comparison := domain.StyleRewriteComparison{
		SchemaVersion:     domain.StyleRewriteComparisonSchemaVersion,
		Chapter:           chapter,
		Mode:              mode,
		ComputedAt:        time.Now().Format(time.RFC3339),
		Before:            before,
		After:             after,
		EditDistanceRatio: editDistanceRatio,
		ChangedRuneRatio:  changedRuneRatio,
		Deltas:            map[string]float64{},
	}
	for _, key := range styleComparisonMetricKeys() {
		beforeMetric, beforeOK := before.Metrics[key]
		afterMetric, afterOK := after.Metrics[key]
		if !beforeOK || !afterOK {
			continue
		}
		delta := afterMetric.Value - beforeMetric.Value
		comparison.Deltas[key+"_delta"] = delta
		switch classifyStyleMetricDelta(key, delta) {
		case "improved":
			comparison.ImprovedMetrics = append(comparison.ImprovedMetrics, key)
		case "worsened":
			comparison.WorsenedMetrics = append(comparison.WorsenedMetrics, key)
		case "unchanged":
			comparison.UnchangedMetrics = append(comparison.UnchangedMetrics, key)
		}
	}
	if len(comparison.Deltas) == 0 {
		comparison.Deltas = nil
	}
	if err := t.store.World.AppendStyleRewriteComparison(comparison); err != nil {
		return nil, err
	}
	return &comparison, nil
}

func rewriteTextChange(beforeText, afterText string) (float64, float64) {
	beforeRunes := []rune(beforeText)
	afterRunes := []rune(afterText)
	maxLen := max(len(beforeRunes), len(afterRunes))
	if maxLen == 0 {
		return 0, 0
	}
	distance := levenshteinCapped(beforeRunes, afterRunes, 2000)
	editRatio := float64(distance) / float64(maxLen)
	changedRatio := float64(distance) / float64(maxLen)
	return round2(editRatio), round2(changedRatio)
}

func levenshteinCapped(a, b []rune, capLen int) int {
	if len(a) > capLen || len(b) > capLen {
		return sampledRuneDistance(a, b, capLen)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev = curr
	}
	return prev[len(b)]
}

func sampledRuneDistance(a, b []rune, capLen int) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	as := sampleRunes(a, capLen)
	bs := sampleRunes(b, capLen)
	base := levenshteinCapped(as, bs, capLen)
	return int(math.Round(float64(base) * float64(max(len(a), len(b))) / float64(max(len(as), len(bs)))))
}

func sampleRunes(runes []rune, limit int) []rune {
	if len(runes) <= limit {
		return runes
	}
	out := make([]rune, 0, limit)
	step := float64(len(runes)) / float64(limit)
	for i := 0; i < limit; i++ {
		out = append(out, runes[int(float64(i)*step)])
	}
	return out
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func styleComparisonMetricKeys() []string {
	return []string{
		domain.StyleMetricSentenceLengthStddev,
		domain.StyleMetricSentenceStartUniqueRate,
		domain.StyleMetricSentenceStartDominantCategoryRatio,
		domain.StyleMetricSentenceStartAbstractConnectorRatio,
		domain.StyleMetricParagraphLengthStddev,
		domain.StyleMetricParagraphUniformRatio,
		domain.StyleMetricPatternDensityPer1000,
		domain.StyleMetricRepeatedNGramRate,
		domain.StyleMetricHomogeneousSentenceRatio,
		domain.StyleMetricEmotionLabelDensityPer1000,
	}
}

func classifyStyleMetricDelta(key string, delta float64) string {
	const epsilon = 0.01
	if delta > -epsilon && delta < epsilon {
		return "unchanged"
	}
	switch key {
	case domain.StyleMetricSentenceLengthStddev, domain.StyleMetricSentenceStartUniqueRate, domain.StyleMetricParagraphLengthStddev:
		if delta > 0 {
			return "improved"
		}
		return "worsened"
	case domain.StyleMetricParagraphUniformRatio, domain.StyleMetricPatternDensityPer1000, domain.StyleMetricRepeatedNGramRate, domain.StyleMetricHomogeneousSentenceRatio, domain.StyleMetricEmotionLabelDensityPer1000, domain.StyleMetricSentenceStartDominantCategoryRatio, domain.StyleMetricSentenceStartAbstractConnectorRatio:
		if delta < 0 {
			return "improved"
		}
		return "worsened"
	default:
		return "unchanged"
	}
}

// buildSkipResult 为"章节已完成的重复提交"构造与正常 commit 对齐的事实返回。
// 协调者据此做后续决策（writer/editor/architect 派发），而不会因为拿到 prose 提示而幻觉。
func (t *CommitChapterTool) buildSkipResult(chapter int, progress *domain.Progress) (json.RawMessage, error) {
	content, wordCount, err := t.loadCommittedChapterContent(chapter)
	if err != nil {
		return nil, fmt.Errorf("load committed chapter content: %w: %w", errs.ErrStoreRead, err)
	}

	result := domain.CommitResult{
		Chapter:     chapter,
		Committed:   true,
		WordCount:   wordCount,
		NextChapter: chapter + 1,
	}

	if progress != nil && progress.Layered {
		if boundary, _ := t.store.Outline.CheckArcBoundary(chapter); boundary != nil {
			result.ArcEnd = boundary.IsArcEnd
			result.VolumeEnd = boundary.IsVolumeEnd
			result.Volume = boundary.Volume
			result.Arc = boundary.Arc
			result.NeedsExpansion = boundary.NeedsExpansion
			result.NeedsNewVolume = boundary.NeedsNewVolume
			result.NextVolume = boundary.NextVolume
			result.NextArc = boundary.NextArc
		}
		result.ReviewRequired, result.ReviewReason = domain.ShouldArcReview(result.ArcEnd, result.VolumeEnd, result.Volume, result.Arc)
	} else if progress != nil {
		result.ReviewRequired, result.ReviewReason = domain.ShouldReview(len(progress.CompletedChapters))
	}

	if progress != nil {
		if progress.Phase == domain.PhaseComplete {
			result.BookComplete = true
		}
		result.Flow = string(progress.Flow)
	}

	styleStats, err := t.store.World.LoadStyleStats(chapter)
	if err != nil {
		return nil, fmt.Errorf("load style stats: %w: %w", errs.ErrStoreRead, err)
	}
	if styleStats == nil && content != "" {
		styleStats, err = t.analyzeAndSaveStyleStats(chapter, content)
		if err != nil {
			return nil, fmt.Errorf("backfill style stats: %w: %w", errs.ErrStoreWrite, err)
		}
	}
	t.refreshDiagnosticGuidance(chapter, styleStats, nil)
	violations := t.checkRules(content, wordCount)
	return json.Marshal(commitOutput{CommitResult: result, RuleViolations: violations, StyleStats: styleStats})
}

func (t *CommitChapterTool) refreshDiagnosticGuidance(chapter int, stats *domain.StyleStats, comparison *domain.StyleRewriteComparison) {
	guidance := buildCommitDiagnosticGuidance(chapter, stats, comparison)
	if len(guidance.Items) == 0 {
		return
	}
	if err := t.store.World.SaveDiagnosticGuidance(guidance); err != nil {
		slog.Warn("诊断回流写入失败，跳过", "module", "commit", "chapter", chapter, "err", err)
	}
}

func buildCommitDiagnosticGuidance(chapter int, stats *domain.StyleStats, comparison *domain.StyleRewriteComparison) domain.DiagnosticGuidance {
	guidance := domain.DiagnosticGuidance{
		SchemaVersion: domain.DiagnosticGuidanceSchemaVersion,
		GeneratedAt:   time.Now().Format(time.RFC3339),
	}
	if stats != nil {
		if item, ok := styleHotspotGuidance(chapter, stats); ok {
			guidance.Items = append(guidance.Items, item)
		}
		guidance.Items = append(guidance.Items, styleMetricGuidance(chapter, stats)...)
	}
	if comparison != nil {
		if item, ok := rewriteComparisonGuidance(*comparison); ok {
			guidance.Items = append(guidance.Items, item)
		}
	}
	if len(guidance.Items) > 4 {
		guidance.Items = guidance.Items[:4]
	}
	return guidance
}

func styleHotspotGuidance(chapter int, stats *domain.StyleStats) (domain.DiagnosticGuidanceItem, bool) {
	if stats == nil || len(stats.Hotspots) == 0 {
		return domain.DiagnosticGuidanceItem{}, false
	}
	counts := map[string]int{}
	for _, hotspot := range stats.Hotspots {
		if hotspot.RuleID != "" {
			counts[hotspot.RuleID]++
		}
	}
	if len(counts) == 0 {
		return domain.DiagnosticGuidanceItem{}, false
	}
	topRule := ""
	topCount := 0
	for rule, count := range counts {
		if count > topCount || (count == topCount && rule < topRule) {
			topRule = rule
			topCount = count
		}
	}
	return domain.DiagnosticGuidanceItem{
		Rule:       "AIFlavorHotspots",
		Severity:   hotspotGuidanceSeverity(stats.Hotspots),
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("第 %d 章 AI 味热点：%s ×%d", chapter, topRule, topCount),
		Signal:     fmt.Sprintf("chapter=%d; hotspots=%d; top_rule=%s", chapter, len(stats.Hotspots), topRule),
		Suggestion: "下一章或返工时优先按 style_stats.hotspots 定位同类句式，做局部 spot-fix，避免直接整章重写。",
	}, true
}

func hotspotGuidanceSeverity(hotspots []domain.StyleHotspot) string {
	for _, hotspot := range hotspots {
		if hotspot.Severity == "error" || hotspot.Severity == "warning" {
			return "warning"
		}
	}
	return "info"
}

func styleMetricGuidance(chapter int, stats *domain.StyleStats) []domain.DiagnosticGuidanceItem {
	if stats == nil || len(stats.Metrics) == 0 {
		return nil
	}
	var items []domain.DiagnosticGuidanceItem
	if value, ok := styleMetricValue(stats, domain.StyleMetricEmotionLabelDensityPer1000); ok && value >= domain.StyleThresholdEmotionLabelDensity {
		items = append(items, domain.DiagnosticGuidanceItem{
			Rule:       "EmotionLabelDensity",
			Severity:   "warning",
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("第 %d 章情绪标签密度偏高", chapter),
			Signal:     fmt.Sprintf("chapter=%d; emotion_label_density_per_1000=%.2f", chapter, value),
			Suggestion: "把紧张/愤怒/悲伤等标签改成身体反应、动作选择和环境压力。",
		})
	}
	if value, ok := styleMetricValue(stats, domain.StyleMetricSentenceStartDominantCategoryRatio); ok && value >= domain.StyleThresholdSentenceStartDominantRatio {
		items = append(items, domain.DiagnosticGuidanceItem{
			Rule:       "SentenceStartDominance",
			Severity:   "info",
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("第 %d 章句首类别过于集中", chapter),
			Signal:     fmt.Sprintf("chapter=%d; sentence_start_dominant_category_ratio=%.2f", chapter, value),
			Suggestion: "后续写作轮换对白、动作、感官、环境和无主语句开头，避免固定起手方式。",
		})
	}
	return items
}

func styleMetricValue(stats *domain.StyleStats, key string) (float64, bool) {
	if stats == nil || stats.Metrics == nil {
		return 0, false
	}
	metric, ok := stats.Metrics[key]
	return metric.Value, ok
}

func rewriteComparisonGuidance(comparison domain.StyleRewriteComparison) (domain.DiagnosticGuidanceItem, bool) {
	lowEdit := comparison.EditDistanceRatio > 0 && comparison.EditDistanceRatio < domain.StyleThresholdLowEditDistanceRatio
	if !lowEdit && len(comparison.WorsenedMetrics) == 0 && !(len(comparison.ImprovedMetrics) == 0 && len(comparison.UnchangedMetrics) >= 3) {
		return domain.DiagnosticGuidanceItem{}, false
	}
	severity := "info"
	if lowEdit || len(comparison.WorsenedMetrics) >= 2 {
		severity = "warning"
	}
	return domain.DiagnosticGuidanceItem{
		Rule:       "RewriteEffectiveness",
		Severity:   severity,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("第 %d 章%s后风格指标未明显改善", comparison.Chapter, commitRewriteModeLabel(comparison.Mode)),
		Signal:     fmt.Sprintf("chapter=%d; mode=%s; edit_distance_ratio=%.2f; worsened=%d; unchanged=%d", comparison.Chapter, comparison.Mode, comparison.EditDistanceRatio, len(comparison.WorsenedMetrics), len(comparison.UnchangedMetrics)),
		Suggestion: "返工应对准 style_stats.hotspots 和 review issue targets，避免只做表层换词。",
	}, true
}

func commitRewriteModeLabel(mode string) string {
	switch mode {
	case "polish":
		return "打磨"
	case "rewrite":
		return "重写"
	default:
		return "返工"
	}
}

func (t *CommitChapterTool) loadCommittedChapterContent(chapter int) (string, int, error) {
	content, err := t.store.Drafts.LoadChapterText(chapter)
	if err != nil {
		return "", 0, err
	}
	if content != "" {
		return content, len([]rune(content)), nil
	}
	return t.store.Drafts.LoadChapterContent(chapter)
}

// loadCoreCharacterNameSet 加载 characters.json 中已有的角色名集合（含别名）。
// 用作 cast_ledger 的"已知核心"过滤集——核心角色不进次要名册。
// 加载失败时返回 nil（merge 时所有 characters 都进 ledger，可接受）。
func loadCoreCharacterNameSet(s *store.Store) map[string]bool {
	chars, err := s.Characters.Load()
	if err != nil || len(chars) == 0 {
		return nil
	}
	set := make(map[string]bool, len(chars)*2)
	for _, c := range chars {
		if c.Name != "" {
			set[c.Name] = true
		}
		for _, alias := range c.Aliases {
			if alias != "" {
				set[alias] = true
			}
		}
	}
	return set
}

// applyCompletion 判断本次 commit 是否使全书完结，若是则 MarkComplete 并返回 true。
//   - 非分层：写完约定总章数即完结。
//   - 分层：架构师显式 save_foundation type=complete_book 是主路径；这里再加一道
//     确定性兜底——当全书已客观满足完结条件（见 layeredBookComplete）时自动收尾。
//     防止模型在终点既不 append_volume 也不 complete_book，导致"写手裸跑越界章节 →
//     越界守卫拦截 → 反复重试"的 livelock（《凡骨》ch204..347 案例的根因）。
func (t *CommitChapterTool) applyCompletion(result *domain.CommitResult, progress *domain.Progress) bool {
	if progress == nil {
		return false
	}
	if progress.Layered {
		if t.layeredBookComplete(progress) {
			_ = t.store.Progress.MarkComplete()
			return true
		}
		return false
	}
	if progress.TotalChapters > 0 && result.NextChapter > progress.TotalChapters {
		_ = t.store.Progress.MarkComplete()
		return true
	}
	return false
}

// layeredBookComplete 用客观事实判断分层长篇是否真正写完，对照 architect-long.md 完结判定
// 清单里可量化的几项 + 结构性事实。全部满足才算完结；任一不满足都让位给架构师继续
// expand_arc / append_volume，绝不抢在故事没写完时收尾。无 compass 时保守判为未完结。
func (t *CommitChapterTool) layeredBookComplete(progress *domain.Progress) bool {
	// 1. 返工队列必须清空
	if len(progress.PendingRewrites) > 0 {
		return false
	}
	volumes, err := t.store.Outline.LoadLayeredOutline()
	if err != nil || len(volumes) == 0 {
		return false
	}
	// 2. 不能还有骨架弧待展开（计划内仍有内容要写）
	for i := range volumes {
		for j := range volumes[i].Arcs {
			if !volumes[i].Arcs[j].IsExpanded() {
				return false
			}
		}
	}
	// 3. 已展开章节必须全部写完
	expanded := len(domain.FlattenOutline(volumes))
	if expanded == 0 || len(progress.CompletedChapters) < expanded {
		return false
	}
	// 4. 活跃伏笔必须归零（承诺已兑现）
	if active, aerr := t.store.World.LoadActiveForeshadow(); aerr != nil || len(active) > 0 {
		return false
	}
	// 5. 指南针活跃长线必须收束（无 compass / 长线未清都交回架构师裁定）
	compass, cerr := t.store.Outline.LoadCompass()
	if cerr != nil || compass == nil || len(compass.OpenThreads) > 0 {
		return false
	}
	return true
}
