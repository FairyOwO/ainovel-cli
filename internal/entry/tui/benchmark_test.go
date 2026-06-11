package tui

import "testing"

func TestBenchmarkImportCommandIsRegisteredAndNeedsIdle(t *testing.T) {
	registry := commandRegistryInstance()
	spec, ok := registry.Find("importbench")
	if !ok {
		t.Fatal("expected /importbench command to be registered")
	}
	if !spec.NeedsIdle {
		t.Fatal("/importbench should require idle state")
	}
	if _, ok := registry.Find("benchmark"); !ok {
		t.Fatal("expected /benchmark alias to be registered")
	}
	if !hasPaletteItem(builtinCommandItems(), "importbench") {
		t.Fatal("expected importbench command in palette")
	}
}

func TestParseBenchmarkImportArgs(t *testing.T) {
	opts, err := parseBenchmarkImportArgs([]string{"./拆文库/示例书", "name=demo-book"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.SourceDir != "./拆文库/示例书" || opts.Name != "demo-book" {
		t.Fatalf("unexpected opts: %+v", opts)
	}

	if _, err := parseBenchmarkImportArgs(nil); err == nil {
		t.Fatal("expected usage error")
	}
	if _, err := parseBenchmarkImportArgs([]string{"./x", "bad"}); err == nil {
		t.Fatal("expected key=value error")
	}
}

func TestBenchmarkImportDoneIgnoresStaleRequest(t *testing.T) {
	m := Model{benchmarkImporter: &benchmarkImportState{reqID: 2}}
	next, _, handled := m.handleRuntimeMsg(benchmarkImportDoneMsg{reqID: 1})
	if !handled {
		t.Fatal("expected benchmarkImportDoneMsg handled")
	}
	got := next.(Model)
	if got.benchmarkImporter == nil || got.benchmarkImporter.done {
		t.Fatal("stale benchmark import result should not finish current state")
	}
}
