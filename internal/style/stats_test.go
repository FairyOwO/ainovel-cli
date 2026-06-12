package style

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestAnalyzeChineseProseDetectsLowSentenceVariance(t *testing.T) {
	stats := AnalyzeChineseProse("他走进雨里。她站在门边。风吹过长街。灯亮在远处。没人再说话。")
	if stats.SchemaVersion == "" || stats.Metrics["sentence_length_stddev"].Value >= 3 {
		t.Fatalf("expected low sentence stddev metric, got %+v", stats.Metrics["sentence_length_stddev"])
	}
	if !hasHotspot(stats.Hotspots, "low_sentence_variance") {
		t.Fatalf("expected low_sentence_variance hotspot, got %+v", stats.Hotspots)
	}
}

func TestAnalyzeChineseProseDetectsDialogueRatioAndSentenceStarts(t *testing.T) {
	stats := AnalyzeChineseProse("他说要走。他说要等。他说要看。她没有回答。风停了。")
	if stats.Metrics["dialogue_ratio"].Value != 0 {
		t.Fatalf("dialogue ratio = %.2f, want 0", stats.Metrics["dialogue_ratio"].Value)
	}
	if !hasHotspot(stats.Hotspots, "low_dialogue_ratio") {
		t.Fatalf("expected low_dialogue_ratio hotspot, got %+v", stats.Hotspots)
	}
	if !hasHotspot(stats.Hotspots, "repeated_sentence_start") {
		t.Fatalf("expected repeated_sentence_start hotspot, got %+v", stats.Hotspots)
	}
}

func TestAnalyzeChineseProseDetectsUniformParagraphsAndPatterns(t *testing.T) {
	text := "命运落在门外。\n命运落在窗前。\n命运落在灯下。\n这一刻风停了。"
	stats := AnalyzeChineseProse(text)
	if !hasHotspot(stats.Hotspots, "uniform_paragraph_length") {
		t.Fatalf("expected uniform_paragraph_length hotspot, got %+v", stats.Hotspots)
	}
	if !hasHotspot(stats.Hotspots, "cliche_summary_in_the_end") {
		t.Fatalf("expected cliche_summary_in_the_end hotspot, got %+v", stats.Hotspots)
	}
	if stats.Metrics["pattern_density_per_1000"].Value == 0 {
		t.Fatal("expected pattern density > 0")
	}
}

func TestAnalyzeChineseProseHandlesQuotedDialogue(t *testing.T) {
	stats := AnalyzeChineseProse("林墨停下脚步。\"我不同意。\"她抬起头。\"那就试试。\"")
	if stats.Metrics["dialogue_ratio"].Value <= 0 {
		t.Fatalf("expected dialogue ratio > 0, got %.2f", stats.Metrics["dialogue_ratio"].Value)
	}
}

func TestAnalyzeChineseProseHandlesMixedQuoteStyles(t *testing.T) {
	stats := AnalyzeChineseProse("林墨说：“先确认 \"红灯\" 代表危险。”她把纸条折起。\"别再提它。\"")
	if stats.Metrics["dialogue_ratio"].Value <= 0 || stats.Metrics["dialogue_ratio"].Value >= 0.9 {
		t.Fatalf("expected mixed quotes to produce bounded dialogue ratio, got %.2f", stats.Metrics["dialogue_ratio"].Value)
	}
}

func TestAnalyzeChineseProseHandlesChineseSentenceBoundaries(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{name: "ellipsis", text: "雨声停了……门外却响起脚步。她握紧刀柄！他还活着？"},
		{name: "short chinese dialogue", text: "林墨问：“走？”她答：“嗯。”风从门缝里钻进来。"},
		{name: "corner quote action tag", text: "她低声说：「走。」林墨没有回头，只把灯吹灭。"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stats := AnalyzeChineseProse(tc.text)
			if stats.Metrics["sentence_length_mean"].Value <= 0 {
				t.Fatalf("expected sentence metrics for %q, got %+v", tc.text, stats.Metrics)
			}
			if tc.name != "ellipsis" && stats.Metrics["dialogue_ratio"].Value <= 0 {
				t.Fatalf("expected dialogue ratio > 0 for %q, got %.2f", tc.text, stats.Metrics["dialogue_ratio"].Value)
			}
		})
	}
}

func hasHotspot(hotspots []domain.StyleHotspot, ruleID string) bool {
	for _, hotspot := range hotspots {
		if hotspot.RuleID == ruleID {
			return true
		}
	}
	return false
}
