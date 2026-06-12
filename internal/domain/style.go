package domain

// StyleStatsSchemaVersion is the persisted schema version for chapter style statistics.
const StyleStatsSchemaVersion = "style_stats.v1"

// StyleRewriteComparisonSchemaVersion is the persisted schema version for rewrite comparisons.
const StyleRewriteComparisonSchemaVersion = "style_rewrite_comparison.v1"

// StyleStats contains deterministic prose observations for one chapter.
type StyleStats struct {
	SchemaVersion  string                 `json:"schema_version"`
	Chapter        int                    `json:"chapter"`
	ComputedAt     string                 `json:"computed_at"`
	RulesetVersion string                 `json:"ruleset_version,omitempty"`
	Model          StyleModelInfo         `json:"model"`
	Metrics        map[string]StyleMetric `json:"metrics"`
	Hotspots       []StyleHotspot         `json:"hotspots,omitempty"`
	Summary        string                 `json:"summary"`
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
	SchemaVersion    string             `json:"schema_version"`
	Chapter          int                `json:"chapter"`
	Mode             string             `json:"mode"`
	ComputedAt       string             `json:"computed_at"`
	Before           *StyleStats        `json:"before,omitempty"`
	After            *StyleStats        `json:"after,omitempty"`
	Deltas           map[string]float64 `json:"deltas,omitempty"`
	ImprovedMetrics  []string           `json:"improved_metrics,omitempty"`
	WorsenedMetrics  []string           `json:"worsened_metrics,omitempty"`
	UnchangedMetrics []string           `json:"unchanged_metrics,omitempty"`
}
