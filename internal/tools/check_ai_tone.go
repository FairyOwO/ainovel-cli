package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
	styleanalyzer "github.com/voocel/ainovel-cli/internal/style"
)

// CheckAIToneTool analyzes draft/final prose for deterministic AI-tone signals.
type CheckAIToneTool struct {
	store *store.Store
}

func NewCheckAIToneTool(store *store.Store) *CheckAIToneTool {
	return &CheckAIToneTool{store: store}
}

func (t *CheckAIToneTool) Name() string { return "check_ai_tone" }
func (t *CheckAIToneTool) Description() string {
	return "检查章节草稿或终稿的 AI 味机械信号，返回 style_stats 摘要、问题证据和局部修补 targets；只读、不入返工队列、不替代审美裁决"
}
func (t *CheckAIToneTool) Label() string { return "AI 味检查" }

func (t *CheckAIToneTool) ReadOnly(_ json.RawMessage) bool        { return true }
func (t *CheckAIToneTool) ConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *CheckAIToneTool) Schema() map[string]any {
	return schema.Object(
		schema.Property("chapter", schema.Int("章节号")).Required(),
		schema.Property("source", schema.Enum("检查来源", "draft", "final")).Required(),
	)
}

func (t *CheckAIToneTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Chapter int    `json:"chapter"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be > 0: %w", errs.ErrToolArgs)
	}
	if a.Source == "" {
		a.Source = "draft"
	}

	content, err := t.loadAIToneSource(a.Chapter, a.Source)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, fmt.Errorf("no %s content found for chapter %d: %w", a.Source, a.Chapter, errs.ErrToolPrecondition)
	}

	stats := styleanalyzer.AnalyzeChineseProse(content)
	stats.Chapter = a.Chapter
	return json.Marshal(buildAIToneResult(a.Chapter, a.Source, content, stats))
}

func (t *CheckAIToneTool) loadAIToneSource(chapter int, source string) (string, error) {
	switch source {
	case "", "draft":
		content, err := t.store.Drafts.LoadDraft(chapter)
		if err != nil {
			return "", fmt.Errorf("load draft chapter %d: %w: %w", chapter, errs.ErrStoreRead, err)
		}
		return content, nil
	case "final":
		content, err := t.store.Drafts.LoadChapterText(chapter)
		if err != nil {
			return "", fmt.Errorf("load final chapter %d: %w: %w", chapter, errs.ErrStoreRead, err)
		}
		return content, nil
	default:
		return "", fmt.Errorf("source must be draft or final: %w", errs.ErrToolArgs)
	}
}

func buildAIToneResult(chapter int, source, content string, stats *domain.StyleStats) map[string]any {
	findings := aiToneFindings(content, stats.Hotspots, 8)
	decision, hardFail, warningCount := aiToneDecision(stats.Hotspots)
	result := map[string]any{
		"chapter":       chapter,
		"source":        source,
		"decision":      decision,
		"hard_fail":     hardFail,
		"summary":       stats.Summary,
		"metrics":       compactStyleMetrics(stats),
		"findings":      findings,
		"warning_count": warningCount,
		"advisory":      "工具只提供机械证据和局部 targets；是否修改、局部 polish 还是整章重写，必须结合剧情功能判断",
	}
	if len(findings) > 0 {
		result["next_step"] = aiToneNextStep(hardFail)
	} else {
		result["next_step"] = "未发现高优先级机械 AI 味热点；若一致性检查也通过，可继续 commit_chapter"
	}
	return result
}

func aiToneDecision(hotspots []domain.StyleHotspot) (string, bool, int) {
	warnings := 0
	severeRules := map[string]int{}
	severityByRule := map[string]int{}
	for _, hotspot := range hotspots {
		if hotspot.Severity == "warning" || hotspot.Severity == "error" {
			warnings++
			severeRules[hotspot.RuleID]++
			severityByRule[hotspot.RuleID]++
			continue
		}
		if hotspot.Severity == "info" {
			severityByRule[hotspot.RuleID]++
		}
	}
	if warnings >= 3 || severeRules["cliche_summary_in_the_end"] > 0 || aiToneHasConcentratedHotspots(severeRules, severityByRule) {
		return "hard_fail", true, warnings
	}
	if len(hotspots) > 0 {
		return "warning", false, warnings
	}
	return "pass", false, warnings
}

func aiToneHasConcentratedHotspots(severeRules map[string]int, severityByRule map[string]int) bool {
	for _, count := range severeRules {
		if count >= 2 {
			return true
		}
	}
	concentratedSignalRules := map[string]struct{}{
		"low_sentence_variance":          {},
		"repeated_sentence_start":        {},
		"uniform_paragraph_length":       {},
		"abstract_connector_sentence_start": {},
		"emotion_label_density":          {},
	}
	concentratedSignals := 0
	for ruleID := range concentratedSignalRules {
		if severityByRule[ruleID] > 0 {
			concentratedSignals++
		}
	}
	return concentratedSignals >= 3
}

func aiToneNextStep(hardFail bool) string {
	if hardFail {
		return "AI 味硬门禁命中：优先按 findings.targets 局部修；若多处结构性模板化，再用 draft_chapter(mode=write) 覆盖重写，然后重新 check_consistency 与 check_ai_tone"
	}
	return "发现可疑机械信号：结合剧情功能判断是否局部 edit_chapter；若保留原句服务叙事，可记录判断后继续 commit_chapter"
}

func aiToneFindings(content string, hotspots []domain.StyleHotspot, limit int) []map[string]any {
	if len(hotspots) == 0 || limit <= 0 {
		return nil
	}
	findings := make([]map[string]any, 0, min(len(hotspots), limit))
	for _, hotspot := range hotspots {
		finding := map[string]any{
			"hotspot_id":      hotspot.ID,
			"rule_id":         hotspot.RuleID,
			"severity":        hotspot.Severity,
			"evidence":        hotspot.Evidence,
			"message":         hotspot.Message,
			"suggestion_type": hotspot.SuggestionType,
		}
		if hotspot.ParagraphIndex > 0 {
			finding["paragraph_index"] = hotspot.ParagraphIndex
		}
		if hotspot.SentenceIndex > 0 {
			finding["sentence_index"] = hotspot.SentenceIndex
		}
		if oldText := aiToneOldText(content, hotspot); oldText != "" {
			finding["target"] = map[string]any{
				"hotspot_id":        hotspot.ID,
				"rule_id":           hotspot.RuleID,
				"paragraph_index":   hotspot.ParagraphIndex,
				"sentence_index":    hotspot.SentenceIndex,
				"old_text":          oldText,
				"suggestion_type":   hotspot.SuggestionType,
				"replace_all":       false,
				"requires_judgment": true,
			}
		}
		findings = append(findings, finding)
		if len(findings) >= limit {
			break
		}
	}
	return findings
}

func aiToneOldText(content string, hotspot domain.StyleHotspot) string {
	if hotspot.Span == nil {
		return hotspot.Evidence
	}
	runes := []rune(content)
	start, end := hotspot.Span.Start, hotspot.Span.End
	if start < 0 || end <= start || start >= len(runes) {
		return hotspot.Evidence
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}
