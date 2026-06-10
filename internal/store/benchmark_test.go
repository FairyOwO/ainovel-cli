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
		Version:            domain.BenchmarkProfileVersion,
		Name:               "demo-benchmark",
		Title:              "Demo",
		Summary:            "summary",
		Structure:          []string{"setup", "turn"},
		ReusableTechniques: []string{"technique-a"},
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
	if err := st.Benchmark.Save(domain.Benchmark{Version: domain.BenchmarkProfileVersion, Name: "../bad"}); err == nil {
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
		if err := st.Benchmark.Save(domain.Benchmark{Version: domain.BenchmarkProfileVersion, Name: name}); err != nil {
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
