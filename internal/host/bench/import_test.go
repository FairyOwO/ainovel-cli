package bench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/store"
)

func TestImportMarkdownBuildsBenchmarkFromMarkdownDirectory(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "拆文库", "demo-book")
	if err := os.MkdirAll(filepath.Join(sourceDir, "章节摘要"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(sourceDir, "拆文报告.md"), `# 《示例书》拆文报告

## 核心摘要
- 开篇用误会制造主角必须马上行动的压力。

## 结构
- 三段式推进：危机暴露、临时结盟、反转收束。

## 节奏与爽点
- 每个场景结尾都留下一个未解释的信息差。

## 钩子
- 章尾用身份反转逼读者进入下一章。
`)
	writeTestFile(t, filepath.Join(sourceDir, "文风.md"), `# 文风

- 短句推进动作，长句承接心理余波。
- 借鉴动作密度，不复制原句。
`)
	writeTestFile(t, filepath.Join(sourceDir, "角色.md"), `# 角色关系

- 主角和导师先互相试探，再通过共同危机建立信任。
`)
	writeTestFile(t, filepath.Join(sourceDir, "设定.md"), `# 世界规则

- 公开规则和隐藏规则并行推进，反转来自隐藏规则揭露。
`)
	writeTestFile(t, filepath.Join(sourceDir, "章节摘要", "01.md"), `# 第一章

- 主角在公开场合被迫接住危机，引出核心矛盾。
`)

	st := store.NewStore(filepath.Join(dir, "output", "novel"))
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}

	result, err := ImportMarkdown(context.Background(), st, Options{SourceDir: sourceDir, Name: "demo-book"})
	if err != nil {
		t.Fatalf("ImportMarkdown: %v", err)
	}
	if result.Name != "demo-book" || result.Files != 5 {
		t.Fatalf("unexpected result: %+v", result)
	}

	benchmark, err := st.Benchmark.Load("demo-book")
	if err != nil {
		t.Fatal(err)
	}
	if benchmark == nil {
		t.Fatal("benchmark was not saved")
	}
	if benchmark.Title != "示例书" {
		t.Fatalf("title = %q, want 示例书", benchmark.Title)
	}
	assertContains(t, benchmark.Summary, "误会制造")
	assertContainsItem(t, benchmark.Structure, "三段式推进")
	assertContainsItem(t, benchmark.Structure, "核心矛盾")
	assertContainsItem(t, benchmark.Pacing, "信息差")
	assertContainsItem(t, benchmark.Hooks, "身份反转")
	assertContainsItem(t, benchmark.CharacterPatterns, "共同危机")
	assertContainsItem(t, benchmark.SettingPatterns, "隐藏规则")
	assertContainsItem(t, benchmark.ReusableTechniques, "短句推进")
	assertContainsItem(t, benchmark.DoNotCopy, "不复制原文")
}

func TestImportMarkdownRejectsMissingMarkdown(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(filepath.Join(dir, "output", "novel"))
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	sourceDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ImportMarkdown(context.Background(), st, Options{SourceDir: sourceDir})
	if err == nil || !strings.Contains(err.Error(), "no Markdown files") {
		t.Fatalf("expected missing markdown error, got %v", err)
	}
}

func TestImportMarkdownSkipsSymlinkedMarkdown(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "bench")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.md")
	writeTestFile(t, outside, `# Secret

## 核心摘要
- should not be imported
`)
	if err := os.Symlink(outside, filepath.Join(sourceDir, "linked.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeTestFile(t, filepath.Join(sourceDir, "real.md"), `# Real

## 核心摘要
- imported summary
`)

	st := store.NewStore(filepath.Join(dir, "output", "novel"))
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	result, err := ImportMarkdown(context.Background(), st, Options{SourceDir: sourceDir, Name: "safe"})
	if err != nil {
		t.Fatalf("ImportMarkdown: %v", err)
	}
	if result.Files != 1 {
		t.Fatalf("imported files = %d, want only real markdown file", result.Files)
	}
	benchmark, err := st.Benchmark.Load("safe")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, benchmark.Summary, "imported summary")
	if strings.Contains(benchmark.Summary, "should not be imported") {
		t.Fatal("symlinked markdown content was imported")
	}
}

func TestImportMarkdownClassifiesCoreHeadingsSpecifically(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "bench")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(sourceDir, "report.md"), `# Demo

## 核心摘要
- 误会推动主角进入危机。

## 核心角色
- 主角先隐藏真实目的，再被迫公开选择。

## 核心设定
- 城市规则由明暗两套契约共同约束。
`)

	st := store.NewStore(filepath.Join(dir, "output", "novel"))
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportMarkdown(context.Background(), st, Options{SourceDir: sourceDir, Name: "core-demo"}); err != nil {
		t.Fatalf("ImportMarkdown: %v", err)
	}
	benchmark, err := st.Benchmark.Load("core-demo")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, benchmark.Summary, "误会推动")
	assertContainsItem(t, benchmark.CharacterPatterns, "隐藏真实目的")
	assertContainsItem(t, benchmark.SettingPatterns, "明暗两套契约")
}

func TestImportMarkdownClassifiesChapterOverviewAsSummary(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "bench")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(sourceDir, "report.md"), `# Demo

## 章节概述
- 第一章用误会逼主角当场做选择。

## 结构
- 危机暴露后进入临时结盟。
`)

	st := store.NewStore(filepath.Join(dir, "output", "novel"))
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportMarkdown(context.Background(), st, Options{SourceDir: sourceDir, Name: "overview-demo"}); err != nil {
		t.Fatalf("ImportMarkdown: %v", err)
	}
	benchmark, err := st.Benchmark.Load("overview-demo")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, benchmark.Summary, "当场做选择")
	for _, item := range benchmark.Structure {
		if strings.Contains(item, "当场做选择") {
			t.Fatalf("chapter overview should be summary, got structure %#v", benchmark.Structure)
		}
	}
	assertContainsItem(t, benchmark.Structure, "临时结盟")
}

func TestParseMarkdownSectionsSkipsIndentedFences(t *testing.T) {
	sections := parseMarkdownSections("## 结构\n" +
		"- 开头危机\n\n" +
		"  ```\n" +
		"- 代码块里的列表不应导入\n" +
		"  ```\n\n" +
		"    ```\n" +
		"- 四空格缩进不是 fenced code block，应按正文保留\n" +
		"    ```\n")
	if len(sections) != 1 {
		t.Fatalf("sections len = %d, want 1", len(sections))
	}
	items := markdownItems(sections[0].lines)
	assertContainsItem(t, items, "开头危机")
	assertContainsItem(t, items, "四空格缩进不是 fenced code block")
	for _, item := range items {
		if strings.Contains(item, "代码块里的列表") {
			t.Fatalf("fenced code content should be skipped, got %#v", items)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertContainsItem(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if strings.Contains(item, want) {
			return
		}
	}
	t.Fatalf("expected one of %#v to contain %q", items, want)
}
