package diag

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// ChronicLowDimension 检测某评审维度跨多章持续低分。
func ChronicLowDimension(snap *Snapshot) []Finding {
	if len(snap.Reviews) < 2 {
		return nil
	}

	dimSums := make(map[string]float64)
	dimCounts := make(map[string]int)
	for _, r := range snap.Reviews {
		for _, d := range r.Dimensions {
			dimSums[d.Dimension] += float64(d.Score)
			dimCounts[d.Dimension]++
		}
	}

	var findings []Finding
	for name, sum := range dimSums {
		count := dimCounts[name]
		if count < 2 {
			continue
		}
		avg := sum / float64(count)
		if avg >= ThresholdDimScoreLow {
			continue
		}
		findings = append(findings, Finding{
			Rule:       "ChronicLowDimension",
			Category:   CatQuality,
			Severity:   SevWarning,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("维度 [%s] 持续低分 (均值 %.0f)", name, avg),
			Evidence:   fmt.Sprintf("共 %d 次评审，均分 %.1f", count, avg),
			Suggestion: fmt.Sprintf("检查 Writer prompt 中关于 %s 的指引是否清晰，或 Editor prompt 的 %s 评分标准是否合理。", name, name),
		})
	}
	return findings
}

// ContractMissPattern 检测合同履约率过低。
func ContractMissPattern(snap *Snapshot) []Finding {
	if len(snap.Reviews) == 0 {
		return nil
	}

	var total, missed int
	var missedChapters []string
	for ch, r := range snap.Reviews {
		total++
		if r.ContractStatus == "partial" || r.ContractStatus == "missed" {
			missed++
			missedChapters = append(missedChapters, fmt.Sprintf("ch%d", ch))
		}
	}
	if total == 0 {
		return nil
	}
	rate := float64(missed) / float64(total)
	if rate <= ThresholdContractMissRate {
		return nil
	}
	return []Finding{{
		Rule:       "ContractMissPattern",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("合同履约率低 (%.0f%% 未达成)", rate*100),
		Evidence:   fmt.Sprintf("未达成: [%s]，共 %d/%d", strings.Join(missedChapters, ", "), missed, total),
		Suggestion: "Writer 可能未读 contract，或 contract required_beats 过于激进。检查 plan_chapter 和 writer.md 的配合。",
	}}
}

// HookWeakChain 检测章节 hook 评分连续偏弱。
func HookWeakChain(snap *Snapshot) []Finding {
	if len(snap.Reviews) < ThresholdHookWeakChain {
		return nil
	}

	chapters := sortedChapterReviews(snap)
	var weakChain []int
	for _, ch := range chapters {
		review := snap.Reviews[ch]
		if review == nil || review.Scope != "chapter" {
			continue
		}
		hook := review.Dimension("hook")
		if hook == nil || hook.Score >= ThresholdHookWeakScore {
			if len(weakChain) >= ThresholdHookWeakChain {
				break
			}
			weakChain = weakChain[:0]
			continue
		}
		weakChain = append(weakChain, ch)
	}
	if len(weakChain) < ThresholdHookWeakChain {
		return nil
	}

	var parts []string
	for _, ch := range weakChain {
		if hook := snap.Reviews[ch].Dimension("hook"); hook != nil {
			parts = append(parts, fmt.Sprintf("ch%d(%d)", ch, hook.Score))
		}
	}
	return []Finding{{
		Rule:       "HookWeakChain",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("章末钩子连续偏弱（连续 %d 章）", len(weakChain)),
		Evidence:   strings.Join(parts, ", "),
		Suggestion: "检查 writer.md 中 hook_goal 的执行是否清晰，必要时在 plan_chapter 中明确本章追读欲望，并校准 Editor 对 hook 的举证标准。",
	}}
}

