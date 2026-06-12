package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestCheckAIToneDraftReturnsCompactFindings(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Drafts.SaveDraft(1, "这一刻，仿佛一切都有了答案。命运落下。他感到紧张。她很悲伤。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"chapter": 1, "source": "draft"})
	raw, err := NewCheckAIToneTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["decision"] != "hard_fail" || out["hard_fail"] != true {
		t.Fatalf("expected hard_fail, got %v", out)
	}
	if out["action_recommendation"] != "rewrite" {
		t.Fatalf("expected rewrite recommendation, got %v", out["action_recommendation"])
	}
	if _, hasContent := out["content"]; hasContent {
		t.Fatalf("check_ai_tone must not return full prose: %v", out)
	}
	findings, ok := out["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Fatalf("expected findings, got %v", out["findings"])
	}
	first := findings[0].(map[string]any)
	if first["rule_id"] == "" || first["evidence"] == "" {
		t.Fatalf("finding missing evidence: %v", first)
	}
	if _, ok := first["target"].(map[string]any); !ok {
		t.Fatalf("expected target in finding: %v", first)
	}
}

func TestCheckAIToneDoesNotPersistStatsOrGuidance(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Drafts.SaveDraft(2, "他说要走。他说要等。他说要看。她没有回答。风停了。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"chapter": 2, "source": "draft"})
	if _, err := NewCheckAIToneTool(s).Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	stats, err := s.World.LoadStyleStats(2)
	if err != nil {
		t.Fatalf("LoadStyleStats: %v", err)
	}
	if stats != nil {
		t.Fatalf("check_ai_tone should not persist style stats, got %+v", stats)
	}
	guidance, err := s.World.LoadDiagnosticGuidance()
	if err != nil {
		t.Fatalf("LoadDiagnosticGuidance: %v", err)
	}
	if guidance != nil {
		t.Fatalf("check_ai_tone should not persist guidance, got %+v", guidance)
	}
}

func TestCheckAIToneReadsFinalSource(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Drafts.SaveFinalChapter(3, "他说。她答。风停。"); err != nil {
		t.Fatalf("SaveFinalChapter: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"chapter": 3, "source": "final"})
	raw, err := NewCheckAIToneTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["source"] != "final" || out["chapter"] != float64(3) {
		t.Fatalf("unexpected output: %v", out)
	}
}

func TestCheckAIToneWarningRecommendsLocalFix(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Drafts.SaveDraft(4, "然而，雨停了。桌上的灯晃了一下。她问去哪。风从门缝里钻进来，门外有人咳嗽了一声。"); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	args, _ := json.Marshal(map[string]any{"chapter": 4, "source": "draft"})
	raw, err := NewCheckAIToneTool(s).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["decision"] != "warning" || out["hard_fail"] != false {
		t.Fatalf("expected warning without hard_fail, got %v", out)
	}
	if out["action_recommendation"] != "local_fix" {
		t.Fatalf("expected local_fix recommendation, got %v", out["action_recommendation"])
	}
}

func TestAIToneActionRecommendation(t *testing.T) {
	cases := map[string]string{
		"pass":      "pass",
		"warning":   "local_fix",
		"hard_fail": "rewrite",
	}
	for decision, want := range cases {
		if got := aiToneActionRecommendation(decision); got != want {
			t.Fatalf("decision %q: got %q want %q", decision, got, want)
		}
	}
}

func TestAIToneDecisionEscalatesConcentratedHotspots(t *testing.T) {
	hotspots := []domain.StyleHotspot{
		{RuleID: "low_sentence_variance", Severity: "warning"},
		{RuleID: "repeated_sentence_start", Severity: "warning"},
		{RuleID: "uniform_paragraph_length", Severity: "warning"},
	}

	decision, hardFail, warningCount := aiToneDecision(hotspots)
	if decision != "hard_fail" || !hardFail {
		t.Fatalf("expected concentrated hotspots to hard fail, got decision=%q hardFail=%v", decision, hardFail)
	}
	if warningCount != 3 {
		t.Fatalf("expected warningCount=3, got %d", warningCount)
	}
}

func TestAIToneDecisionEscalatesRepeatedSameRule(t *testing.T) {
	hotspots := []domain.StyleHotspot{
		{RuleID: "abstract_connector_sentence_start", Severity: "warning"},
		{RuleID: "abstract_connector_sentence_start", Severity: "warning"},
	}

	decision, hardFail, warningCount := aiToneDecision(hotspots)
	if decision != "hard_fail" || !hardFail {
		t.Fatalf("expected repeated same-rule hotspots to hard fail, got decision=%q hardFail=%v", decision, hardFail)
	}
	if warningCount != 2 {
		t.Fatalf("expected warningCount=2, got %d", warningCount)
	}
}
