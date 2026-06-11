# oh-story 吸收路线

本文记录从 `worldwonderer/oh-story-claudecode` 吸收经验到 `ainovel-cli` 的后续路线。核心原则是不替换现有 Go 运行时架构，只吸收网文方法论、质量门禁、拆文产物结构和上下文召回策略。

## 边界原则

- 保留现有 `Host -> Flow Router -> Coordinator -> SubAgents -> Tools -> Store` 架构。
- `Store` 仍是事实源，Markdown 只作为参考资料、导出视图或用户可读材料。
- 优先做低风险增强：references、rules、prompt、context payload。
- 不在短期内引入浏览器/CDP 扫榜、外部平台采集或新的调度框架。

## 当前已完成

- `assets/references/anti-ai-tone.md` 增加 Gate A-F、程度分级和保留剧情功能原则。
- `assets/references/quality-checklist.md` 增加网文商业门禁和 S1-S4 严重度映射。
- `assets/prompts/writer.md` 增加最简记忆包、商业门禁和提交前自检。
- `assets/prompts/editor.md` 增加 Gate A-F 审美检查和 S1-S4 到现有 severity 的映射。
- `assets/rules/default.md` 增加低争议 AI 套句和疲劳词。
- `internal/rules/loader.go` 修复规则目录路径被误建成文件时的 conflict 暴露。
- `novel_context(chapter=N)` 返回 `selected_memory.minimal_context`，结构化提供角色状态、因果历史、世界约束和本章意图，缺数据时保持空字段不阻断写作。
- `save_review` 增加可选 `risk_level: S1|S2|S3|S4` 落盘与 verdict 升级：S1 自动 rewrite，S2 至少 polish，S3/S4 不单独触发 rewrite。
- `/importbench <拆文目录> [name=...]` 支持把 Markdown 拆文库确定性导入 `meta/benchmarks/{name}.json`，并进入命令面板。

## 阶段 1：方法论资料库扩展

目标：把 oh-story 中高价值、低耦合的写作方法沉淀为本项目 references。

候选资料：

- 钩子与悬念：章首钩子、章尾钩子、段落级钩子。
- 反转工具箱：身份、动机、阵营、信息、命运反转。
- 对话技法：潜台词、信息控制、角色声音区分。
- 情绪与爽点：情绪曲线、期待感管理、爽点释放。
- 题材与平台：起点、番茄、晋江、知乎盐言的读者期待差异。

建议落点：

- 新增或扩展 `assets/references/hook-techniques.md`。
- 新增 `assets/references/reversal-toolkit.md`。
- 扩展 `assets/references/dialogue-writing.md`。
- 扩展 `assets/references/longform-planning.md`。

验收标准：

- `assets/load.go` 只在确有必要时新增字段；优先复用现有 `reference_pack.references`。
- `go test ./...` 通过。
- Writer/Architect prompt 不出现大段重复方法论，只引用 `reference_pack` 中的资料。

## 阶段 2：最简记忆包结构化

目标：把“只保留不知道就会写错的信息”从 prompt 要求升级为 `novel_context` 的结构化字段。

建议新增字段：

```json
"selected_memory": {
  "minimal_context": {
    "character_states": [],
    "causal_history": [],
    "world_constraints": [],
    "chapter_intent": ""
  }
}
```

数据来源：

- `current_chapter_outline` 和 `chapter_contract`。
- `character_snapshots` / `characters`。
- `foreshadow_ledger` / `story_threads`。
- `recent_state_changes` / `relationship_state`。
- `world_rules`。

建议文件：

- `internal/tools/novel_context_builders.go`
- `internal/tools/novel_context_test.go`
- `assets/prompts/writer.md`

验收标准：

- Writer 路径 `novel_context(chapter=N)` 返回 `selected_memory.minimal_context`。
- 长篇和非分层模式都有测试覆盖。
- 未命中相关数据时字段可为空，但不得阻断写作。

## 阶段 3：审稿严重度落盘

目标：让 S1-S4 不只停留在 prompt 中，而是成为 `save_review` 可记录、可诊断、可统计的结构化信息。

设计方向：

- 保持现有 `critical/error/warning` schema 兼容。
- 可新增可选字段 `risk_level: S1|S2|S3|S4`。
- `save_review` 根据 `risk_level` 与现有 severity 共同决定 `final_verdict`。

建议文件：

