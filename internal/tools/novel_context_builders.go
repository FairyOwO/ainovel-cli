package tools

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/rules"
	styleanalyzer "github.com/voocel/ainovel-cli/internal/style"
	"github.com/voocel/ainovel-cli/internal/stylestat"
)

var benchmarkChapterPattern = regexp.MustCompile(`(?i)(?:第\s*(\d+)\s*[章节回话]|ch(?:apter)?\s*\.?\s*(\d+))`)

type contextBuildState struct {
	chapter         int
	profile         domain.ContextProfile
	progress        *domain.Progress
	runMeta         *domain.RunMeta
	currentEntry    *domain.OutlineEntry
	chapterPlan     *domain.ChapterPlan
	storyThreads    []domain.RecallItem
	foreshadow      []domain.ForeshadowEntry
	relationships   []domain.RelationshipEntry
	allStateChanges []domain.StateChange
	styleRules      *domain.WritingStyleRules
	styleStats      *domain.StyleStats
	benchmarks      []domain.BenchmarkCompact
}

type chapterContextEnvelope struct {
	Working    map[string]any
	Episodic   map[string]any
	References map[string]any
	Selected   map[string]any
}

type architectContextEnvelope struct {
	Planning   map[string]any
	Foundation map[string]any
	References map[string]any
}

func newChapterContextEnvelope() chapterContextEnvelope {
	return chapterContextEnvelope{
		Working:    make(map[string]any),
		Episodic:   make(map[string]any),
		References: make(map[string]any),
		Selected:   make(map[string]any),
	}
}

func newArchitectContextEnvelope() architectContextEnvelope {
	return architectContextEnvelope{
		Planning:   make(map[string]any),
		Foundation: make(map[string]any),
		References: make(map[string]any),
	}
}

func (e chapterContextEnvelope) apply(result map[string]any) {
	// 合并而非替换：Execute 的章节路径会先后 apply 两个信封（seed + buildChapterContext），
	// 整体赋值会让第二次 apply 丢弃 seed 的容器内容，working_memory.* 等 canonical
	// 路径随之失效（prompt 指针指向空气，模型只能靠顶层镜像模糊容错）。
	mergeEnvelopeSection(result, "working_memory", e.Working)
	mergeEnvelopeSection(result, "episodic_memory", e.Episodic)
	mergeEnvelopeSection(result, "reference_pack", e.References)
	if len(e.Selected) > 0 {
		mergeEnvelopeSection(result, "selected_memory", e.Selected)
	}
	mergeContextSection(result, e.Working)
	mergeContextSection(result, e.Episodic)
	mergeContextSection(result, e.References)
}

// mergeEnvelopeSection 把 section 合并进 result[key] 的既有容器；容器不存在时直接挂载。
func mergeEnvelopeSection(result map[string]any, key string, section map[string]any) {
	if existing, ok := result[key].(map[string]any); ok {
		maps.Copy(existing, section)
		return
	}
	result[key] = section
}

func (e architectContextEnvelope) apply(result map[string]any) {
	result["planning_memory"] = e.Planning
	result["foundation_memory"] = e.Foundation
	result["reference_pack"] = e.References
	mergeContextSection(result, e.Planning)
	mergeContextSection(result, e.Foundation)
	mergeContextSection(result, e.References)
}

func (t *ContextTool) buildBenchmarkSummaries(result map[string]any, sectionKey string, warn func(string, error)) {
	benchmarks, err := t.store.Benchmark.LoadSummaries()
	if err != nil {
		warn("benchmark_summaries", err)
	}
	if len(benchmarks) == 0 {
		return
	}
	section, ok := result[sectionKey].(map[string]any)
	if !ok {
		section = map[string]any{}
		result[sectionKey] = section
	}
	section["benchmark_summaries"] = benchmarks
	result["benchmark_summaries"] = true
}

func mergeContextSection(result map[string]any, section map[string]any) {
	maps.Copy(result, section)
}

// buildProgressStatus 仅在 Coordinator 调用（不传 chapter）时返回进度摘要,
// Writer 不需要这些信息,避免干扰写作。
func (t *ContextTool) buildProgressStatus(result map[string]any) {
	progress, err := t.store.Progress.Load()
	if err != nil || progress == nil {
		return
	}
	status := map[string]any{
		"phase":              string(progress.Phase),
		"flow":               string(progress.Flow),
		"completed_chapters": len(progress.CompletedChapters),
		"total_chapters":     progress.TotalChapters,
		"next_chapter":       progress.NextChapter(),
		"total_word_count":   progress.TotalWordCount,
	}
	if progress.InProgressChapter > 0 {
		status["in_progress_chapter"] = progress.InProgressChapter
	}
	if len(progress.PendingRewrites) > 0 {
		status["pending_rewrites"] = progress.PendingRewrites
		status["rewrite_reason"] = progress.RewriteReason
	}
	if progress.Layered {
		status["layered"] = true
		status["current_volume"] = progress.CurrentVolume
		status["current_arc"] = progress.CurrentArc
	}
	if progress.Phase == domain.PhaseComplete {
		status["finished"] = true
	}
	result["progress_status"] = status
}

// buildUserRules 把合并后的 Bundle 注入 working_memory.user_rules（canonical 路径）。
//
// 单点注入：writer / editor / architect / coordinator 任一路径调用 novel_context
// 都能在 working_memory.user_rules 拿到一致的偏好。architect 路径原本没有 working_memory，
// 由本函数按需新建（仅装 user_rules）；chapter > 0 路径下 working_memory 已存在，直接嵌入。
//
// 即便 Bundle 为空也注入，保持字段稳定，避免 LLM 看到 user_rules=null 而走异常分支。
//
// 注入策略：只给 LLM 看 structured + preferences——这两项才是创作时需要遵循的偏好。
// sources / conflicts 是诊断信息（用户冲突排查），不进 LLM；由 CLI 启动诊断面板按需展示。
func (t *ContextTool) buildUserRules(result map[string]any) {
	bundle := rules.Merge(rules.Load(t.rulesOpts))
	payload := map[string]any{
		"structured":  bundle.Structured,
		"preferences": bundle.Preferences,
	}
	working, ok := result["working_memory"].(map[string]any)
	if !ok {
		working = map[string]any{}
		result["working_memory"] = working
	}
	working["user_rules"] = payload
}