// PayoffMissPattern 检测带 payoff_points 的章节长期未兑现。
func PayoffMissPattern(snap *Snapshot) []Finding {
	var total, missed int
	var details []string
	for ch, plan := range snap.Plans {
		if plan == nil || len(plan.Contract.PayoffPoints) == 0 {
			continue
		}
		review := snap.Reviews[ch]
		if review == nil {
			continue
		}
		total++
		if review.ContractStatus == "partial" || review.ContractStatus == "missed" {
			missed++
			details = append(details, fmt.Sprintf("ch%d(%d项 payoff)", ch, len(plan.Contract.PayoffPoints)))
		}
	}
	if total < 2 {
		return nil
	}
	rate := float64(missed) / float64(total)
	if rate <= ThresholdPayoffMissRate {
		return nil
	}
	sort.Strings(details)
	return []Finding{{
		Rule:       "PayoffMissPattern",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("爽点/情节点兑现率偏低 (%.0f%% 未达成)", rate*100),
		Evidence:   fmt.Sprintf("未兑现章节: [%s]，共 %d/%d", strings.Join(details, ", "), missed, total),
		Suggestion: "检查 plan_chapter 的 payoff_points 是否过多或过空，确保 Writer 在正文里明确兑现，而不是只做铺垫。",
	}}
}

// ExcessiveRewrites 检测改写率过高。
func ExcessiveRewrites(snap *Snapshot) []Finding {
	if len(snap.Reviews) < 2 {
		return nil
	}

	var total, rewrites int
	for _, r := range snap.Reviews {
		total++
		if r.Verdict == "rewrite" {
			rewrites++
		}
	}
	if total == 0 {
		return nil
	}
	rate := float64(rewrites) / float64(total)
	if rate <= ThresholdRewriteRate {
		return nil
	}
	return []Finding{{
		Rule:       "ExcessiveRewrites",
		Category:   CatQuality,
		Severity:   SevWarning,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.editor",
		Title:      fmt.Sprintf("改写率过高 (%d/%d = %.0f%%)", rewrites, total, rate*100),
		Evidence:   fmt.Sprintf("共 %d 次评审，%d 次 rewrite", total, rewrites),
		Suggestion: "Writer 持续产出低于 Editor 阈值的内容。检查 Writer prompt 的质量标准是否与 Editor 的评审标准对齐。",
	}}
}

// WordCountAnomaly 检测章节字数异常。
func WordCountAnomaly(snap *Snapshot) []Finding {
	if snap.Progress == nil || len(snap.Progress.ChapterWordCounts) < 3 {
		return nil
	}
	wc := snap.Progress.ChapterWordCounts

	var sum float64
	for _, w := range wc {
		sum += float64(w)
	}
	avg := sum / float64(len(wc))
	if avg == 0 {
		return nil
	}

	var anomalies []string
	for ch, w := range wc {
		ratio := float64(w) / avg
		if ratio < ThresholdWordShortRatio {
			anomalies = append(anomalies, fmt.Sprintf("ch%d(%d字,%.0f%%)", ch, w, ratio*100))
		} else if ratio > ThresholdWordLongRatio {
			anomalies = append(anomalies, fmt.Sprintf("ch%d(%d字,%.0f%%)", ch, w, ratio*100))
		}
	}
	if len(anomalies) == 0 {
		return nil
	}
	return []Finding{{
		Rule:       "WordCountAnomaly",
		Category:   CatQuality,
		Severity:   SevInfo,
		Confidence: ConfLow,
		AutoLevel:  AutoNone,
		Target:     "context.window",
		Title:      fmt.Sprintf("章节字数异常 (均值 %d 字)", int(math.Round(avg))),
		Evidence:   strings.Join(anomalies, "; "),
		Suggestion: "极短章节可能是输出截断（token 限制），极长章节可能消耗过多上下文窗口。检查模型 max_tokens 配置。",
	}}
}

