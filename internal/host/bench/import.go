package bench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

const (
	maxImportedFieldItems = 48
	maxImportedLineRunes  = 160
)

var defaultDoNotCopy = []string{
	"不复制原文句子、角色名、专有设定或桥段，只借鉴结构、节奏、钩子和技法。",
}

func ImportMarkdown(ctx context.Context, st *store.Store, opts Options) (Result, error) {
	if st == nil {
		return Result{}, fmt.Errorf("store is nil")
	}
	sourceDir := strings.TrimSpace(opts.SourceDir)
	if sourceDir == "" {
		return Result{}, fmt.Errorf("benchmark source dir is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	absDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("benchmark source must be a directory: %s", sourceDir)
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = sanitizeBenchmarkName(filepath.Base(absDir))
	}
	if err := domain.ValidateBenchmarkName(name); err != nil {
		return Result{}, err
	}

	files, err := markdownFiles(absDir)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fmt.Errorf("no Markdown files found in %s", sourceDir)
	}

	now := time.Now().Format(time.RFC3339)
	builder := benchmarkImportBuilder{benchmark: domain.Benchmark{
		BenchmarkCompact: domain.BenchmarkCompact{
			Version:   domain.BenchmarkProfileVersion,
			Name:      name,
			UpdatedAt: now,
		},
		CreatedAt: now,
		Source:    absDir,
	}}

	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		builder.addDocument(filepath.ToSlash(rel), string(data))
	}

	benchmark := builder.finalize(filepath.Base(absDir))
	if err := st.Benchmark.Save(benchmark); err != nil {
		return Result{}, err
	}
	return Result{
		Name:      benchmark.Name,
		Title:     benchmark.Title,
		SourceDir: absDir,
		Files:     len(files),
		Path:      filepath.ToSlash(filepath.Join("meta", "benchmarks", benchmark.Name+".json")),
	}, nil
}

func RunImport(ctx context.Context, st *store.Store, opts Options) (<-chan Event, error) {
	if st == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if strings.TrimSpace(opts.SourceDir) == "" {
		return nil, fmt.Errorf("benchmark source dir is required")
	}
	events := make(chan Event, 8)
	go func() {
		defer close(events)
		emit := func(stage Stage, msg string, err error) {
			ev := Event{Time: time.Now(), Stage: stage, Message: msg, Err: err}
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}
		emit(StageScan, "扫描 Markdown 拆文目录...", nil)
		result, err := ImportMarkdown(ctx, st, opts)
		if err != nil {
			emit(StageError, "导入对标拆文失败", err)
			return
		}
		emit(StageDone, fmt.Sprintf("对标拆文已导入：%s（%d 个 Markdown）", result.Name, result.Files), nil)
	}()
	return events, nil
}

type benchmarkImportBuilder struct {
	benchmark domain.Benchmark
}

func (b *benchmarkImportBuilder) addDocument(rel, text string) {
	sections := parseMarkdownSections(text)
	if b.benchmark.Title == "" {
		for _, section := range sections {
			if title := cleanMarkdownLine(section.heading); title != "" {
				b.benchmark.Title = trimTitleSuffix(title)
				break
			}
		}
	}
	for _, section := range sections {
		items := markdownItems(section.lines)
		if len(items) == 0 {
			continue
		}
		switch sectionCategory(rel, section.heading) {
		case "summary":
			if b.benchmark.Summary == "" {
				b.benchmark.Summary = items[0]
			}
		case "structure":
			b.benchmark.Structure = appendUniqueLimited(b.benchmark.Structure, items...)
		case "pacing":
			b.benchmark.Pacing = appendUniqueLimited(b.benchmark.Pacing, items...)
		case "hooks":
			b.benchmark.Hooks = appendUniqueLimited(b.benchmark.Hooks, items...)
		case "characters":
			b.benchmark.CharacterPatterns = appendUniqueLimited(b.benchmark.CharacterPatterns, items...)
		case "setting":
			b.benchmark.SettingPatterns = appendUniqueLimited(b.benchmark.SettingPatterns, items...)
		case "techniques":
			b.benchmark.ReusableTechniques = appendUniqueLimited(b.benchmark.ReusableTechniques, items...)
		case "anchors":
			b.benchmark.AuthorizedAnchors = appendUniqueLimited(b.benchmark.AuthorizedAnchors, items...)
		case "do_not_copy":
			b.benchmark.DoNotCopy = appendUniqueLimited(b.benchmark.DoNotCopy, items...)
		}
	}
}

func (b *benchmarkImportBuilder) finalize(fallbackTitle string) domain.Benchmark {
	benchmark := b.benchmark
	if benchmark.Title == "" {
		benchmark.Title = fallbackTitle
	}
	if benchmark.Summary == "" {
		benchmark.Summary = fmt.Sprintf("从 %s 导入的对标拆文摘要。", benchmark.Title)
	}
	benchmark.DoNotCopy = appendUniqueLimited(benchmark.DoNotCopy, defaultDoNotCopy...)
	return benchmark
}