// buildUserDirectives 把用户长效创作要求注入 working_memory.user_directives（canonical 路径）。
//
// 与 buildUserRules 同为单点注入：writer / editor / architect / coordinator 任一路径
// 都拿到一致的列表。空列表也注入 []，保持字段稳定（同 user_rules 先例），
// 也让 prompt 指针一致性测试天然可解析。条目形状见 directiveFacts。
func (t *ContextTool) buildUserDirectives(result map[string]any, warn func(string, error)) {
	list, err := t.store.Directives.Load()
	if err != nil {
		warn("user_directives", err)
		return
	}
	working, ok := result["working_memory"].(map[string]any)
	if !ok {
		working = map[string]any{}
		result["working_memory"] = working
	}
	working["user_directives"] = directiveFacts(list)
}

// buildDiagnosticGuidance injects compact /diag feedback without letting diagnostics control flow.
func (t *ContextTool) buildDiagnosticGuidance(result map[string]any, warn func(string, error)) {
	guidance, err := t.store.World.LoadDiagnosticGuidance()
	if err != nil {
		warn("diag_guidance", err)
		return
	}
	if guidance == nil || len(guidance.Items) == 0 {
		return
	}
	working, ok := result["working_memory"].(map[string]any)
	if !ok {
		working = map[string]any{}
		result["working_memory"] = working
	}
	working["diag_guidance"] = map[string]any{
		"schema_version": guidance.SchemaVersion,
		"generated_at":   guidance.GeneratedAt,
		"items":          guidance.Items,
		"usage":          "advisory only; use as calibration for style review and local spot-fix, not as automatic rewrite routing",
	}
}

func (t *ContextTool) buildSimulationProfile(result map[string]any, sectionKey string, warn func(string, error)) {
	profile, err := t.store.Simulation.Load()
	if err != nil {
		warn("simulation_profile", err)
		return
	}
	compact := domain.CompactSimulationProfile(profile)
	if compact == nil {
		return
	}
	section, ok := result[sectionKey].(map[string]any)
	if !ok {
		section = map[string]any{}
		result[sectionKey] = section
	}
	section["simulation_profile"] = compact
	result["simulation_profile"] = true
}

func (t *ContextTool) buildBaseContext(result map[string]any, warn func(string, error)) {
	if premise, err := t.store.Outline.LoadPremise(); err == nil && premise != "" {
		result["premise"] = premise
		if sections := parsePremiseSections(premise); len(sections) > 0 {
			result["premise_sections"] = sections
		}
		tier := domain.PlanningTier("")
		if meta, err := t.store.RunMeta.Load(); err == nil && meta != nil {
			tier = meta.PlanningTier
		}
		result["premise_structure"] = premiseStructure(premise, tier)
	} else {
		warn("premise", err)
	}
	if outline, err := t.store.Outline.LoadOutline(); err == nil && outline != nil {
		result["outline"] = outline
	} else {
		warn("outline", err)
	}
	if rules, err := t.store.World.LoadWorldRules(); err == nil && len(rules) > 0 {
		result["world_rules"] = rules
	} else {
		warn("world_rules", err)
	}
}

