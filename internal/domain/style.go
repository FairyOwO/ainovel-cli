package domain

import "strings"

// StyleStatsSchemaVersion is the persisted schema version for chapter style statistics.
const StyleStatsSchemaVersion = "style_stats.v1"

// StyleRewriteComparisonSchemaVersion is the persisted schema version for rewrite comparisons.
const StyleRewriteComparisonSchemaVersion = "style_rewrite_comparison.v1"

// DiagnosticGuidanceSchemaVersion is the persisted schema version for compact diagnostic feedback.
const DiagnosticGuidanceSchemaVersion = "diag_guidance.v1"

const (
	StyleMetricSentenceLengthStddev                = "sentence_length_stddev"
	StyleMetricSentenceStartUniqueRate             = "sentence_start_unique_rate"
	StyleMetricSentenceStartDominantCategoryRatio  = "sentence_start_dominant_category_ratio"
	StyleMetricSentenceStartAbstractConnectorRatio = "sentence_start_abstract_connector_ratio"
	StyleMetricParagraphLengthStddev               = "paragraph_length_stddev"
	StyleMetricParagraphUniformRatio               = "paragraph_uniform_ratio"
	StyleMetricPatternDensityPer1000               = "pattern_density_per_1000"
	StyleMetricRepeatedNGramRate                   = "repeated_ngram_rate"
	StyleMetricHomogeneousSentenceRatio            = "homogeneous_sentence_ratio"
	StyleMetricEmotionLabelDensityPer1000          = "emotion_label_density_per_1000"
)

const (
	StyleThresholdEmotionLabelDensity        = 6.0
	StyleThresholdSentenceStartDominantRatio = 0.55
	StyleThresholdAbstractConnectorRatio     = 0.3
	StyleThresholdLowEditDistanceRatio       = 0.08
	StyleThresholdLowSentenceStddev          = 3.0
	StyleThresholdParagraphUniformRatio      = 0.8
)

var EmotionLabels = []string{
	"紧张", "愤怒", "悲伤", "恐惧", "害怕", "焦虑", "绝望", "震惊", "惊讶", "痛苦", "难过", "不安", "委屈", "兴奋", "激动", "羞愧", "尴尬", "五味杂陈",
}

var FunctionWords = []string{"的", "了", "是", "在", "和", "就", "也", "都", "还", "又", "把", "被", "他", "她", "它", "我", "你", "这", "那"}

// CategorizeSentenceStart classifies the first meaningful sentence token for style statistics.
func CategorizeSentenceStart(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "“") || strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "「") || strings.HasPrefix(trimmed, "『") {
		return "dialogue"
	}
	if hasAnyPrefix(trimmed, []string{"然而", "与此同时", "此时", "这时", "可是", "但是", "随后", "于是", "后来", "终于", "忽然", "突然"}) {
		return "abstract_connector"
	}
	if hasAnyPrefix(trimmed, []string{"夜里", "夜色", "清晨", "黎明", "天亮", "晨光", "黄昏", "雨里", "雨声", "门外", "窗外", "街上", "屋里", "院中"}) {
		return "time_place"
	}
	first := firstNonPunctuationRune(trimmed)
	if first == 0 {
		return ""
	}
	if strings.ContainsRune("他她它我你", first) {
		return "pronoun"
	}
	if strings.ContainsRune("走站坐伸抬低握推拉看看听问说笑转拿放踩奔跑躲捡撕扔", first) {
		return "action"
	}
	return "other"
}

func hasAnyPrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func firstNonPunctuationRune(text string) rune {
	for _, r := range text {
		if strings.ContainsRune("，、；：,.!?！？。…“”‘’「」『』", r) {
			continue
		}
		return r
	}
	return 0
}

// StyleStats contains deterministic prose observations for one chapter.
type StyleStats struct {
	SchemaVersion  string                 `json:"schema_version"`
	Chapter        int                    `json:"chapter"`
	ComputedAt     string                 `json:"computed_at"`
	RulesetVersion string                 `json:"ruleset_version,omitempty"`
	Model          StyleModelInfo         `json:"model"`
	External       *ExternalDetectorScore `json:"external_detector,omitempty"`
	Metrics        map[string]StyleMetric `json:"metrics"`
	Hotspots       []StyleHotspot         `json:"hotspots,omitempty"`
	Summary        string                 `json:"summary"`
}

