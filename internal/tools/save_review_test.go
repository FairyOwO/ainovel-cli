package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func passingReviewDimensions() []map[string]any {
	return []map[string]any{
		{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "基本一致"},
		{"dimension": "character", "score": 82, "verdict": "pass", "comment": "人设稳定"},
		{"dimension": "pacing", "score": 81, "verdict": "pass", "comment": "节奏稳定"},
		{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "连贯"},
		{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "正常"},
		{"dimension": "hook", "score": 83, "verdict": "pass", "comment": "钩子明确"},
		{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "语言基本成立"},
	}
}

func TestSaveReviewPersistsContractAssessment(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":           3,
		"scope":             "chapter",
		"dimensions":        []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "基本一致"}, {"dimension": "character", "score": 82, "verdict": "pass", "comment": "人设稳定"}, {"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "略慢"}, {"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "连贯"}, {"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "正常"}, {"dimension": "hook", "score": 76, "verdict": "warning", "comment": "钩子一般"}, {"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "语言基本成立"}},
		"issues":            []map[string]any{},
		"contract_status":   "partial",
		"contract_misses":   []string{"未明确埋下内门试炼邀请"},
		"contract_notes":    "主线推进达成，但 contract 中的第二个推进项没有落地。",
		"verdict":           "polish",
		"summary":           "本章基本完成目标，但 contract 仍有漏项。",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	review, err := s.World.LoadReview(3)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review == nil {
		t.Fatal("expected review saved, got nil")
	}
	if review.ContractStatus != "partial" {
		t.Fatalf("unexpected contract status: %q", review.ContractStatus)
	}
	if len(review.ContractMisses) != 1 || review.ContractMisses[0] != "未明确埋下内门试炼邀请" {
		t.Fatalf("unexpected contract misses: %+v", review.ContractMisses)
	}
	if review.Dimension("aesthetic") == nil {
		t.Fatalf("expected aesthetic dimension persisted, got %+v", review.Dimensions)
	}
}

func TestSaveReviewRejectsMissingDimensions(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": []map[string]any{{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "基本一致"}},
		"issues":     []map[string]any{},
		"verdict":    "accept",
		"summary":    "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "dimensions must contain exactly") {
		t.Fatalf("expected dimensions validation error, got %v", err)
	}
}

func TestSaveReviewRejectsInconsistentScoreVerdict(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 55, "verdict": "pass", "comment": "不一致"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "稳定"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "略慢"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "连贯"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "正常"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "钩子一般"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "语言基本成立"},
		},
		"issues":  []map[string]any{},
		"verdict": "accept",
		"summary": "ok",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "inconsistent score/verdict") {
		t.Fatalf("expected score/verdict validation error, got %v", err)
	}
}

func TestSaveReviewRejectsMissingAffectedChaptersForRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "基本一致"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "人设稳定"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "略慢"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "连贯"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "正常"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "钩子一般"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "语言基本成立"},
		},
		"issues":  []map[string]any{},
		"verdict": "rewrite",
		"summary": "需要重写",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "affected_chapters is required") {
		t.Fatalf("expected affected_chapters validation error, got %v", err)
	}
}

func TestSaveReviewRejectsIssueWithoutEvidence(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter": 3,
		"scope":   "chapter",
		"dimensions": []map[string]any{
			{"dimension": "consistency", "score": 85, "verdict": "pass", "comment": "基本一致"},
			{"dimension": "character", "score": 82, "verdict": "pass", "comment": "人设稳定"},
			{"dimension": "pacing", "score": 78, "verdict": "warning", "comment": "略慢"},
			{"dimension": "continuity", "score": 84, "verdict": "pass", "comment": "连贯"},
			{"dimension": "foreshadow", "score": 80, "verdict": "pass", "comment": "正常"},
			{"dimension": "hook", "score": 76, "verdict": "warning", "comment": "钩子一般"},
			{"dimension": "aesthetic", "score": 81, "verdict": "pass", "comment": "语言基本成立"},
		},
		"issues": []map[string]any{
			{"type": "hook", "severity": "warning", "description": "章末钩子偏弱"},
		},
		"verdict":           "polish",
		"summary":           "需要补强钩子。",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "issue evidence is required") {
		t.Fatalf("expected issue evidence validation error, got %v", err)
	}
}

