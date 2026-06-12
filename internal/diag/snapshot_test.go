package diag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestLoadSnapshotLoadsStyleStatsForCompletedChapters(t *testing.T) {
	s := store.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, 100, "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	if err := s.World.SaveStyleStats(domain.StyleStats{SchemaVersion: domain.StyleStatsSchemaVersion, Chapter: 1, Summary: "统计摘要"}); err != nil {
		t.Fatalf("SaveStyleStats: %v", err)
	}
	if err := s.World.AppendStyleRewriteComparison(domain.StyleRewriteComparison{SchemaVersion: domain.StyleRewriteComparisonSchemaVersion, Chapter: 1, Mode: "polish"}); err != nil {
		t.Fatalf("AppendStyleRewriteComparison: %v", err)
	}

	snap := Load(s)
	if len(snap.LoadErrors) != 0 {
		t.Fatalf("LoadErrors: %v", snap.LoadErrors)
	}
	if snap.StyleStats[1] == nil || snap.StyleStats[1].Summary != "统计摘要" {
		t.Fatalf("style stats not loaded: %+v", snap.StyleStats)
	}
	if len(snap.StyleRewriteComparisons) != 1 || snap.StyleRewriteComparisons[0].Mode != "polish" {
		t.Fatalf("style rewrite comparisons not loaded: %+v", snap.StyleRewriteComparisons)
	}
}

func TestLoadSnapshotRecordsCorruptStyleStats(t *testing.T) {
	dir := t.TempDir()
	s := store.NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Progress.Init("test", 3); err != nil {
		t.Fatalf("InitProgress: %v", err)
	}
	if err := s.Progress.MarkChapterComplete(1, 100, "mystery", "quest"); err != nil {
		t.Fatalf("MarkChapterComplete: %v", err)
	}
	path := filepath.Join(dir, "meta", "stats", "chapter_1.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	snap := Load(s)
	if len(snap.LoadErrors) == 0 || !strings.Contains(snap.LoadErrors[0], "style_stats_ch1") {
		t.Fatalf("expected style_stats load error, got %v", snap.LoadErrors)
	}
}