// AIFlavorHotspots reports repeated transparent style-stat signals.
func AIFlavorHotspots(snap *Snapshot) []Finding {
	if len(snap.StyleStats) == 0 {
		return nil
	}
	type hotspotEvidence struct {
		chapter int
		ruleID  string
		text    string
	}
	counts := map[string]int{}
	var samples []hotspotEvidence
	for ch, stats := range snap.StyleStats {
		if stats == nil {
			continue
		}
		for _, hotspot := range stats.Hotspots {
			if hotspot.RuleID == "" {
				continue
			}
			counts[hotspot.RuleID]++
			if len(samples) < 5 {
				evidence := strings.TrimSpace(hotspot.Evidence)
				if evidence == "" {
					evidence = hotspot.Message
				}
				samples = append(samples, hotspotEvidence{chapter: ch, ruleID: hotspot.RuleID, text: evidence})
			}
		}
	}
	if len(counts) == 0 {
		return nil
	}
	var total int
	topRule := ""
	topCount := 0
	for rule, count := range counts {
		total += count
		if count > topCount || (count == topCount && rule < topRule) {
			topRule = rule
			topCount = count
		}
	}
	if total < 3 && topCount < 2 {
		return nil
	}
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].chapter == samples[j].chapter {
			return samples[i].ruleID < samples[j].ruleID
		}
		return samples[i].chapter < samples[j].chapter
	})
	parts := make([]string, 0, len(samples))
	for _, sample := range samples {
		parts = append(parts, fmt.Sprintf("ch%d:%s=%q", sample.chapter, sample.ruleID, truncateEvidence(sample.text, 24)))
	}
	severity := SevInfo
	if topCount >= 3 || total >= 6 {
		severity = SevWarning
	}
	return []Finding{{
		Rule:       "AIFlavorHotspots",
		Category:   CatQuality,
		Severity:   severity,
		Confidence: ConfMedium,
		AutoLevel:  AutoNone,
		Target:     "prompt.writer",
		Title:      fmt.Sprintf("AI 味热点集中：%s ×%d", topRule, topCount),
		Evidence:   fmt.Sprintf("style_stats 共 %d 个热点；样例: %s", total, strings.Join(parts, "; ")),
		Suggestion: "检查 Writer 去 AI 味约束和 StyleCard；优先按 review issue targets 做局部 spot-fix，不要因单项指标直接整章重写。",
	}}
}

// RewriteEffectiveness reports polish/rewrite attempts whose style signals did not improve.
func RewriteEffectiveness(snap *Snapshot) []Finding {
	if len(snap.StyleRewriteComparisons) == 0 {
		return nil
	}
	var findings []Finding
	for _, comparison := range snap.StyleRewriteComparisons {
		if comparison.Chapter <= 0 {
			continue
		}
		worsened := len(comparison.WorsenedMetrics)
		unchanged := len(comparison.UnchangedMetrics)
		improved := len(comparison.ImprovedMetrics)
		lowEdit := comparison.EditDistanceRatio > 0 && comparison.EditDistanceRatio < domain.StyleThresholdLowEditDistanceRatio
		if improved > 0 && worsened == 0 {
			if !lowEdit {
				continue
			}
		}
		if !lowEdit {
			if worsened < 2 && !(improved == 0 && unchanged >= 3) {
				continue
			}
		}
		severity := SevInfo
		if worsened >= 2 || lowEdit {
			severity = SevWarning
		}
		findings = append(findings, Finding{
			Rule:       "RewriteEffectiveness",
			Category:   CatQuality,
			Severity:   severity,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("第 %d 章%s后风格指标未改善", comparison.Chapter, rewriteModeLabel(comparison.Mode)),
			Evidence:   formatRewriteEffectivenessEvidence(comparison),
			Suggestion: "检查 rewrite_brief 是否引用了 style_stats.hotspots，Writer 是否只改了表面文本但未处理热点；必要时让 Editor issue targets 更具体。",
		})
	}
	return findings
}

type metricSample struct {
	chapter int
	value   float64
}