func (t *ContextTool) prepareChapterContext(chapter int, envelope *chapterContextEnvelope, warn func(string, error)) contextBuildState {
	state := contextBuildState{
		chapter: chapter,
		profile: domain.NewContextProfile(0),
	}

	progress, err := t.store.Progress.Load()
	warn("progress", err)
	runMeta, err := t.store.RunMeta.Load()
	warn("run_meta", err)
	state.progress = progress
	state.runMeta = runMeta

	if runMeta != nil && runMeta.PlanningTier != "" {
		envelope.Episodic["planning_tier"] = runMeta.PlanningTier
	}
	if progress != nil && progress.TotalChapters > 0 {
		state.profile = domain.NewContextProfile(progress.TotalChapters)
	}
	if progress == nil || !progress.Layered {
		state.profile.Layered = false
	}

	currentEntry, currentEntryErr := t.store.Outline.GetChapterOutline(chapter)
	if currentEntryErr == nil {
		envelope.Working["current_chapter_outline"] = currentEntry
	} else {
		warn("current_chapter_outline", currentEntryErr)
	}
	state.currentEntry = currentEntry

	chapterPlan, chapterPlanErr := t.store.Drafts.LoadChapterPlan(chapter)
	if chapterPlanErr == nil && chapterPlan != nil {
		envelope.Working["chapter_plan"] = chapterPlan
		if len(chapterPlan.Contract.RequiredBeats) > 0 ||
			len(chapterPlan.Contract.ForbiddenMoves) > 0 ||
			len(chapterPlan.Contract.ContinuityChecks) > 0 ||
			len(chapterPlan.Contract.EvaluationFocus) > 0 ||
			chapterPlan.Contract.EmotionTarget != "" ||
			len(chapterPlan.Contract.PayoffPoints) > 0 ||
			chapterPlan.Contract.HookGoal != "" {
			envelope.Working["chapter_contract"] = chapterPlan.Contract
		}
	} else {
		warn("chapter_plan", chapterPlanErr)
	}
	state.chapterPlan = chapterPlan

	// 是否正在重写本章：决定 novel_context 是否补"重写专用"事实。
	isRewrite := progress != nil && slices.Contains(progress.PendingRewrites, chapter)
	styleStats, styleStatsErr := t.store.World.LoadStyleStats(chapter)
	warn("style_stats", styleStatsErr)
	state.styleStats = styleStats

	// 暴露 draft 是否已存在的事实：让 writer 被重派时能自行判断跳过重写还是覆盖。
	// 只暴露 exists + word_count，不注入正文（正文让 writer 按需用 read_chapter 拉）。
	if _, draftWords, draftErr := t.store.Drafts.LoadChapterContent(chapter); draftErr == nil && draftWords > 0 {
		envelope.Working["chapter_draft"] = map[string]any{
			"exists":     true,
			"word_count": draftWords,
		}
	} else if draftErr != nil {
		warn("chapter_draft", draftErr)
	}

	// 重写时把"为什么改 + 改哪里"交给 writer：理由来自返工队列，具体批评来自本章评审
	// （selectReviewLessons 只召回 chapter-1..chapter-3，恰好漏掉本章本身，writer 又无读评审的工具）。
	// 正文不在此注入——保持"正文按需 read_chapter 拉"的约定不破。
	if isRewrite {
		brief := map[string]any{"reason": progress.RewriteReason}
		if compact := compactStyleStats(styleStats); compact != nil {
			brief["style_stats"] = compact
		}
		if review, reviewErr := t.store.World.LoadReview(chapter); reviewErr == nil && review != nil {
			if review.Summary != "" {
				brief["review_summary"] = review.Summary
			}
			if len(review.Issues) > 0 {
				brief["issues"] = review.Issues
			}
			if len(review.ContractMisses) > 0 {
				brief["contract_misses"] = review.ContractMisses
			}
		} else if reviewErr != nil {
			warn("rewrite_review", reviewErr)
		}
		envelope.Working["rewrite_brief"] = brief
	}

	foreshadow, foreshadowErr := t.store.World.LoadActiveForeshadow()
	warn("foreshadow_ledger", foreshadowErr)
	state.foreshadow = foreshadow

	relationships, relErr := t.store.World.LoadRelationships()
	warn("relationship_state", relErr)
	if len(relationships) > 0 {
		envelope.Episodic["relationship_state"] = relationships
	}
	state.relationships = relationships

	allStateChanges, scErr := t.store.World.LoadStateChanges()
	warn("recent_state_changes", scErr)
	state.allStateChanges = allStateChanges
	if recent := recentStateChanges(chapter, allStateChanges); len(recent) > 0 {
		envelope.Episodic["recent_state_changes"] = recent
	}

	styleRules, styleErr := t.store.World.LoadStyleRules()
	warn("style_rules", styleErr)
	state.styleRules = styleRules
	benchmarks, benchmarkErr := t.store.Benchmark.LoadSummaries()
	warn("benchmark_summaries", benchmarkErr)
	state.benchmarks = benchmarks
	state.storyThreads = t.selectStoryThreads(state)
	if len(state.storyThreads) > 0 && len(state.storyThreads) < storyThreadRecallMinSelected {
		state.storyThreads = nil
	}

	return state
}

func (t *ContextTool) buildChapterContext(result map[string]any, state contextBuildState, warn func(string, error)) {
	envelope := newChapterContextEnvelope()
	result["memory_policy"] = domain.NewChapterMemoryPolicy(state.progress, state.profile, state.currentEntry != nil)

	if state.profile.Layered {
		t.loadLayeredCharacters(envelope.Episodic, state.chapter, warn)
	} else {
		t.loadFilteredCharacters(envelope.Episodic, state.chapter, warn)
	}

	t.buildChapterEpisodicMemory(&envelope, state, warn)
	t.buildChapterWorkingMemory(&envelope, state, warn)
	t.buildChapterReferencePack(&envelope, state)
	t.buildChapterSelectedMemory(&envelope, state, warn)
	t.buildStyleStats(&envelope, state)
	envelope.apply(result)
}

// buildStyleStats 对全部已完成章节做全书级风格统计，注入 episodic_memory.style_stats。
// 弧内评审窗口对"章均几十次的句式 tic、章末形态同构、跨章复读"天然失明，只有
// 全书统计能暴露——统计归代码（确定性），裁定归 LLM（editor 在 aesthetic 维度
// 按数字判分，writer 据此自避免）。章数不足时 stylestat 返回 nil，不注入。
func (t *ContextTool) buildStyleStats(envelope *chapterContextEnvelope, state contextBuildState) {
	if state.progress == nil || len(state.progress.CompletedChapters) == 0 {
		return
	}
	completed := slices.Clone(state.progress.CompletedChapters)
	slices.Sort(completed)
	chapters := make([]string, 0, len(completed))
	for _, ch := range completed {
		// 个别章读取失败跳过：统计是 best-effort 事实，不因单章缺失放弃全书视野
		if text, err := t.store.Drafts.LoadChapterText(ch); err == nil && text != "" {
			chapters = append(chapters, text)
		}
	}

	var titles []string
	if outline, err := t.store.Outline.LoadOutline(); err == nil {
		for _, entry := range outline {
			titles = append(titles, entry.Title)
		}
	}

	stats := stylestat.Compute(stylestat.Input{
		Chapters:  chapters,
		Titles:    titles,
		Stopwords: t.styleStopwords(),
	})
	if stats == nil {
		return
	}
	envelope.Episodic["style_stats"] = stats
	if guidance := styleGuidance(nil, compactBookStyleStats(stats)); len(guidance) > 0 {
		envelope.Working["style_guidance"] = appendStringGuidance(envelope.Working["style_guidance"], guidance)
	}
}

