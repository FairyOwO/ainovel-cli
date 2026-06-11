package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/host/bench"
)

type benchmarkImportState struct {
	reqID      int
	source     string
	startedAt  time.Time
	finishedAt time.Time
	result     *bench.Result
	err        error
	done       bool
	cancel     context.CancelFunc
	viewport   viewport.Model
}

type benchmarkImportDoneMsg struct {
	reqID  int
	result *bench.Result
	err    error
}

func newBenchmarkImportState(reqID int, source string, width, height int, cancel context.CancelFunc) *benchmarkImportState {
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	state := &benchmarkImportState{
		reqID:     reqID,
		source:    source,
		startedAt: time.Now(),
		cancel:    cancel,
		viewport:  viewport.New(contentW, boxH-4),
	}
	state.refresh(contentW)
	return state
}

func (s *benchmarkImportState) finish(result *bench.Result, err error, finishedAt time.Time, contentW int) {
	s.result = result
	s.err = err
	s.done = true
	s.finishedAt = finishedAt
	s.refresh(contentW)
}

func (s *benchmarkImportState) refresh(contentW int) {
	titleStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	errStyle := lipgloss.NewStyle().Foreground(colorError)

	var b strings.Builder
	b.WriteString(titleStyle.Render("导入对标拆文"))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("来源 "))
	b.WriteString(s.source)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("开始 "))
	b.WriteString(formatReportTime(s.startedAt))
	if !s.finishedAt.IsZero() {
		b.WriteString(dimStyle.Render("  完成 "))
		b.WriteString(formatReportTime(s.finishedAt))
	}
	b.WriteString("\n\n")
	if !s.done {
		b.WriteString("正在扫描并导入 Markdown 拆文目录...")
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Esc 取消导入"))
	} else if s.err != nil {
		b.WriteString(errStyle.Render("导入失败"))
		b.WriteString("\n")
		b.WriteString(wrapText(s.err.Error(), contentW))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Esc 关闭面板"))
	} else {
		b.WriteString(okStyle.Render(formatBenchmarkImportSuccess(s.result)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("后续 Agent 会从 novel_context 读取 compact benchmark_summaries"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Esc 关闭面板"))
	}
	s.viewport.SetContent(b.String())
}

func renderBenchmarkImportModal(width, height int, state *benchmarkImportState) string {
	if state == nil {
		return ""
	}
	boxW, boxH := reportModalSize(width, height)
	contentW := paddedModalContentWidth(boxW)
	if state.viewport.Width != contentW {
		state.viewport.Width = contentW
		state.refresh(contentW)
	}
	if state.viewport.Height != boxH-4 {
		state.viewport.Height = boxH - 4
	}
	hint := "  ↑↓ 滚动 · Esc 取消/关闭"
	modal := renderPaddedModalFrame(boxW, boxH, "对标拆文导入", hint, strings.Split(state.viewport.View(), "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) handleBenchmarkImportKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.benchmarkImporter == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		if !m.benchmarkImporter.done && m.benchmarkImporter.cancel != nil {
			m.benchmarkImporter.cancel()
			return m, nil
		}
		m.benchmarkImporter = nil
		return m, m.textarea.Focus()
	case tea.KeyUp:
		m.benchmarkImporter.viewport.ScrollUp(1)
	case tea.KeyDown:
		m.benchmarkImporter.viewport.ScrollDown(1)
	case tea.KeyPgUp:
		m.benchmarkImporter.viewport.HalfPageUp()
	case tea.KeyPgDown:
		m.benchmarkImporter.viewport.HalfPageDown()
	}
	return m, nil
}

func startBenchmarkImport(rt *host.Host, reqID int, args []string, width, height int) (*benchmarkImportState, tea.Cmd, error) {
	opts, err := parseBenchmarkImportArgs(args)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := newBenchmarkImportState(reqID, opts.SourceDir, width, height, cancel)
	cmd := func() tea.Msg {
		res, err := rt.ImportBenchmarkMarkdown(ctx, opts)
		return benchmarkImportDoneMsg{reqID: reqID, result: res, err: err}
	}
	return state, cmd, nil
}

func parseBenchmarkImportArgs(args []string) (bench.Options, error) {
	if len(args) == 0 {
		return bench.Options{}, fmt.Errorf("用法：/importbench <拆文目录> [name=benchmark_name]")
	}
	opts := bench.Options{SourceDir: args[0]}
	for _, arg := range args[1:] {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			return bench.Options{}, fmt.Errorf("参数应为 key=value：%q", arg)
		}
		switch strings.ToLower(key) {
		case "name":
			opts.Name = value
		default:
			return bench.Options{}, fmt.Errorf("未知参数 %q（支持：name）", key)
		}
	}
	return opts, nil
}

func formatBenchmarkImportSuccess(res *bench.Result) string {
	if res == nil {
		return "✓ 对标拆文已导入"
	}
	return fmt.Sprintf("✓ 对标拆文已导入：%s（%d 个 Markdown）→ %s", res.Name, res.Files, res.Path)
}