func TestSaveReviewRiskLevelS1UpgradesAcceptToRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": passingReviewDimensions(),
		"issues": []map[string]any{
			{"type": "character", "severity": "warning", "risk_level": "S1", "description": "主角动机崩塌", "evidence": "原文突然放弃核心目标"},
		},
		"verdict": "accept",
		"summary": "风险分级要求重写。",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload struct {
		FinalVerdict string `json:"final_verdict"`
		NextFlow     string `json:"next_flow"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.FinalVerdict != "rewrite" || payload.NextFlow != "rewriting" {
		t.Fatalf("expected S1 to force rewrite flow, got final=%q flow=%q", payload.FinalVerdict, payload.NextFlow)
	}
	review, err := s.World.LoadReview(3)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review.Issues[0].RiskLevel != "S1" {
		t.Fatalf("expected risk_level persisted, got %+v", review.Issues[0])
	}
}

func TestSaveReviewRiskLevelS2UpgradesAcceptToPolish(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": passingReviewDimensions(),
		"issues": []map[string]any{
			{"type": "pacing", "severity": "warning", "risk_level": "S2", "description": "爽点释放过迟", "evidence": "高潮直到章末才出现"},
		},
		"verdict": "accept",
		"summary": "风险分级要求打磨。",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload struct {
		FinalVerdict string `json:"final_verdict"`
		NextFlow     string `json:"next_flow"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.FinalVerdict != "polish" || payload.NextFlow != "polishing" {
		t.Fatalf("expected S2 to force polish flow, got final=%q flow=%q", payload.FinalVerdict, payload.NextFlow)
	}
}

