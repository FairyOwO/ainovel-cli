package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestContextToolTrimsBenchmarkSummariesWhenOverBudget(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Benchmark.Save(domain.Benchmark{
		Version:   domain.BenchmarkProfileVersion,
		Name:      "oversized",
		Summary:   longBenchmarkText(140000),
		Structure: []string{longBenchmarkText(140000)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "Start", CoreEvent: "Begin"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("test", 1); err != nil {
		t.Fatal(err)
	}

	tool := NewContextTool(st, References{}, "default", rules.LoadOptions{})
	raw, err := tool.Execute(context.Background(), json.RawMessage(`{"chapter":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["benchmark_summaries"]; exists {
		t.Fatal("top-level benchmark_summaries marker should be trimmed")
	}
	working, ok := payload["working_memory"].(map[string]any)
	if !ok {
		t.Fatal("expected working_memory")
	}
	if _, exists := working["benchmark_summaries"]; exists {
		t.Fatal("working_memory benchmark_summaries should be trimmed")
	}
	assertTrimmedKey(t, payload, "benchmark_summaries")
}

func TestContextToolWarnsAndKeepsValidBenchmarkSummaries(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Benchmark.Save(domain.Benchmark{
		Version:   domain.BenchmarkProfileVersion,
		Name:      "valid",
		Structure: []string{"opening", "turn", "payoff"},
	}); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, "meta", "benchmarks", "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"version":"bad","name":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("test", 1); err != nil {
		t.Fatal(err)
	}

	tool := NewContextTool(st, References{}, "default", rules.LoadOptions{})
	raw, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	assertCompactBenchmarkSummaries(t, payload, "planning_memory")
	warnings, ok := payload["_warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected benchmark warning, got %#v", payload["_warnings"])
	}
	if !strings.Contains(warnings[0].(string), "benchmark_summaries") {
		t.Fatalf("warnings = %#v, want benchmark_summaries warning", warnings)
	}
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
	if _, exists := item["source"]; exists {
		t.Fatal("compact benchmark summary must not include local source path")
	}
	if got := len(item["structure"].([]any)); got != 3 {
		t.Fatalf("structure len = %d, want 3", got)
	}
}

func assertTrimmedKey(t *testing.T, payload map[string]any, key string) {
	t.Helper()
	trimmed, ok := payload["_trimmed"].([]any)
	if !ok {
		t.Fatalf("expected _trimmed to contain %q", key)
	}
	for _, item := range trimmed {
		if item == key {
			return
		}
	}
	t.Fatalf("_trimmed = %#v, want %q", trimmed, key)
}

func longBenchmarkText(n int) string {
	text := make([]byte, n)
	for i := range text {
		text[i] = 'x'
	}
	return string(text)
}