// MetricStyleSignals reports deterministic metric-level AI-tone signals.
func MetricStyleSignals(snap *Snapshot) []Finding {
	var emotionSamples, startSamples []metricSample
	for ch, stats := range snap.StyleStats {
		if value, ok := metricValue(stats, domain.StyleMetricEmotionLabelDensityPer1000); ok && value >= domain.StyleThresholdEmotionLabelDensity {
			emotionSamples = append(emotionSamples, metricSample{chapter: ch, value: value})
		}
		if value, ok := metricValue(stats, domain.StyleMetricSentenceStartDominantCategoryRatio); ok && value >= domain.StyleThresholdSentenceStartDominantRatio {
			startSamples = append(startSamples, metricSample{chapter: ch, value: value})
		}
	}
	var findings []Finding
	if len(emotionSamples) >= 2 {
		findings = append(findings, Finding{
			Rule:       "EmotionLabelDensity",
			Category:   CatQuality,
			Severity:   SevWarning,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("情绪标签密度偏高（%d章）", len(emotionSamples)),
			Evidence:   formatMetricSamples(emotionSamples, "hits/千字"),
			Suggestion: "把紧张/愤怒/悲伤等标签改为身体反应、动作选择和环境压力；优先更新 anti-ai-tone Gate C 相关示例。",
		})
	}
	if len(startSamples) >= 2 {
		findings = append(findings, Finding{
			Rule:       "SentenceStartDominance",
			Category:   CatQuality,
			Severity:   SevInfo,
			Confidence: ConfMedium,
			AutoLevel:  AutoNone,
			Target:     "prompt.writer",
			Title:      fmt.Sprintf("句首类别过于集中（%d章）", len(startSamples)),
			Evidence:   formatMetricSamples(startSamples, "ratio"),
			Suggestion: "让 Writer 按 working_memory.style_guidance 轮换对白、动作、感官、环境和无主语句开头，避免固定起手方式。",
		})
	}
	return findings
}

// EditorConsistency reports accepted chapters whose observable style signals keep degrading.
func EditorConsistency(snap *Snapshot) []Finding {
	if len(snap.Reviews) < 3 || len(snap.StyleStats) < 3 {
		return nil
	}
	chapters := sortedChapterReviews(snap)
	var chain []int
	checkChain := func() []Finding {
		if len(chain) < 3 {
			return nil
		}
		degradations := styleTrendDegradations(chain, snap.StyleStats)
		if len(degradations) < 2 {
			return nil
		}
		return []Finding{{
			Rule:       "EditorConsistency",
			Category:   CatQuality,
			Severity:   SevWarning,
			Confidence: ConfLow,
			AutoLevel:  AutoNone,
			Target:     "prompt.editor",
			Title:      fmt.Sprintf("连续 accept 但风格指标恶化（%d 章）", len(chain)),
			Evidence:   fmt.Sprintf("章节: %s；趋势: %s", formatChapterList(chain), strings.Join(degradations, "; ")),
			Suggestion: "校准 Editor aesthetic 维度：accept 前应核对 working_memory.style_stats 的热点趋势，避免审美阈值与可观测事实脱节。",
		}}
	}
	for _, ch := range chapters {
		review := snap.Reviews[ch]
		if review == nil || review.Scope != "chapter" || review.Verdict != "accept" || snap.StyleStats[ch] == nil {
			if findings := checkChain(); len(findings) > 0 {
				return findings
			}
			chain = chain[:0]
			continue
		}
		chain = append(chain, ch)
	}
	return checkChain()
}

func rewriteModeLabel(mode string) string {
	switch mode {
	case "polish":
		return "打磨"
	case "rewrite":
		return "重写"
	default:
		return "返工"
	}
}

