package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestSaveArcSummaryPersistsStyleCard(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tool := NewSaveArcSummaryTool(s)
	args, _ := json.Marshal(map[string]any{
		"volume":     1,
		"arc":        2,
		"title":      "试炼弧",
		"summary":    "主角完成试炼。",
		"key_events": []string{"完成试炼"},
		"character_snapshots": []map[string]any{
			{"name": "林砚", "status": "受伤", "motivation": "进入内门"},
		},
		"style_rules": map[string]any{
			"prose":    []string{"动作句短促"},
			"dialogue": []map[string]any{},
			"taboos":   []string{"章末升华"},
		},
		"style_card": map[string]any{
			"sentence_std_floor":        3,
			"dialogue_ratio_target":     "0.15-0.35",
			"paragraph_variance_target": "短中长交错",
			"sensory_preferences":       []string{"触觉", "嗅觉"},
			"banned_patterns":           []string{"这一刻", "仿佛一切"},
			"dialogue_dna": []map[string]any{
				{"name": "林砚", "traits": []string{"短句", "不主动解释动机"}},
			},
			"chapter_ending_policy": "落到动作或选择，不写总结金句",
			"chapter_type_profiles": []map[string]any{
				{"type": "打斗", "sentence_std_range": "4-8", "dialogue_ratio_target": "0.05-0.2", "paragraph_variance_target": "短段密集", "notes": "高潮处允许碎句"},
			},
		},
	})
	raw, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if out["style_card_saved"] != true {
		t.Fatalf("expected style_card_saved=true, got %v", out)
	}
	rules, err := s.World.LoadStyleRules()
	if err != nil {
		t.Fatalf("LoadStyleRules: %v", err)
	}
	if rules == nil || rules.StyleCard == nil {
		t.Fatalf("style card not persisted: %+v", rules)
	}
	if rules.StyleCard.ChapterEndingPolicy != "落到动作或选择，不写总结金句" {
		t.Fatalf("unexpected style card: %+v", rules.StyleCard)
	}
	if rules.Volume != 1 || rules.Arc != 2 || len(rules.Prose) != 1 || len(rules.StyleCard.BannedPatterns) != 2 {
		t.Fatalf("style rules/card mismatch: %+v", rules)
	}
	if rules.StyleCard.ParagraphVarianceTarget != "短中长交错" || len(rules.StyleCard.SensoryPreferences) != 2 || len(rules.StyleCard.DialogueDNA) != 1 || len(rules.StyleCard.ChapterTypeProfiles) != 1 {
		t.Fatalf("full style card fields missing: %+v", rules.StyleCard)
	}
}

func TestSaveArcSummaryStyleCardOnlyPreservesExistingStyleRules(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.World.SaveStyleRules(domain.WritingStyleRules{
		Volume:   1,
		Arc:      1,
		Prose:    []string{"叙述保持克制"},
		Dialogue: []domain.CharacterVoice{{Name: "林砚", Rules: []string{"短句"}}},
		Taboos:   []string{"章末升华"},
		StyleCard: &domain.StyleCard{
			DialogueRatioTarget: "0.10-0.20",
		},
		UpdatedAt: "old",
	}); err != nil {
		t.Fatalf("SaveStyleRules: %v", err)
	}

	tool := NewSaveArcSummaryTool(s)
	args, _ := json.Marshal(map[string]any{
		"volume":     1,
		"arc":        2,
		"title":      "试炼弧",
		"summary":    "主角完成试炼。",
		"key_events": []string{"完成试炼"},
		"style_card": map[string]any{
			"dialogue_ratio_target":     "0.15-0.35",
			"paragraph_variance_target": "短中长交错",
		},
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	rules, err := s.World.LoadStyleRules()
	if err != nil {
		t.Fatalf("LoadStyleRules: %v", err)
	}
	if rules == nil || len(rules.Prose) != 1 || rules.Prose[0] != "叙述保持克制" {
		t.Fatalf("prose rules not preserved: %+v", rules)
	}
	if len(rules.Dialogue) != 1 || rules.Dialogue[0].Name != "林砚" || len(rules.Taboos) != 1 || rules.Taboos[0] != "章末升华" {
		t.Fatalf("dialogue/taboos not preserved: %+v", rules)
	}
	if rules.Volume != 1 || rules.Arc != 2 || rules.StyleCard == nil || rules.StyleCard.DialogueRatioTarget != "0.15-0.35" {
		t.Fatalf("style card update mismatch: %+v", rules)
	}
}