func TestSaveReviewRiskLevelS2DoesNotDowngradeRewrite(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": passingReviewDimensions(),
		"issues": []map[string]any{
			{"type": "pacing", "severity": "error", "risk_level": "S2", "description": "节奏破坏阅读期待", "evidence": "关键冲突被整章跳过"},
		},
		"verdict":           "rewrite",
		"summary":           "明确要求重写。",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload struct {
		FinalVerdict     string `json:"final_verdict"`
		EscalationReason string `json:"escalation_reason"`
		NextFlow         string `json:"next_flow"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.FinalVerdict != "rewrite" || payload.NextFlow != "rewriting" {
		t.Fatalf("expected S2 not to downgrade rewrite, got final=%q flow=%q", payload.FinalVerdict, payload.NextFlow)
	}
	if strings.Contains(payload.EscalationReason, "polish") {
		t.Fatalf("rewrite result should not report polish escalation reason: %q", payload.EscalationReason)
	}
}

func TestSaveReviewRejectsInvalidRiskLevel(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": passingReviewDimensions(),
		"issues": []map[string]any{
			{"type": "hook", "severity": "warning", "risk_level": "S5", "description": "非法风险分级", "evidence": "测试输入"},
		},
		"verdict": "accept",
		"summary": "非法风险分级应被拒绝。",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if _, err := tool.Execute(context.Background(), args); err == nil || !strings.Contains(err.Error(), "invalid risk_level") {
		t.Fatalf("expected invalid risk_level validation error, got %v", err)
	}
}

func TestSaveReviewRiskLevelS3S4DoNotForceRewrite(t *testing.T) {
	for _, riskLevel := range []string{"S3", "S4"} {
		t.Run(riskLevel, func(t *testing.T) {
			s := store.NewStore(t.TempDir())
			if err := s.Init(); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if err := s.Progress.Init("test", 6); err != nil {
				t.Fatalf("InitProgress: %v", err)
			}

			tool := NewSaveReviewTool(s)
			args, err := json.Marshal(map[string]any{
				"chapter":    3,
				"scope":      "chapter",
				"dimensions": passingReviewDimensions(),
				"issues": []map[string]any{
					{"type": "hook", "severity": "warning", "risk_level": riskLevel, "description": "局部建议", "evidence": "章末句子略平"},
				},
				"verdict": "accept",
				"summary": "低风险建议不应触发重写。",
			})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			result, err := tool.Execute(context.Background(), args)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			var payload struct {
				FinalVerdict string `json:"final_verdict"`
				NextFlow     string `json:"next_flow"`
			}
			if err := json.Unmarshal(result, &payload); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if payload.FinalVerdict == "rewrite" || payload.NextFlow == "rewriting" {
				t.Fatalf("expected %s not to force rewrite, got final=%q flow=%q", riskLevel, payload.FinalVerdict, payload.NextFlow)
			}
		})
	}
}

func TestReviewIssueRiskLevelIsOptionalForOldJSON(t *testing.T) {
	var review domain.ReviewEntry
	data := []byte(`{"chapter":3,"scope":"chapter","issues":[{"type":"hook","severity":"warning","description":"钩子弱","evidence":"章末缺悬念"}],"verdict":"polish","summary":"旧格式"}`)
	if err := json.Unmarshal(data, &review); err != nil {
		t.Fatalf("Unmarshal old review JSON: %v", err)
	}
	if review.Issues[0].RiskLevel != "" {
		t.Fatalf("expected missing risk_level to load as empty, got %q", review.Issues[0].RiskLevel)
	}
	if len(review.Issues[0].Targets) != 0 {
		t.Fatalf("expected missing targets to load empty, got %+v", review.Issues[0].Targets)
	}
}

func TestSaveReviewPersistsIssueTargetsWithoutChangingRouting(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 6); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}

	tool := NewSaveReviewTool(s)
	args, err := json.Marshal(map[string]any{
		"chapter":    3,
		"scope":      "chapter",
		"dimensions": passingReviewDimensions(),
		"issues": []map[string]any{
			{
				"type":        "aesthetic",
				"severity":    "error",
				"risk_level":  "S2",
				"description": "章末总结腔明显",
				"evidence":    "这一刻，仿佛一切都有了答案。",
				"suggestion":  "删掉总结句，改为角色动作收束。",
				"targets": []map[string]any{
					{
						"hotspot_id":      "hs_004",
						"rule_id":         "cliche_summary_in_the_end",
						"paragraph_index": 18,
						"sentence_index":  63,
						"old_text":        "这一刻，仿佛一切都有了答案。",
						"suggestion_type": "remove_summary",
					},
				},
			},
		},
		"verdict":           "polish",
		"summary":           "只需局部打磨章末总结腔。",
		"affected_chapters": []int{3},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out struct {
		FinalVerdict     string `json:"final_verdict"`
		AffectedChapters []int  `json:"affected_chapters"`
		NextFlow         string `json:"next_flow"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if out.FinalVerdict != "polish" || out.NextFlow != "polishing" || len(out.AffectedChapters) != 1 || out.AffectedChapters[0] != 3 {
		t.Fatalf("targets must not change routing, got %+v", out)
	}
	review, err := s.World.LoadReview(3)
	if err != nil {
		t.Fatalf("LoadReview: %v", err)
	}
	if review == nil || len(review.Issues) != 1 || len(review.Issues[0].Targets) != 1 {
		t.Fatalf("expected persisted issue target, got %+v", review)
	}
	target := review.Issues[0].Targets[0]
	if target.RuleID != "cliche_summary_in_the_end" || target.OldText == "" || target.SuggestionType != "remove_summary" {
		t.Fatalf("unexpected target: %+v", target)
	}
}