// ExternalDetectorScore is an optional AI-text detector observation.
type ExternalDetectorScore struct {
	Provider   string  `json:"provider,omitempty"`
	Score      float64 `json:"score,omitempty"`
	Label      string  `json:"label,omitempty"`
	Status     string  `json:"status,omitempty"`
	CheckedAt  string  `json:"checked_at,omitempty"`
	Confidence string  `json:"confidence,omitempty"`
}

// StyleModelInfo records the model active when the prose was committed.
type StyleModelInfo struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// StyleMetric is one transparent measurable signal used to explain prose texture.
type StyleMetric struct {
	Value   float64 `json:"value"`
	Unit    string  `json:"unit,omitempty"`
	Message string  `json:"message,omitempty"`
}

// StyleHotspot points to local evidence for a style concern.
type StyleHotspot struct {
	ID             string    `json:"id"`
	RuleID         string    `json:"rule_id"`
	Severity       string    `json:"severity"`
	Span           *TextSpan `json:"span,omitempty"`
	ParagraphIndex int       `json:"paragraph_index,omitempty"`
	SentenceIndex  int       `json:"sentence_index,omitempty"`
	Evidence       string    `json:"evidence"`
	Message        string    `json:"message"`
	SuggestionType string    `json:"suggestion_type"`
}

// TextSpan is a rune-based location in the analyzed text.
type TextSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// StyleCard is the project-level quantitative style target consumed by writer/editor.
type StyleCard struct {
	SentenceStdFloor        float64              `json:"sentence_std_floor,omitempty"`
	DialogueRatioTarget     string               `json:"dialogue_ratio_target,omitempty"`
	ParagraphVarianceTarget string               `json:"paragraph_variance_target,omitempty"`
	SensoryPreferences      []string             `json:"sensory_preferences,omitempty"`
	BannedPatterns          []string             `json:"banned_patterns,omitempty"`
	DialogueDNA             []StyleDialogueDNA   `json:"dialogue_dna,omitempty"`
	ChapterEndingPolicy     string               `json:"chapter_ending_policy,omitempty"`
	ChapterTypeProfiles     []ChapterTypeProfile `json:"chapter_type_profiles,omitempty"`
}

// StyleDialogueDNA records project-specific dialogue traits for one character or role.
type StyleDialogueDNA struct {
	Name   string   `json:"name"`
	Traits []string `json:"traits,omitempty"`
}

// ChapterTypeProfile stores expected style ranges for a chapter type.
type ChapterTypeProfile struct {
	Type                    string `json:"type"`
	SentenceStdRange        string `json:"sentence_std_range,omitempty"`
	DialogueRatioTarget     string `json:"dialogue_ratio_target,omitempty"`
	ParagraphVarianceTarget string `json:"paragraph_variance_target,omitempty"`
	Notes                   string `json:"notes,omitempty"`
}

// StyleRewriteComparison preserves before/after style observations for one rewrite or polish commit.
type StyleRewriteComparison struct {
	SchemaVersion     string             `json:"schema_version"`
	Chapter           int                `json:"chapter"`
	Mode              string             `json:"mode"`
	ComputedAt        string             `json:"computed_at"`
	Before            *StyleStats        `json:"before,omitempty"`
	After             *StyleStats        `json:"after,omitempty"`
	EditDistanceRatio float64            `json:"edit_distance_ratio,omitempty"`
	ChangedRuneRatio  float64            `json:"changed_rune_ratio,omitempty"`
	Deltas            map[string]float64 `json:"deltas,omitempty"`
	ImprovedMetrics   []string           `json:"improved_metrics,omitempty"`
	WorsenedMetrics   []string           `json:"worsened_metrics,omitempty"`
	UnchangedMetrics  []string           `json:"unchanged_metrics,omitempty"`
}

// DiagnosticGuidance is compact, non-prose feedback from /diag for future agent context.
type DiagnosticGuidance struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at"`
	Items         []DiagnosticGuidanceItem `json:"items,omitempty"`
}

// DiagnosticGuidanceItem is an agent-facing instruction distilled from one diagnostic finding.
type DiagnosticGuidanceItem struct {
	Rule       string `json:"rule"`
	Severity   string `json:"severity"`
	Target     string `json:"target"`
	Title      string `json:"title"`
	Signal     string `json:"signal,omitempty"`
	Suggestion string `json:"suggestion"`
}