func formatRewriteEffectivenessEvidence(comparison domain.StyleRewriteComparison) string {
	parts := []string{fmt.Sprintf("mode=%s", comparison.Mode)}
	if comparison.EditDistanceRatio > 0 {
		parts = append(parts, fmt.Sprintf("edit_distance_ratio=%.2f", comparison.EditDistanceRatio))
	}
	if comparison.ChangedRuneRatio > 0 {
		parts = append(parts, fmt.Sprintf("changed_rune_ratio=%.2f", comparison.ChangedRuneRatio))
	}
	if len(comparison.ImprovedMetrics) > 0 {
		parts = append(parts, "改善="+strings.Join(comparison.ImprovedMetrics, ","))
	}
	if len(comparison.WorsenedMetrics) > 0 {
		parts = append(parts, "恶化="+strings.Join(comparison.WorsenedMetrics, ","))
	}
	if len(comparison.UnchangedMetrics) > 0 {
		parts = append(parts, "无变化="+strings.Join(comparison.UnchangedMetrics, ","))
	}
	if len(comparison.Deltas) > 0 {
		keys := make([]string, 0, len(comparison.Deltas))
		for key := range comparison.Deltas {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var deltas []string
		for _, key := range keys {
			deltas = append(deltas, fmt.Sprintf("%s=%.2f", key, comparison.Deltas[key]))
		}
		parts = append(parts, "delta="+strings.Join(deltas, ","))
	}
	return strings.Join(parts, "；")
}

func formatMetricSamples(samples []metricSample, unit string) string {
	sort.Slice(samples, func(i, j int) bool { return samples[i].chapter < samples[j].chapter })
	if len(samples) > 5 {
		samples = samples[:5]
	}
	parts := make([]string, 0, len(samples))
	for _, sample := range samples {
		parts = append(parts, fmt.Sprintf("ch%d=%.2f%s", sample.chapter, sample.value, unit))
	}
	return strings.Join(parts, "; ")
}

func styleTrendDegradations(chapters []int, stats map[int]*domain.StyleStats) []string {
	type trendMetric struct {
		key    string
		label  string
		better int
	}
	metrics := []trendMetric{
		{key: "sentence_length_stddev", label: "句长标准差下降", better: 1},
		{key: "sentence_start_unique_rate", label: "句首独特率下降", better: 1},
		{key: "pattern_density_per_1000", label: "套话密度上升", better: -1},
		{key: "paragraph_uniform_ratio", label: "段落均匀度上升", better: -1},
	}
	var out []string
	first := stats[chapters[0]]
	last := stats[chapters[len(chapters)-1]]
	if first == nil || last == nil {
		return nil
	}
	for _, metric := range metrics {
		firstValue, firstOK := metricValue(first, metric.key)
		lastValue, lastOK := metricValue(last, metric.key)
		if !firstOK || !lastOK {
			continue
		}
		delta := lastValue - firstValue
		if metric.better > 0 && delta < -0.05 {
			out = append(out, fmt.Sprintf("%s %.2f→%.2f", metric.label, firstValue, lastValue))
		}
		if metric.better < 0 && delta > 0.05 {
			out = append(out, fmt.Sprintf("%s %.2f→%.2f", metric.label, firstValue, lastValue))
		}
	}
	firstHotspots := len(first.Hotspots)
	lastHotspots := len(last.Hotspots)
	if lastHotspots-firstHotspots >= 2 {
		out = append(out, fmt.Sprintf("热点数量上升 %d→%d", firstHotspots, lastHotspots))
	}
	return out
}

func metricValue(stats *domain.StyleStats, key string) (float64, bool) {
	if stats == nil || stats.Metrics == nil {
		return 0, false
	}
	metric, ok := stats.Metrics[key]
	return metric.Value, ok
}

func formatChapterList(chapters []int) string {
	parts := make([]string, 0, len(chapters))
	for _, ch := range chapters {
		parts = append(parts, fmt.Sprintf("ch%d", ch))
	}
	return strings.Join(parts, ",")
}

func truncateEvidence(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func sortedChapterReviews(snap *Snapshot) []int {
	chapters := make([]int, 0, len(snap.Reviews))
	for ch := range snap.Reviews {
		chapters = append(chapters, ch)
	}
	sort.Ints(chapters)
	return chapters
}
