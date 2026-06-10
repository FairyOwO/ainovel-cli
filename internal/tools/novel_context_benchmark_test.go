package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestContextToolInjectsCompactBenchmarkSummaries(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	benchmark := domain.Benchmark{
		Version:            domain.BenchmarkProfileVersion,
		Name:               "demo-benchmark",
		Title:              "Demo",
		Source:             "manual",
		Summary:            "compact summary only",
		Structure:          []string{"opening", "escalation", "turn"},
		Pacing:             []string{"fast"},
		Hooks:              []string{"chapter hook"},
		CharacterPatterns:  []string{"protagonist"},
		SettingPatterns:    []string{"urban"},
		ReusableTechniques: []string{"technique-a"},
		AuthorizedAnchors:  []string{"anchor-a"},
		DoNotCopy:          []string{"do not copy"},
	}
	if err := st.Benchmark.Save(benchmark); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "Start", CoreEvent: "Begin"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("test", 1); err != nil {
		t.Fatal(err)
	}

	tool := NewContextTool(st, References{}, "default", rules.LoadOptions{})
	architectRaw, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("architect Execute: %v", err)
	}
	var architect map[string]any
	if err := json.Unmarshal(architectRaw, &architect); err != nil {
		t.Fatal(err)
	}
	assertCompactBenchmarkSummaries(t, architect, "planning_memory")

	chapterRaw, err := tool.Execute(context.Background(), json.RawMessage(`{"chapter":1}`))
	if err != nil {
		t.Fatalf("chapter Execute: %v", err)
	}
	var chapter map[string]any
	if err := json.Unmarshal(chapterRaw, &chapter); err != nil {
		t.Fatal(err)
	}
	assertCompactBenchmarkSummaries(t, chapter, "working_memory")
}

func assertCompactBenchmarkSummaries(t *testing.T, payload map[string]any, section string) {
	t.Helper()
	if got := payload["benchmark_summaries"]; got != true {
		t.Fatalf("expected top-level benchmark_summaries marker, got %#v", got)
	}
	sectionMap, ok := payload[section].(map[string]any)
	if !ok {
		t.Fatalf("expected %s", section)
	}
	summaries, ok := sectionMap["benchmark_summaries"].([]any)
	if !ok {
		t.Fatalf("expected benchmark_summaries under %s", section)
	}
	if len(summaries) != 1 {
		t.Fatalf("benchmark_summaries len = %d, want 1", len(summaries))
	}
	item, ok := summaries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected compact benchmark item, got %#v", summaries[0])
	}
	if _, exists := item["raw_text"]; exists {
		t.Fatal("compact benchmark summary must not include raw_text")
	}
	if got := len(item["structure"].([]any)); got != 3 {
		t.Fatalf("structure len = %d, want 3", got)
	}
}
