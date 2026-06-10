package domain

import (
	"strconv"
	"testing"
)

func TestCompactBenchmarkCapsArrays(t *testing.T) {
	benchmark := &Benchmark{
		Version:            BenchmarkProfileVersion,
		Name:               "demo-benchmark",
		Structure:          longBenchmarkList("structure", maxCompactBenchmarkItems+4),
		Pacing:             longBenchmarkList("pacing", maxCompactBenchmarkItems+4),
		Hooks:              longBenchmarkList("hook", maxCompactBenchmarkItems+4),
		CharacterPatterns:  longBenchmarkList("character", maxCompactBenchmarkItems+4),
		SettingPatterns:    longBenchmarkList("setting", maxCompactBenchmarkItems+4),
		ReusableTechniques: longBenchmarkList("technique", maxCompactBenchmarkItems+4),
		AuthorizedAnchors:  longBenchmarkList("anchor", maxCompactBenchmarkItems+4),
		DoNotCopy:          longBenchmarkList("copy", maxCompactBenchmarkItems+4),
	}

	compact := CompactBenchmark(benchmark)
	if compact == nil {
		t.Fatal("compact benchmark is nil")
	}
	assertBenchmarkCompactLen(t, "Structure", compact.Structure)
	assertBenchmarkCompactLen(t, "Pacing", compact.Pacing)
	assertBenchmarkCompactLen(t, "Hooks", compact.Hooks)
	assertBenchmarkCompactLen(t, "CharacterPatterns", compact.CharacterPatterns)
	assertBenchmarkCompactLen(t, "SettingPatterns", compact.SettingPatterns)
	assertBenchmarkCompactLen(t, "ReusableTechniques", compact.ReusableTechniques)
	assertBenchmarkCompactLen(t, "AuthorizedAnchors", compact.AuthorizedAnchors)
	assertBenchmarkCompactLen(t, "DoNotCopy", compact.DoNotCopy)
	if got := len(benchmark.Structure); got != maxCompactBenchmarkItems+4 {
		t.Fatalf("CompactBenchmark mutated source benchmark, len = %d", got)
	}
}

func TestValidateBenchmarkNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../bad", "bad/name", "", "bad name"} {
		if err := ValidateBenchmarkName(name); err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
}

func assertBenchmarkCompactLen(t *testing.T, name string, got []string) {
	t.Helper()
	if len(got) != maxCompactBenchmarkItems {
		t.Fatalf("%s len = %d, want %d", name, len(got), maxCompactBenchmarkItems)
	}
}

func longBenchmarkList(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = prefix + "-" + strconv.Itoa(i)
	}
	return out
}