// styleStopwords 收集角色名与别名供短语挖掘过滤——出场人名天然高频，不是文风问题。
func (t *ContextTool) styleStopwords() []string {
	var words []string
	if chars, err := t.store.Characters.Load(); err == nil {
		for _, c := range chars {
			words = append(words, c.Name)
			words = append(words, c.Aliases...)
		}
	}
	if cast, err := t.store.Cast.RecentActive(50); err == nil {
		for _, e := range cast {
			words = append(words, e.Name)
			words = append(words, e.Aliases...)
		}
	}
	return words
}

func (t *ContextTool) buildChapterWorkingMemory(envelope *chapterContextEnvelope, state contextBuildState, warn func(string, error)) {
	if next, err := t.store.Outline.GetChapterOutline(state.chapter + 1); err == nil && next != nil {
		envelope.Working["next_chapter_outline"] = next
	}

	if state.profile.Layered {
		t.loadLayeredSummaries(envelope.Working, state.chapter, state.profile.SummaryWindow, warn)
	} else {
		if summaries, err := t.store.Summaries.LoadRecentSummaries(state.chapter, state.profile.SummaryWindow); err == nil && len(summaries) > 0 {
			envelope.Working["recent_summaries"] = summaries
		} else {
			warn("recent_summaries", err)
		}
	}

	if timeline, err := t.store.World.LoadRecentTimeline(state.chapter, state.profile.TimelineWindow); err == nil && len(timeline) > 0 {
		envelope.Working["timeline"] = timeline
	} else {
		warn("timeline", err)
	}

	if state.progress != nil {
		checkpoint := map[string]any{
			"in_progress_chapter": state.progress.InProgressChapter,
		}
		if len(state.progress.StrandHistory) > 0 {
			checkpoint["strand_history"] = state.progress.StrandHistory
		}
		if len(state.progress.HookHistory) > 0 {
			checkpoint["hook_history"] = state.progress.HookHistory
		}
		envelope.Working["checkpoint"] = checkpoint
	}

	if compact := compactStyleStats(state.styleStats); compact != nil {
		envelope.Working["style_stats"] = compact
		if guidance := styleGuidance(compact, nil); len(guidance) > 0 {
			envelope.Working["style_guidance"] = guidance
		}
	}
	if state.progress != nil && slices.Contains(state.progress.PendingRewrites, state.chapter) {
		if draftStats := t.compactDraftStyleStats(state.chapter, state.styleStats, warn); draftStats != nil {
			envelope.Working["style_stats_draft"] = draftStats
		}
	}

	if state.chapter > 1 {
		if prevText, err := t.store.Drafts.LoadChapterText(state.chapter - 1); err == nil && prevText != "" {
			runes := []rune(prevText)
			if len(runes) > 800 {
				runes = runes[len(runes)-800:]
			}
			envelope.Working["previous_tail"] = string(runes)
		}
	}
}

