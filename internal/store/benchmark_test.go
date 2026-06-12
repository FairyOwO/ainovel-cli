package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestBenchmarkStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}

	benchmark := domain.Benchmark{
		BenchmarkCompact: domain.BenchmarkCompact{
			Version:            domain.BenchmarkProfileVersion,
			Name:               "demo-benchmark",
			Title:              "Demo",
			Summary:            "summary",
			Structure:          []string{"setup", "turn"},
			ReusableTechniques: []string{"technique-a"},
		},
	}
	if err := st.Benchmark.Save(benchmark); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.Benchmark.Load("demo-benchmark")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Name != benchmark.Name || loaded.Title != benchmark.Title {
		t.Fatalf("loaded benchmark mismatch: %#v", loaded)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta", "benchmarks", "demo-benchmark.json")); err != nil {
		t.Fatal(err)
	}
	missing, err := st.Benchmark.Load("missing")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatalf("expected missing benchmark to load nil, got %#v", missing)
	}
}

func TestBenchmarkStoreRejectsTraversalName(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Benchmark.Save(newTestBenchmark("../bad")); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBenchmarkStoreLoadAllStableOrder(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zeta", "alpha"} {
		if err := st.Benchmark.Save(newTestBenchmark(name)); err != nil {
			t.Fatal(err)
		}
	}

	benchmarks, err := st.Benchmark.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(benchmarks) != 2 {
		t.Fatalf("LoadAll len = %d, want 2", len(benchmarks))
	}
	if benchmarks[0].Name != "alpha" || benchmarks[1].Name != "zeta" {
		t.Fatalf("LoadAll order = %q, %q", benchmarks[0].Name, benchmarks[1].Name)
	}
}

func TestBenchmarkStoreLoadAllReturnsMalformedWarnings(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Benchmark.Save(newTestBenchmark("valid")); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, "meta", "benchmarks", "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"version":"bad","name":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	benchmarks, err := st.Benchmark.LoadAll()
	if err == nil {
		t.Fatal("expected malformed benchmark warning")
	}
	if len(benchmarks) != 1 || benchmarks[0].Name != "valid" {
		t.Fatalf("LoadAll benchmarks = %#v, want only valid benchmark", benchmarks)
	}
}

func TestBenchmarkStoreLoadSummariesSortsByUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	benchmarks := []domain.Benchmark{
		newTestBenchmarkWithUpdatedAt("alpha", "2026-01-01T00:00:00Z"),
		newTestBenchmarkWithUpdatedAt("zeta", "2026-03-01T00:00:00Z"),
		newTestBenchmarkWithUpdatedAt("middle", "2026-02-01T00:00:00Z"),
	}
	for _, benchmark := range benchmarks {
		if err := st.Benchmark.Save(benchmark); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := st.Benchmark.LoadSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 {
		t.Fatalf("LoadSummaries len = %d, want 3", len(summaries))
	}
	if summaries[0].Name != "zeta" || summaries[1].Name != "middle" || summaries[2].Name != "alpha" {
		t.Fatalf("LoadSummaries order = %q, %q, %q", summaries[0].Name, summaries[1].Name, summaries[2].Name)
	}
}

func newTestBenchmark(name string) domain.Benchmark {
	return newTestBenchmarkWithUpdatedAt(name, "")
}

func newTestBenchmarkWithUpdatedAt(name, updatedAt string) domain.Benchmark {
	return domain.Benchmark{BenchmarkCompact: domain.BenchmarkCompact{
		Version:   domain.BenchmarkProfileVersion,
		Name:      name,
		UpdatedAt: updatedAt,
	}}
}
