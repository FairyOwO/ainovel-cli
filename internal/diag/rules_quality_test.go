package diag

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestHookWeakChain(t *testing.T) {
	snap := &Snapshot{
		Reviews: map[int]*domain.ReviewEntry{
			1: {Chapter: 1, Scope: "chapter", Dimensions: []domain.DimensionScore{{Dimension: "hook", Score: 72, Verdict: "warning"}}},
			2: {Chapter: 2, Scope: "chapter", Dimensions: []domain.DimensionScore{{Dimension: "hook", Score: 68, Verdict: "warning"}}},
			3: {Chapter: 3, Scope: "chapter", Dimensions: []domain.DimensionScore{{Dimension: "hook", Score: 74, Verdict: "warning"}}},
			4: {Chapter: 4, Scope: "chapter", Dimensions: []domain.DimensionScore{{Dimension: "hook", Score: 88, Verdict: "pass"}}},
		},
	}

	findings := HookWeakChain(snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "HookWeakChain" {
		t.Fatalf("unexpected rule: %+v", findings[0])
	}
	if !strings.Contains(findings[0].Evidence, "ch1(72)") || !strings.Contains(findings[0].Evidence, "ch3(74)") {
		t.Fatalf("unexpected evidence: %s", findings[0].Evidence)
	}
}

func TestPayoffMissPattern(t *testing.T) {
	snap := &Snapshot{
		Plans: map[int]*domain.ChapterPlan{
			1: {Chapter: 1, Contract: domain.ChapterContract{PayoffPoints: []string{"首战取胜"}}},
			2: {Chapter: 2, Contract: domain.ChapterContract{PayoffPoints: []string{"确认搭档关系"}}},
			3: {Chapter: 3, Contract: domain.ChapterContract{PayoffPoints: []string{"揭开真相一角"}}},
		},
		Reviews: map[int]*domain.ReviewEntry{
			1: {Chapter: 1, Scope: "chapter", ContractStatus: "partial"},
			2: {Chapter: 2, Scope: "chapter", ContractStatus: "missed"},
			3: {Chapter: 3, Scope: "chapter", ContractStatus: "met"},
		},
	}

	findings := PayoffMissPattern(snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "PayoffMissPattern" {
		t.Fatalf("unexpected rule: %+v", findings[0])
	}
	if !strings.Contains(findings[0].Evidence, "ch1(1项 payoff)") || !strings.Contains(findings[0].Evidence, "2/3") {
		t.Fatalf("unexpected evidence: %s", findings[0].Evidence)
	}
}

func TestAIFlavorHotspots(t *testing.T) {
	snap := &Snapshot{
		StyleStats: map[int]*domain.StyleStats{
			1: {Chapter: 1, Hotspots: []domain.StyleHotspot{
				{RuleID: "cliche_summary_in_the_end", Evidence: "这一刻，仿佛一切都有了答案。"},
				{RuleID: "low_sentence_variance", Evidence: "句长std=1.2"},
			}},
			2: {Chapter: 2, Hotspots: []domain.StyleHotspot{
				{RuleID: "cliche_summary_in_the_end", Evidence: "仿佛一切"},
				{RuleID: "repeated_sentence_start", Evidence: "他说/他说/他说"},
			}},
			3: {Chapter: 3, Hotspots: []domain.StyleHotspot{
				{RuleID: "cliche_summary_in_the_end", Evidence: "这一刻"},
			}},
		},
	}

	findings := AIFlavorHotspots(snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "AIFlavorHotspots" || findings[0].Severity != SevWarning {
		t.Fatalf("unexpected finding: %+v", findings[0])
	}
	if !strings.Contains(findings[0].Title, "cliche_summary_in_the_end") {
		t.Fatalf("expected top rule in title, got %q", findings[0].Title)
	}
	if !strings.Contains(findings[0].Evidence, "style_stats 共 5 个热点") || !strings.Contains(findings[0].Evidence, "ch1:cliche_summary_in_the_end") {
		t.Fatalf("unexpected evidence: %s", findings[0].Evidence)
	}
}

func TestRewriteEffectiveness(t *testing.T) {
	snap := &Snapshot{
		StyleRewriteComparisons: []domain.StyleRewriteComparison{
			{
				SchemaVersion:    domain.StyleRewriteComparisonSchemaVersion,
				Chapter:          2,
				Mode:             "polish",
				WorsenedMetrics:  []string{"pattern_density_per_1000", "paragraph_uniform_ratio"},
				UnchangedMetrics: []string{"sentence_start_unique_rate"},
				Deltas: map[string]float64{
					"pattern_density_per_1000_delta": 1.2,
					"paragraph_uniform_ratio_delta":  0.3,
				},
			},
		},
	}

	findings := RewriteEffectiveness(snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "RewriteEffectiveness" || findings[0].Severity != SevWarning {
		t.Fatalf("unexpected finding: %+v", findings[0])
	}
	if !strings.Contains(findings[0].Evidence, "恶化=pattern_density_per_1000,paragraph_uniform_ratio") {
		t.Fatalf("unexpected evidence: %s", findings[0].Evidence)
	}
}

func TestEditorConsistency(t *testing.T) {
	snap := &Snapshot{
		Reviews: map[int]*domain.ReviewEntry{
			1: {Chapter: 1, Scope: "chapter", Verdict: "accept"},
			2: {Chapter: 2, Scope: "chapter", Verdict: "accept"},
			3: {Chapter: 3, Scope: "chapter", Verdict: "accept"},
		},
		StyleStats: map[int]*domain.StyleStats{
			1: {Chapter: 1, Metrics: map[string]domain.StyleMetric{
				"sentence_length_stddev":     {Value: 6.0},
				"sentence_start_unique_rate": {Value: 0.9},
				"pattern_density_per_1000":   {Value: 0.2},
			}, Hotspots: []domain.StyleHotspot{{RuleID: "low_dialogue_ratio"}}},
			2: {Chapter: 2, Metrics: map[string]domain.StyleMetric{
				"sentence_length_stddev":     {Value: 4.0},
				"sentence_start_unique_rate": {Value: 0.7},
				"pattern_density_per_1000":   {Value: 1.0},
			}, Hotspots: []domain.StyleHotspot{{RuleID: "low_dialogue_ratio"}, {RuleID: "repeated_sentence_start"}}},
			3: {Chapter: 3, Metrics: map[string]domain.StyleMetric{
				"sentence_length_stddev":     {Value: 2.0},
				"sentence_start_unique_rate": {Value: 0.4},
				"pattern_density_per_1000":   {Value: 2.5},
			}, Hotspots: []domain.StyleHotspot{{RuleID: "low_dialogue_ratio"}, {RuleID: "repeated_sentence_start"}, {RuleID: "cliche_summary"}}},
		},
	}

	findings := EditorConsistency(snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "EditorConsistency" || findings[0].Severity != SevWarning {
		t.Fatalf("unexpected finding: %+v", findings[0])
	}
	if !strings.Contains(findings[0].Evidence, "ch1,ch2,ch3") || !strings.Contains(findings[0].Evidence, "句长标准差下降") {
		t.Fatalf("unexpected evidence: %s", findings[0].Evidence)
	}
}

func TestBuildStatsIncludesStyleStats(t *testing.T) {
	snap := &Snapshot{
		Progress: &domain.Progress{CompletedChapters: []int{1, 2}, TotalChapters: 4, TotalWordCount: 6000},
		Reviews:  map[int]*domain.ReviewEntry{},
		StyleStats: map[int]*domain.StyleStats{
			1: {Chapter: 1, Hotspots: []domain.StyleHotspot{{RuleID: "low_dialogue_ratio"}, {RuleID: "low_dialogue_ratio"}}},
			2: {Chapter: 2, Hotspots: []domain.StyleHotspot{{RuleID: "cliche_summary_in_the_end"}}},
		},
	}
	stats := buildStats(snap)
	if stats.StyleStatsCount != 2 || stats.AIHotspotCount != 3 || stats.TopAIHotspotRule != "low_dialogue_ratio" {
		t.Fatalf("unexpected style stats summary: %+v", stats)
	}
}