func (t *ContextTool) compactDraftStyleStats(chapter int, finalStats *domain.StyleStats, warn func(string, error)) map[string]any {
	text, _, err := t.store.Drafts.LoadChapterContent(chapter)
	if err != nil {
		warn("draft_style_stats", err)
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	draft := styleanalyzer.AnalyzeChineseProse(text)
	draft.Chapter = chapter
	compact := compactStyleStats(draft)
	if compact == nil {
		return nil
	}
	if finalStats != nil {
		compact["comparison"] = compareStyleStats(finalStats, draft)
	}
	return compact
}

func compactStyleStats(stats *domain.StyleStats) map[string]any {
	if stats == nil {
		return nil
	}
	out := map[string]any{
		"schema_version": stats.SchemaVersion,
		"chapter":        stats.Chapter,
		"summary":        stats.Summary,
	}
	metrics := compactStyleMetrics(stats)
	if len(metrics) > 0 {
		out["metrics"] = metrics
	}
	if stats.External != nil {
		out["external_detector"] = stats.External
	}
	hotspots := compactStyleHotspots(stats.Hotspots, 5)
	if len(hotspots) > 0 {
		out["hotspots"] = hotspots
	}
	return out
}

func compactStyleMetrics(stats *domain.StyleStats) map[string]float64 {
	if stats == nil || len(stats.Metrics) == 0 {
		return nil
	}
	keys := []string{
		"sentence_length_stddev",
		"paragraph_uniform_ratio",
		"dialogue_ratio",
		"sentence_start_unique_rate",
		"sentence_start_dominant_category_ratio",
		"sentence_start_abstract_connector_ratio",
		"emotion_label_density_per_1000",
		"pattern_density_per_1000",
	}
	out := make(map[string]float64, len(keys))
	for _, key := range keys {
		if metric, ok := stats.Metrics[key]; ok {
			out[key] = metric.Value
		}
	}
	return out
}

func compactBookStyleStats(stats *stylestat.Stats) map[string]any {
	if stats == nil {
		return nil
	}
	out := map[string]any{
		"chapters": stats.Chapters,
	}
	if len(stats.Patterns) > 0 {
		out["patterns"] = stats.Patterns
	}
	if len(stats.TopPhrases) > 0 {
		out["top_phrases"] = stats.TopPhrases
	}
	if len(stats.RepeatedSentences) > 0 {
		out["repeated_sentences"] = stats.RepeatedSentences
	}
	out["ending"] = stats.Ending
	out["opening_time_rate"] = stats.OpeningTimeRate
	if stats.TitleFormats != nil {
		out["title_formats"] = stats.TitleFormats
	}
	if fp := compactFingerprint(stats.Fingerprint); fp != nil {
		out["fingerprint"] = fp
	}
	return out
}

func compactFingerprint(fp *stylestat.Fingerprint) map[string]any {
	if fp == nil {
		return nil
	}
	out := map[string]any{
		"recent_chapters": fp.RecentChapters,
	}
	if fp.BaselineChapters > 0 {
		out["baseline_chapters"] = fp.BaselineChapters
	}
	if fp.FunctionWordDrift > 0 {
		out["function_word_drift"] = fp.FunctionWordDrift
	}
	if fp.StartCategoryDrift > 0 {
		out["start_category_drift"] = fp.StartCategoryDrift
	}
	if fp.DominantStartCategory != "" {
		out["dominant_start_category"] = fp.DominantStartCategory
		out["dominant_start_category_ratio"] = fp.DominantStartCategoryRatio
	}
	if fp.RecentEmotionLabelDensity > 0 {
		out["recent_emotion_label_density_per_1000"] = fp.RecentEmotionLabelDensity
	}
	return out
}

func styleGuidance(chapterStats map[string]any, bookStats map[string]any) []string {
	var guidance []string
	if metrics, ok := chapterStats["metrics"].(map[string]float64); ok {
		if metrics["emotion_label_density_per_1000"] >= 6 {
			guidance = append(guidance, "情绪标签偏高：删掉紧张/愤怒/悲伤等标签，改用身体反应、动作和选择呈现。")
		}
		if metrics["sentence_start_dominant_category_ratio"] >= 0.55 {
			guidance = append(guidance, "句首类别过于集中：下一轮刻意轮换动作、对白、感官、环境和无主语句开头。")
		}
		if metrics["sentence_start_abstract_connector_ratio"] >= 0.3 {
			guidance = append(guidance, "抽象/连接词起句偏多：少用然而/与此同时/这时，直接切入动作或对白。")
		}
		if metrics["paragraph_uniform_ratio"] > 0.8 || metrics["sentence_length_stddev"] <= 3 {
			guidance = append(guidance, "节奏过整：提高长短句和段落落差，temperature_hint=raise_variety。")
		}
	}
	if fingerprint, ok := bookStats["fingerprint"].(map[string]any); ok {
		if drift, ok := fingerprint["function_word_drift"].(float64); ok && drift >= 0.15 {
			guidance = append(guidance, "近期功能词分布漂移：保持本书既有叙述口吻，避免突然换成另一种模型腔。")
		}
		if ratio, ok := fingerprint["dominant_start_category_ratio"].(float64); ok && ratio >= 0.5 {
			guidance = append(guidance, "近期章句首类型固化：新章开头和段首避免继续使用主导起手方式。")
		}
	}
	return guidance
}

func appendStringGuidance(existing any, extra []string) []string {
	out := make([]string, 0, len(extra)+4)
	switch v := existing.(type) {
	case []string:
		out = append(out, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	for _, item := range extra {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func compactStyleHotspots(hotspots []domain.StyleHotspot, limit int) []map[string]any {
	if len(hotspots) == 0 || limit <= 0 {
		return nil
	}
	ordered := slices.Clone(hotspots)
	slices.SortStableFunc(ordered, func(a, b domain.StyleHotspot) int {
		return styleSeverityRank(a.Severity) - styleSeverityRank(b.Severity)
	})
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	out := make([]map[string]any, 0, len(ordered))
	for _, hotspot := range ordered {
		item := map[string]any{
			"rule_id":         hotspot.RuleID,
			"severity":        hotspot.Severity,
			"evidence":        hotspot.Evidence,
			"message":         hotspot.Message,
			"suggestion_type": hotspot.SuggestionType,
		}
		if hotspot.ID != "" {
			item["id"] = hotspot.ID
		}
		if hotspot.ParagraphIndex > 0 {
			item["paragraph_index"] = hotspot.ParagraphIndex
		}
		if hotspot.SentenceIndex > 0 {
			item["sentence_index"] = hotspot.SentenceIndex
		}
		if hotspot.Span != nil {
			item["span"] = hotspot.Span
		}
		out = append(out, item)
	}
	return out
}

func styleSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "error":
		return 1
	case "warning":
		return 2
	default:
		return 3
	}
}

func compareStyleStats(finalStats, draftStats *domain.StyleStats) map[string]float64 {
	// Keep this prompt-facing comparison smaller than the durable rewrite comparison.
	keys := []string{"sentence_length_stddev", "dialogue_ratio", "sentence_start_unique_rate", "pattern_density_per_1000"}
	out := make(map[string]float64, len(keys))
	for _, key := range keys {
		finalMetric, finalOK := finalStats.Metrics[key]
		draftMetric, draftOK := draftStats.Metrics[key]
		if finalOK && draftOK {
			out[key+"_delta"] = draftMetric.Value - finalMetric.Value
		}
	}
	return out
}

func (t *ContextTool) buildChapterSelectedMemory(envelope *chapterContextEnvelope, state contextBuildState, warn func(string, error)) {
	envelope.Selected["minimal_context"] = t.buildMinimalContext(state, warn)
	if len(state.storyThreads) > 0 {
		envelope.Selected["story_threads"] = state.storyThreads
	}
	if lessons := t.selectReviewLessons(state.chapter, warn); len(lessons) > 0 {
		envelope.Selected["review_lessons"] = lessons
	}
}

func (t *ContextTool) buildMinimalContext(state contextBuildState, warn func(string, error)) map[string]any {
	characterStates := uniqueStrings(t.minimalCharacterStates(state, warn))
	causalHistory := uniqueStrings(t.minimalCausalHistory(state))
	worldConstraints := uniqueStrings(t.minimalWorldConstraints(state, warn))

	return map[string]any{
		"character_states":  characterStates,
		"causal_history":    causalHistory,
		"world_constraints": worldConstraints,
		"chapter_intent":    minimalChapterIntent(state),
	}
}

func (t *ContextTool) minimalCharacterStates(state contextBuildState, warn func(string, error)) []string {
	var items []string
	if state.profile.Layered {
		if snapshots, err := t.store.Characters.LoadLatestSnapshots(); err == nil {
			for _, snapshot := range snapshots {
				items = append(items, joinNonEmpty(" / ", snapshot.Name, snapshot.Status, snapshot.Power, snapshot.Motivation, snapshot.Relations))
			}
		} else {
			warn("character_snapshots", err)
		}
	} else if chars, err := t.store.Characters.Load(); err == nil {
		for _, char := range chars {
			items = append(items, joinNonEmpty(" / ", char.Name, char.Role, char.Description, char.Arc, strings.Join(char.Traits, "、")))
		}
	} else {
		warn("characters", err)
	}

	for _, change := range recentStateChanges(state.chapter, state.allStateChanges) {
		items = append(items, joinNonEmpty(" ", change.Entity, change.Field, change.OldValue+"->"+change.NewValue))
	}
	for _, rel := range state.relationships {
		items = append(items, joinNonEmpty(" ", rel.CharacterA, rel.Relation, rel.CharacterB))
	}
	return items
}

func (t *ContextTool) minimalCausalHistory(state contextBuildState) []string {
	var items []string
	for _, thread := range state.storyThreads {
		items = append(items, thread.Summary)
	}
	for _, entry := range state.foreshadow {
		items = append(items, joinNonEmpty(" ", entry.ID, entry.Description, entry.Status))
	}
	if state.chapterPlan != nil {
		for _, payoff := range state.chapterPlan.Contract.PayoffPoints {
			items = append(items, "Payoff: "+payoff)
		}
		if state.chapterPlan.Contract.HookGoal != "" {
			items = append(items, "Hook: "+state.chapterPlan.Contract.HookGoal)
		}
	}
	return items
}

func (t *ContextTool) minimalWorldConstraints(state contextBuildState, warn func(string, error)) []string {
	var items []string
	if rules, err := t.store.World.LoadWorldRules(); err == nil {
		for _, rule := range rules {
			items = append(items, joinNonEmpty(" / ", rule.Category, rule.Rule, rule.Boundary))
		}
	} else {
		warn("world_rules", err)
	}
	if state.chapterPlan != nil {
		items = append(items, state.chapterPlan.Contract.ForbiddenMoves...)
		items = append(items, state.chapterPlan.Contract.ContinuityChecks...)
	}
	return items
}

func minimalChapterIntent(state contextBuildState) string {
	if state.chapterPlan != nil && strings.TrimSpace(state.chapterPlan.Goal) != "" {
		return state.chapterPlan.Goal
	}
	if state.currentEntry != nil {
		if strings.TrimSpace(state.currentEntry.CoreEvent) != "" {
			return state.currentEntry.CoreEvent
		}
		return state.currentEntry.Hook
	}
	return ""
}

func recentStateChanges(chapter int, changes []domain.StateChange) []domain.StateChange {
	start := max(chapter-2, 1)
	var recent []domain.StateChange
	for _, change := range changes {
		if change.Chapter >= start && change.Chapter < chapter {
			recent = append(recent, change)
		}
	}
	return recent
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func joinNonEmpty(sep string, parts ...string) string {
	compact := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "->" {
			compact = append(compact, part)
		}
	}
	return strings.Join(compact, sep)
}

func (t *ContextTool) buildChapterEpisodicMemory(envelope *chapterContextEnvelope, state contextBuildState, warn func(string, error)) {
	if len(state.foreshadow) > 0 && len(state.storyThreads) == 0 {
		envelope.Episodic["foreshadow_ledger"] = state.foreshadow
	}

	// 配角名册：召回最近活跃的次要角色，让 Writer 在引入旧角色时能保持口吻/定位一致
	// 不召回所有条目（长篇会膨胀），只给最近活跃的前 N 个，按 LastSeenChapter 倒序
	if recentCast, err := t.store.Cast.RecentActive(15); err == nil && len(recentCast) > 0 {
		simplified := make([]map[string]any, 0, len(recentCast))
		for _, e := range recentCast {
			item := map[string]any{
				"name":             e.Name,
				"first_seen":       e.FirstSeenChapter,
				"last_seen":        e.LastSeenChapter,
				"appearance_count": e.AppearanceCount,
			}
			if e.BriefRole != "" {
				item["brief_role"] = e.BriefRole
			}
			if len(e.Aliases) > 0 {
				item["aliases"] = e.Aliases
			}
			simplified = append(simplified, item)
		}
		envelope.Episodic["recent_cast"] = simplified
	} else if err != nil {
		warn("recent_cast", err)
	}

	if state.progress != nil && state.progress.TotalChapters > 30 && state.currentEntry != nil {
		if related := t.buildRelatedChapters(
			state.chapter,
			state.currentEntry,
			state.foreshadow,
			state.relationships,
			state.allStateChanges,
		); len(related) > 0 {
			envelope.Episodic["related_chapters"] = related
		}
	}

	if state.profile.Layered && state.progress != nil {
		pos := map[string]any{
			"volume": state.progress.CurrentVolume,
			"arc":    state.progress.CurrentArc,
		}
		if volumes, err := t.store.Outline.LoadLayeredOutline(); err == nil {
			globalCh := 1
			for _, v := range volumes {
				if v.Index == state.progress.CurrentVolume {
					pos["volume_title"] = v.Title
					pos["volume_theme"] = v.Theme
				}
				for _, arc := range v.Arcs {
					if v.Index == state.progress.CurrentVolume && arc.Index == state.progress.CurrentArc {
						pos["arc_title"] = arc.Title
						pos["arc_goal"] = arc.Goal
						if n := len(arc.Chapters); n > 0 {
							pos["arc_total_chapters"] = n
							pos["arc_chapter_index"] = state.chapter - globalCh + 1
						}
					}
					globalCh += len(arc.Chapters)
				}
			}
		} else {
			warn("layered_outline", err)
		}
		envelope.Episodic["position"] = pos
	}
}

func (t *ContextTool) buildChapterReferencePack(envelope *chapterContextEnvelope, state contextBuildState) {
	if state.styleRules != nil {
		envelope.References["style_rules"] = state.styleRules
		if state.styleRules.StyleCard != nil {
			envelope.References["style_card"] = state.styleRules.StyleCard
		}
	} else {
		var maxCompleted int
		if state.progress != nil {
			maxCompleted = maxCompletedChapter(state.progress.CompletedChapters)
		}
		if anchors := t.store.Drafts.ExtractStyleAnchors(3, maxCompleted); len(anchors) > 0 {
			envelope.References["style_anchors"] = anchors
		}

		if state.currentEntry != nil {
			var voiceSamples []map[string]any
			chars, _ := t.store.Characters.Load()
			for _, c := range chars {
				if c.Tier == "secondary" || c.Tier == "decorative" {
					continue
				}
				samples := t.store.Drafts.ExtractDialogue(c.Name, c.Aliases, 3, maxCompleted)
				if len(samples) > 0 {
					voiceSamples = append(voiceSamples, map[string]any{
						"character": c.Name,
						"samples":   samples,
					})
				}
				if len(voiceSamples) >= 5 {
					break
				}
			}
			if len(voiceSamples) > 0 {
				envelope.References["voice_samples"] = voiceSamples
			}
		}
	}

	if style := benchmarkStyleForChapter(state); len(style) > 0 {
		envelope.References["benchmark_style"] = style
	}

	envelope.References["references"] = t.writerReferences(state.chapter)
}

func benchmarkStyleForChapter(state contextBuildState) map[string]any {
	if len(state.benchmarks) == 0 {
		return nil
	}
	focusTerms := recallFocusTerms(state.currentEntry, state.chapterPlan)
	best := selectBenchmarkStyle(state.benchmarks, focusTerms)
	if best.benchmark == nil {
		return nil
	}

	profile := benchmarkStyleProfile(*best.benchmark)
	techniques := benchmarkStyleTechniques(*best.benchmark)
	if profile == "" && len(techniques) == 0 {
		return nil
	}

	gaps := benchmarkStyleGaps(*best.benchmark, best.score)
	style := map[string]any{
		"name":       best.benchmark.Name,
		"profile":    profile,
		"techniques": techniques,
		"gaps":       gaps,
	}
	if best.benchmark.Title != "" {
		style["title"] = best.benchmark.Title
	}
	if best.matchedChapter > 0 {
		style["matched_chapter"] = best.matchedChapter
	}
	if len(best.benchmark.DoNotCopy) > 0 {
		style["do_not_copy"] = best.benchmark.DoNotCopy
	}
	return style
}

type benchmarkStyleMatch struct {
	benchmark      *domain.BenchmarkCompact
	score          int
	matchedChapter int
}

func selectBenchmarkStyle(benchmarks []domain.BenchmarkCompact, focusTerms []string) benchmarkStyleMatch {
	best := benchmarkStyleMatch{}
	for i := range benchmarks {
		benchmark := &benchmarks[i]
		score, matched := benchmarkStyleScore(*benchmark, focusTerms)
		if best.benchmark == nil || score > best.score {
			best = benchmarkStyleMatch{
				benchmark:      benchmark,
				score:          score,
				matchedChapter: benchmarkMatchedChapter(matched),
			}
		}
	}
	return best
}

func benchmarkStyleScore(benchmark domain.BenchmarkCompact, focusTerms []string) (int, string) {
	score := 0
	matched := ""
	add := func(items []string, weight int) {
		for _, item := range items {
			if !benchmarkMatchesFocus(item, focusTerms) {
				continue
			}
			score += weight
			if matched == "" {
				matched = item
			}
		}
	}
	if benchmarkMatchesFocus(benchmark.Summary, focusTerms) {
		score += 2
		matched = benchmark.Summary
	}
	add(benchmark.Hooks, 4)
	add(benchmark.Pacing, 3)
	add(benchmark.ReusableTechniques, 3)
	add(benchmark.Structure, 2)
	add(benchmark.CharacterPatterns, 1)
	add(benchmark.SettingPatterns, 1)
	return score, matched
}

func benchmarkMatchesFocus(text string, focusTerms []string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	return matchesRecallTerms(text, focusTerms)
}

func benchmarkStyleProfile(benchmark domain.BenchmarkCompact) string {
	parts := make([]string, 0, 4)
	if benchmark.Summary != "" {
		parts = append(parts, truncateRunes(benchmark.Summary, 80))
	}
	appendPart := func(label string, items []string) {
		if len(items) > 0 {
			parts = append(parts, label+": "+truncateRunes(items[0], 60))
		}
	}
	appendPart("结构", benchmark.Structure)
	appendPart("节奏", benchmark.Pacing)
	appendPart("钩子", benchmark.Hooks)
	return strings.Join(parts, "；")
}

func benchmarkStyleTechniques(benchmark domain.BenchmarkCompact) []string {
	var techniques []string
	for _, items := range [][]string{
		benchmark.ReusableTechniques,
		benchmark.Pacing,
		benchmark.Hooks,
		benchmark.Structure,
	} {
		techniques = append(techniques, items...)
	}
	return limitStrings(uniqueStrings(techniques), 6)
}

func benchmarkStyleGaps(benchmark domain.BenchmarkCompact, score int) []string {
	var gaps []string
	if len(benchmark.ReusableTechniques) == 0 {
		gaps = append(gaps, "未导入文风/技法条目，优先参考结构、节奏和钩子。")
	}
	if score == 0 {
		gaps = append(gaps, "未命中本章情绪/钩子关键词，使用整体对标风格。")
	}
	return gaps
}

func benchmarkMatchedChapter(text string) int {
	match := benchmarkChapterPattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return 0
	}
	for _, group := range match[1:] {
		if group == "" {
			continue
		}
		chapter, err := strconv.Atoi(group)
		if err == nil {
			return chapter
		}
	}
	return 0
}

func limitStrings(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func (t *ContextTool) buildArchitectContext(result map[string]any, warn func(string, error)) {
	envelope := newArchitectContextEnvelope()
	result["memory_policy"] = domain.NewArchitectMemoryPolicy()
	t.buildArchitectPlanning(&envelope, warn)
	t.buildArchitectFoundation(&envelope, warn)
	t.buildArchitectReferences(&envelope, warn)
	envelope.apply(result)
}

func (t *ContextTool) buildArchitectPlanning(envelope *architectContextEnvelope, warn func(string, error)) {
	runMeta, err := t.store.RunMeta.Load()
	warn("run_meta", err)
	if runMeta != nil && runMeta.PlanningTier != "" {
		envelope.Planning["planning_tier"] = runMeta.PlanningTier
	}

	var layered []domain.VolumeOutline
	if l, err := t.store.Outline.LoadLayeredOutline(); err == nil && len(l) > 0 {
		layered = l
		envelope.Planning["layered_outline"] = layered
		var skeletonArcs []map[string]any
		for _, v := range layered {
			for _, a := range v.Arcs {
				if !a.IsExpanded() {
					skeletonArcs = append(skeletonArcs, map[string]any{
						"volume":             v.Index,
						"arc":                a.Index,
						"title":              a.Title,
						"goal":               a.Goal,
						"estimated_chapters": a.EstimatedChapters,
					})
				}
			}
		}
		if len(skeletonArcs) > 0 {
			envelope.Planning["skeleton_arcs"] = skeletonArcs
		}
	} else {
		warn("layered_outline", err)
	}

	var compass *domain.StoryCompass
	if c, err := t.store.Outline.LoadCompass(); err == nil && c != nil {
		compass = c
		envelope.Planning["compass"] = compass
	} else {
		warn("compass", err)
	}
	if volSummaries, err := t.store.Summaries.LoadAllVolumeSummaries(); err == nil && len(volSummaries) > 0 {
		envelope.Planning["volume_summaries"] = volSummaries
	} else {
		warn("volume_summaries", err)
	}

	// completion_signals 把"全书是否该结尾"的关键事实集中呈现，
	// 让架构师在裁定 complete_book / append_volume 时一眼看到对照面。
	// 散落在 progress / compass / foreshadow / layered_outline 里靠 LLM 脑算容易漏。
	envelope.Planning["completion_signals"] = t.completionSignals(layered, compass)
}

func (t *ContextTool) completionSignals(layered []domain.VolumeOutline, compass *domain.StoryCompass) map[string]any {
	signals := map[string]any{}
	if progress, _ := t.store.Progress.Load(); progress != nil {
		signals["completed_chapters"] = len(progress.CompletedChapters)
		signals["total_word_count"] = progress.TotalWordCount
		signals["phase"] = string(progress.Phase)
	}
	if len(layered) > 0 {
		signals["planned_chapters"] = len(domain.FlattenOutline(layered))
		signals["volumes_total"] = len(layered)
	}
	if compass != nil {
		if compass.EstimatedScale != "" {
			signals["compass_estimated_scale"] = compass.EstimatedScale
		}
		signals["open_threads_count"] = len(compass.OpenThreads)
	}
	if active, err := t.store.World.LoadActiveForeshadow(); err == nil {
		signals["active_foreshadow_count"] = len(active)
	}
	return signals
}

func (t *ContextTool) buildArchitectFoundation(envelope *architectContextEnvelope, warn func(string, error)) {
	if premise, err := t.store.Outline.LoadPremise(); err == nil && premise != "" {
		if sections := parsePremiseSections(premise); len(sections) > 0 {
			envelope.Foundation["premise_sections"] = sections
		}
		tier := domain.PlanningTier("")
		if meta, err := t.store.RunMeta.Load(); err == nil && meta != nil {
			tier = meta.PlanningTier
		}
		envelope.Foundation["premise_structure"] = premiseStructure(premise, tier)
	} else {
		warn("premise", err)
	}

	if chars, err := t.store.Characters.Load(); err == nil && chars != nil {
		envelope.Foundation["characters"] = chars
	} else {
		warn("characters", err)
	}

	if snapshots, err := t.store.Characters.LoadLatestSnapshots(); err == nil && len(snapshots) > 0 {
		envelope.Foundation["character_snapshots"] = snapshots
	} else {
		warn("character_snapshots", err)
	}
	if rules, err := t.store.World.LoadWorldRules(); err == nil && len(rules) > 0 {
		envelope.Foundation["world_rules"] = rules
	} else {
		warn("world_rules", err)
	}
	if foreshadow, err := t.store.World.LoadActiveForeshadow(); err == nil && len(foreshadow) > 0 {
		envelope.Foundation["foreshadow_ledger"] = foreshadow
	} else {
		warn("foreshadow_ledger", err)
	}
	envelope.Foundation["foundation_status"] = t.foundationStatus()
}

func (t *ContextTool) buildArchitectReferences(envelope *architectContextEnvelope, warn func(string, error)) {
	if styleRules, err := t.store.World.LoadStyleRules(); err == nil && styleRules != nil {
		envelope.References["style_rules"] = styleRules
		if styleRules.StyleCard != nil {
			envelope.References["style_card"] = styleRules.StyleCard
		}
	} else {
		warn("style_rules", err)
	}

	envelope.References["references"] = t.architectReferences()
}