- `internal/domain/review.go`
- `internal/tools/save_review.go`
- `internal/tools/save_review_test.go`
- `assets/prompts/editor.md`

验收标准：

- 旧 review JSON 不需要迁移即可读取。
- S1 自动升级 rewrite。
- S2 至少升级 polish，破坏主线/动机时允许 rewrite。
- S3/S4 不单独触发 rewrite。

## 阶段 4：对标拆文库导入

目标：把 oh-story 的 `拆文库/{书名}/` 思路转成本项目可消费的结构化 benchmark 数据。

建议形态：

- 新增 `output/{novel}/meta/benchmarks/{name}.json`。
- 支持从 Markdown 拆文目录导入：`拆文报告.md`、`文风.md`、章节摘要、角色、剧情、设定。
- 与现有 `simulation_profile` 区分：simulation 偏风格画像，benchmark 偏可召回的结构化对标素材。

最小切片已完成：

- `internal/domain/benchmark.go` / `internal/store/benchmark.go` 已支持 `meta/benchmarks/{name}.json` 持久化与校验，name 仅允许字母、数字、`_`、`-`。
- `novel_context` 已在 chapter / architect 路径注入 compact `benchmark_summaries`，仅保留方法、节奏、结构和少量授权锚点，不注入原始对标文本。
- 对应持久化与注入测试已补齐。

本轮已推进：

- Markdown 导入器：递归读取 `.md` / `.markdown`，按标题和文件名抽取摘要、结构、节奏、钩子、角色、设定、技法和禁抄项。
- TUI 命令联动：新增 `/importbench <拆文目录> [name=benchmark_name]`，空闲状态下异步导入并在事件流反馈结果。

后续仍待完成：

- 阶段 5 的文风召回与匹配章节。
- 阶段 6 的诊断报告扩展。

建议文件：

- `internal/store/benchmark.go`
- `internal/domain/benchmark.go`
- `internal/host/imp` 或新增导入模块（后续）。
- `internal/tools/novel_context_builders.go`（最小切片已完成）。

验收标准：

- 可导入一个 oh-story 拆文 demo 的核心摘要。
- `novel_context` 能返回 compact benchmark summary。
- 不复制对标原文进 prompt，只提供方法、节奏、结构摘要和少量用户授权锚点。

## 阶段 5：文风召回与匹配章节

目标：写每章前根据本章情绪/钩子/目标，从 benchmark 中召回一小段风格和技法提示。

建议字段：

```json
"reference_pack": {
  "benchmark_style": {
    "profile": "...",
    "matched_chapter": 12,
    "techniques": [],
    "gaps": []
  }
}
```

匹配依据：

- 本章 `chapter_contract.emotion_target`。
- 当前大纲 `hook` / `core_event`。
- benchmark 章节摘要中的基调、爽点、钩子类型。

验收标准：

- 无 benchmark 时静默跳过。
- benchmark 缺 `文风.md` 时提示 warning，不阻断写作。
- Writer prompt 明确“学技法，不抄句子、人物、桥段”。

## 阶段 6：诊断报告扩展

目标：把新增门禁和 benchmark 信息纳入 `/report` 或诊断模块。

候选诊断：

- AI Gate 命中趋势。
- S1/S2 问题密度。
- 章节是否缺最小剧情循环。
- 角色关系突变。
- 伏笔密度过高/过低。
- benchmark 文风召回缺失或退化。

建议文件：

- `internal/diag/rules_quality.go`
- `internal/diag/rules_context.go`
- `internal/diag/types.go`

验收标准：

- 诊断只读 Store，不改变流程。
- 输出包含证据和建议目标，例如 `prompt.writer`、`rules.default`、`novel_context`。

## 延后事项

- 扫榜和浏览器/CDP 采集：价值高但维护成本、合规风险和依赖复杂度都高，暂不进入核心路线。
- 封面生成：与小说写作主循环关系弱，可作为独立可选功能。
- 完整 Claude Code skill 兼容层：与本项目定位不一致，除非未来明确支持外部 skill 包。

## 推荐优先级

1. 阶段 1：方法论资料库扩展。
2. 阶段 2：最简记忆包结构化。
3. 阶段 3：审稿严重度落盘。
4. 阶段 4：对标拆文库导入。
5. 阶段 5：文风召回与匹配章节。
6. 阶段 6：诊断报告扩展。
