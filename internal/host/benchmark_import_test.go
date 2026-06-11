package host

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/host/bench"
)

func TestMaintenanceBusyBlocksLifecycleEntries(t *testing.T) {
	h := &Host{lifecycle: lifecycleIdle, maintenanceBusy: true}

	if err := h.StartPrepared("start"); err == nil {
		t.Fatal("StartPrepared should fail while maintenance task is busy")
	}
	if _, err := h.Resume(); err == nil {
		t.Fatal("Resume should fail while maintenance task is busy")
	}
	if err := h.Continue("continue"); err == nil || !strings.Contains(err.Error(), "后台任务") {
		t.Fatalf("Continue error = %v, want maintenance busy error", err)
	}
	if _, err := h.ImportBenchmarkMarkdown(context.Background(), bench.Options{SourceDir: "x"}); err == nil {
		t.Fatal("ImportBenchmarkMarkdown should fail while maintenance task is busy")
	}
}