type markdownSection struct {
	heading string
	lines   []string
}

func parseMarkdownSections(text string) []markdownSection {
	var sections []markdownSection
	current := markdownSection{}
	inCode := false
	flush := func() {
		if current.heading != "" || len(current.lines) > 0 {
			sections = append(sections, current)
		}
		current = markdownSection{}
	}
	for raw := range strings.SplitSeq(text, "\n") {
		line := strings.TrimSpace(raw)
		if isMarkdownFence(raw) {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if heading, ok := markdownHeading(line); ok {
			flush()
			current.heading = heading
			continue
		}
		if line != "" {
			current.lines = append(current.lines, line)
		}
	}
	flush()
	return sections
}

func isMarkdownFence(line string) bool {
	line = strings.TrimRight(line, " \t\r")
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	line = line[indent:]
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func markdownHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	level := 0
	for _, r := range line {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 || level > 6 || len(line) <= level || !unicode.IsSpace(rune(line[level])) {
		return "", false
	}
	return cleanMarkdownLine(line[level:]), true
}

func markdownItems(lines []string) []string {
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		item := cleanMarkdownLine(line)
		if item == "" {
			continue
		}
		items = appendUniqueLimited(items, item)
	}
	return items
}

func cleanMarkdownLine(line string) string {
	line = strings.TrimSpace(line)
	for strings.HasPrefix(line, ">") {
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
	}
	line = trimListMarker(line)
	line = strings.Trim(line, " \t*-_`~")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "`", "")
	line = strings.Join(strings.Fields(line), " ")
	return truncateRunes(line, maxImportedLineRunes)
}

func trimListMarker(line string) string {
	line = strings.TrimSpace(line)
	if len(line) >= 2 && strings.ContainsRune("-*+", rune(line[0])) && unicode.IsSpace(rune(line[1])) {
		return strings.TrimSpace(line[2:])
	}
	runes := []rune(line)
	i := 0
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}
	if i > 0 && i+1 < len(runes) && (runes[i] == '.' || runes[i] == ')') && unicode.IsSpace(runes[i+1]) {
		return strings.TrimSpace(string(runes[i+2:]))
	}
	return line
}

func sectionCategory(rel, heading string) string {
	if cat := categoryFromText(heading); cat != "" {
		return cat
	}
	return categoryFromText(rel)
}

func categoryFromText(text string) string {
	text = strings.ToLower(text)
	switch {
	case containsAny(text, "章节摘要", "chapter summary", "chapter summaries"):
		return "structure"
	case containsAny(text, "不要复制", "不可复制", "禁忌", "避雷", "do not copy", "dont copy"):
		return "do_not_copy"
	case containsAny(text, "锚点", "授权", "可借鉴", "anchor"):
		return "anchors"
	case containsAny(text, "文风", "风格", "技法", "方法", "可复用", "写法", "语言", "technique", "style"):
		return "techniques"
	case containsAny(text, "节奏", "爽点", "情绪", "推进", "节拍", "pacing", "beat"):
		return "pacing"
	case containsAny(text, "钩子", "悬念", "期待", "反转", "章尾", "开篇", "hook", "suspense"):
		return "hooks"
	case containsAny(text, "角色", "人物", "人设", "关系", "character"):
		return "characters"
	case containsAny(text, "设定", "世界", "背景", "规则", "setting", "world"):
		return "setting"
	case containsAny(text, "摘要", "概述", "总览", "核心", "报告", "summary", "overview"):
		return "summary"
	case containsAny(text, "结构", "剧情", "情节", "章节", "大纲", "主线", "拆文", "structure", "plot"):
		return "structure"
	default:
		return ""
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func markdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".md" || ext == ".markdown" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func appendUniqueLimited(dst []string, items ...string) []string {
	known := make(map[string]struct{}, len(dst)+len(items))
	for _, item := range dst {
		known[item] = struct{}{}
	}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := known[item]; ok {
			continue
		}
		if len(dst) >= maxImportedFieldItems {
			return dst
		}
		known[item] = struct{}{}
		dst = append(dst, item)
	}
	return dst
}

func sanitizeBenchmarkName(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '.' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	name := strings.Trim(b.String(), "-_")
	if name == "" {
		sum := sha256.Sum256([]byte(input))
		return "benchmark-" + hex.EncodeToString(sum[:4])
	}
	return name
}

func trimTitleSuffix(title string) string {
	for _, suffix := range []string{"拆文报告", "拆文", "报告"} {
		title = strings.TrimSpace(strings.TrimSuffix(title, suffix))
	}
	return strings.Trim(title, " 《》")
}

func truncateRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
