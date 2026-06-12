package diag

import (
	"regexp"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const maxDiagnosticGuidanceItems = 6

var diagnosticGuidanceRules = map[string]struct{}{
	"AIFlavorHotspots":       {},
	"RewriteEffectiveness":   {},
	"EmotionLabelDensity":    {},
	"SentenceStartDominance": {},
	"EditorConsistency":      {},
}

var quotedEvidenceRe = regexp.MustCompile(`"[^"]*"|“[^”]*”|『[^』]*』|「[^」]*」`)

// BuildDiagnosticGuidance converts selected /diag quality findings into compact agent-facing feedback.
func BuildDiagnosticGuidance(findings []Finding) domain.DiagnosticGuidance {
	guidance := domain.DiagnosticGuidance{
		SchemaVersion: domain.DiagnosticGuidanceSchemaVersion,
		GeneratedAt:   time.Now().Format(time.RFC3339),
	}
	seen := map[string]struct{}{}
	for _, finding := range findings {
		if len(guidance.Items) >= maxDiagnosticGuidanceItems {
			break
		}
		if finding.Category != CatQuality || finding.Suggestion == "" {
			continue
		}
		if _, ok := diagnosticGuidanceRules[finding.Rule]; !ok {
			continue
		}
		key := finding.Rule + "|" + finding.Target + "|" + finding.Title
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		guidance.Items = append(guidance.Items, domain.DiagnosticGuidanceItem{
			Rule:       finding.Rule,
			Severity:   string(finding.Severity),
			Target:     finding.Target,
			Title:      finding.Title,
			Signal:     compactGuidanceSignal(finding.Evidence),
			Suggestion: finding.Suggestion,
		})
	}
	return guidance
}

func compactGuidanceSignal(evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return ""
	}
	evidence = quotedEvidenceRe.ReplaceAllString(evidence, "<evidence>")
	evidence = strings.ReplaceAll(evidence, "\n", " ")
	fields := strings.Fields(evidence)
	evidence = strings.Join(fields, " ")
	runes := []rune(evidence)
	if len(runes) > 160 {
		return string(runes[:160]) + "..."
	}
	return evidence
}
